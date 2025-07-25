# Serialization

## Protobuf

Each field in a Protobuf message is serialized as:

```
[key][value]
```

### The Key

The key is a single **varint** that encodes two things:

* **Field number** — from the `.proto` definition (e.g., `id = 1`)
* **Wire type** — specifies how the value is encoded (e.g., varint, length-delimited)

The key is computed as:

```
key = (field_number << 3) | wire_type
```

* The **lower 3 bits** represent the [wire type](https://protobuf.dev/programming-guides/encoding/) (values 0–5)
* The **remaining upper bits** represent the field number

##### The Value

The value is encoded depending on the field type:

* **Varint** for integers
* **Length-delimited** for strings and bytes

---

##### Example

Assume the message is:

```go
req := &EchoRequest{
  Id:       42,
  Score:    300,
  Username: "alice",
  Content:  "hello world",
}
```

### Field 1: `id = 42`

* Key: `(1 << 3) | 0` = `0x08` → field 1, wire type 0 (varint)
* Value: `0x2a` (42)
* **Bytes**: `08 2a`

---

### Field 2: `score = 300`

* Key: `(2 << 3) | 0` = `0x10`
* Value: `0xac 02` (300 as varint)
* **Bytes**: `10 ac 02`

---

### Field 3: `username = "alice"`

* Key: `(3 << 3) | 2` = `0x1a` → wire type 2 (length-delimited)
* Length: `0x05`
* Value: `"alice"` = `61 6c 69 63 65`
* **Bytes**: `1a 05 61 6c 69 63 65`

---

### Field 4: `content = "hello world"`

* Key: `(4 << 3) | 2` = `0x22`
* Length: `0x0b`
* Value: `"hello world"` = `68 65 6c 6c 6f 20 77 6f 72 6c 64`
* **Bytes**: `22 0b 68 65 6c 6c 6f 20 77 6f 72 6c 64`



---

##### How Varint Works

* Encodes integers using **1 to 10 bytes**
* Each byte contains:

  * **7 bits** of actual data
  * **1 continuation bit** (MSB):

    * `1` → more bytes follow
    * `0` → last byte in sequence

This makes small numbers very compact while supporting large values when needed.

## Cap’n Proto

Cap’n Proto encodes data as a flat, word-aligned memory image that can be directly memory-mapped or transmitted over the wire with **zero parsing or copying**. All data is laid out in **64-bit words** (8 bytes), and each message consists of:

1. **Segment Table**: Describes the number and size of segments.
2. **Segment Data**: Contains all structs and pointers in a linear array of words.

Each **struct** consists of:

* A **data section** (raw field values like integers)
* A **pointer section** (references to strings, lists, or nested structs)

---

### Message Example

Cap’n Proto schema:

```capnp
struct EchoRequest {
  id @0 :Int32;
  score @1 :Int32;
  username @2 :Text;
  content @3 :Text;
}
```

Serialized output (`hex`):

```
00000000 07000000 
00000000 01000200 
2a000000 2c010000 
05000000 32000000 
05000000 62000000 
616c6963 65000000 
68656c6c 6f20776f 
726c6400 00000000
```

---

### Field Breakdown

#### Segment Table

```
00000000 07000000 // Word 1
```

* 1 segment total
* segment 0 has 7 words (56 bytes)

#### Struct Data Section

## Struct Pointer

```
00000000 01000200 // Word 2
```

Note sure what this means for now.

## Data section

```
2a000000 2c010000 // Word 3
```

* `0x2a000000` = 42 (`id`)
* `0x2c010000` = 300 (`score`)

#### Pointer section

```
05000000 32000000  // Word 4 - Pointer to "alice"
05000000 62000000  // Word 5 - Pointer to "hello world"
```

Each pointer gives:

* Offset to the string data (relative to the pointer’s position)
* String size (including null terminator)

#### String Data

```
616c6963 65000000 // Word 6
68656c6c 6f20776f // Word 7
726c6400 00000000 // Word 8
```

* `"alice"` = `61 6c 69 63 65 00` padded to 8 bytes → `616c6963 65000000`
* `"hello world"` = `68656c6c 6f20776f 726c6400` padded to 16 bytes

## Symphony

See [wire-format.md](../../docs/wire-format.md)