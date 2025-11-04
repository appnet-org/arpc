# Reliable Transport Handler

A symmetric, message-level reliable transport layer for aRPC that provides ACK-based reliability, duplicate detection, and automatic retransmission.

## Core Features

- **Message-Level ACKs**: ACKs sent after receiving all segments of a message (not per-packet)
- **Out-of-Order Handling**: Tracks segments using bitsets, supports arbitrary arrival order
- **Duplicate Detection**: Duplicate detection via per-connection state, duplicates trigger ACK resend
- **Automatic Retransmission**: Automatically retransmits unACKed messages after timeout (1 second default)
- **Connection State Management**: Per-connection tracking with configurable timeout (30s default)
- **Symmetric Design**: Client and server handlers share ~80% of code via common base

## Usage

### Client-Side Setup

```go
import (
    "github.com/appnet-org/arpc/pkg/custom/reliable"
    "github.com/appnet-org/arpc/pkg/transport"
    "github.com/appnet-org/arpc/pkg/packet"
)

// Create UDP transport
udpTransport, _ := transport.NewUDPTransport(":0")
defer udpTransport.Close()

// Register ACK packet type
ackPacketType, _ := udpTransport.RegisterPacketType(reliable.AckPacketName, &reliable.ACKPacketCodec{})

// Create reliable client handler (default 30s connection timeout)
clientHandler := reliable.NewReliableClientHandler(
    udpTransport,
    udpTransport.GetTimerManager(),
)
defer clientHandler.Cleanup()

// Or with custom timeout
clientHandler := reliable.NewReliableClientHandlerWithTimeout(
    udpTransport,
    udpTransport.GetTimerManager(),
    60*time.Second, // Custom connection timeout
)

// Register for REQUEST packets (OnSend)
requestChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeRequest.TypeID, 
    transport.RoleClient,
)
requestChain.AddHandler(clientHandler)

// Register for RESPONSE packets (OnReceive)
responseChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeResponse.TypeID, 
    transport.RoleClient,
)
responseChain.AddHandler(clientHandler)

// Register handler chain for ACK packets
// Note: Custom packet types need handler chains created and registered
ackChain := transport.NewHandlerChain("ClientACKHandlerChain", clientHandler)
udpTransport.RegisterHandlerChain(ackPacketType.TypeID, ackChain, transport.RoleClient)
```

### Server-Side Setup

```go
// Create UDP transport
udpTransport, _ := transport.NewUDPTransport(":8080")
defer udpTransport.Close()

// Register ACK packet type
ackPacketType, _ := udpTransport.RegisterPacketType(reliable.AckPacketName, &reliable.ACKPacketCodec{})

// Create reliable server handler (default 30s connection timeout)
serverHandler := reliable.NewReliableServerHandler(
    udpTransport,
    udpTransport.GetTimerManager(),
)
defer serverHandler.Cleanup()

// Register for REQUEST packets (OnReceive)
requestChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeRequest.TypeID, 
    transport.RoleServer,
)
requestChain.AddHandler(serverHandler)

// Register for RESPONSE packets (OnSend)
responseChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeResponse.TypeID, 
    transport.RoleServer,
)
responseChain.AddHandler(serverHandler)

// Register handler chain for ACK packets
// Note: Custom packet types need handler chains created and registered
ackChain := transport.NewHandlerChain("ServerACKHandlerChain", serverHandler)
udpTransport.RegisterHandlerChain(ackPacketType.TypeID, ackChain, transport.RoleServer)
```

### Handler Behavior

#### Client Handler (`ReliableClientHandler`)

- **OnSend**: 
  - REQUEST packets: Tracks in per-server connection state with send timestamp
  - ACK(kind=1) packets: Updates connection activity
- **OnReceive**: 
  - RESPONSE packets: Accumulates segments via bitset, sends ACK(kind=1) when complete
  - Duplicate RESPONSEs: Resends ACK, returns nil (allows RPC layer to handle duplicates)
  - ACK(kind=0) packets: Marks REQUEST as ACKed (zeros timestamp)
- **Timers**:
  - Cleanup: Removes server connections after 30s inactivity (default)
  - Retransmission: Retransmits REQUESTs not ACKed within 1s (every 100ms check)

#### Server Handler (`ReliableServerHandler`)

- **OnReceive**:
  - REQUEST packets: Accumulates segments via bitset, sends ACK(kind=0) when complete
  - Duplicate REQUESTs: Resends ACK, returns nil (allows RPC layer to handle duplicates)
  - ACK(kind=1) packets: Marks RESPONSE as ACKed (zeros timestamp)
- **OnSend**:
  - RESPONSE packets: Tracks in per-client connection state with send timestamp
  - ACK(kind=0) packets: Updates connection activity
- **Timers**:
  - Cleanup: Removes client connections after 30s inactivity (default)
  - Retransmission: Retransmits RESPONSEs not ACKed within 1s (every 100ms check)

## Architecture

### Symmetric Design

Both client and server handlers embed a common `ReliableHandler` base that provides:

```go
type ReliableHandler struct {
    connections    map[string]*ConnectionState  // "IP:Port" -> state
    defaultTimeout time.Duration                // Connection timeout
    transport      TransportSender
    timerMgr       TimerScheduler
}
```

Each connection maintains:

```go
type ConnectionState struct {
    ConnID       ConnectionID
    LastActivity time.Time
    
    // Tx tracking (REQUEST for client, RESPONSE for server)
    TxMsg map[uint64]*MsgTx
    
    // Rx tracking (RESPONSE for client, REQUEST for server)
    RxMsgSeen     map[uint64]*Bitset
    RxMsgCount    map[uint64]uint32
    RxMsgComplete map[uint64]bool
}

type MsgTx struct {
    Count      uint32
    SendTs     time.Time
    DstAddr    string                 // Destination address for retransmission
    PacketType packet.PacketType      // Packet type for retransmission
    Segments   map[uint16][]byte      // Buffered packet data by segment number
}
```

### Key Design Decisions

1. **Duplicate Detection & ACK Resending**: 
   - Server/client retains message state after ACKing
   - Duplicates detected via `RxMsgComplete` map
   - Resends ACK for duplicates, returns nil (RPC layer handles deduplication)

2. **Automatic Retransmission**:
   - Messages with non-zero `SendTs` checked every 100ms
   - If `SendTs > 1s` old, all segments are retransmitted automatically
   - Packet data buffered in `MsgTx.Segments` map during send
   - ACKed messages marked with `SendTs = time.Time{}` and segments cleared
   - Retransmission updates `SendTs` for next retry cycle

3. **Connection Lifecycle**:
   - State created on first packet from connection
   - Activity updated on every packet
   - Cleaned up after timeout (30s default)
   - After timeout, RPCID collision possible but rare

### Interfaces

```go
type TransportSender interface {
    Send(addr string, rpcID uint64, data []byte, pktType packet.PacketType) error
    GetPacketRegistry() *packet.PacketRegistry
}

type TimerScheduler interface {
    SchedulePeriodic(id transport.TimerKey, interval time.Duration, callback transport.TimerCallback)
    StopTimer(id transport.TimerKey) bool
}
```

**Production**: Uses `*transport.UDPTransport` and `*transport.TimerManager`

