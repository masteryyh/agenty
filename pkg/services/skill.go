/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package services

import (
	"context"
	json "github.com/bytedance/sonic"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"go.yaml.in/yaml/v3"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	agentsSkillsDir = ".agents/skills"
	claudeSkillsDir = ".claude/skills"
	skillMDFileName = "SKILL.md"
)

type SkillService struct {
	db               *gorm.DB
	globalWatcher    *fsnotify.Watcher
	projectWatchers  map[uuid.UUID]*projectWatcherState
	sessionTables    map[uuid.UUID]bool
	mu               sync.RWMutex
	globalSkillsPath string
}

type projectWatcherState struct {
	watcher  *fsnotify.Watcher
	cwd      string
	cancelFn context.CancelFunc
}

var (
	skillService *SkillService
	skillOnce    sync.Once
)

func GetSkillService() *SkillService {
	skillOnce.Do(func() {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			slog.Error("failed to get user home directory", "error", err)
			homeDir = ""
		}
		skillService = &SkillService{
			db:               conn.GetDB(),
			projectWatchers:  make(map[uuid.UUID]*projectWatcherState),
			sessionTables:    make(map[uuid.UUID]bool),
			globalSkillsPath: filepath.Join(homeDir, agentsSkillsDir),
		}
	})
	return skillService
}

func (s *SkillService) Initialize(ctx context.Context) error {
	if err := s.scanGlobalSkills(ctx); err != nil {
		return fmt.Errorf("failed to scan global skills: %w", err)
	}

	if err := s.startGlobalWatcher(ctx); err != nil {
		slog.WarnContext(ctx, "failed to start global skills watcher", "error", err)
	}
	return nil
}

func (s *SkillService) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.globalWatcher != nil {
		s.globalWatcher.Close()
		s.globalWatcher = nil
	}

	for sessionID, state := range s.projectWatchers {
		if state.cancelFn != nil {
			state.cancelFn()
		}
		if state.watcher != nil {
			state.watcher.Close()
		}
		delete(s.projectWatchers, sessionID)
	}
}

type parsedSkill struct {
	Name        string
	Description string
	Path        string
	SourceDir   string
	Metadata    []byte
}

type skillFrontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license"`
	Compatibility string            `yaml:"compatibility"`
	Metadata      map[string]string `yaml:"metadata"`
	AllowedTools  string            `yaml:"allowed-tools"`
}

func (s *SkillService) parseSkillMD(skillMDPath string) (*parsedSkill, error) {
	content, err := os.ReadFile(skillMDPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SKILL.md: %w", err)
	}

	str := strings.TrimLeft(string(content), " \r\n")
	var fm skillFrontmatter

	if strings.HasPrefix(str, "---") {
		after := str[3:]
		if len(after) > 0 && after[0] == '\n' {
			after = after[1:]
		}
		before, _, ok := strings.Cut(after, "\n---")
		if ok {
			if err := yaml.Unmarshal([]byte(before), &fm); err != nil {
				return nil, fmt.Errorf("failed to parse SKILL.md frontmatter: %w", err)
			}
		}
	}

	if fm.Name == "" {
		fm.Name = filepath.Base(filepath.Dir(skillMDPath))
	}
	if fm.Description == "" {
		fm.Description = fmt.Sprintf("Skill: %s", strings.ReplaceAll(fm.Name, "-", " "))
	}

	metadataJSON, err := json.Marshal(fm.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal skill metadata to JSON: %w", err)
	}

	return &parsedSkill{
		Name:        fm.Name,
		Description: fm.Description,
		Path:        skillMDPath,
		SourceDir:   filepath.Dir(skillMDPath),
		Metadata:    metadataJSON,
	}, nil
}

func (s *SkillService) scanDirectory(dirPath string) ([]*parsedSkill, error) {
	var skills []*parsedSkill

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return skills, nil
		}
		return nil, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillMDPath := filepath.Join(dirPath, entry.Name(), skillMDFileName)
		if _, err := os.Stat(skillMDPath); os.IsNotExist(err) {
			continue
		}

		parsed, err := s.parseSkillMD(skillMDPath)
		if err != nil {
			slog.Warn("failed to parse SKILL.md", "path", skillMDPath, "error", err)
			continue
		}
		skills = append(skills, parsed)
	}

	return skills, nil
}

func (s *SkillService) scanGlobalSkills(ctx context.Context) error {
	stat, err := os.Stat(s.globalSkillsPath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.InfoContext(ctx, "global skills directory does not exist, skipping scan", "path", s.globalSkillsPath)
			return nil
		}
		return fmt.Errorf("failed to stat global skills path: %w", err)
	}

	if !stat.IsDir() {
		slog.WarnContext(ctx, "global skills path is not a directory, skipping scan", "path", s.globalSkillsPath)
		return nil
	}

	skills, err := s.scanDirectory(s.globalSkillsPath)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1=1").Delete(&models.Skill{}).Error; err != nil {
			return fmt.Errorf("failed to clear existing skills: %w", err)
		}

		for _, parsed := range skills {
			skill := &models.Skill{
				Name:        parsed.Name,
				Description: parsed.Description,
				SkillMDPath: parsed.Path,
				Metadata:    datatypes.JSON(parsed.Metadata),
			}
			if err := tx.Create(skill).Error; err != nil {
				return fmt.Errorf("failed to insert skill %s: %w", parsed.Name, err)
			}
		}

		slog.InfoContext(ctx, "scanned global skills", "count", len(skills))
		return nil
	})
}

func (s *SkillService) startGlobalWatcher(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	s.mu.Lock()
	s.globalWatcher = watcher
	s.mu.Unlock()

	if err := watcher.Add(s.globalSkillsPath); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to add global skills path to watcher: %w", err)
	}

	safe.GoSafeWithCtx("global-skills-watcher", ctx, s.runGlobalWatcher)

	slog.InfoContext(ctx, "started global skills watcher", "path", s.globalSkillsPath)
	return nil
}

func (s *SkillService) runGlobalWatcher(ctx context.Context) {
	s.mu.RLock()
	watcher := s.globalWatcher
	s.mu.RUnlock()

	if watcher == nil {
		return
	}

	debounce := make(map[string]time.Time)
	debounceDuration := 500 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if time.Since(debounce[event.Name]) < debounceDuration {
				continue
			}
			debounce[event.Name] = time.Now()

			s.handleGlobalFileEvent(ctx, event)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			slog.ErrorContext(ctx, "global watcher error", "error", err)
		}
	}
}

func (s *SkillService) handleGlobalFileEvent(ctx context.Context, event fsnotify.Event) {
	skillDir := event.Name
	if filepath.Base(event.Name) == skillMDFileName {
		skillDir = filepath.Dir(event.Name)
	}

	skillMDPath := filepath.Join(skillDir, skillMDFileName)

	switch {
	case event.Op&fsnotify.Create != 0 || event.Op&fsnotify.Write != 0:
		if _, err := os.Stat(skillMDPath); os.IsNotExist(err) {
			return
		}

		parsed, err := s.parseSkillMD(skillMDPath)
		if err != nil {
			slog.WarnContext(ctx, "failed to parse SKILL.md on change", "path", skillMDPath, "error", err)
			return
		}

		s.upsertGlobalSkill(ctx, parsed)

	case event.Op&fsnotify.Remove != 0 || event.Op&fsnotify.Rename != 0:
		s.removeGlobalSkillByPath(ctx, skillMDPath)
	}
}

func (s *SkillService) upsertGlobalSkill(ctx context.Context, parsed *parsedSkill) {
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.Skill
		err := tx.Where("skill_md_path = ?", parsed.Path).First(&existing).Error
		if err == nil {
			return tx.Model(&existing).Updates(map[string]any{
				"name":        parsed.Name,
				"description": parsed.Description,
				"metadata":    datatypes.JSON(parsed.Metadata),
				"updated_at":  time.Now(),
			}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		skill := &models.Skill{
			Name:        parsed.Name,
			Description: parsed.Description,
			SkillMDPath: parsed.Path,
			Metadata:    datatypes.JSON(parsed.Metadata),
		}
		return tx.Create(skill).Error
	}); err != nil {
		slog.ErrorContext(ctx, "failed to upsert global skill", "path", parsed.Path, "error", err)
		return
	}

	s.mu.RLock()
	sessionIDs := make([]uuid.UUID, 0, len(s.sessionTables))
	for sessionID := range s.sessionTables {
		sessionIDs = append(sessionIDs, sessionID)
	}
	s.mu.RUnlock()

	for _, sessionID := range sessionIDs {
		tableName := s.sessionTableName(sessionID)
		query := fmt.Sprintf(`
			INSERT INTO %s (id, name, description, skill_md_path, scope, source_dir, metadata, created_at, updated_at)
			VALUES (uuidv7(), $1, $2, $3, 'global', $4, $5, NOW(), NOW())
			ON CONFLICT (skill_md_path) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				metadata = EXCLUDED.metadata,
				updated_at = NOW()
		`, tableName)

		var metadataArg any
		if len(parsed.Metadata) > 0 {
			metadataArg = string(parsed.Metadata)
		}

		if err := s.db.WithContext(ctx).Exec(query, parsed.Name, parsed.Description, parsed.Path, s.globalSkillsPath, metadataArg).Error; err != nil {
			slog.WarnContext(ctx, "failed to propagate skill to session", "sessionId", sessionID, "error", err)
		}
	}
}

func (s *SkillService) removeGlobalSkillByPath(ctx context.Context, skillMDPath string) {
	if err := s.db.WithContext(ctx).
		Where("skill_md_path = ?", skillMDPath).
		Delete(&models.Skill{}).Error; err != nil {
		slog.ErrorContext(ctx, "failed to remove global skill", "path", skillMDPath, "error", err)
		return
	}

	s.removeGlobalSkillFromSessions(ctx, skillMDPath)
}

func (s *SkillService) removeGlobalSkillFromSessions(ctx context.Context, skillMDPath string) {
	s.mu.RLock()
	sessionIDs := make([]uuid.UUID, 0, len(s.sessionTables))
	for sessionID := range s.sessionTables {
		sessionIDs = append(sessionIDs, sessionID)
	}
	s.mu.RUnlock()

	for _, sessionID := range sessionIDs {
		tableName := s.sessionTableName(sessionID)
		query := fmt.Sprintf(`DELETE FROM %s WHERE skill_md_path = $1 AND scope = 'global'`, tableName)
		if err := s.db.WithContext(ctx).Exec(query, skillMDPath).Error; err != nil {
			slog.WarnContext(ctx, "failed to remove skill from session", "sessionId", sessionID, "error", err)
		}
	}
}

func (s *SkillService) sessionTableName(sessionID uuid.UUID) string {
	return fmt.Sprintf("session_skills_%s", strings.ReplaceAll(sessionID.String(), "-", ""))
}

func (s *SkillService) CreateSessionTable(ctx context.Context, sessionID uuid.UUID) error {
	tableName := s.sessionTableName(sessionID)

	createTableSQL := fmt.Sprintf(`
		CREATE UNLOGGED TABLE IF NOT EXISTS %s (
			id               UUID         PRIMARY KEY DEFAULT uuidv7(),
			name             VARCHAR(255) NOT NULL,
			description      TEXT         NOT NULL,
			skill_md_path    TEXT         NOT NULL UNIQUE,
			scope            VARCHAR(20)  NOT NULL DEFAULT 'global',
			source_dir       TEXT         NOT NULL,
			metadata         JSONB,
			created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
		)
	`, tableName)

	if err := s.db.WithContext(ctx).Exec(createTableSQL).Error; err != nil {
		return fmt.Errorf("failed to create session skills table: %w", err)
	}

	createIndexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_%s_bm25
		ON %s
		USING bm25 (id, name, description)
		WITH (key_field = 'id')
	`, strings.ReplaceAll(sessionID.String(), "-", ""), tableName)

	if err := s.db.WithContext(ctx).Exec(createIndexSQL).Error; err != nil {
		slog.WarnContext(ctx, "failed to create BM25 index on session table", "table", tableName, "error", err)
	}

	s.mu.Lock()
	s.sessionTables[sessionID] = true
	s.mu.Unlock()

	return nil
}

func (s *SkillService) DropSessionTable(ctx context.Context, sessionID uuid.UUID) error {
	tableName := s.sessionTableName(sessionID)

	dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
	if err := s.db.WithContext(ctx).Exec(dropSQL).Error; err != nil {
		return fmt.Errorf("failed to drop session skills table: %w", err)
	}

	s.mu.Lock()
	delete(s.sessionTables, sessionID)
	s.mu.Unlock()

	s.stopProjectWatcher(sessionID)

	return nil
}

func (s *SkillService) PopulateSessionSkills(ctx context.Context, sessionID uuid.UUID, cwd string) error {
	tableName := s.sessionTableName(sessionID)

	copyGlobalSQL := fmt.Sprintf(`
		INSERT INTO %s (id, name, description, skill_md_path, scope, source_dir, metadata, created_at, updated_at)
		SELECT uuidv7(), name, description, skill_md_path, 'global', $1, metadata, created_at, updated_at
		FROM skills
	`, tableName)

	if err := s.db.WithContext(ctx).Exec(copyGlobalSQL, s.globalSkillsPath).Error; err != nil {
		return fmt.Errorf("failed to copy global skills to session table: %w", err)
	}

	projectDirs := []string{
		filepath.Join(cwd, agentsSkillsDir),
		filepath.Join(cwd, claudeSkillsDir),
	}

	for _, dir := range projectDirs {
		skills, err := s.scanDirectory(dir)
		if err != nil {
			slog.WarnContext(ctx, "failed to scan project skills directory", "dir", dir, "error", err)
			continue
		}

		for _, parsed := range skills {
			insertSQL := fmt.Sprintf(`
				INSERT INTO %s (id, name, description, skill_md_path, scope, source_dir, metadata, created_at, updated_at)
				VALUES (uuidv7(), $1, $2, $3, 'project', $4, $5, NOW(), NOW())
				ON CONFLICT (skill_md_path) DO NOTHING
			`, tableName)

			var metadataArg any
			if len(parsed.Metadata) > 0 {
				metadataArg = string(parsed.Metadata)
			}

			if err := s.db.WithContext(ctx).Exec(insertSQL, parsed.Name, parsed.Description, parsed.Path, dir, metadataArg).Error; err != nil {
				slog.WarnContext(ctx, "failed to insert project skill", "path", parsed.Path, "error", err)
			}
		}
	}

	if err := s.startProjectWatcher(ctx, sessionID, cwd); err != nil {
		slog.WarnContext(ctx, "failed to start project watcher", "sessionId", sessionID, "cwd", cwd, "error", err)
	}
	return nil
}

func (s *SkillService) startProjectWatcher(ctx context.Context, sessionID uuid.UUID, cwd string) error {
	s.stopProjectWatcher(sessionID)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create project watcher: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)

	state := &projectWatcherState{
		watcher:  watcher,
		cwd:      cwd,
		cancelFn: cancel,
	}

	s.mu.Lock()
	s.projectWatchers[sessionID] = state
	s.mu.Unlock()

	projectDirs := []string{
		filepath.Join(cwd, agentsSkillsDir),
		filepath.Join(cwd, claudeSkillsDir),
	}

	for _, dir := range projectDirs {
		if _, err := os.Stat(dir); err == nil {
			if err := watcher.Add(dir); err != nil {
				slog.WarnContext(ctx, "failed to watch project skills directory", "dir", dir, "error", err)
			}
		}
	}

	safe.GoSafeWithCtx("project-skills-watcher-"+sessionID.String(), watchCtx, func(ctx context.Context) {
		s.runProjectWatcher(ctx, sessionID, state)
	})

	return nil
}

func (s *SkillService) stopProjectWatcher(sessionID uuid.UUID) {
	s.mu.Lock()
	state, exists := s.projectWatchers[sessionID]
	if exists {
		delete(s.projectWatchers, sessionID)
	}
	s.mu.Unlock()

	if exists && state != nil {
		if state.cancelFn != nil {
			state.cancelFn()
		}
		if state.watcher != nil {
			state.watcher.Close()
		}
	}
}

func (s *SkillService) runProjectWatcher(ctx context.Context, sessionID uuid.UUID, state *projectWatcherState) {
	debounce := make(map[string]time.Time)
	debounceDuration := 500 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-state.watcher.Events:
			if !ok {
				return
			}

			if time.Since(debounce[event.Name]) < debounceDuration {
				continue
			}
			debounce[event.Name] = time.Now()

			s.handleProjectFileEvent(ctx, sessionID, event)
		case err, ok := <-state.watcher.Errors:
			if !ok {
				return
			}
			slog.ErrorContext(ctx, "project watcher error", "sessionId", sessionID, "error", err)
		}
	}
}

func (s *SkillService) handleProjectFileEvent(ctx context.Context, sessionID uuid.UUID, event fsnotify.Event) {
	skillDir := event.Name
	if filepath.Base(event.Name) == skillMDFileName {
		skillDir = filepath.Dir(event.Name)
	}

	skillMDPath := filepath.Join(skillDir, skillMDFileName)
	sourceDir := filepath.Dir(skillDir)

	tableName := s.sessionTableName(sessionID)

	switch {
	case event.Op&fsnotify.Create != 0 || event.Op&fsnotify.Write != 0:
		if _, err := os.Stat(skillMDPath); os.IsNotExist(err) {
			return
		}

		parsed, err := s.parseSkillMD(skillMDPath)
		if err != nil {
			slog.WarnContext(ctx, "failed to parse SKILL.md on change", "path", skillMDPath, "error", err)
			return
		}

		insertSQL := fmt.Sprintf(`
			INSERT INTO %s (id, name, description, skill_md_path, scope, source_dir, metadata, created_at, updated_at)
			VALUES (uuidv7(), $1, $2, $3, 'project', $4, $5, NOW(), NOW())
			ON CONFLICT (skill_md_path) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				metadata = EXCLUDED.metadata,
				updated_at = NOW()
		`, tableName)

		var metadataArg any
		if len(parsed.Metadata) > 0 {
			metadataArg = string(parsed.Metadata)
		}

		if err := s.db.WithContext(ctx).Exec(insertSQL, parsed.Name, parsed.Description, parsed.Path, sourceDir, metadataArg).Error; err != nil {
			slog.WarnContext(ctx, "failed to upsert project skill", "path", parsed.Path, "error", err)
		}

	case event.Op&fsnotify.Remove != 0 || event.Op&fsnotify.Rename != 0:
		deleteSQL := fmt.Sprintf(`DELETE FROM %s WHERE skill_md_path = $1 AND scope = 'project'`, tableName)
		if err := s.db.WithContext(ctx).Exec(deleteSQL, skillMDPath).Error; err != nil {
			slog.WarnContext(ctx, "failed to remove project skill", "path", skillMDPath, "error", err)
		}
	}
}

func (s *SkillService) SearchSkills(ctx context.Context, sessionID *uuid.UUID, query string, limit int) ([]models.SkillSearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	var results []models.SkillSearchResult

	if sessionID != nil {
		s.mu.RLock()
		hasTable := s.sessionTables[*sessionID]
		s.mu.RUnlock()

		if hasTable {
			tableName := s.sessionTableName(*sessionID)
			searchSQL := fmt.Sprintf(`
				SELECT id, name, description, skill_md_path, scope, paradedb.score(id) as score
				FROM %s
				WHERE name @@@ ? OR description @@@ ?
				ORDER BY score DESC
				LIMIT ?
			`, tableName)

			rows, err := s.db.WithContext(ctx).Raw(searchSQL, query, query, limit).Rows()
			if err != nil {
				return nil, fmt.Errorf("skill search failed, error: %w", err)
			}
			defer rows.Close()

			for rows.Next() {
				var r models.SkillSearchResult
				if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.SkillMDPath, &r.Scope, &r.Score); err != nil {
					continue
				}
				results = append(results, r)
			}
			return results, nil
		}
	}

	searchSQL := `
		SELECT id, name, description, skill_md_path, paradedb.score(id) as score
		FROM skills
		WHERE name @@@ ? OR description @@@ ?
		ORDER BY score DESC
		LIMIT ?
	`

	rows, err := s.db.WithContext(ctx).Raw(searchSQL, query, query, limit).Rows()
	if err != nil {
		return nil, fmt.Errorf("skill search failed, error: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r models.SkillSearchResult
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.SkillMDPath, &r.Score); err != nil {
			continue
		}
		results = append(results, r)
	}

	return results, nil
}

func (s *SkillService) ListSkills(ctx context.Context, sessionID *uuid.UUID) ([]models.SkillDto, error) {
	var results []models.SkillDto

	if sessionID != nil {
		s.mu.RLock()
		hasTable := s.sessionTables[*sessionID]
		s.mu.RUnlock()

		if hasTable {
			tableName := s.sessionTableName(*sessionID)
			query := fmt.Sprintf(`
				SELECT id, name, description, skill_md_path, scope, source_dir, created_at, updated_at
				FROM %s
				ORDER BY scope, name
			`, tableName)

			rows, err := s.db.WithContext(ctx).Raw(query).Rows()
			if err != nil {
				return nil, fmt.Errorf("failed to list session skills: %w", err)
			}
			defer rows.Close()

			for rows.Next() {
				var dto models.SkillDto
				if err := rows.Scan(&dto.ID, &dto.Name, &dto.Description, &dto.SkillMDPath, &dto.Scope, &dto.SourceDir, &dto.CreatedAt, &dto.UpdatedAt); err != nil {
					continue
				}
				results = append(results, dto)
			}
			return results, nil
		}
	}

	var skills []models.Skill
	if err := s.db.WithContext(ctx).
		Order("name").
		Find(&skills).Error; err != nil {
		return nil, fmt.Errorf("failed to list global skills: %w", err)
	}

	for _, skill := range skills {
		results = append(results, *skill.ToDto())
	}

	return results, nil
}

func (s *SkillService) GetSkillContent(ctx context.Context, name string, sessionID *uuid.UUID) (string, error) {
	var skillMDPath string

	if sessionID != nil {
		s.mu.RLock()
		hasTable := s.sessionTables[*sessionID]
		s.mu.RUnlock()

		if hasTable {
			tableName := s.sessionTableName(*sessionID)
			query := fmt.Sprintf(`SELECT skill_md_path FROM %s WHERE name = ? LIMIT 1`, tableName)
			rows, err := s.db.WithContext(ctx).Raw(query, name).Rows()
			if err != nil {
				return "", fmt.Errorf("failed to query session skill: %w", err)
			}
			defer rows.Close()
			if rows.Next() {
				if err := rows.Scan(&skillMDPath); err != nil {
					return "", fmt.Errorf("failed to scan skill path: %w", err)
				}
			}
		}
	}

	if skillMDPath == "" {
		var skill models.Skill
		if err := s.db.WithContext(ctx).Where("name = ?", name).First(&skill).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", customerrors.ErrSkillNotFound
			}
			return "", fmt.Errorf("failed to query skill: %w", err)
		}
		skillMDPath = skill.SkillMDPath
	}

	content, err := os.ReadFile(skillMDPath)
	if err != nil {
		return "", fmt.Errorf("failed to read SKILL.md: %w", err)
	}
	return string(content), nil
}

func (s *SkillService) HasSessionTable(sessionID uuid.UUID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionTables[sessionID]
}

func (s *SkillService) CountSessionSkills(ctx context.Context, sessionID uuid.UUID) (int64, error) {
	s.mu.RLock()
	hasTable := s.sessionTables[sessionID]
	s.mu.RUnlock()

	if hasTable {
		tableName := s.sessionTableName(sessionID)
		var count int64
		if err := s.db.WithContext(ctx).Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count).Error; err != nil {
			return 0, fmt.Errorf("failed to count session skills: %w", err)
		}
		return count, nil
	}

	var count int64
	if err := s.db.WithContext(ctx).Model(&models.Skill{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count global skills: %w", err)
	}
	return count, nil
}

func (s *SkillService) ListSessionSkillSummaries(ctx context.Context, sessionID uuid.UUID) ([]models.SkillDto, error) {
	s.mu.RLock()
	hasTable := s.sessionTables[sessionID]
	s.mu.RUnlock()

	if hasTable {
		tableName := s.sessionTableName(sessionID)
		query := fmt.Sprintf("SELECT id, name, description, skill_md_path, scope FROM %s ORDER BY scope, name", tableName)
		rows, err := s.db.WithContext(ctx).Raw(query).Rows()
		if err != nil {
			return nil, fmt.Errorf("failed to list session skill summaries: %w", err)
		}
		defer rows.Close()

		var results []models.SkillDto
		for rows.Next() {
			var dto models.SkillDto
			if err := rows.Scan(&dto.ID, &dto.Name, &dto.Description, &dto.SkillMDPath, &dto.Scope); err != nil {
				continue
			}
			results = append(results, dto)
		}
		return results, nil
	}

	var skills []models.Skill
	if err := s.db.WithContext(ctx).Order("name").Find(&skills).Error; err != nil {
		return nil, fmt.Errorf("failed to list global skill summaries: %w", err)
	}

	results := make([]models.SkillDto, 0, len(skills))
	for _, skill := range skills {
		results = append(results, models.SkillDto{
			ID:          skill.ID,
			Name:        skill.Name,
			Description: skill.Description,
			SkillMDPath: skill.SkillMDPath,
		})
	}
	return results, nil
}
