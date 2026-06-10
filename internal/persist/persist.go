// Package persist contains the transport-agnostic stub file mutation logic
// for directory-based storage shared between the HTTP and gRPC handlers.
package persist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Package-level compiled regexp for filename sanitization
var filenameUnsafeChars = regexp.MustCompile(`[^a-zA-Z0-9_\-.]`)

// ReadDir aggregates all .json files in a directory into a JSON array.
// Returns [] for empty or non-existent directories.
func ReadDir(dirPath string) ([]byte, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte("[]\n"), nil
		}
		return nil, fmt.Errorf("reading directory %q: %w", dirPath, err)
	}

	// Non-nil empty slice so an existing-but-empty directory marshals to "[]"
	// rather than "null", keeping the "[] for empty or non-existent" contract.
	items := []interface{}{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading file %q: %w", filePath, err)
		}

		var item interface{}
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("parsing JSON file %q: %w", filePath, err)
		}

		items = append(items, item)
	}

	// os.ReadDir returns entries sorted by filename for deterministic ordering

	result, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling directory contents: %w", err)
	}

	// Add trailing newline for consistency
	result = append(result, '\n')
	return result, nil
}

// Update reads a JSON object file, shallow-merges incoming fields, and writes back.
// Returns the merged object.
func Update(filePath string, incoming map[string]interface{}) (map[string]interface{}, error) {
	// Guard: reject directories early with a clear error instead of a
	// cryptic "read <path>: is a directory" from os.ReadFile.
	if info, err := os.Stat(filePath); err == nil && info.IsDir() {
		return nil, &ConfigError{
			Msg: fmt.Sprintf("persist update target %q is a directory, not a file; check the 'file' field in the route case", filePath),
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &NotFoundError{Key: "file", Value: filePath}
		}
		return nil, fmt.Errorf("reading file %q: %w", filePath, err)
	}

	var existing map[string]interface{}
	if err := json.Unmarshal(data, &existing); err != nil {
		return nil, &ConfigError{
			Msg: fmt.Sprintf("file %q does not contain a JSON object: %v", filePath, err),
		}
	}

	// Shallow merge: incoming fields overwrite existing
	for k, v := range incoming {
		existing[k] = v
	}

	if err := WriteStub(filePath, existing); err != nil {
		return nil, err
	}

	notify(filePath, FileUpdated)
	return existing, nil
}

// AppendToDir creates a new JSON file in a directory. If key is specified and
// the incoming record has that field, it's used as the filename. Otherwise,
// a UUID is generated. If key is specified but missing from the record,
// the UUID is injected into the record.
//
// Returns the created file path, the (possibly enriched) record, and any error.
func AppendToDir(dirPath, key string, incoming map[string]interface{}) (string, map[string]interface{}, error) {
	var filename string

	if key != "" {
		if value, exists := incoming[key]; exists && value != nil && fmt.Sprintf("%v", value) != "" {
			filename = fmt.Sprintf("%v", value)
		} else {
			// Generate UUID and inject into record
			id := uuid.New().String()
			incoming[key] = id
			filename = id
		}
	} else {
		// No key field - just use UUID as filename
		filename = uuid.New().String()
	}

	// Sanitize filename - allow only safe characters
	filename = sanitizeFilename(filename)

	// Ensure directory exists
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", nil, fmt.Errorf("creating directory %q: %w", dirPath, err)
	}

	filePath := filepath.Join(dirPath, filename+".json")
	if err := WriteStub(filePath, incoming); err != nil {
		return "", nil, err
	}

	notify(filePath, FileCreated)
	return filePath, incoming, nil
}

// DeleteFile removes a single JSON file from disk.
func DeleteFile(filePath string) error {
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return &NotFoundError{Key: "file", Value: filePath}
		}
		return fmt.Errorf("checking file %q: %w", filePath, err)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("removing file %q: %w", filePath, err)
	}

	notify(filePath, FileDeleted)
	return nil
}

// WriteStub marshals a map to JSON and writes it to a file with indentation.
// NOTE: This is a low-level helper; it does NOT trigger change notifications.
// Use AppendToDir, Update, or DeleteFile for operations that notify listeners.
func WriteStub(filePath string, data map[string]interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling data: %w", err)
	}
	jsonData = append(jsonData, '\n')
	return os.WriteFile(filePath, jsonData, 0644)
}

// sanitizeFilename removes or replaces unsafe characters in filenames.
func sanitizeFilename(filename string) string {
	// Allow only alphanumeric, underscore, hyphen, and dot
	return filenameUnsafeChars.ReplaceAllString(filename, "_")
}

// NotFoundError is returned when a file or record cannot be found.
type NotFoundError struct {
	Key   string
	Value string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("not found: %s=%s", e.Key, e.Value)
}

// IsNotFound reports whether err (or any error in its chain) is a *NotFoundError.
func IsNotFound(err error) bool {
	var t *NotFoundError
	return errors.As(err, &t)
}

// ConfigError is returned for invalid configuration or file format issues.
// Maps to HTTP 400 / gRPC INVALID_ARGUMENT.
type ConfigError struct {
	Msg string
}

func (e *ConfigError) Error() string { return e.Msg }

// IsConfigError reports whether err (or any error in its chain) is a *ConfigError.
func IsConfigError(err error) bool {
	var t *ConfigError
	return errors.As(err, &t)
}
