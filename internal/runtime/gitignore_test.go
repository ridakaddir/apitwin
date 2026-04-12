package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureGitignore_CreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()

	if err := EnsureGitignore(dir); err != nil {
		t.Fatalf("EnsureGitignore: %v", err)
	}

	data := readGitignore(t, dir)
	if !strings.Contains(data, GitignoreLine) {
		t.Errorf("expected %q in new .gitignore, got %q", GitignoreLine, data)
	}
}

func TestEnsureGitignore_AppendsWhenLineMissing(t *testing.T) {
	dir := t.TempDir()

	existing := "node_modules\ndist\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0644); err != nil {
		t.Fatalf("seed .gitignore: %v", err)
	}

	if err := EnsureGitignore(dir); err != nil {
		t.Fatalf("EnsureGitignore: %v", err)
	}

	data := readGitignore(t, dir)
	if !strings.Contains(data, "node_modules") {
		t.Errorf("pre-existing entry lost: %q", data)
	}
	if !strings.Contains(data, GitignoreLine) {
		t.Errorf("runtime line not appended: %q", data)
	}
}

func TestEnsureGitignore_NoOpWhenAlreadyPresent(t *testing.T) {
	dir := t.TempDir()

	existing := "foo\n" + GitignoreLine + "\nbar\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := EnsureGitignore(dir); err != nil {
		t.Fatalf("EnsureGitignore: %v", err)
	}

	data := readGitignore(t, dir)
	count := strings.Count(data, GitignoreLine)
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of %q, got %d: %q", GitignoreLine, count, data)
	}
}

func TestEnsureGitignore_AppendsNewlineWhenFileLacksTrailingNewline(t *testing.T) {
	dir := t.TempDir()

	// Deliberately no trailing newline.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("foo"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := EnsureGitignore(dir); err != nil {
		t.Fatalf("EnsureGitignore: %v", err)
	}

	data := readGitignore(t, dir)
	if strings.Contains(data, "foo"+GitignoreLine) {
		t.Errorf("new line glued to previous entry without separator: %q", data)
	}
	if !strings.Contains(data, "foo\n"+GitignoreLine) {
		t.Errorf("expected 'foo\\n%s' in: %q", GitignoreLine, data)
	}
}

func TestCheckGitignore_DetectsExactLine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(GitignoreLine+"\n"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if !CheckGitignore(dir) {
		t.Error("expected CheckGitignore=true for exact-match .gitignore")
	}
}

func TestCheckGitignore_DetectsParentApitwin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".apitwin/\n"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if !CheckGitignore(dir) {
		t.Error("expected CheckGitignore=true for '.apitwin/' pattern")
	}
}

func TestCheckGitignore_FalseWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules\n"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Mark as repo root so CheckGitignore stops walking up.
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	if CheckGitignore(dir) {
		t.Error("expected CheckGitignore=false for unrelated .gitignore")
	}
}

func readGitignore(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	return string(data)
}
