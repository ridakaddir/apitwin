# gRPC Directory-Based Persistence Example

This example demonstrates directory-based CRUD operations for gRPC services. Each item is stored as a separate JSON file.

## Features

- **gRPC directory aggregation** - ListItems aggregates all .json files
- **protobuf JSON mapping** - Automatic conversion between protobuf and JSON
- **Dynamic file paths** - Uses request fields to resolve file paths
- **Full CRUD operations** - Create, Read, Update, Delete via gRPC calls

## Running the Example

```bash
# Start the mock gRPC server
apitwin --config examples/grpc-directory-persist --grpc-proto examples/grpc-directory-persist/items.proto

# The server listens on :50051 by default
```

## Using grpcurl

```bash
# List all items (directory aggregation)
grpcurl -plaintext -d '{}' localhost:50051 items.ItemService/ListItems

# Get specific item
grpcurl -plaintext -d '{"itemId":"item-1"}' localhost:50051 items.ItemService/GetItem

# Create new item (with explicit ID)
grpcurl -plaintext -d '{
  "itemId": "item-3",
  "name": "Laptop",
  "description": "High-performance laptop",
  "price": 1299.99,
  "available": true
}' localhost:50051 items.ItemService/CreateItem

# Create new item (auto-generate ID)
grpcurl -plaintext -d '{
  "name": "Tablet",
  "description": "Lightweight tablet for reading",
  "price": 299.99,
  "available": true
}' localhost:50051 items.ItemService/CreateItem

# Update item (shallow merge)
grpcurl -plaintext -d '{
  "itemId": "item-1",
  "price": 179.99,
  "available": false
}' localhost:50051 items.ItemService/UpdateItem

# Delete item
grpcurl -plaintext -d '{"itemId":"item-2"}' localhost:50051 items.ItemService/DeleteItem
```

## File Structure

```
stubs/
└── items/
    ├── item-1.json    # Wireless Headphones
    ├── item-2.json    # Smartphone
    └── ...
```

Each file contains the JSON representation of an Item protobuf message.

## gRPC-Specific Features

- **protojson conversion** - Request/response automatically converted between protobuf and JSON
- **Field name mapping** - Supports both `item_id` (proto) and `itemId` (JSON) field names
- **Status codes** - Returns appropriate gRPC status codes (OK, NOT_FOUND, etc.)
- **Persist responses** - Update and append return the persisted data; delete returns an empty message
