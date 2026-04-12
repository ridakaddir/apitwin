// Package runtime manages the mutable mirror directory that keeps runtime
// stub mutations out of the seed (git-tracked) stub tree.
//
// On startup, the proxy server calls Mirror() with the config directory. That
// copies the whole config-dir tree (minus top-level config files and the
// runtime dir itself) into .apitwin/state/ next to the config. From then on,
// every read and write is redirected to the runtime dir via
// config.Loader.StubRoot(), so the seed files are never mutated by request
// traffic and git status stays clean after a session.
package runtime

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ridakaddir/apitwin/internal/logger"
)

// stateSubdir is the path (relative to the config dir) of the runtime mirror.
const stateSubdir = ".apitwin/state"

// DefaultPath returns the default runtime state directory for a given config
// directory: <configDir>/.apitwin/state.
func DefaultPath(configDir string) string {
	return filepath.Join(configDir, ".apitwin", "state")
}

// IsRuntimePath reports whether path looks like an apitwin runtime state
// directory. Used as a safety check in the reset subcommand so we refuse to
// delete unrelated paths.
func IsRuntimePath(path string) bool {
	clean := filepath.Clean(path)
	return strings.HasSuffix(clean, filepath.FromSlash(stateSubdir))
}

// Mirror wipes the runtime state dir under configDir and repopulates it by
// copying every file in the config tree (except top-level config files and
// the runtime dir itself). Returns the absolute runtime dir path.
//
// The mirror is intentionally idempotent and destructive: a fresh server
// start always gives a pristine runtime state. If you need persistence
// across runs, omit the flag or use a seed-only workflow.
func Mirror(configDir string) (string, error) {
	absConfig, err := filepath.Abs(configDir)
	if err != nil {
		return "", fmt.Errorf("resolving config dir %q: %w", configDir, err)
	}

	runtimeDir := DefaultPath(absConfig)

	// Wipe any previous runtime state so we start from a clean mirror.
	if err := os.RemoveAll(runtimeDir); err != nil {
		return "", fmt.Errorf("wiping runtime dir %q: %w", runtimeDir, err)
	}
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return "", fmt.Errorf("creating runtime dir %q: %w", runtimeDir, err)
	}

	// Walk the config tree and copy every file that is not:
	//   - a top-level config file (apitwin.toml, *.yaml, etc.)
	//   - anything under .apitwin/ (including the runtime dir itself)
	walkErr := filepath.WalkDir(absConfig, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(absConfig, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}

		// Skip the runtime dir and anything under .apitwin/.
		if rel == ".apitwin" || strings.HasPrefix(rel, ".apitwin"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip top-level config files — they live in the seed dir, not runtime.
		if !d.IsDir() && filepath.Dir(rel) == "." && isConfigFilename(d.Name()) {
			return nil
		}

		dst := filepath.Join(runtimeDir, rel)

		// Follow symlinks for regular files (dereference), skip symlinked dirs
		// with a warning to avoid pulling in content from outside the tree.
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, terr := os.Stat(path)
			if terr != nil {
				logger.Warn("runtime mirror: skipping broken symlink", "path", rel, "err", terr)
				return nil
			}
			if target.IsDir() {
				logger.Warn("runtime mirror: skipping symlinked directory", "path", rel)
				return nil
			}
			// Fall through to copyFile which reads through the symlink.
		}

		if d.IsDir() {
			return os.MkdirAll(dst, info.Mode().Perm()|0700)
		}
		return copyFile(path, dst, info.Mode().Perm())
	})
	if walkErr != nil {
		return "", fmt.Errorf("mirroring config tree: %w", walkErr)
	}

	return runtimeDir, nil
}

// Ephemeral creates a fresh temporary directory for runtime state. Used under
// --ephemeral so nothing touches the config dir on disk. Callers must invoke
// Cleanup on shutdown to remove it.
func Ephemeral() (string, error) {
	dir, err := os.MkdirTemp("", "apitwin-state-*")
	if err != nil {
		return "", fmt.Errorf("creating ephemeral runtime dir: %w", err)
	}
	return dir, nil
}

// MirrorInto copies the config tree into an existing runtime dir. Used for
// ephemeral mode where the destination is a tempdir, not under configDir.
// The destination is not wiped — the caller is responsible for creating it
// empty.
func MirrorInto(configDir, runtimeDir string) error {
	absConfig, err := filepath.Abs(configDir)
	if err != nil {
		return fmt.Errorf("resolving config dir %q: %w", configDir, err)
	}
	absRuntime, err := filepath.Abs(runtimeDir)
	if err != nil {
		return fmt.Errorf("resolving runtime dir %q: %w", runtimeDir, err)
	}

	return filepath.WalkDir(absConfig, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(absConfig, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		if rel == ".apitwin" || strings.HasPrefix(rel, ".apitwin"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && filepath.Dir(rel) == "." && isConfigFilename(d.Name()) {
			return nil
		}

		dst := filepath.Join(absRuntime, rel)
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, terr := os.Stat(path)
			if terr != nil {
				logger.Warn("runtime mirror: skipping broken symlink", "path", rel, "err", terr)
				return nil
			}
			if target.IsDir() {
				logger.Warn("runtime mirror: skipping symlinked directory", "path", rel)
				return nil
			}
		}

		if d.IsDir() {
			return os.MkdirAll(dst, info.Mode().Perm()|0700)
		}
		return copyFile(path, dst, info.Mode().Perm())
	})
}

// Cleanup removes a runtime dir. Used for ephemeral mode on shutdown and for
// the reset subcommand.
func Cleanup(runtimeDir string) error {
	if runtimeDir == "" {
		return nil
	}
	return os.RemoveAll(runtimeDir)
}

// copyFile copies src to dst with the given mode, creating parent
// directories as needed.
func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// isConfigFilename reports whether a filename at the root of the config dir
// should be treated as a config file and therefore excluded from the mirror.
func isConfigFilename(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".toml", ".yaml", ".yml", ".json":
		return true
	}
	return false
}
