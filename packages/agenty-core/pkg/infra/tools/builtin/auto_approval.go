package builtin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
)

const (
	patchBeginMarker  = "*** Begin Patch"
	patchEndMarker    = "*** End Patch"
	patchUpdateMarker = "*** Update File:"
	patchDeleteMarker = "*** Delete File:"
	patchAddMarker    = "*** Add File:"
	patchMoveMarker   = "*** Move to:"
)

func (tool *readFileTool) CanAutoApprove(ctx agentloop.CallContext, input []byte) bool {
	var args readFileArguments
	return decodeArguments(input, &args) == nil && ordinaryPathInsideCwd(args.Path, ctx.Cwd, false)
}

func (tool *grepTool) CanAutoApprove(ctx agentloop.CallContext, input []byte) bool {
	var args grepArguments
	if decodeArguments(input, &args) != nil || !ordinaryPathInsideCwd(args.Path, ctx.Cwd, true) {
		return false
	}
	root, err := resolvePath(args.Path, ctx.Cwd, true)
	if err != nil {
		return false
	}
	return ordinarySearchTargets(root, args.Glob)
}

func (tool *globTool) CanAutoApprove(ctx agentloop.CallContext, input []byte) bool {
	var args globArguments
	return decodeArguments(input, &args) == nil && pathInsideCwd(args.Path, ctx.Cwd, true)
}

func (tool *listTool) CanAutoApprove(ctx agentloop.CallContext, input []byte) bool {
	var args listArguments
	return decodeArguments(input, &args) == nil && pathInsideCwd(args.Path, ctx.Cwd, true)
}

func (tool *applyPatchTool) CanAutoApprove(ctx agentloop.CallContext, input []byte) bool {
	var args applyPatchArguments
	if decodeArguments(input, &args) != nil {
		return false
	}
	if args.Operation != nil {
		if strings.TrimSpace(args.Patch) != "" || !validPatchOperation(args.Operation) || !patchPathInsideCwd(args.Operation.Path, ctx.Cwd) {
			return false
		}
		return args.Operation.MoveTo == "" || patchPathInsideCwd(args.Operation.MoveTo, ctx.Cwd)
	}

	paths, ok := patchPaths(args.Patch)
	if !ok {
		return false
	}
	for _, path := range paths {
		if !patchPathInsideCwd(path, ctx.Cwd) {
			return false
		}
	}
	return true
}

func (tool *textEditorTool) CanAutoApprove(ctx agentloop.CallContext, input []byte) bool {
	var args textEditorArguments
	if decodeArguments(input, &args) != nil || validateTextEditorArguments(args) != nil {
		return false
	}
	if args.Command == "view" {
		return ordinaryPathInsideCwd(args.Path, ctx.Cwd, false)
	}
	return ordinaryPathInsideCwd(args.Path, ctx.Cwd, false)
}

func validPatchOperation(operation *conversation.ApplyPatchOperation) bool {
	if operation == nil {
		return false
	}
	switch operation.Type {
	case conversation.ApplyPatchCreateFile, conversation.ApplyPatchUpdateFile, conversation.ApplyPatchDeleteFile:
	default:
		return false
	}
	return operation.MoveTo == "" || operation.Type == conversation.ApplyPatchUpdateFile
}

func pathInsideCwd(path, cwd string, allowEmpty bool) bool {
	resolved, err := resolvePath(path, cwd, allowEmpty)
	if err != nil {
		return false
	}
	return absolutePathInsideCwd(resolved, cwd)
}

func patchPathInsideCwd(path, cwd string) bool {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(cwd) == "" {
		return false
	}

	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(cwd, resolved)
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return false
	}
	return ordinaryAbsolutePathInsideCwd(filepath.Clean(abs), cwd)
}

// Content-reading tools and patches do not bypass review for sensitive targets.
// Listing tools only reveal names and continue using the containment check.
func ordinaryPathInsideCwd(path, cwd string, allowEmpty bool) bool {
	resolved, err := resolvePath(path, cwd, allowEmpty)
	return err == nil && ordinaryAbsolutePathInsideCwd(resolved, cwd)
}

func ordinaryAbsolutePathInsideCwd(path, cwd string) bool {
	if !absolutePathInsideCwd(path, cwd) || sensitiveApprovalPath(path) {
		return false
	}
	target, err := canonicalExistingPrefix(path)
	return err == nil && !sensitiveApprovalPath(target)
}

func sensitiveApprovalPath(path string) bool {
	normalized := strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
	// Absolute OS roots remain sensitive even when the user chose one as cwd.
	systemPath := strings.TrimPrefix(normalized, strings.ToLower(filepath.VolumeName(path)))
	for _, root := range []string{
		"/etc", "/private/etc", "/system", "/library", "/usr", "/bin", "/sbin",
		"/boot", "/proc", "/sys", "/dev", "/windows", "/program files", "/programdata",
	} {
		if systemPath == root || strings.HasPrefix(systemPath, root+"/") {
			return true
		}
	}
	for _, part := range strings.Split(normalized, "/") {
		switch part {
		case ".ssh", ".aws", ".azure", ".gnupg", ".kube", ".git", "gcloud", "keychains",
			".npmrc", ".netrc", "_netrc", ".pypirc", ".git-credentials", "credentials.json",
			"service-account.json", "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519":
			return true
		}
		if strings.HasSuffix(part, ".key") || strings.HasSuffix(part, ".pem") ||
			strings.HasSuffix(part, ".p12") || strings.HasSuffix(part, ".pfx") {
			return true
		}
		if part == ".env" || strings.HasPrefix(part, ".env.") {
			if !strings.HasSuffix(part, ".example") && !strings.HasSuffix(part, ".sample") &&
				!strings.HasSuffix(part, ".template") {
				return true
			}
		}
	}
	return strings.Contains(normalized, "/.docker/config.json") ||
		strings.Contains(normalized, "/.agenty/providers/")
}

// grep reads recursively, including hidden files. Inspect names only (never
// contents), honoring its glob and symlink behavior. Bound the metadata walk;
// large or unreadable trees go to the reviewer instead of blocking indefinitely.
func ordinarySearchTargets(root, glob string) bool {
	if glob != "" && validateGlob(glob) != nil {
		return false
	}
	visited := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		visited++
		if visited > 4096 {
			return errors.New("search target inspection limit reached")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			// grep follows a symlink supplied as its root, but not descendants.
			if path == root {
				return errors.New("search root is a symlink")
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			relative = filepath.Base(root)
		}
		if glob != "" {
			matches, err := matchGlob(glob, filepath.ToSlash(relative))
			if err != nil {
				return err
			}
			if !matches {
				return nil
			}
		}
		if sensitiveApprovalPath(path) {
			return errors.New("search includes sensitive targets")
		}
		return nil
	})
	return err == nil
}

func absolutePathInsideCwd(path, cwd string) bool {
	if strings.TrimSpace(cwd) == "" {
		return false
	}

	root, err := filepath.Abs(cwd)
	if err != nil {
		return false
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return false
	}

	target, err := canonicalExistingPrefix(path)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func canonicalExistingPrefix(path string) (string, error) {
	path = filepath.Clean(path)
	missing := make([]string, 0)
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return filepath.Join(append([]string{resolved}, missing...)...), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		missing = append([]string{filepath.Base(path)}, missing...)
		path = parent
	}
}

func patchPaths(patch string) ([]string, bool) {
	lines := strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) < 3 || lines[0] != patchBeginMarker || lines[len(lines)-1] != patchEndMarker {
		return nil, false
	}

	paths := make([]string, 0)
	for index := 1; index < len(lines)-1; {
		path, update, ok := patchOperationPath(lines[index])
		if !ok {
			return nil, false
		}
		paths = append(paths, path)
		index++

		if update && index < len(lines)-1 && strings.HasPrefix(lines[index], patchMoveMarker) {
			moveTo := strings.TrimSpace(strings.TrimPrefix(lines[index], patchMoveMarker))
			if moveTo == "" {
				return nil, false
			}
			paths = append(paths, moveTo)
			index++
		}

		for index < len(lines)-1 && !isPatchOperationHeader(lines[index]) {
			index++
		}
	}
	return paths, len(paths) > 0
}

func patchOperationPath(line string) (string, bool, bool) {
	for _, marker := range []struct {
		value  string
		update bool
	}{
		{value: patchUpdateMarker, update: true},
		{value: patchDeleteMarker},
		{value: patchAddMarker},
	} {
		if strings.HasPrefix(line, marker.value) {
			path := strings.TrimSpace(strings.TrimPrefix(line, marker.value))
			return path, marker.update, path != ""
		}
	}
	return "", false, false
}

func isPatchOperationHeader(line string) bool {
	_, _, ok := patchOperationPath(line)
	return ok
}
