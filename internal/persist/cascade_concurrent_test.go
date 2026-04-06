package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteCascade_ConcurrentOperations(t *testing.T) {
	// Setup temporary test directory
	tmpDir, err := os.MkdirTemp("", "cascade_concurrent_test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create test directory structure
	endpointDir := filepath.Join(tmpDir, "endpoints")
	deploymentDir := filepath.Join(tmpDir, "deployments", "test-endpoint")
	require.NoError(t, os.MkdirAll(endpointDir, 0755))
	require.NoError(t, os.MkdirAll(deploymentDir, 0755))

	// Create initial files for concurrent testing
	const numConcurrentOps = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []error

	for i := 0; i < numConcurrentOps; i++ {
		endpointFile := filepath.Join(endpointDir, fmt.Sprintf("endpoint-%d.json", i))
		deploymentFile := filepath.Join(deploymentDir, fmt.Sprintf("deployment-%d.json", i))

		// Create initial files
		initialEndpoint := map[string]interface{}{
			"endpointId": fmt.Sprintf("endpoint-%d", i),
			"trafficSplit": map[string]interface{}{
				"deployment-1": 100,
			},
		}

		initialDeployment := map[string]interface{}{
			"deploymentId": fmt.Sprintf("deployment-%d", i),
			"deploymentSpec": map[string]interface{}{
				"trafficSplit": map[string]interface{}{
					"deployment-1": 100,
				},
			},
		}

		writeConcurrentTestJSONFile(t, endpointFile, initialEndpoint)
		writeConcurrentTestJSONFile(t, deploymentFile, initialDeployment)

		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Configure cascade operation
			caseConfig := config.Case{
				Merge: "cascade",
				Primary: &config.CascadePrimary{
					File:  filepath.Join(endpointDir, fmt.Sprintf("endpoint-%d.json", idx)),
					Merge: "update",
					Path:  "trafficSplit",
				},
				Cascade: []config.CascadeTarget{
					{
						Pattern:   filepath.Join(deploymentDir, fmt.Sprintf("deployment-%d.json", idx)),
						Merge:     "update",
						Path:      "deploymentSpec.trafficSplit",
						Transform: "$.trafficSplit",
					},
				},
			}

			updateData := map[string]interface{}{
				"trafficSplit": map[string]interface{}{
					"deployment-1": 50 + idx, // Different values to test concurrency
				},
			}

			context := RequestContext{
				Body: updateData,
				PathParams: map[string]string{
					"endpointId": fmt.Sprintf("endpoint-%d", idx),
				},
				ConfigDir: tmpDir,
			}

			// Execute cascade
			err := ExecuteCascade(caseConfig, updateData, context)

			// Store result
			mu.Lock()
			results = append(results, err)
			mu.Unlock()
		}(i)
	}

	// Wait for all operations to complete
	wg.Wait()

	// Verify all operations succeeded
	for i, err := range results {
		assert.NoError(t, err, "Operation %d should succeed", i)
	}

	// Verify final state consistency
	for i := 0; i < numConcurrentOps; i++ {
		endpointFile := filepath.Join(endpointDir, fmt.Sprintf("endpoint-%d.json", i))
		deploymentFile := filepath.Join(deploymentDir, fmt.Sprintf("deployment-%d.json", i))

		// Read and verify endpoint file
		endpointData := readConcurrentTestJSONFile(t, endpointFile)
		expectedValue := 50 + i
		actualValue := int(endpointData["trafficSplit"].(map[string]interface{})["deployment-1"].(float64))
		assert.Equal(t, expectedValue, actualValue, "Endpoint %d should have correct traffic split", i)

		// Read and verify deployment file
		deploymentData := readConcurrentTestJSONFile(t, deploymentFile)
		deploymentSpec := deploymentData["deploymentSpec"].(map[string]interface{})
		trafficSplit := deploymentSpec["trafficSplit"].(map[string]interface{})
		actualDeploymentValue := int(trafficSplit["deployment-1"].(float64))
		assert.Equal(t, expectedValue, actualDeploymentValue, "Deployment %d should have correct traffic split", i)
	}
}

func TestExecuteCascade_ConcurrentRollbacks(t *testing.T) {
	// Setup temporary test directory
	tmpDir, err := os.MkdirTemp("", "cascade_rollback_concurrent_test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create test directory structure
	endpointDir := filepath.Join(tmpDir, "endpoints")
	require.NoError(t, os.MkdirAll(endpointDir, 0755))

	// Test concurrent operations where some will fail and rollback
	const numConcurrentOps = 5
	var wg sync.WaitGroup
	var mu sync.Mutex
	var successCount, rollbackCount int

	for i := 0; i < numConcurrentOps; i++ {
		endpointFile := filepath.Join(endpointDir, fmt.Sprintf("endpoint-%d.json", i))
		invalidFile := filepath.Join(tmpDir, fmt.Sprintf("invalid-%d.json", i))

		// Create valid endpoint file
		initialEndpoint := map[string]interface{}{
			"endpointId": fmt.Sprintf("endpoint-%d", i),
			"trafficSplit": map[string]interface{}{
				"deployment-1": 100,
			},
		}
		writeConcurrentTestJSONFile(t, endpointFile, initialEndpoint)

		// Create invalid file for some operations (to trigger rollback)
		if i%2 == 0 {
			err := os.WriteFile(invalidFile, []byte("invalid json"), 0644)
			require.NoError(t, err)
		}

		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Configure cascade operation (some will fail)
			cascadeTargets := []config.CascadeTarget{}
			if idx%2 == 0 {
				// This will fail due to invalid JSON
				cascadeTargets = append(cascadeTargets, config.CascadeTarget{
					Pattern:   filepath.Join(tmpDir, fmt.Sprintf("invalid-%d.json", idx)),
					Merge:     "update",
					Path:      "deploymentSpec.trafficSplit",
					Transform: "$.trafficSplit",
				})
			} else {
				// This will succeed - create a valid target file
				validFile := filepath.Join(tmpDir, fmt.Sprintf("valid-%d.json", idx))
				validData := map[string]interface{}{
					"deploymentSpec": map[string]interface{}{
						"trafficSplit": map[string]interface{}{
							"deployment-1": 100,
						},
					},
				}
				writeConcurrentTestJSONFile(t, validFile, validData)

				cascadeTargets = append(cascadeTargets, config.CascadeTarget{
					Pattern:   validFile,
					Merge:     "update",
					Path:      "deploymentSpec.trafficSplit",
					Transform: "$.trafficSplit",
				})
			}

			caseConfig := config.Case{
				Merge: "cascade",
				Primary: &config.CascadePrimary{
					File:  filepath.Join(endpointDir, fmt.Sprintf("endpoint-%d.json", idx)),
					Merge: "update",
					Path:  "trafficSplit",
				},
				Cascade: cascadeTargets,
			}

			updateData := map[string]interface{}{
				"trafficSplit": map[string]interface{}{
					"deployment-1": 50,
				},
			}

			context := RequestContext{
				Body:      updateData,
				ConfigDir: tmpDir,
			}

			// Execute cascade
			err := ExecuteCascade(caseConfig, updateData, context)

			// Count results
			mu.Lock()
			if err != nil {
				rollbackCount++
			} else {
				successCount++
			}
			mu.Unlock()
		}(i)
	}

	// Wait for all operations to complete
	wg.Wait()

	t.Logf("Success count: %d, Rollback count: %d", successCount, rollbackCount)

	// Verify we had both successes and rollbacks
	assert.Greater(t, successCount, 0, "Should have some successful operations")
	assert.Greater(t, rollbackCount, 0, "Should have some rollback operations")

	// Verify that rolled back files are in their original state
	for i := 0; i < numConcurrentOps; i++ {
		if i%2 == 0 { // These should have rolled back
			endpointFile := filepath.Join(endpointDir, fmt.Sprintf("endpoint-%d.json", i))
			endpointData := readConcurrentTestJSONFile(t, endpointFile)
			actualValue := int(endpointData["trafficSplit"].(map[string]interface{})["deployment-1"].(float64))
			assert.Equal(t, 100, actualValue, "Rolled back endpoint %d should have original value", i)
		}
	}
}

// Helper functions for concurrent tests
func writeConcurrentTestJSONFile(t *testing.T, filePath string, data interface{}) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(filePath, jsonData, 0644)
	require.NoError(t, err)
}

func readConcurrentTestJSONFile(t *testing.T, filePath string) map[string]interface{} {
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	return result
}
