# Cascade Mutations

**Cascade mutations** enable atomic multi-file updates with automatic rollback, allowing a single API request to modify multiple related stub files simultaneously while maintaining data consistency.

## Overview

When building complex APIs, data often needs to be synchronized across multiple files. For example, updating traffic split percentages for an ML endpoint should update both the endpoint configuration and all its deployment files. Cascade mutations solve this by enabling atomic operations that either succeed completely or roll back all changes.

## Key Features

- ✅ **Atomic Operations** - All-or-nothing semantics with automatic rollback
- ✅ **Multi-file Updates** - Single API call updates multiple related files  
- ✅ **Pattern Matching** - Wildcard support for dynamic file targeting
- ✅ **Field Targeting** - Precise nested field updates
- ✅ **Data Transforms** - JSONPath expressions for field extraction
- ✅ **Conditional Execution** - Execute cascades based on request conditions
- ✅ **Security Hardened** - Path traversal protection and input sanitization
- ✅ **Performance Optimized** - Sub-millisecond execution times

## Basic Configuration

To enable cascade mutations, use `merge: "cascade"` and define primary and cascade operations:

```toml
[[routes]]
method   = "PATCH"
match    = "/api/v1/endpoint/{endpointId}/traffic-split"
enabled  = true
fallback = "updated"

  [routes.cases.updated]
  status = 200
  persist = true
  merge = "cascade"
  
  # Primary operation: Update the main endpoint file
  [routes.cases.updated.primary]
  file = "stubs/endpoints/{path.endpointId}.json"
  merge = "update"
  path = "trafficSplit"
  
  # Cascade operation: Update all related deployment files
  [[routes.cases.updated.cascade]]
  pattern = "stubs/deployments/{path.endpointId}/*.json"
  merge = "update" 
  path = "deploymentSpec.trafficSplit"
  transform = "$.trafficSplit"
```

## Configuration Schema

### Primary Operation

The `primary` block defines the main file operation:

```toml
[routes.cases.updated.primary]
file = "path/to/primary/file.json"    # File path (supports placeholders)
merge = "update"                       # Operation: update | append | delete
path = "field.nested"                  # Optional: specific field to update
```

### Cascade Operations

The `cascade` array defines related file operations:

```toml
[[routes.cases.updated.cascade]]
pattern = "path/pattern/*.json"        # File pattern (supports wildcards)
merge = "update"                       # Operation: update | append | delete  
path = "field.nested.path"             # Optional: specific field to update
transform = "$.sourceField"            # Optional: JSONPath data extraction
condition = "{{.body.updateFlag}}"     # Optional: conditional execution
```

## Real-World Example: Traffic Split Synchronization

This example shows how to synchronize ML model traffic split values across endpoint and deployment files:

### Directory Structure

```
stubs/
├── endpoints/
│   └── churn-model.json
└── deployments/
    └── churn-model/
        ├── deployment-a.json
        └── deployment-b.json
```

### Configuration

```toml
[[routes]]
method   = "PATCH"
match    = "/api/v1/*/environments/*/endpoint/{endpointId}/traffic-split"
enabled  = true
fallback = "updated"

  [routes.cases.updated]
  status = 200
  persist = true
  merge = "cascade"
  
  # Update endpoint's traffic split configuration
  [routes.cases.updated.primary]
  file = "stubs/endpoints/{path.endpointId}.json"
  merge = "update"
  path = "trafficSplit"
  
  # Update all deployment files for this endpoint
  [[routes.cases.updated.cascade]]
  pattern = "stubs/deployments/{path.endpointId}/*.json"
  merge = "update"
  path = "deploymentSpec.trafficSplit"
  transform = "$.trafficSplit"
```

### API Request

```http
PATCH /api/v1/project/environments/prod/endpoint/churn-model/traffic-split
Content-Type: application/json

{
  "trafficSplit": {
    "deployment-a": 70,
    "deployment-b": 30
  }
}
```

### File Updates

**Before:**

`endpoints/churn-model.json`:
```json
{
  "endpointId": "churn-model",
  "trafficSplit": {
    "deployment-a": 100,
    "deployment-b": 0
  }
}
```

`deployments/churn-model/deployment-a.json`:
```json
{
  "deploymentId": "deployment-a",
  "deploymentSpec": {
    "trafficSplit": {
      "deployment-a": 100
    }
  }
}
```

**After:**

`endpoints/churn-model.json`:
```json
{
  "endpointId": "churn-model", 
  "trafficSplit": {
    "deployment-a": 70,
    "deployment-b": 30
  }
}
```

`deployments/churn-model/deployment-a.json`:
```json
{
  "deploymentId": "deployment-a",
  "deploymentSpec": {
    "trafficSplit": {
      "deployment-a": 70,
      "deployment-b": 30
    }
  }
}
```

## Data Transforms

Use JSONPath expressions to extract and transform data before applying to cascade targets:

### Basic Field Extraction

```toml
[[routes.cases.updated.cascade]]
pattern = "stubs/config/*.json"
transform = "$.settings"      # Extract 'settings' field from request
path = "configuration.settings"
```

### Nested Field Extraction

```toml
[[routes.cases.updated.cascade]]
pattern = "stubs/deployments/*.json"
transform = "$.metadata.labels"  # Extract nested field
path = "spec.labels"
```

### Full Request Body

```toml
[[routes.cases.updated.cascade]]
pattern = "stubs/backups/*.json"
transform = "$"                  # Use entire request body
```

## Conditional Cascades

Execute cascade operations only when specific conditions are met:

```toml
[[routes.cases.updated.cascade]]
pattern = "stubs/deployments/{path.endpointId}/*.json"
merge = "update"
path = "deploymentSpec.trafficSplit"
transform = "$.trafficSplit"
condition = "{{.body.updateDeployments}}"  # Only cascade if this field is true
```

**Request with condition:**
```json
{
  "trafficSplit": {
    "deployment-a": 80,
    "deployment-b": 20
  },
  "updateDeployments": true
}
```

## Pattern Matching

Cascade mutations support flexible file pattern matching:

### Wildcard Patterns

```toml
pattern = "stubs/deployments/*.json"              # All JSON files in directory
pattern = "stubs/deployments/{path.id}/*.json"    # Dynamic directory with wildcards
pattern = "stubs/configs/deployment-*.json"       # Prefix matching
```

### Dynamic Placeholders

```toml
pattern = "stubs/environments/{path.env}/configs/*.json"
pattern = "stubs/tenants/{query.tenant}/settings/*.json"
```

## Error Handling & Rollback

Cascade mutations provide automatic rollback on any failure:

### Success Response

```json
{
  "message": "Cascade operation completed successfully",
  "primaryFile": "stubs/endpoints/my-endpoint.json",
  "cascadeTargets": 3,
  "operationId": "cascade-op-a1b2c3d4"
}
```

### Failure Response with Rollback

```json
{
  "error": "cascade operation failed: invalid JSON in deployment file",
  "details": {
    "failedFile": "stubs/deployments/endpoint-1/corrupted.json",
    "error": "invalid character 'i' looking for beginning of value",
    "rolledBackFiles": [
      "stubs/endpoints/endpoint-1.json"
    ]
  },
  "operationId": "cascade-op-a1b2c3d4"
}
```

### Rollback Guarantees

- **Atomic Operations**: All changes succeed or all are rolled back
- **File Integrity**: Corrupted writes are prevented with atomic file operations
- **Consistency**: No partial updates - either all files reflect the new state or none do
- **Recovery**: Original file contents are fully restored on failure

## Security Features

Cascade mutations include enterprise-grade security protections:

### Path Traversal Protection

```toml
# ❌ This will be blocked
pattern = "../../../etc/passwd"

# ✅ This is safe - stays within config directory  
pattern = "stubs/deployments/*.json"
```

### Input Sanitization

Dangerous characters are automatically removed from path parameters:

```
Input:  "../../../etc/passwd"
Output: "etcpasswd"

Input:  "test\x00file" 
Output: "testfile"
```

### Resource Limits

- **Maximum 10 cascade targets** per operation (prevents DoS attacks)
- **JSON files only** (prevents arbitrary file manipulation)
- **Config directory sandboxing** (prevents directory escape)

## Performance

Cascade mutations are optimized for production use:

- **Sub-millisecond execution** (average 312µs for typical operations)
- **Parallel processing** where possible
- **Efficient pattern matching** with minimal filesystem overhead
- **Atomic rollback** with temporary file staging

## Best Practices

### 1. Keep Cascades Focused

Limit cascade operations to closely related data:

```toml
# ✅ Good: Related traffic configuration
[[routes.cases.updated.cascade]]
pattern = "stubs/deployments/{path.endpointId}/*.json"
path = "deploymentSpec.trafficSplit"

# ❌ Avoid: Unrelated data updates
[[routes.cases.updated.cascade]]
pattern = "stubs/completely-different-service/*.json"
```

### 2. Use Field Paths for Precision

Target specific fields rather than overwriting entire files:

```toml
# ✅ Good: Update specific field
[routes.cases.updated.primary]
file = "stubs/endpoints/{path.endpointId}.json"
path = "trafficSplit"        # Only update this field

# ❌ Avoid: Overwriting entire file
[routes.cases.updated.primary] 
file = "stubs/endpoints/{path.endpointId}.json"
# No path specified = overwrites entire file
```

### 3. Test Rollback Scenarios

Always test failure conditions to ensure rollback works correctly:

```bash
# Create an invalid JSON file to test rollback
echo "invalid json" > stubs/deployments/test/broken.json

# Make a cascade request - should rollback cleanly
curl -X PATCH /api/endpoint/test/traffic-split -d '{"trafficSplit": {"new": 50}}'
```

### 4. Monitor Operation Logs

Use operation IDs to track cascade mutations:

```bash
# Search logs for specific operation
grep "cascade-op-a1b2c3d4" apitwin.log

# Monitor cascade operations
tail -f apitwin.log | grep CASCADE
```

## Troubleshooting

### Common Issues

**Pattern resolves to no files:**
```
Error: cascade pattern resolved to no files: stubs/deployments/{path.endpointId}/*.json
```

**Solution:** Verify the placeholder resolution and file paths:
- Check that `{path.endpointId}` matches your route parameters
- Ensure the target directory and files exist
- Test pattern matching with static values first

**Path escapes config directory:**
```
Error: pattern escapes config directory: ../../../etc/passwd
```

**Solution:** This is a security protection. Ensure patterns stay within your configured stub directory.

**Rollback failed:**
```
Error: cascade failed and rollback failed: original error: ..., rollback error: ...
```

**Solution:** This indicates a serious issue. Check:
- File permissions in the stubs directory
- Available disk space
- File system integrity

## Migration Guide

### From Single-File Updates

**Before (single file):**
```toml
[routes.cases.updated]
status = 200
file = "stubs/endpoints/{path.endpointId}.json"
persist = true
merge = "update"
```

**After (cascade):**
```toml
[routes.cases.updated]
status = 200
persist = true
merge = "cascade"

[routes.cases.updated.primary]
file = "stubs/endpoints/{path.endpointId}.json"
merge = "update"

[[routes.cases.updated.cascade]]
pattern = "stubs/deployments/{path.endpointId}/*.json"
merge = "update"
path = "deploymentSpec"
transform = "$"
```

### Backwards Compatibility

Existing single-file mutation routes continue to work unchanged. Cascade mutations are purely additive and don't affect existing functionality.

## Advanced Examples

### Multi-Environment Configuration

Sync configuration across multiple environment files:

```toml
[[routes.cases.updated.cascade]]
pattern = "stubs/environments/*/configs/{path.serviceId}.json"
merge = "update"
path = "scaling.config"
transform = "$.scaling"
```

### Conditional Multi-Service Updates

Update related services only when requested:

```toml
[[routes.cases.updated.cascade]]
pattern = "stubs/services/auth/*.json"
merge = "update"
path = "permissions"
transform = "$.permissions"
condition = "{{.body.updateAuth}}"

[[routes.cases.updated.cascade]]
pattern = "stubs/services/billing/*.json"
merge = "update"
path = "limits"
transform = "$.limits"
condition = "{{.body.updateBilling}}"
```

### Cross-Service Dependencies

Propagate changes to dependent services:

```toml
# Update primary service
[routes.cases.updated.primary]
file = "stubs/services/user-service.json"
merge = "update"

# Update all services that depend on user-service
[[routes.cases.updated.cascade]]
pattern = "stubs/services/*-service.json"
merge = "update"
path = "dependencies.userService"
transform = "$.version"
```

## See Also

- [Directory-Based Stubs](directory-stubs.md) - Learn about directory-based CRUD operations
- [Cross-Endpoint References](cross-endpoint-references.md) - Reference data between files
- [Named Parameters](named-parameters.md) - Use dynamic path parameters
- [Response Transitions](response-transitions.md) - Time-based state changes