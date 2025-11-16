# Flow Control Handler

Flow control prevents buffer overflow by managing send/receive windows between client and server. It ensures the sender doesn't overwhelm the receiver with too much data.

## What It Does

- **Prevents Buffer Overflow**: Controls data rate to match receiver's capacity
- **Connection-Level**: One flow controller per connection (IP:Port)
- **Threshold-Based**: Automatically sends window updates when 75% of receive buffer is consumed
- **Auto-Tuning**: Window size adapts based on network conditions and RTT

## Key Features

- Symmetric design: Both client and server use the same flow control logic
- Lightweight: 9-byte fixed-size feedback packets
- Independent: Works with or without reliable transport and congestion control

## Usage

### Client-Side Setup

```go
import (
    "github.com/appnet-org/arpc/pkg/custom/flowcontrol"
    "github.com/appnet-org/arpc/pkg/transport"
    "github.com/appnet-org/arpc/pkg/packet"
)

// Create UDP transport
udpTransport, _ := transport.NewUDPTransport(":0")
defer udpTransport.Close()

// Register FCFeedback packet type
fcFeedbackType, _ := udpTransport.RegisterPacketType(
    flowcontrol.FCFeedbackPacketName, 
    &flowcontrol.FCFeedbackCodec{},
)

// Create FC client handler (defaults: 15MB initial, 25MB max window)
clientFCHandler := flowcontrol.NewFCClientHandler(
    udpTransport,
    udpTransport.GetTimerManager(),
)
defer clientFCHandler.Cleanup()

// Register for REQUEST packets (OnSend)
requestChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeRequest.TypeID, 
    transport.RoleClient,
)
requestChain.AddHandler(clientFCHandler)

// Register for RESPONSE packets (OnReceive)
responseChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeResponse.TypeID, 
    transport.RoleClient,
)
responseChain.AddHandler(clientFCHandler)

// Register handler chain for FCFeedback packets
fcFeedbackChain := transport.NewHandlerChain("ClientFCFeedbackChain", clientFCHandler)
udpTransport.RegisterHandlerChain(fcFeedbackType.TypeID, fcFeedbackChain, transport.RoleClient)
```

### Server-Side Setup

```go
// Create UDP transport
udpTransport, _ := transport.NewUDPTransport(":8080")
defer udpTransport.Close()

// Register FCFeedback packet type
fcFeedbackType, _ := udpTransport.RegisterPacketType(
    flowcontrol.FCFeedbackPacketName, 
    &flowcontrol.FCFeedbackCodec{},
)

// Create FC server handler
serverFCHandler := flowcontrol.NewFCServerHandler(
    udpTransport,
    udpTransport.GetTimerManager(),
)
defer serverFCHandler.Cleanup()

// Register for REQUEST packets (OnReceive)
requestChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeRequest.TypeID, 
    transport.RoleServer,
)
requestChain.AddHandler(serverFCHandler)

// Register for RESPONSE packets (OnSend)
responseChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeResponse.TypeID, 
    transport.RoleServer,
)
responseChain.AddHandler(serverFCHandler)

// Register handler chain for FCFeedback packets
fcFeedbackChain := transport.NewHandlerChain("ServerFCFeedbackChain", serverFCHandler)
udpTransport.RegisterHandlerChain(fcFeedbackType.TypeID, fcFeedbackChain, transport.RoleServer)
```

### Custom Configuration

```go
// Create handler with custom window sizes
clientFCHandler := flowcontrol.NewFCClientHandlerWithConfig(
    udpTransport,
    udpTransport.GetTimerManager(),
    10*1024*1024, // 10 MB initial receive window (default: 15 MB)
    20*1024*1024, // 20 MB max receive window (default: 25 MB)
)
```

## How It Works

1. **Sender checks window** before sending data
2. **Receiver tracks** bytes received and consumed
3. **When 75% of receive buffer is consumed**, receiver sends FCFeedback with new window size
4. **Sender updates** its send window and can send more data
5. **Automatic cleanup** removes idle connections after 30 seconds

## Configuration

**Default Window Sizes:**
- Initial receive window: 15 MB
- Max receive window: 25 MB
- Initial send window: 0 (updated by peer's first feedback)

These defaults work well for most applications but can be customized if needed.