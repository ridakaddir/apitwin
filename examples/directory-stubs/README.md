# Directory-Based Stub Storage Example

This example demonstrates the directory-based approach for stateful mocking where each resource is stored as a separate JSON file, with **defaults** to enrich created resources with server-generated fields.

## Features Demonstrated

- **Directory aggregation** - GET list operations aggregate all .json files in a directory
- **Individual file operations** - POST/PATCH/DELETE work on individual files
- **Auto-UUID generation** - Automatically generates UUIDs for new resources when ID is missing
- **Dynamic file paths** - Uses path parameters to resolve specific files
- **Persist defaults** - POST creates include server-generated fields (`userId`, `role`, `active`, `createdAt`) from a defaults file

## How It Works

### File Structure

```
stubs/
├── defaults/
│   └── user.json      # Default values for new users (with {{uuid}}, {{now}} tokens)
└── users/
    ├── 1.json         # {"userId": "1", "name": "Alice Johnson", ...}
    ├── 2.json         # {"userId": "2", "name": "Bob Smith", ...}
    └── 3.json         # {"userId": "3", "name": "Charlie Davis", ...}
```

### Defaults File

`stubs/defaults/user.json` defines server-generated fields:

```json
{
  "userId": "{{uuid}}",
  "role": "user",
  "active": true,
  "createdAt": "{{now}}"
}
```

When a POST creates a new user, apitwin:
1. Reads the defaults file and resolves template tokens (`{{uuid}}` becomes a real UUID, `{{now}}` becomes an ISO timestamp)
2. Deep-merges defaults with the request body — **request body always wins** on conflicts
3. Saves the merged result as the new file and returns it as the response

### API Operations

1. **GET /users** - Aggregates all files in `stubs/users/` into a JSON array
2. **POST /users** - Deep-merges defaults + request body, creates new file
3. **GET /users/{userId}** - Reads the specific file `stubs/users/{userId}.json`
4. **PATCH /users/{userId}** - Shallow-merges request body into the existing file
5. **DELETE /users/{userId}** - Removes the file `stubs/users/{userId}.json`

## Running the Example

```bash
# Start the mock server
apitwin --config examples/directory-stubs

# List users (directory aggregation)
curl http://localhost:4000/users

# Get specific user
curl http://localhost:4000/users/1

# Create new user (defaults fill in userId, role, active, createdAt)
curl -X POST http://localhost:4000/users \
  -H "Content-Type: application/json" \
  -d '{"name": "Diana Wilson", "email": "diana@example.com"}'
# Response includes defaults:
# {
#   "userId": "a1b2c3d4-...",
#   "name": "Diana Wilson",
#   "email": "diana@example.com",
#   "role": "user",
#   "active": true,
#   "createdAt": "2026-03-24T10:30:00Z"
# }

# Create user with explicit values (overrides defaults)
curl -X POST http://localhost:4000/users \
  -H "Content-Type: application/json" \
  -d '{"userId": "custom-id", "name": "Eve Brown", "email": "eve@example.com", "role": "admin"}'
# role is "admin" (body wins over default "user")

# Update user (shallow merge)
curl -X PATCH http://localhost:4000/users/1 \
  -H "Content-Type: application/json" \
  -d '{"email": "alice.johnson@newdomain.com", "active": false}'

# Delete user
curl -X DELETE http://localhost:4000/users/3
```

## Defaults Merge Order

```
result = deepMerge(defaults, requestBody)
```

- **Defaults provide the base** - server-generated fields like `userId`, `role`, `createdAt`
- **Request body wins** - any field sent by the client overrides the default
- **Template tokens** - `{{uuid}}`, `{{now}}`, `{{timestamp}}` are resolved before merging
- **Nested merge** - nested objects are merged recursively, not replaced wholesale

## Benefits

- **Single source of truth** - Each resource is a separate file
- **Version control friendly** - Easy to track changes to individual resources
- **Scalable** - No size limits imposed by single-file arrays
- **Intuitive** - File structure mirrors API structure
- **Realistic responses** - Defaults make POST responses match what a real API returns
- **Nested resources** - Supports subdirectories for hierarchical data

## Comparison to Array-Based Approach

| Aspect | Directory-Based | Array-Based (Old) |
|--------|----------------|-------------------|
| File structure | One file per resource | All resources in one array |
| GET list | Aggregates directory | Returns array from file |
| POST create | Creates new file | Appends to array |
| PATCH update | Merges into file | Finds & merges array element |
| DELETE | Removes file | Removes array element |
| Scalability | Unlimited | Limited by file size |
| Version control | Clean diffs per resource | Noisy diffs on shared file |
