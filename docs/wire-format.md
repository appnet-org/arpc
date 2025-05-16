# aRPC Wire Format

The wire format is a binary protocol used for communication between ARPC clients and servers. Here's a detailed breakdown:

## Message Structure

### Request Format
```
[serviceLen][service][methodLen][method][headerLen][headers][payload]
```

Where:
- `serviceLen` (2 bytes): Length of the service name as uint16 in little-endian
- `service` (variable): Service name as UTF-8 bytes
- `methodLen` (2 bytes): Length of the method name as uint16 in little-endian
- `method` (variable): Method name as UTF-8 bytes
- `headerLen` (2 bytes): Length of the headers as uint16 in little-endian
- `headers` (variable): Serialized metadata headers in format: [count][kLen][key][vLen][value]...
  - `count` (2 bytes): Number of key-value pairs as uint16 in little-endian
  - `kLen` (2 bytes): Length of each key as uint16 in little-endian
  - `key` (variable): The key string in UTF-8
  - `vLen` (2 bytes): Length of each value as uint16 in little-endian
  - `value` (variable): The value string in UTF-8
- `payload` (variable): Serialized request payload

### Response Format
The response follows the exact same structure as the request:
```
[serviceLen][service][methodLen][method][headerLen][headers][payload]
```

## Example

For a request to service "UserService" method "GetUser" with metadata {"username": "Bob"}, the binary format would look like:
```
[0x0B][UserService][0x07][GetUser][0x0F][0x01][0x08][username][0x03][Bob][payload...]
```

Where:
- `0x0B` is the length of "UserService" (11 bytes)
- `0x07` is the length of "GetUser" (7 bytes)
- `0x0F` is the length of the headers (15 bytes)
- `0x01` is the number of key-value pairs (1)
- `0x08` is the length of "username" (8 bytes)
- `0x03` is the length of "Bob" (3 bytes)

This wire format is implemented in the `frameRequest` and `frameResponse` functions in the client and server code, with corresponding parsing functions `parseFramedRequest` and `parseFramedResponse` to handle the binary format.
