# Symphony Hybrid Serialization

Symphony Hybrid is a hybrid serialization format that combines the best of both worlds:
- **Public Segment**: Uses Symphony format for zero-copy access and efficient in-place updates
- **Private Segment**: Uses Protobuf format for compact size

## Design

### Hybrid Format Structure

The hybrid format maintains the same 13-byte header structure as Symphony:

```
[Header (13 bytes)][Public Segment (Symphony)][Private Segment (Protobuf)]
```

#### Header (13 bytes)
- **1 byte**: Version byte (`0x01`)
- **4 bytes**: `offsetToPrivate` - offset where private segment starts
- **4 bytes**: `service_id` (reserved)
- **4 bytes**: `method_id` (reserved)

#### Public Segment (Symphony Format)
- Uses the same table-plus-payload structure as Symphony
- Enables zero-copy reads for public fields
- Supports efficient in-place updates
- Starts at byte 13 (after header)

#### Private Segment (Protobuf Format)
- Uses standard Protobuf encoding (`proto.Marshal`/`proto.Unmarshal`)
- More compact than Symphony format for private fields
- No version byte needed (protobuf is self-describing)
- Starts at `offsetToPrivate`

### Field Classification

Fields are classified as public or private using the `is_public` extension:

```protobuf
extend google.protobuf.FieldOptions {
  bool is_public = 50001;
}

message Example {
  int32 public_field = 1 [(is_public) = true];  // Goes to public segment
  string private_field = 2;                     // Goes to private segment
}
```

## Usage

### Installation

To install the hybrid code generator:

```bash
go install ./cmd/symphony-gen-arpc/protoc-gen-symphony-hybrid
```

Make sure `$GOBIN` is in your `PATH`.

### Code Generation

To generate hybrid serialization code:

```bash
protoc --symphony-hybrid_out=paths=source_relative:. \
       --go_out=paths=source_relative:. \
       your_message.proto
```

This will generate:
- `<your-proto-file>.pb.go`: Standard Protobuf struct and getter/setter methods
- `<your-proto-file>.hybrid.go`: Hybrid serialization methods

### API

#### Marshal

```go
msg := &Example{
    PublicField:  42,
    PrivateField: "secret",
}
data, err := msg.MarshalSymphonyHybrid()
```

#### Unmarshal

```go
var output Example
err := output.UnmarshalSymphonyHybrid(data)
```

## Benefits

### Size Efficiency
- Private segment uses Protobuf encoding, which is typically more compact than Symphony's table-plus-payload format
- Ideal for fields that are rarely accessed but need to be transmitted

### Performance
- Public segment uses Symphony format, enabling zero-copy access
- Public fields can be read without deserializing the entire message
- Supports efficient in-place updates for public fields

### Use Cases
- **Network protocols**: Public fields for routing/forwarding, private fields for payload
- **Caching**: Fast access to public metadata, compact storage of private data
- **In-network processing**: Process public fields without deserializing private data

## Comparison with Pure Symphony

| Feature | Symphony | Symphony Hybrid |
|---------|----------|-----------------|
| Public segment format | Symphony | Symphony |
| Private segment format | Symphony | Protobuf |
| Zero-copy public access | ✅ | ✅ |
| Private segment size | Larger | Smaller |
| Compatibility | N/A | New format |

## Nested Messages

Nested messages are also serialized using the hybrid format:

```go
type Parent struct {
    PublicChild  *Child  // Uses hybrid format
    PrivateChild *Child  // Uses hybrid format
}
```

Both public and private nested messages use the hybrid format recursively.

## Notes

- The hybrid format is **not backward compatible** with pure Symphony format
- Generated files use the `.hybrid.go` extension to avoid conflicts
- Private fields are always marshaled/unmarshaled using Protobuf, even if they're in nested messages
- Public fields maintain Symphony's zero-copy and in-place update capabilities

