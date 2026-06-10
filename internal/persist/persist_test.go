package persist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// === Helper Functions ===

func createTempDir(t *testing.T) string {
	return t.TempDir()
}

func createJSONFile(t *testing.T, dir, filename string, content interface{}) string {
	data, err := json.MarshalIndent(content, "", "  ")
	require.NoError(t, err)

	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, data, 0644))
	return path
}

func readJSONFile(t *testing.T, path string) map[string]interface{} {
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}

func readJSONArray(t *testing.T, data []byte) []interface{} {
	var result []interface{}
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}

// === ReadDir Tests ===

func TestReadDir(t *testing.T) {
	dir := createTempDir(t)

	// Create two JSON files
	createJSONFile(t, dir, "user1.json", map[string]interface{}{
		"id":   "1",
		"name": "Alice",
	})
	createJSONFile(t, dir, "user2.json", map[string]interface{}{
		"id":   "2",
		"name": "Bob",
	})

	result, err := ReadDir(dir)
	require.NoError(t, err)

	items := readJSONArray(t, result)
	require.Len(t, items, 2)

	// Should be sorted by filename: user1.json, user2.json
	item1 := items[0].(map[string]interface{})
	item2 := items[1].(map[string]interface{})
	assert.Equal(t, "1", item1["id"])
	assert.Equal(t, "Alice", item1["name"])
	assert.Equal(t, "2", item2["id"])
	assert.Equal(t, "Bob", item2["name"])
}

func TestReadDirEmpty(t *testing.T) {
	dir := createTempDir(t)

	result, err := ReadDir(dir)
	require.NoError(t, err)

	// Raw bytes must be "[]\n", not "null\n": an existing-but-empty directory
	// should serialize to an empty JSON array, not JSON null.
	assert.Equal(t, "[]\n", string(result))

	items := readJSONArray(t, result)
	assert.Empty(t, items)
}

func TestReadDirNonExistent(t *testing.T) {
	result, err := ReadDir("/non/existent/directory")
	require.NoError(t, err)

	items := readJSONArray(t, result)
	assert.Empty(t, items)
}

func TestReadDirSkipsNonJSON(t *testing.T) {
	dir := createTempDir(t)

	// Create a JSON file and a non-JSON file
	createJSONFile(t, dir, "data.json", map[string]interface{}{"id": "1"})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0644))

	result, err := ReadDir(dir)
	require.NoError(t, err)

	items := readJSONArray(t, result)
	require.Len(t, items, 1)

	item := items[0].(map[string]interface{})
	assert.Equal(t, "1", item["id"])
}

func TestReadDirSkipsSubdirectories(t *testing.T) {
	dir := createTempDir(t)

	// Create a JSON file and a subdirectory
	createJSONFile(t, dir, "data.json", map[string]interface{}{"id": "1"})
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0755))

	result, err := ReadDir(dir)
	require.NoError(t, err)

	items := readJSONArray(t, result)
	require.Len(t, items, 1)

	item := items[0].(map[string]interface{})
	assert.Equal(t, "1", item["id"])
}

func TestReadDirSortOrder(t *testing.T) {
	dir := createTempDir(t)

	// Create files in reverse alphabetical order
	createJSONFile(t, dir, "c.json", map[string]interface{}{"name": "C"})
	createJSONFile(t, dir, "a.json", map[string]interface{}{"name": "A"})
	createJSONFile(t, dir, "b.json", map[string]interface{}{"name": "B"})

	result, err := ReadDir(dir)
	require.NoError(t, err)

	items := readJSONArray(t, result)
	require.Len(t, items, 3)

	// Should be sorted by filename: a.json, b.json, c.json
	names := []string{
		items[0].(map[string]interface{})["name"].(string),
		items[1].(map[string]interface{})["name"].(string),
		items[2].(map[string]interface{})["name"].(string),
	}
	assert.Equal(t, []string{"A", "B", "C"}, names)
}

func TestReadDirInvalidJSON(t *testing.T) {
	dir := createTempDir(t)

	// Create a malformed JSON file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "invalid.json"), []byte("{invalid json"), 0644))

	_, err := ReadDir(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing JSON file")
}

// === Update Tests ===

func TestUpdate(t *testing.T) {
	dir := createTempDir(t)
	path := createJSONFile(t, dir, "user.json", map[string]interface{}{
		"id":    "1",
		"name":  "Alice",
		"email": "alice@old.com",
	})

	incoming := map[string]interface{}{
		"email": "alice@new.com",
		"age":   25,
	}

	result, err := Update(path, incoming)
	require.NoError(t, err)

	// Check returned result
	assert.Equal(t, "1", result["id"])
	assert.Equal(t, "Alice", result["name"])
	assert.Equal(t, "alice@new.com", result["email"])
	assert.Equal(t, 25, result["age"]) // Original type preserved in memory

	// Check file was updated
	fileContent := readJSONFile(t, path)
	assert.Equal(t, "1", fileContent["id"])
	assert.Equal(t, "Alice", fileContent["name"])
	assert.Equal(t, "alice@new.com", fileContent["email"])
	assert.Equal(t, float64(25), fileContent["age"])
}

func TestUpdatePreservesExisting(t *testing.T) {
	dir := createTempDir(t)
	path := createJSONFile(t, dir, "user.json", map[string]interface{}{
		"id":   "1",
		"name": "Alice",
		"role": "admin",
	})

	incoming := map[string]interface{}{
		"email": "alice@example.com",
	}

	result, err := Update(path, incoming)
	require.NoError(t, err)

	// All original fields should be preserved
	assert.Equal(t, "1", result["id"])
	assert.Equal(t, "Alice", result["name"])
	assert.Equal(t, "admin", result["role"])
	assert.Equal(t, "alice@example.com", result["email"])
}

func TestUpdateNotFound(t *testing.T) {
	incoming := map[string]interface{}{"name": "Alice"}

	_, err := Update("/non/existent/file.json", incoming)
	assert.True(t, IsNotFound(err))

	var notFound *NotFoundError
	require.True(t, errors.As(err, &notFound))
	assert.Equal(t, "file", notFound.Key)
	assert.Equal(t, "/non/existent/file.json", notFound.Value)
}

func TestUpdateInvalidJSON(t *testing.T) {
	dir := createTempDir(t)
	path := filepath.Join(dir, "invalid.json")
	require.NoError(t, os.WriteFile(path, []byte("[1, 2, 3]"), 0644)) // array instead of object

	incoming := map[string]interface{}{"name": "Alice"}

	_, err := Update(path, incoming)
	assert.True(t, IsConfigError(err))
	assert.Contains(t, err.Error(), "does not contain a JSON object")
}

// === AppendToDir Tests ===

func TestAppendToDirWithKey(t *testing.T) {
	dir := createTempDir(t)

	incoming := map[string]interface{}{
		"userId": "123",
		"name":   "Alice",
	}

	createdPath, result, err := AppendToDir(dir, "userId", incoming)
	require.NoError(t, err)

	// Should return the incoming data unchanged
	assert.Equal(t, incoming, result)

	// Should create 123.json and return the path
	expectedPath := filepath.Join(dir, "123.json")
	assert.Equal(t, expectedPath, createdPath)
	assert.FileExists(t, expectedPath)

	content := readJSONFile(t, expectedPath)
	expected := map[string]interface{}{
		"userId": "123",
		"name":   "Alice",
	}
	assert.Equal(t, expected, content)
}

func TestAppendToDirAutoID(t *testing.T) {
	dir := createTempDir(t)

	incoming := map[string]interface{}{
		"name": "Alice",
	}

	_, result, err := AppendToDir(dir, "userId", incoming)
	require.NoError(t, err)

	// Should have injected a userId
	userId, exists := result["userId"]
	assert.True(t, exists)
	assert.NotEmpty(t, userId)
	assert.Equal(t, "Alice", result["name"])

	// Should create {uuid}.json
	expectedPath := filepath.Join(dir, userId.(string)+".json")
	assert.FileExists(t, expectedPath)

	content := readJSONFile(t, expectedPath)
	assert.Equal(t, result, content)
}

func TestAppendToDirNoKey(t *testing.T) {
	dir := createTempDir(t)

	incoming := map[string]interface{}{
		"name": "Alice",
	}

	_, result, err := AppendToDir(dir, "", incoming)
	require.NoError(t, err)

	// Should return unchanged (no key injection)
	assert.Equal(t, incoming, result)

	// Should create a UUID filename but no userId field
	_, hasUserId := result["userId"]
	assert.False(t, hasUserId)

	// Find the created file (UUID filename)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	filename := entries[0].Name()
	assert.True(t, strings.HasSuffix(filename, ".json"))

	path := filepath.Join(dir, filename)
	content := readJSONFile(t, path)
	assert.Equal(t, incoming, content)
}

func TestAppendToDirCreatesDir(t *testing.T) {
	tempBase := createTempDir(t)
	nonExistentDir := filepath.Join(tempBase, "new", "nested", "dir")

	incoming := map[string]interface{}{
		"id":   "1",
		"name": "Alice",
	}

	_, _, err := AppendToDir(nonExistentDir, "id", incoming)
	require.NoError(t, err)

	// Directory should now exist
	assert.DirExists(t, nonExistentDir)

	// File should exist
	expectedPath := filepath.Join(nonExistentDir, "1.json")
	assert.FileExists(t, expectedPath)
}

func TestAppendToDirSanitizesFilename(t *testing.T) {
	dir := createTempDir(t)

	incoming := map[string]interface{}{
		"id":   "user@example.com/profile",
		"name": "Alice",
	}

	_, _, err := AppendToDir(dir, "id", incoming)
	require.NoError(t, err)

	// Check what files were actually created
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	actualFilename := entries[0].Name()
	t.Logf("Actual filename: %s", actualFilename)

	// Should sanitize the filename: @ and / become _, but . is preserved
	expectedFilename := "user_example.com_profile.json"
	assert.Equal(t, expectedFilename, actualFilename)
}

// === DeleteFile Tests ===

func TestDeleteFile(t *testing.T) {
	dir := createTempDir(t)
	path := createJSONFile(t, dir, "user.json", map[string]interface{}{"id": "1"})

	err := DeleteFile(path)
	require.NoError(t, err)

	// File should no longer exist
	assert.NoFileExists(t, path)
}

func TestDeleteFileNotFound(t *testing.T) {
	err := DeleteFile("/non/existent/file.json")
	assert.True(t, IsNotFound(err))

	var notFound *NotFoundError
	require.True(t, errors.As(err, &notFound))
	assert.Equal(t, "file", notFound.Key)
	assert.Equal(t, "/non/existent/file.json", notFound.Value)
}

// === WriteStub Tests ===

func TestWriteStub(t *testing.T) {
	dir := createTempDir(t)
	path := filepath.Join(dir, "test.json")

	data := map[string]interface{}{
		"id":     "1",
		"name":   "Alice",
		"active": true,
	}

	err := WriteStub(path, data)
	require.NoError(t, err)

	// Check file was created with proper formatting
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expectedJSON := `{
  "active": true,
  "id": "1",
  "name": "Alice"
}
`
	assert.Equal(t, expectedJSON, string(content))
}

// === Error Type Tests ===

func TestNotFoundError(t *testing.T) {
	err := &NotFoundError{Key: "id", Value: "123"}
	assert.Equal(t, "not found: id=123", err.Error())
	assert.True(t, IsNotFound(err))
}

func TestConfigError(t *testing.T) {
	err := &ConfigError{Msg: "invalid configuration"}
	assert.Equal(t, "invalid configuration", err.Error())
	assert.True(t, IsConfigError(err))
}

func TestIsNotFoundWithWrappedError(t *testing.T) {
	originalErr := &NotFoundError{Key: "file", Value: "test.json"}
	wrappedErr := fmt.Errorf("operation failed: %w", originalErr)

	assert.True(t, IsNotFound(wrappedErr))
}

func TestIsConfigErrorWithWrappedError(t *testing.T) {
	originalErr := &ConfigError{Msg: "bad config"}
	wrappedErr := fmt.Errorf("validation failed: %w", originalErr)

	assert.True(t, IsConfigError(wrappedErr))
}
