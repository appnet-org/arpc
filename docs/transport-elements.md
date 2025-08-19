# Transport Elements

The aRPC transport layer uses a modular element-based architecture that allows for flexible composition of transport functionality. Transport elements can be chained together to add features like logging, reliability, encryption, or custom processing to the underlying UDP transport.

## Overview

Transport elements implement the `TransportElement` interface and can be composed into chains that process data as it flows through the transport layer. Each element can modify, inspect, or add functionality to the data being sent or received.

## Architecture

### TransportElement Interface

All transport elements must implement the following interface:

```go
type TransportElement interface {
    // ProcessSend processes outgoing data before it's sent
    ProcessSend(addr string, data []byte, rpcID uint64) ([]byte, error)

    // ProcessReceive processes incoming data after it's received
    ProcessReceive(data []byte, rpcID uint64, packetType protocol.PacketType, addr *net.UDPAddr, conn *net.UDPConn) ([]byte, error)

    // Name returns the name of the transport element
    Name() string

    // GetRole returns the role of this element (client/caller or server/callee)
    GetRole() Role
}
```

### Element Chain

Elements are organized in chains that process data in sequence:

- **Send Processing**: Data flows through elements in forward order before transmission
- **Receive Processing**: Data flows through elements in reverse order after reception

This design allows for symmetric processing where elements can undo their send-side modifications during receive processing.

