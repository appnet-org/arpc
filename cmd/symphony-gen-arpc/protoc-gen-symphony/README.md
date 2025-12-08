# Symphony Serialization

Symphony is a high-performance, deterministic serialization library for Protocol Buffers that uses a table-plus-payload structure to enable zero-copy reads and efficient in-place updates.

## Design

### Table-Plus-Payload Structure

Symphony uses a two-part layout for serialized data:

1. **Version Byte**: A single magic byte (`0x01`) at the start
2. **Table**: Fixed-size entries for each field, stored in field number order
3. **Payload**: Variable-length data referenced by offsets in the table

```
[Version Byte][Table Entries][Payload Data]
```

### Field Storage

#### Fixed-Length Fields (int32, int64, uint32, uint64, bool, float, double)
- **Table Entry**: Stores the value directly
- **Payload**: Not used

#### Singular Variable-Length Fields (string, bytes)
- **Table Entry**: 32-bit offset pointing to payload
- **Payload**: `[32-bit length][data]`

#### Repeated Fixed-Length Fields (repeated int32, repeated bool, etc.)
- **Table Entry**: 32-bit offset pointing to payload
- **Payload**: `[32-bit count][element₁][element₂]...[elementₙ]`

#### Repeated Variable-Length Fields (repeated string, repeated bytes)
- **Table Entry**: 32-bit offset pointing to payload
- **Payload**: `[32-bit count][32-bit len₁][data₁][32-bit len₂][data₂]...[32-bit lenₙ][dataₙ]`

#### Singular Nested Messages
- **Table Entry**: 32-bit offset pointing to payload
- **Payload**: `[32-bit size][marshaled nested message]`
- Nested messages are marshaled recursively using the same format

#### Repeated Nested Messages
- **Table Entry**: 32-bit offset pointing to payload
- **Payload**: `[32-bit count][32-bit size₁][message₁][32-bit size₂][message₂]...[32-bit sizeₙ][messageₙ]`

### Raw Types

Each message type has a corresponding `Raw` type (e.g., `FixedRaw`, `LeafRaw`) that is simply `type XxxRaw []byte`. Raw types provide:

- **Zero-Copy Reads**: Getters return direct slices or values without copying
- **In-Place Updates**: Setters update data in-place when the new size is ≤ original size
- **Automatic Remarshaling**: When new size > original size, the entire message is unmarshaled, updated, and remarshaled

#### In-Place Update Strategy

For variable-length and nested fields:
- If `newSize ≤ oldSize`: Update in-place (waste unused space)
- If `newSize > oldSize`: Unmarshal → Update → Remarshal

This minimizes allocations for common update patterns where values shrink or stay similar in size.

## Usage

### Standard Struct API

Use the generated struct types for standard marshal/unmarshal operations:

```go
// Marshal
msg := &Fixed{
    FInt32: 42,
    FBool:  true,
}
data, err := msg.MarshalSymphony()

// Unmarshal
var output Fixed
err := output.UnmarshalSymphony(data)
```

### Raw Type API

Use Raw types for zero-copy access and efficient updates:

```go
// Unmarshal into Raw type
var raw FixedRaw
err := raw.UnmarshalSymphony(data)

// Read values (zero-copy for variable-length fields)
value := raw.GetFInt32()
str := raw.GetVString()  // Returns string directly

// Update values
err := raw.SetFInt32(100)
err := raw.SetVString("new")  // In-place if size allows

// Marshal back
newData, err := raw.MarshalSymphony()
```

### Field Type Examples

#### Fixed-Length Fields
```go
raw.SetFInt32(42)
raw.SetFBool(true)
raw.SetFFloat(3.14)

value := raw.GetFInt32()
flag := raw.GetFBool()
```

#### Variable-Length Fields
```go
raw.SetVString("hello")
raw.SetVBytes([]byte{1, 2, 3})

str := raw.GetVString()      // Returns string
bytes := raw.GetVBytes()      // Returns []byte
```

#### Repeated Fixed-Length Fields
```go
raw.SetRInt32([]int32{1, 2, 3})
raw.SetRBool([]bool{true, false})

slice := raw.GetRInt32()      // Returns []int32
```

#### Repeated Variable-Length Fields
```go
raw.SetRString([]string{"a", "b"})
raw.SetRBytes([][]byte{{1}, {2, 3}})

strings := raw.GetRString()   // Returns []string
bytes := raw.GetRBytes()       // Returns [][]byte
```

#### Nested Messages
```go
// Get nested message as Raw type (zero-copy)
leafRaw := raw.GetNestedLeaf()  // Returns LeafRaw

// Modify nested message
leafRaw.SetLeafVal("new value")

// Set nested message (in-place if size allows)
err := raw.SetNestedLeaf(leafRaw)
```

#### Repeated Nested Messages
```go
// Get repeated nested messages as Raw types
roots := raw.GetRepeatedNested()  // Returns []RootRaw

// Modify individual nested message
roots[0].SetRootId(100)

// Set repeated nested messages
err := raw.SetRepeatedNested(roots)
```

### Nested Message Access Pattern

For deeply nested messages, you can chain getters to access inner fields:

```go
rootRaw := ... // RootRaw

// Navigate down the hierarchy
l1Raw := rootRaw.GetL1()        // Returns Level1Raw
l2Raw := l1Raw.GetL2()          // Returns Level2Raw
leafRaw := l2Raw.GetLeaf()      // Returns LeafRaw

// Read/modify leaf
value := leafRaw.GetLeafVal()
leafRaw.SetLeafVal("modified")

// Write back up the hierarchy
l2Raw.SetLeaf(leafRaw)
l1Raw.SetL2(l2Raw)
rootRaw.SetL1(l1Raw)
```

All getters for nested messages return Raw types, enabling zero-copy access throughout the message hierarchy.
