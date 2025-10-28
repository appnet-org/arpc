# Reliable Transport Handler

A message-level reliable transport layer for aRPC that provides ACK-based reliability and automatic retransmission.

## Core Features

- **Message-Level ACKs**: ACKs sent after receiving all segments of a message (not per-packet)
- **Out-of-Order Handling**: Tracks segments using bitsets, supports arbitrary arrival order
- **Automatic Retransmission**: Periodic timeout checks (RTO = 4×RTT_min, minimum 100ms)
- **RTT Tracking**: Measures round-trip time from ACK timestamps
- **Statistics**: Tracks bytes acknowledged, messages lost, and minimum RTT

## Usage

### Setup with Real Transport

```go
import (
    "github.com/appnet-org/arpc/pkg/custom/reliable"
    "github.com/appnet-org/arpc/pkg/transport"
)

// Create UDP transport
udpTransport, _ := transport.NewUDPTransport(":8080")
defer udpTransport.Close()

// Register ACK packet type
udpTransport.RegisterPacketType(reliable.AckPacketName, &reliable.ACKPacketCodec{})

// Create reliable handler
handler := reliable.NewReliableClientHandler(
    udpTransport,
    udpTransport.GetTimerManager(),
)
defer handler.Cleanup()

// Integrate into handler chains
requestChain := transport.NewHandlerChain(udpTransport)
requestChain.AddHandler(handler)
udpTransport.RegisterHandlerChain(
    packet.PacketTypeRequest.TypeID, 
    requestChain, 
    transport.RoleClient,
)

responseChain := transport.NewHandlerChain(udpTransport)
responseChain.AddHandler(handler)
udpTransport.RegisterHandlerChain(
    packet.PacketTypeResponse.TypeID, 
    responseChain, 
    transport.RoleClient,
)
```

### Handler Behavior

- **OnSend**: Tracks outgoing REQUEST packets with timestamps
- **OnReceive**: 
  - For RESPONSE packets: Tracks segments, sends ACK when complete
  - For ACK packets: Cleans up request state, updates RTT
- **Background Timer**: Checks for timeouts every 1ms, retransmits expired requests

### Get Statistics

```go
bytesAcked, msgsLost, rttMin := handler.GetStats()
```

## Architecture

### Interfaces

The handler uses interfaces for testability:

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

- **Production**: Uses `*transport.UDPTransport` and `*transport.TimerManager`
- **Testing**: Uses mocks from `test_helpers.go` for fast, deterministic tests

## Testing

Tests use mock clock and timers for fast execution (<10ms total):

```bash
go test -v
```

See `handlers_test.go` for comprehensive test coverage including:
- Basic request/response tracking
- Out-of-order segment delivery
- RTT measurement and RTO calculation
- Multiple concurrent requests
- Edge cases (large messages, duplicates, etc.)

