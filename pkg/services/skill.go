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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	builtinskill "github.com/masteryyh/agenty/pkg/skill"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"go.yaml.in/yaml/v3"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	agentsSkillsDir                = ".agents/skills"
	claudeSkillsDir                = ".claude/skills"
	skillMDFileName                = "SKILL.md"
	sessionSkillTableTTL           = 5 * 24 * time.Hour
	sessionSkillTableCleanupPeriod = 24 * time.Hour
)

type SkillService struct {
	db                *gorm.DB
	globalWatcher     *fsnotify.Watcher
	projectWatchers   map[uuid.UUID]*projectWatcherState
	sessionTables     map[uuid.UUID]bool
	mu                sync.RWMutex
	sessionTableMu    sync.Mutex
	cleanupCancelFn   context.CancelFunc
	globalSkillsPath  string
	builtinSkillsPath string
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
		builtinSkillsPath, err := builtinskill.BuiltinDir()
		if err != nil {
			slog.Error("failed to resolve builtin skills directory", "error", err)
		}
		skillService = &SkillService{
			db:                conn.GetDB(),
			projectWatchers:   make(map[uuid.UUID]*projectWatcherState),
			sessionTables:     make(map[uuid.UUID]bool),
			globalSkillsPath:  filepath.Join(homeDir, agentsSkillsDir),
			builtinSkillsPath: builtinSkillsPath,
		}
	})
	return skillService
}

func (s *SkillService) Initialize(ctx context.Context) error {
	if err := s.ensureBuiltinSkills(ctx); err != nil {
		return fmt.Errorf("failed to ensure builtin skills: %w", err)
	}

	if err := s.scanGlobalSkills(ctx); err != nil {
		return fmt.Errorf("failed to scan global skills: %w", err)
	}

	if err := s.startGlobalWatcher(ctx); err != nil {
		slog.WarnContext(ctx, "failed to start global skills watcher", "error", err)
	}
	s.startSessionTableCleanup(ctx)
	return nil
}

func (s *SkillService) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.globalWatcher != nil {
		s.globalWatcher.Close()
		s.globalWatcher = nil
	}

	if s.cleanupCancelFn != nil {
		s.cleanupCancelFn()
		s.cleanupCancelFn = nil
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
	ID          *uuid.UUID
	Name        string
	Description string
	Path        string
	Metadata    []byte
}

func (s *SkillService) ensureBuiltinSkills(ctx context.Context) error {
	if s.builtinSkillsPath == "" {
		return nil
	}

	skills, err := builtinskill.ListBuiltinSkills()
	if err != nil {
		return err
	}

	written := 0
	for _, skill := range skills {
		skillDir := filepath.Join(s.builtinSkillsPath, skill.Name)
		skillMDPath := filepath.Join(skillDir, skillMDFileName)

		if _, err := os.Stat(skillMDPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat builtin skill %s: %w", skill.Name, err)
		}

		if err := os.MkdirAll(skillDir, 0755); err != nil {
			return fmt.Errorf("failed to create builtin skill directory %s: %w", skillDir, err)
		}

		if err := os.WriteFile(skillMDPath, skill.Content, 0644); err != nil {
			return fmt.Errorf("failed to write builtin skill %s: %w", skill.Name, err)
		}
		written++
	}

	if written > 0 {
		slog.InfoContext(ctx, "installed builtin skills", "count", written, "path", s.builtinSkillsPath)
	}
	return nil
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

	var id *uuid.UUID
	if rawID := strings.TrimSpace(fm.Metadata["id"]); rawID != "" {
		parsedID, err := uuid.Parse(rawID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse skill metadata id: %w", err)
		}
		id = &parsedID
	}

	return &parsedSkill{
		ID:          id,
		Name:        fm.Name,
		Description: fm.Description,
		Path:        skillMDPath,
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
	sources := []string{s.globalSkillsPath, s.builtinSkillsPath}

	var discovered []*parsedSkill
	for _, source := range sources {
		if source == "" {
			continue
		}

		stat, err := os.Stat(source)
		if err != nil {
			if os.IsNotExist(err) {
				slog.InfoContext(ctx, "skills directory does not exist, skipping scan", "path", source)
				continue
			}
			return fmt.Errorf("failed to stat skills path %s: %w", source, err)
		}

		if !stat.IsDir() {
			slog.WarnContext(ctx, "skills path is not a directory, skipping scan", "path", source)
			continue
		}

		skills, err := s.scanDirectory(source)
		if err != nil {
			return err
		}
		discovered = append(discovered, skills...)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1=1").Delete(&models.Skill{}).Error; err != nil {
			return fmt.Errorf("failed to clear existing skills: %w", err)
		}

		for _, parsed := range discovered {
			skill := &models.Skill{
				Name:        parsed.Name,
				Description: parsed.Description,
				SkillMDPath: parsed.Path,
				Metadata:    datatypes.JSON(parsed.Metadata),
			}
			if parsed.ID != nil {
				skill.ID = *parsed.ID
			}
			if err := tx.Create(skill).Error; err != nil {
				return fmt.Errorf("failed to insert skill %s: %w", parsed.Name, err)
			}
		}

		slog.InfoContext(ctx, "scanned global skills", "count", len(discovered))
		return nil
	})
}

func (s *SkillService) startGlobalWatcher(ctx context.Context) error {
	if s.globalSkillsPath == "" {
		return nil
	}

	stat, err := os.Stat(s.globalSkillsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat global skills path: %w", err)
	}
	if !stat.IsDir() {
		return nil
	}

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
			updates := map[string]any{
				"name":        parsed.Name,
				"description": parsed.Description,
				"metadata":    datatypes.JSON(parsed.Metadata),
				"updated_at":  time.Now(),
			}
			if parsed.ID != nil {
				updates["id"] = *parsed.ID
			}
			return tx.Model(&existing).Updates(updates).Error
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
		if parsed.ID != nil {
			skill.ID = *parsed.ID
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

		var metadataArg any
		if len(parsed.Metadata) > 0 {
			metadataArg = string(parsed.Metadata)
		}

		if parsed.ID != nil {
			query := fmt.Sprintf(`
				INSERT INTO %s (id, name, description, skill_md_path, scope, source_dir, metadata, created_at, updated_at)
				VALUES (?, ?, ?, ?, 'global', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
				ON CONFLICT (skill_md_path) DO UPDATE SET
					id = EXCLUDED.id,
					name = EXCLUDED.name,
					description = EXCLUDED.description,
					metadata = EXCLUDED.metadata,
					updated_at = CURRENT_TIMESTAMP
			`, tableName)

			if err := s.db.WithContext(ctx).Exec(query, *parsed.ID, parsed.Name, parsed.Description, parsed.Path, parsed.Path, metadataArg).Error; err != nil {
				slog.WarnContext(ctx, "failed to propagate skill to session", "sessionId", sessionID, "error", err)
			}
			continue
		}

		query := fmt.Sprintf(`
			INSERT INTO %s (id, name, description, skill_md_path, scope, source_dir, metadata, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'global', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (skill_md_path) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				metadata = EXCLUDED.metadata,
				updated_at = CURRENT_TIMESTAMP
		`, tableName)

		id, idErr := uuid.NewV7()
		if idErr != nil {
			slog.WarnContext(ctx, "failed to generate skill id", "sessionId", sessionID, "error", idErr)
			continue
		}
		if err := s.db.WithContext(ctx).Exec(query, id, parsed.Name, parsed.Description, parsed.Path, parsed.Path, metadataArg).Error; err != nil {
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
		query := fmt.Sprintf(`DELETE FROM %s WHERE skill_md_path = ? AND scope = 'global'`, tableName)
		if err := s.db.WithContext(ctx).Exec(query, skillMDPath).Error; err != nil {
			slog.WarnContext(ctx, "failed to remove skill from session", "sessionId", sessionID, "error", err)
		}
	}
}

func (s *SkillService) sessionTableName(sessionID uuid.UUID) string {
	return fmt.Sprintf("session_skills_%s", strings.ReplaceAll(sessionID.String(), "-", ""))
}

func (s *SkillService) sessionFTSTableName(sessionID uuid.UUID) string {
	return fmt.Sprintf("%s_fts", s.sessionTableName(sessionID))
}

func (s *SkillService) createSessionTableSQL(sessionID uuid.UUID) string {
	tableName := s.sessionTableName(sessionID)
	if usingSQLite() {
		ftsTableName := s.sessionFTSTableName(sessionID)
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				description TEXT NOT NULL,
				skill_md_path TEXT NOT NULL UNIQUE,
				scope TEXT NOT NULL DEFAULT 'global',
				source_dir TEXT NOT NULL,
				metadata TEXT,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			);
			CREATE VIRTUAL TABLE IF NOT EXISTS %s USING fts5(id UNINDEXED, name, description);
			CREATE TRIGGER IF NOT EXISTS %s_fts_ai AFTER INSERT ON %s BEGIN
				INSERT INTO %s(rowid, id, name, description) VALUES (new.rowid, new.id, new.name, new.description);
			END;
			CREATE TRIGGER IF NOT EXISTS %s_fts_ad AFTER DELETE ON %s BEGIN
				DELETE FROM %s WHERE rowid = old.rowid;
			END;
			CREATE TRIGGER IF NOT EXISTS %s_fts_au AFTER UPDATE ON %s BEGIN
				DELETE FROM %s WHERE rowid = old.rowid;
				INSERT INTO %s(rowid, id, name, description) VALUES (new.rowid, new.id, new.name, new.description);
			END
		`, tableName, ftsTableName, tableName, tableName, ftsTableName, tableName, tableName, ftsTableName, tableName, tableName, ftsTableName, ftsTableName)
	}

	indexName := "idx_" + strings.ReplaceAll(sessionID.String(), "-", "") + "_bm25"
	return fmt.Sprintf(`
		CREATE UNLOGGED TABLE IF NOT EXISTS %s (
			id UUID PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			description TEXT NOT NULL,
			skill_md_path TEXT NOT NULL UNIQUE,
			scope VARCHAR(20) NOT NULL DEFAULT 'global',
			source_dir TEXT NOT NULL,
			metadata JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS %s
		ON %s
		USING bm25 (id, name, description)
		WITH (key_field = 'id')
	`, tableName, indexName, tableName)
}

func (s *SkillService) markSessionTable(sessionID uuid.UUID) {
	s.mu.Lock()
	s.sessionTables[sessionID] = true
	s.mu.Unlock()
}

func (s *SkillService) sessionTableExists(ctx context.Context, sessionID uuid.UUID) (bool, error) {
	tableName := s.sessionTableName(sessionID)
	var count int64
	if usingSQLite() {
		if err := s.db.WithContext(ctx).
			Raw(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).
			Scan(&count).Error; err != nil {
			return false, fmt.Errorf("failed to inspect session skills table: %w", err)
		}
		return count > 0, nil
	}

	if err := s.db.WithContext(ctx).
		Raw(`SELECT COUNT(*) FROM pg_class WHERE oid = to_regclass(?)`, tableName).
		Scan(&count).Error; err != nil {
		return false, fmt.Errorf("failed to inspect session skills table: %w", err)
	}
	return count > 0, nil
}

func (s *SkillService) countSessionTableRows(ctx context.Context, sessionID uuid.UUID) (int64, error) {
	tableName := s.sessionTableName(sessionID)
	var count int64
	if err := s.db.WithContext(ctx).Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count session skills: %w", err)
	}
	return count, nil
}

func (s *SkillService) EnsureSessionTable(ctx context.Context, sessionID uuid.UUID, cwd string) error {
	if strings.TrimSpace(cwd) == "" {
		return nil
	}

	s.sessionTableMu.Lock()
	defer s.sessionTableMu.Unlock()

	exists, err := s.sessionTableExists(ctx, sessionID)
	if err != nil {
		return err
	}

	if exists {
		s.markSessionTable(sessionID)
		count, err := s.countSessionTableRows(ctx, sessionID)
		if err != nil {
			return err
		}
		if count == 0 {
			return s.PopulateSessionSkills(ctx, sessionID, cwd)
		}
		if err := s.startProjectWatcher(ctx, sessionID, cwd); err != nil {
			slog.WarnContext(ctx, "failed to start project watcher", "sessionId", sessionID, "cwd", cwd, "error", err)
		}
		return nil
	}

	if err := s.CreateSessionTable(ctx, sessionID); err != nil {
		return err
	}
	if err := s.PopulateSessionSkills(ctx, sessionID, cwd); err != nil {
		return err
	}
	return nil
}

func (s *SkillService) ensureSessionTableForSession(ctx context.Context, sessionID uuid.UUID) error {
	s.mu.RLock()
	hasTable := s.sessionTables[sessionID]
	s.mu.RUnlock()
	if hasTable {
		exists, err := s.sessionTableExists(ctx, sessionID)
		if err != nil {
			return err
		}
		if exists {
			count, err := s.countSessionTableRows(ctx, sessionID)
			if err != nil {
				return err
			}
			if count > 0 {
				return nil
			}
		}
		s.mu.Lock()
		delete(s.sessionTables, sessionID)
		s.mu.Unlock()
	}

	var session models.ChatSession
	if err := s.db.WithContext(ctx).
		Select("id", "cwd").
		Where("id = ? AND deleted_at IS NULL", sessionID).
		First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("failed to load chat session for skills: %w", err)
	}
	if session.Cwd == nil {
		return nil
	}
	return s.EnsureSessionTable(ctx, sessionID, *session.Cwd)
}

func (s *SkillService) CreateSessionTable(ctx context.Context, sessionID uuid.UUID) error {
	if err := execStatements(ctx, s.db, s.createSessionTableSQL(sessionID)); err != nil {
		return fmt.Errorf("failed to create session skills table: %w", err)
	}

	s.markSessionTable(sessionID)

	return nil
}

func (s *SkillService) DropSessionTable(ctx context.Context, sessionID uuid.UUID) error {
	tableName := s.sessionTableName(sessionID)
	if usingSQLite() {
		ftsTableName := s.sessionFTSTableName(sessionID)
		if err := s.db.WithContext(ctx).Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", ftsTableName)).Error; err != nil {
			return fmt.Errorf("failed to drop session skills fts table: %w", err)
		}
	}

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

func (s *SkillService) startSessionTableCleanup(ctx context.Context) {
	s.mu.Lock()
	if s.cleanupCancelFn != nil {
		s.mu.Unlock()
		return
	}
	cleanupCtx, cancel := context.WithCancel(ctx)
	s.cleanupCancelFn = cancel
	s.mu.Unlock()

	safe.GoSafeWithCtx("session-skills-table-cleanup", cleanupCtx, func(ctx context.Context) {
		s.runSessionTableCleanup(ctx)
	})
}

func (s *SkillService) runSessionTableCleanup(ctx context.Context) {
	s.cleanupOldSessionTables(ctx)

	ticker := time.NewTicker(sessionSkillTableCleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupOldSessionTables(ctx)
		}
	}
}

func (s *SkillService) cleanupOldSessionTables(ctx context.Context) {
	cutoff := time.Now().Add(-sessionSkillTableTTL)
	var sessions []models.ChatSession
	if err := s.db.WithContext(ctx).
		Select("id").
		Where("cwd IS NOT NULL AND (deleted_at IS NOT NULL OR updated_at < ?)", cutoff).
		Find(&sessions).Error; err != nil {
		slog.WarnContext(ctx, "failed to list old sessions for skills table cleanup", "error", err)
		return
	}

	for _, session := range sessions {
		if err := s.DropSessionTable(ctx, session.ID); err != nil {
			slog.WarnContext(ctx, "failed to drop old session skills table", "sessionId", session.ID, "error", err)
		}
	}
}

func (s *SkillService) PopulateSessionSkills(ctx context.Context, sessionID uuid.UUID, cwd string) error {
	tableName := s.sessionTableName(sessionID)

	copyGlobalSQL := fmt.Sprintf(`
		INSERT INTO %s (id, name, description, skill_md_path, scope, source_dir, metadata, created_at, updated_at)
		SELECT id, name, description, skill_md_path, 'global', skill_md_path, metadata, created_at, updated_at
		FROM skills
	`, tableName)

	if err := s.db.WithContext(ctx).Exec(copyGlobalSQL).Error; err != nil {
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
				VALUES (?, ?, ?, ?, 'project', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
				ON CONFLICT (skill_md_path) DO NOTHING
			`, tableName)

			var metadataArg any
			if len(parsed.Metadata) > 0 {
				metadataArg = string(parsed.Metadata)
			}

			id, idErr := uuid.NewV7()
			if idErr != nil {
				slog.WarnContext(ctx, "failed to generate project skill id", "path", parsed.Path, "error", idErr)
				continue
			}
			if err := s.db.WithContext(ctx).Exec(insertSQL, id, parsed.Name, parsed.Description, parsed.Path, dir, metadataArg).Error; err != nil {
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
			VALUES (?, ?, ?, ?, 'project', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (skill_md_path) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				metadata = EXCLUDED.metadata,
				updated_at = CURRENT_TIMESTAMP
		`, tableName)

		var metadataArg any
		if len(parsed.Metadata) > 0 {
			metadataArg = string(parsed.Metadata)
		}

		id, idErr := uuid.NewV7()
		if idErr != nil {
			slog.WarnContext(ctx, "failed to generate project skill id", "path", parsed.Path, "error", idErr)
			return
		}
		if err := s.db.WithContext(ctx).Exec(insertSQL, id, parsed.Name, parsed.Description, parsed.Path, sourceDir, metadataArg).Error; err != nil {
			slog.WarnContext(ctx, "failed to upsert project skill", "path", parsed.Path, "error", err)
		}

	case event.Op&fsnotify.Remove != 0 || event.Op&fsnotify.Rename != 0:
		deleteSQL := fmt.Sprintf(`DELETE FROM %s WHERE skill_md_path = ? AND scope = 'project'`, tableName)
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
		if err := s.ensureSessionTableForSession(ctx, *sessionID); err != nil {
			return nil, err
		}

		s.mu.RLock()
		hasTable := s.sessionTables[*sessionID]
		s.mu.RUnlock()

		if hasTable {
			tableName := s.sessionTableName(*sessionID)
			var searchSQL string
			var args []any
			if usingSQLite() {
				ftsQuery := sqliteFTSQuery(query)
				if ftsQuery == "" {
					return results, nil
				}
				ftsTableName := s.sessionFTSTableName(*sessionID)
				searchSQL = fmt.Sprintf(`
					SELECT %s.id, %s.name, %s.description, %s.skill_md_path, %s.scope, -bm25(%s) as score
					FROM %s
					JOIN %s ON %s.id = %s.id
					WHERE %s MATCH ?
					ORDER BY bm25(%s) ASC
					LIMIT ?
				`, tableName, tableName, tableName, tableName, tableName, ftsTableName, ftsTableName, tableName, tableName, ftsTableName, ftsTableName, ftsTableName)
				args = []any{ftsQuery, limit}
			} else {
				searchSQL = fmt.Sprintf(`
					SELECT id, name, description, skill_md_path, scope, paradedb.score(id) as score
					FROM %s
					WHERE name @@@ ? OR description @@@ ?
					ORDER BY score DESC
					LIMIT ?
				`, tableName)
				args = []any{query, query, limit}
			}

			rows, err := s.db.WithContext(ctx).Raw(searchSQL, args...).Rows()
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

	var searchSQL string
	var args []any
	if usingSQLite() {
		ftsQuery := sqliteFTSQuery(query)
		if ftsQuery == "" {
			return results, nil
		}
		searchSQL = `
			SELECT skills.id, skills.name, skills.description, skills.skill_md_path, -bm25(skills_fts) as score
			FROM skills_fts
			JOIN skills ON skills.id = skills_fts.id
			WHERE skills_fts MATCH ?
			ORDER BY bm25(skills_fts) ASC
			LIMIT ?
		`
		args = []any{ftsQuery, limit}
	} else {
		searchSQL = `
			SELECT id, name, description, skill_md_path, paradedb.score(id) as score
			FROM skills
			WHERE name @@@ ? OR description @@@ ?
			ORDER BY score DESC
			LIMIT ?
		`
		args = []any{query, query, limit}
	}

	rows, err := s.db.WithContext(ctx).Raw(searchSQL, args...).Rows()
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
		if err := s.ensureSessionTableForSession(ctx, *sessionID); err != nil {
			return nil, err
		}

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
		if err := s.ensureSessionTableForSession(ctx, *sessionID); err != nil {
			return "", err
		}

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
	if err := s.ensureSessionTableForSession(ctx, sessionID); err != nil {
		return 0, err
	}

	s.mu.RLock()
	hasTable := s.sessionTables[sessionID]
	s.mu.RUnlock()

	if hasTable {
		return s.countSessionTableRows(ctx, sessionID)
	}

	var count int64
	if err := s.db.WithContext(ctx).Model(&models.Skill{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count global skills: %w", err)
	}
	return count, nil
}

func (s *SkillService) ListSessionSkillSummaries(ctx context.Context, sessionID uuid.UUID) ([]models.SkillDto, error) {
	if err := s.ensureSessionTableForSession(ctx, sessionID); err != nil {
		return nil, err
	}

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
			Scope:       models.SkillScopeGlobal,
		})
	}
	return results, nil
}
