package runtime

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GitignoreLine is the line appended to .gitignore by EnsureGitignore and
// checked by CheckGitignore. Keeping it in one place ensures --init, generate
// and the startup warning all agree on what "ignored" means.
const GitignoreLine = ".apitwin/state/"

// EnsureGitignore creates or appends to <dir>/.gitignore so that the runtime
// state directory is excluded from git. Safe to call repeatedly: if the line
// (or any enclosing pattern) is already present, this is a no-op.
func EnsureGitignore(dir string) error {
	path := filepath.Join(dir, ".gitignore")

	// If the file already contains a matching pattern, do nothing.
	if hasIgnorePattern(path) {
		return nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}
	defer func() { _ = f.Close() }()

	// If the file is non-empty and doesn't end with a newline, add one so our
	// line doesn't glue onto the previous entry.
	info, statErr := f.Stat()
	prefix := ""
	if statErr == nil && info.Size() > 0 {
		if !endsWithNewline(path) {
			prefix = "\n"
		}
	}

	_, err = f.WriteString(prefix + GitignoreLine + "\n")
	return err
}

// CheckGitignore reports whether .apitwin/state/ is ignored by a .gitignore
// in dir or any parent directory up to (and including) the repo root. This
// is a best-effort lexical check — it does not invoke git.
func CheckGitignore(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}

	current := abs
	for {
		if hasIgnorePattern(filepath.Join(current, ".gitignore")) {
			return true
		}
		// Stop walking up at the repo root.
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

// hasIgnorePattern returns true if path exists and contains a line that
// would match the runtime state directory. Handles the obvious variations:
// ".apitwin/state/", ".apitwin/state", ".apitwin/", ".apitwin".
func hasIgnorePattern(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip trailing slash for comparison so ".apitwin/state" and
		// ".apitwin/state/" both match.
		stripped := strings.TrimSuffix(line, "/")
		switch stripped {
		case ".apitwin/state", ".apitwin":
			return true
		}
	}
	return false
}

// endsWithNewline reports whether the file at path ends with '\n'. Returns
// false on any read error — callers treat "unknown" as "need newline".
func endsWithNewline(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return false
	}
	buf := make([]byte, 1)
	if _, err := f.ReadAt(buf, info.Size()-1); err != nil {
		return false
	}
	return buf[0] == '\n'
}
