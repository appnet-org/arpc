# Serialization Size Comparison Test

This test compares the serialized size of different serialization formats using the Money and CurrencyConversionRequest schema.

## Schema

```protobuf
message Money {
    string currency_code = 1 [(is_public) = true];
    int64 units = 2 [(is_public) = true];
    int32 nanos = 3;
}

message CurrencyConversionRequest {
    Money from = 1 [(is_public) = true];
    string to_code = 2 [(is_public) = true];
    string user_id = 3;
}
```

## Formats Tested

1. **Protobuf** - Standard Protocol Buffers serialization
2. **Symphony** - Symphony serialization format
3. **Hybrid Symphony** - Hybrid format (Symphony for public, Protobuf for private)
4. **FlatBuffers** - Google's FlatBuffers serialization
5. **Cap'n Proto** - Cap'n Proto serialization

## Prerequisites

Before running the test, you need to generate code for all serialization formats:

### 1. Install Required Tools

```bash
# Install protoc-gen-go
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Install protoc-gen-symphony
go install ./cmd/symphony-gen-arpc/protoc-gen-symphony

# Install protoc-gen-symphony-hybrid
go install ./cmd/symphony-gen-arpc/protoc-gen-symphony-hybrid

# Install flatc (FlatBuffers compiler)
# Download from: https://github.com/google/flatbuffers/releases

# Install capnp and capnpc-go
go install capnproto.org/go/capnp/v3/capnpc-go@latest
```

### 2. Generate Code

Generate code for each format. All protobuf-based formats (Protobuf, Symphony, Hybrid) will be in the same `money/` package:

```bash
cd test
mkdir -p money

# Protobuf (generates money.pb.go in money/ directory)
protoc --go_out=paths=source_relative:./money money.proto

# Symphony (generates money.syn.go in money/ directory)
protoc --symphony_out=paths=source_relative:./money money.proto

# Hybrid Symphony (generates money.hybrid.go in money/ directory)
protoc --symphony-hybrid_out=paths=source_relative:./money money.proto
```

For FlatBuffers and Cap'n Proto, you'll need to create separate schema files and generate code in separate directories:

```bash
mkdir -p money/{flatbuffers,capnp}

# FlatBuffers (requires money.fbs schema - convert from money.proto)
# Create money.fbs manually or use a converter, then:
flatc --go -o money/flatbuffers money.fbs

# Cap'n Proto (requires money.capnp schema - convert from money.proto)
# Create money.capnp manually or use a converter, then:
capnp compile -I$(go list -f '{{.Dir}}' capnproto.org/go/capnp/v3)/std -ogo money/capnp money.capnp
```

### 3. Update Import Paths

The test file uses import paths that assume the generated code structure above. If you use a different structure, update the import paths in `size_test.go` accordingly.

## Running the Test

```bash
cd test
go run size.go
```

## Output

The test will:
1. Generate random test data (currency codes, units, nanos, user ID)
2. Serialize the data using each format
3. Print a comparison table showing:
   - Format name
   - Serialized size in bytes
   - Percentage difference from the smallest format

Example output:
```
Serialization Size Comparison
==============================

Test Data:
  Currency Code: USD
  Units: 12345
  Nanos: 678900000
  To Code: EUR
  User ID: user_1234_1234567890

Results:
--------
Format               Size (bytes)    vs Smallest
--------------------------------------------------
Protobuf                     45           0.0%
Symphony                     52          15.6%
Hybrid Symphony              48           6.7%
FlatBuffers                  56          24.4%
Cap'n Proto                  64          42.2%
```

## Notes

- The test uses random data, so results may vary between runs
- For Hybrid Symphony, fields marked with `is_public = true` go to the public segment (Symphony format), while unmarked fields go to the private segment (Protobuf format)
- Some formats may require additional setup or schema conversion (FlatBuffers and Cap'n Proto need their own schema formats)

