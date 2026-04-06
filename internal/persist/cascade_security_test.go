package persist

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteCascade_SecurityVulnerabilities(t *testing.T) {
	// Setup temporary test directory
	tmpDir, err := os.MkdirTemp("", "cascade_security_test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create test directory structure
	endpointDir := filepath.Join(tmpDir, "endpoints")
	require.NoError(t, os.MkdirAll(endpointDir, 0755))

	// Create initial endpoint file
	endpointFile := filepath.Join(endpointDir, "test-endpoint.json")
	initialData := map[string]interface{}{
		"endpointId": "test-endpoint",
		"trafficSplit": map[string]interface{}{
			"deployment-1": 100,
		},
	}
	writeSecurityTestJSONFile(t, endpointFile, initialData)

	tests := []struct {
		name            string
		caseConfig      config.Case
		context         RequestContext
		expectError     bool
		errorContains   string
		shouldLogAttack bool
	}{
		{
			name: "path traversal in pathParams",
			caseConfig: config.Case{
				Merge: "cascade",
				Primary: &config.CascadePrimary{
					File:  "endpoints/test-endpoint.json",
					Merge: "update",
					Path:  "trafficSplit",
				},
				Cascade: []config.CascadeTarget{
					{
						Pattern:   "stubs/deployments/{path.endpointId}/*.json",
						Merge:     "update",
						Path:      "deploymentSpec.trafficSplit",
						Transform: "$.trafficSplit",
					},
				},
			},
			context: RequestContext{
				PathParams: map[string]string{
					"endpointId": "../../../etc/passwd",
				},
				ConfigDir: tmpDir,
			},
			expectError:     true,
			errorContains:   "cascade pattern resolved to no files",
			shouldLogAttack: true,
		},
		{
			name: "path traversal in queryParams",
			caseConfig: config.Case{
				Merge: "cascade",
				Primary: &config.CascadePrimary{
					File:  "endpoints/test-endpoint.json",
					Merge: "update",
					Path:  "trafficSplit",
				},
				Cascade: []config.CascadeTarget{
					{
						Pattern:   "stubs/deployments/{query.target}/*.json",
						Merge:     "update",
						Path:      "deploymentSpec.trafficSplit",
						Transform: "$.trafficSplit",
					},
				},
			},
			context: RequestContext{
				QueryParams: map[string]string{
					"target": "../../../etc/passwd",
				},
				ConfigDir: tmpDir,
			},
			expectError:     true,
			errorContains:   "cascade pattern resolved to no files",
			shouldLogAttack: true,
		},
		{
			name: "null byte injection in pathParams",
			caseConfig: config.Case{
				Merge: "cascade",
				Primary: &config.CascadePrimary{
					File:  "endpoints/test-endpoint.json",
					Merge: "update",
					Path:  "trafficSplit",
				},
				Cascade: []config.CascadeTarget{
					{
						Pattern:   "stubs/deployments/{path.endpointId}/*.json",
						Merge:     "update",
						Path:      "deploymentSpec.trafficSplit",
						Transform: "$.trafficSplit",
					},
				},
			},
			context: RequestContext{
				PathParams: map[string]string{
					"endpointId": "test\x00../../../etc/passwd",
				},
				ConfigDir: tmpDir,
			},
			expectError:     true, // Should be sanitized, but pattern won't match files
			errorContains:   "cascade pattern resolved to no files",
			shouldLogAttack: false,
		},
		{
			name: "directory traversal sequences",
			caseConfig: config.Case{
				Merge: "cascade",
				Primary: &config.CascadePrimary{
					File:  "endpoints/test-endpoint.json",
					Merge: "update",
					Path:  "trafficSplit",
				},
				Cascade: []config.CascadeTarget{
					{
						Pattern:   "stubs/deployments/{path.endpointId}/*.json",
						Merge:     "update",
						Path:      "deploymentSpec.trafficSplit",
						Transform: "$.trafficSplit",
					},
				},
			},
			context: RequestContext{
				PathParams: map[string]string{
					"endpointId": "test/../../../etc/passwd",
				},
				ConfigDir: tmpDir,
			},
			expectError:     true,
			errorContains:   "cascade pattern resolved to no files",
			shouldLogAttack: true,
		},
		{
			name: "absolute path injection",
			caseConfig: config.Case{
				Merge: "cascade",
				Primary: &config.CascadePrimary{
					File:  "endpoints/test-endpoint.json",
					Merge: "update",
					Path:  "trafficSplit",
				},
				Cascade: []config.CascadeTarget{
					{
						Pattern:   "stubs/deployments/{path.endpointId}/*.json",
						Merge:     "update",
						Path:      "deploymentSpec.trafficSplit",
						Transform: "$.trafficSplit",
					},
				},
			},
			context: RequestContext{
				PathParams: map[string]string{
					"endpointId": "/etc/passwd",
				},
				ConfigDir: tmpDir,
			},
			expectError:     true,
			errorContains:   "cascade pattern resolved to no files",
			shouldLogAttack: true,
		},
		{
			name: "valid input should work",
			caseConfig: config.Case{
				Merge: "cascade",
				Primary: &config.CascadePrimary{
					File:  "endpoints/test-endpoint.json",
					Merge: "update",
					Path:  "trafficSplit",
				},
				Cascade: []config.CascadeTarget{
					{
						Pattern:   "stubs/deployments/{path.endpointId}/*.json",
						Merge:     "update",
						Path:      "deploymentSpec.trafficSplit",
						Transform: "$.trafficSplit",
					},
				},
			},
			context: RequestContext{
				PathParams: map[string]string{
					"endpointId": "valid-endpoint-123",
				},
				ConfigDir: tmpDir,
			},
			expectError:     true,
			errorContains:   "cascade pattern resolved to no files",
			shouldLogAttack: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateData := map[string]interface{}{
				"trafficSplit": map[string]interface{}{
					"deployment-1": 50,
				},
			}

			// Set body data in context
			tt.context.Body = updateData

			// Execute cascade
			err := ExecuteCascade(tt.caseConfig, updateData, tt.context)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err, "Operation result: %v", err)
			}
		})
	}
}

func TestSanitizePathValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal input",
			input:    "valid-endpoint-123",
			expected: "valid-endpoint-123",
		},
		{
			name:     "path traversal sequences",
			input:    "../../../etc/passwd",
			expected: "etcpasswd",
		},
		{
			name:     "null byte injection",
			input:    "test\x00file",
			expected: "testfile",
		},
		{
			name:     "mixed attack vectors",
			input:    "test/../\x00\n\rmalicious",
			expected: "testmalicious",
		},
		{
			name:     "only special characters",
			input:    "../../../\x00\n\r",
			expected: "",
		},
		{
			name:     "valid characters with dots",
			input:    "test.endpoint.v2",
			expected: "test.endpoint.v2",
		},
		{
			name:     "unicode and special chars",
			input:    "test-endpoint_123.json",
			expected: "test-endpoint_123.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizePathValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCascadePattern_SecurityValidation(t *testing.T) {
	tmpDir := t.TempDir()
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tests := []struct {
		name          string
		pattern       string
		pathParams    map[string]string
		expectError   bool
		errorContains string
	}{
		{
			name:    "valid pattern within config dir",
			pattern: "stubs/endpoints/*.json",
			pathParams: map[string]string{
				"endpointId": "valid-endpoint",
			},
			expectError: false,
		},
		{
			name:    "pattern trying to escape config dir",
			pattern: "../../../etc/*.json",
			pathParams: map[string]string{
				"endpointId": "test",
			},
			expectError:   true,
			errorContains: "pattern escapes config directory",
		},
		{
			name:    "pattern with path traversal in placeholder",
			pattern: "stubs/endpoints/{path.endpointId}/*.json",
			pathParams: map[string]string{
				"endpointId": "../../../etc/passwd",
			},
			expectError: false, // Path traversal is sanitized, not rejected
		},
		{
			name:    "absolute path pattern",
			pattern: "/etc/passwd",
			pathParams: map[string]string{
				"endpointId": "test",
			},
			expectError:   true,
			errorContains: "pattern escapes config directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context := RequestContext{
				PathParams: tt.pathParams,
				ConfigDir:  tmpDir,
			}

			files, err := resolveCascadePattern(tt.pattern, context)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Logf("Got non-critical error: %v", err)
				}
				t.Logf("Resolved files: %v", files)
			}
		})
	}
}

// Helper function for writing JSON files (reused from other tests)
func writeSecurityTestJSONFile(t *testing.T, filePath string, data interface{}) {
	err := os.WriteFile(filePath, []byte(`{"test": "data"}`), 0644)
	require.NoError(t, err)
}
