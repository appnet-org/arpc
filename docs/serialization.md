# Symphony Serialization Format

This document describes the **serialization format** used by the Symphony compiler plugin (`protoc-gen-symphony`) to encode protocol buffer messages into a compact binary layout optimized for high-performance networking and in-network processing.

Serialization is handled by an auto-generated `MarshalSymphony` function for each message type.

---

## Overview

Each serialized message consists of two parts:

1. **Header** (static-length metadata)
2. **Data Region** (fixed and variable-length fields)

The structure ensures:

* Fixed offsets for fast access (no parsing required)
* Compatibility with eBPF, kernel, or P4-based processing
* Efficient appending of variable-length fields (like strings or bytes)

---

## Format Layout

```
[layout_header][field_order][offset_table][data_region]
```

### 1. Layout Header

* **1 byte**: currently always `0x00`, reserved for future layout versioning.

### 2. Field Order

* **4 bytes**: an array of 4 field numbers (as `byte`) indicating field order.
* This allows flexible code generation while keeping encoding deterministic.

### 3. Offset Table

For each **variable-length** field:

* **1 byte**: Field number (as `byte`)
* **2 bytes**: Offset (as `uint16`) from the start of the data region
* **2 bytes**: Length (as `uint16`) of this field

Fields are listed in the order they appear in the proto message.

Fixed-length fields **do not appear** in this table; their position is determined statically by accumulation of byte sizes during codegen.

### 4. Data Region

* All **fixed-length** fields (e.g., `int32`, `float64`, `bool`) are written directly using `binary.LittleEndian`.
* All **variable-length** fields (e.g., `string`, `bytes`) are appended in the order they are declared, with offsets computed during marshalling.

---

## Example

For the following proto message:

```proto
message EchoRequest {
  int32 id = 1;
  int32 score = 2;
  string username = 3;
  string content = 4;
}
```

The generated serialization will follow:

```
[layout_header = 0x00]
[field_order = {1, 2, 3, 4}]
[offset_table for username and content]
[id (int32)]
[score (int32)]
[username (raw bytes)]
[content (raw bytes)]
```

---

## Generated Code Snippet

Auto-generated `MarshalSymphony` will look like:

```go
func (m *EchoRequest) MarshalSymphony() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(0x00) // layout header
	buf.Write([]byte{1, 2, 3, 4}) // field order

	offset := 0
	offset += 4 // int32 id
	offset += 4 // int32 score

	// username (field 3)
	binary.Write(&buf, binary.LittleEndian, byte(3))
	binary.Write(&buf, binary.LittleEndian, uint16(offset))
	binary.Write(&buf, binary.LittleEndian, uint16(len(m.Username)))
	offset += len(m.Username)

	// content (field 4)
	binary.Write(&buf, binary.LittleEndian, byte(4))
	binary.Write(&buf, binary.LittleEndian, uint16(offset))
	binary.Write(&buf, binary.LittleEndian, uint16(len(m.Content)))
	offset += len(m.Content)

	// Fixed fields
	binary.Write(&buf, binary.LittleEndian, m.Id)
	binary.Write(&buf, binary.LittleEndian, m.Score)

	// Variable fields
	buf.Write([]byte(m.Username))
	buf.Write([]byte(m.Content))

	return buf.Bytes(), nil
}
```

---

## Notes

* All numeric values are encoded in **little-endian** format.
* Offsets are relative to the start of the **data region** (i.e., immediately after the offset table).
* The `UnmarshalSymphony` counterpart uses the offset table to decode variable-length fields.