package persist

import (
	"os"
	"strings"
)

// IsDirectoryPath determines if a file path should be treated as a directory
// for stub aggregation. Returns true if the resolved path is an existing
// directory on disk, or if the original config path ended with "/" (indicating
// directory intent even when the directory does not yet exist).
//
// Shared by REST and gRPC persist dispatchers.
func IsDirectoryPath(resolvedPath, originalConfigFile string) bool {
	info, err := os.Stat(resolvedPath)
	if err == nil && info.IsDir() {
		return true
	}
	if os.IsNotExist(err) && strings.HasSuffix(originalConfigFile, "/") {
		return true
	}
	return false
}
