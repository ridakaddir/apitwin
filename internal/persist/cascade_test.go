package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteCascade_BasicTrafficSplit(t *testing.T) {
	// Setup temporary test directory
	tmpDir, err := os.MkdirTemp("", "cascade_test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create test directory structure
	endpointDir := filepath.Join(tmpDir, "endpoints")
	deploymentDir := filepath.Join(tmpDir, "deployments", "test-endpoint")
	require.NoError(t, os.MkdirAll(endpointDir, 0755))
	require.NoError(t, os.MkdirAll(deploymentDir, 0755))

	// Create initial files
	endpointFile := filepath.Join(endpointDir, "test-endpoint.json")
	deployment1File := filepath.Join(deploymentDir, "deployment-1.json")
	deployment2File := filepath.Join(deploymentDir, "deployment-2.json")

	// Initial endpoint data
	initialEndpoint := map[string]interface{}{
		"endpointId": "test-endpoint",
		"name":       "Test Endpoint",
		"trafficSplit": map[string]interface{}{
			"deployment-1": 100,
			"deployment-2": 0,
		},
	}

	// Initial deployment data
	initialDeployment1 := map[string]interface{}{
		"deploymentId": "deployment-1",
		"deploymentSpec": map[string]interface{}{
			"trafficSplit": map[string]interface{}{
				"deployment-1": 100,
			},
		},
	}

	initialDeployment2 := map[string]interface{}{
		"deploymentId": "deployment-2",
		"deploymentSpec": map[string]interface{}{
			"trafficSplit": map[string]interface{}{
				"deployment-2": 0,
			},
		},
	}

	// Write initial files
	writeJSONFile(t, endpointFile, initialEndpoint)
	writeJSONFile(t, deployment1File, initialDeployment1)
	writeJSONFile(t, deployment2File, initialDeployment2)

	// Configure cascade operation
	caseConfig := config.Case{
		Merge: "cascade",
		Primary: &config.CascadePrimary{
			File:  endpointFile,
			Merge: "update",
			Path:  "trafficSplit",
		},
		Cascade: []config.CascadeTarget{
			{
				Pattern:   filepath.Join(deploymentDir, "*.json"),
				Merge:     "update",
				Path:      "deploymentSpec.trafficSplit",
				Transform: "$.trafficSplit",
			},
		},
	}

	// Update data
	updateData := map[string]interface{}{
		"trafficSplit": map[string]interface{}{
			"deployment-1": 70,
			"deployment-2": 30,
		},
	}

	// Context
	context := RequestContext{
		Body: updateData,
		PathParams: map[string]string{
			"endpointId": "test-endpoint",
		},
	}

	// Execute cascade
	err = ExecuteCascade(caseConfig, updateData, context)
	require.NoError(t, err)

	// Verify endpoint file updated
	endpointData := readTestJSONFile(t, endpointFile)
	assert.Equal(t, 70, int(endpointData["trafficSplit"].(map[string]interface{})["deployment-1"].(float64)))
	assert.Equal(t, 30, int(endpointData["trafficSplit"].(map[string]interface{})["deployment-2"].(float64)))

	// Verify deployment files updated
	deployment1Data := readTestJSONFile(t, deployment1File)
	deploymentSpec1 := deployment1Data["deploymentSpec"].(map[string]interface{})
	trafficSplit1 := deploymentSpec1["trafficSplit"].(map[string]interface{})
	assert.Equal(t, 70, int(trafficSplit1["deployment-1"].(float64)))
	assert.Equal(t, 30, int(trafficSplit1["deployment-2"].(float64)))

	deployment2Data := readTestJSONFile(t, deployment2File)
	deploymentSpec2 := deployment2Data["deploymentSpec"].(map[string]interface{})
	trafficSplit2 := deploymentSpec2["trafficSplit"].(map[string]interface{})
	assert.Equal(t, 70, int(trafficSplit2["deployment-1"].(float64)))
	assert.Equal(t, 30, int(trafficSplit2["deployment-2"].(float64)))
}

func TestExecuteCascade_RollbackOnFailure(t *testing.T) {
	// Setup temporary test directory
	tmpDir, err := os.MkdirTemp("", "cascade_rollback_test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create test files
	endpointFile := filepath.Join(tmpDir, "endpoint.json")
	invalidFile := filepath.Join(tmpDir, "invalid.json")

	// Initial endpoint data
	initialEndpoint := map[string]interface{}{
		"trafficSplit": map[string]interface{}{"deployment-1": 100},
	}
	writeJSONFile(t, endpointFile, initialEndpoint)

	// Create invalid JSON file that will cause update to fail
	err = os.WriteFile(invalidFile, []byte("invalid json"), 0644)
	require.NoError(t, err)

	// Configure cascade operation that will fail
	caseConfig := config.Case{
		Merge: "cascade",
		Primary: &config.CascadePrimary{
			File:  endpointFile,
			Merge: "update",
		},
		Cascade: []config.CascadeTarget{
			{
				Pattern: invalidFile,
				Merge:   "update",
			},
		},
	}

	updateData := map[string]interface{}{
		"trafficSplit": map[string]interface{}{"deployment-1": 50},
	}

	context := RequestContext{Body: updateData}

	// Execute cascade - should fail and rollback
	err = ExecuteCascade(caseConfig, updateData, context)
	assert.Error(t, err)

	// Verify endpoint file was rolled back to original state
	endpointData := readTestJSONFile(t, endpointFile)
	trafficSplit := endpointData["trafficSplit"].(map[string]interface{})
	assert.Equal(t, 100, int(trafficSplit["deployment-1"].(float64)))
}

func TestExecuteCascade_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		caseConfig  config.Case
		expectError string
	}{
		{
			name: "missing primary",
			caseConfig: config.Case{
				Merge:   "cascade",
				Primary: nil,
			},
			expectError: "cascade operation requires primary file configuration",
		},
		{
			name: "no cascade targets",
			caseConfig: config.Case{
				Merge: "cascade",
				Primary: &config.CascadePrimary{
					File:  "test.json",
					Merge: "update",
				},
				Cascade: []config.CascadeTarget{},
			},
			expectError: "cascade operation requires at least one cascade target",
		},
		{
			name: "too many cascade targets",
			caseConfig: config.Case{
				Merge: "cascade",
				Primary: &config.CascadePrimary{
					File:  "test.json",
					Merge: "update",
				},
				Cascade: make([]config.CascadeTarget, 11), // More than 10
			},
			expectError: "too many cascade targets: 11 (maximum 10 allowed)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExecuteCascade(tt.caseConfig, map[string]interface{}{}, RequestContext{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectError)
		})
	}
}

// Helper functions
func writeJSONFile(t *testing.T, filePath string, data interface{}) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(filePath, jsonData, 0644)
	require.NoError(t, err)
}

func readTestJSONFile(t *testing.T, filePath string) map[string]interface{} {
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	return result
}
