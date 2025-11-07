# Congestion Control Handler

A symmetric, connection-level congestion control handler that integrates with CUBIC algorithm, fully decoupled from reliable transport.

## Key Design Decisions

### 1. **Monotonic Packet ID from RPCID and SeqNum**

**Problem:** Both client and server need to compute the same packet ID for CUBIC tracking, but RPCID was random.

**Solution:** 
- Generate RPCID using monotonic timestamp (no random component)
- Construct packet ID: `packetID = (RPCID << 16) | SeqNum`
- Both sides use the same formula → same packet ID!

**Why this works:**
- RPCID is monotonically increasing (timestamp-based)
- SeqNum is 0-indexed within each RPC (0-65535)
- If RPCID increases → packetID increases (left shift dominates)
- If same RPCID but SeqNum increases → packetID increases (right bits)

**Example:**
```
RPC 100, seq 0 → packetID = (100 << 16) | 0 = 6,553,600
RPC 100, seq 1 → packetID = (100 << 16) | 1 = 6,553,601
RPC 101, seq 0 → packetID = (101 << 16) | 0 = 6,619,136  ← Increases!
```

### 2. **Symmetric Client/Server Design**

Both client and server have identical congestion control logic:
- **Client**: Tracks REQUEST sends, RESPONSE receives, sends/receives CCFeedback
- **Server**: Tracks RESPONSE sends, REQUEST receives, sends/receives CCFeedback
- ~80% code reuse via shared `CCHandler` base

**Pattern:**
```go
// utils.go - Base handler (~80% of logic)
type CCHandler struct {
    connections    map[string]*CCConnectionState
    // ... shared logic
}

// client_handler.go - Client-specific wrapper
type CCClientHandler struct {
    *CCHandler
}

// server_handler.go - Server-specific wrapper
type CCServerHandler struct {
    *CCHandler
}
```

### 3. **Count-Based Feedback (No Timeout)**

**Feedback triggers:**
- Every N packets (default: 10 packets)
- No timeout-based feedback (simplified design)

**Benefits:**
- Reduces feedback overhead (10x less than per-packet ACKs)
- Simple and predictable feedback pattern
- Decouples congestion control from reliable transport

**Feedback packet format:**
- Contains array of individual packet IDs that were received
- Includes total acked count and bytes
- Sent directly via UDP (bypasses handler chain)

### 4. **Individual Packet Tracking**

**Design:**
- Each sent packet is tracked individually in `SentPackets` map
- Each received packet is tracked individually in `ReceivedPackets` map
- No batching - direct per-packet tracking

**Benefits:**
- Accurate bytes-in-flight calculation
- Precise loss detection
- Per-packet CUBIC updates

### 5. **Timeout-Based Loss Detection**

**Conservative approach:**
- Sender tracks each packet with send time
- Periodic timer (every 100ms) checks for packets exceeding timeout
- Timeout = `defaultPacketTimeout * feedbackInterval` (e.g., 200ms * 10 = 2s)
- If packet timeout expires → assume loss
- Overestimating loss → more conservative → safer!

**Why this works:**
- Congestion control doesn't need to guarantee delivery
- Conservative loss estimation prevents over-aggressive sending
- Timeout-based approach works without reliable layer

### 6. **Per-Packet CUBIC Updates**

**When feedback arrives:**
- Feedback contains array of packet IDs
- For each acked packet ID, call `OnPacketAcked()` once
- For each lost packet (detected via timeout or gap), call `OnCongestionEvent()`

```go
for _, packetID := range feedback.PacketIDs {
    cubic.OnPacketAcked(
        packetID,  // Individual packet ID
        bytes,      // Individual packet bytes
        priorInFlight,
        eventTime,
    )
}
```

This maintains CUBIC's internal state (numAckedPackets, cwnd growth) correctly.

### 7. **Congestion Control Checks**

**Before sending packets:**
- Check `HasPacingBudget()` - ensures pacing budget is available
- Check `CanSend()` - ensures congestion window has space
- Currently logs warnings but doesn't block (can be enabled to return errors)

**Implementation:**
```go
// Check HasPacingBudget first
if !h.ccAlgorithm.HasPacingBudget(nowMonotime) {
    timeUntilSend := h.ccAlgorithm.TimeUntilSend(...)
    // Log warning (currently doesn't block)
}

// Check CanSend
if !h.ccAlgorithm.CanSend(protocol.ByteCount(bytesInFlight)) {
    // Log warning (currently doesn't block)
}
```

### 8. **Periodic Timers**

**Connection cleanup timer:**
- Runs every 1 second
- Removes connections that haven't been active for `defaultConnectionTimeout` (30s)

**Packet timeout check timer:**
- Runs every 100ms
- Checks for packets that haven't received feedback within `packetTimeout`
- Assumes loss and triggers CUBIC congestion event

## Usage

### Client-Side Setup

```go
import (
    "github.com/appnet-org/arpc/pkg/custom/congestion"
    "github.com/appnet-org/arpc/pkg/transport"
    "github.com/appnet-org/arpc/pkg/packet"
)

// Create UDP transport
udpTransport, _ := transport.NewUDPTransport(":0")
defer udpTransport.Close()

// Register CCFeedback packet type
ccFeedbackType, _ := udpTransport.RegisterPacketType(
    congestion.CCFeedbackPacketName, 
    &congestion.CCFeedbackCodec{},
)

// Create CC client handler (defaults: 10 packets feedback interval)
clientCCHandler := congestion.NewCCClientHandler(
    udpTransport,
    udpTransport.GetTimerManager(),
)
defer clientCCHandler.Cleanup()

// Register for REQUEST packets (OnSend)
requestChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeRequest.TypeID, 
    transport.RoleClient,
)
requestChain.AddHandler(clientCCHandler)

// Register for RESPONSE packets (OnReceive)
responseChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeResponse.TypeID, 
    transport.RoleClient,
)
responseChain.AddHandler(clientCCHandler)

// Register handler chain for CCFeedback packets
ccFeedbackChain := transport.NewHandlerChain("ClientCCFeedbackChain", clientCCHandler)
udpTransport.RegisterHandlerChain(ccFeedbackType.TypeID, ccFeedbackChain, transport.RoleClient)
```

### Server-Side Setup

```go
// Create UDP transport
udpTransport, _ := transport.NewUDPTransport(":8080")
defer udpTransport.Close()

// Register CCFeedback packet type
ccFeedbackType, _ := udpTransport.RegisterPacketType(
    congestion.CCFeedbackPacketName, 
    &congestion.CCFeedbackCodec{},
)

// Create CC server handler
serverCCHandler := congestion.NewCCServerHandler(
    udpTransport,
    udpTransport.GetTimerManager(),
)
defer serverCCHandler.Cleanup()

// Register for REQUEST packets (OnReceive)
requestChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeRequest.TypeID, 
    transport.RoleServer,
)
requestChain.AddHandler(serverCCHandler)

// Register for RESPONSE packets (OnSend)
responseChain, _ := udpTransport.GetHandlerRegistry().GetHandlerChain(
    packet.PacketTypeResponse.TypeID, 
    transport.RoleServer,
)
responseChain.AddHandler(serverCCHandler)

// Register handler chain for CCFeedback packets
ccFeedbackChain := transport.NewHandlerChain("ServerCCFeedbackChain", serverCCHandler)
udpTransport.RegisterHandlerChain(ccFeedbackType.TypeID, ccFeedbackChain, transport.RoleServer)
```

### Advanced Usage (Custom Configuration)

```go
import (
    "github.com/appnet-org/arpc/pkg/custom/congestion"
)

// Create handler with custom config
clientCCHandler := congestion.NewCCClientHandlerWithConfig(
    udpTransport,
    udpTransport.GetTimerManager(),
    20,  // feedback every 20 packets
)

// Query state
canSend := clientCCHandler.CanSend()
clientCCHandler.SetFeedbackInterval(30) // adjust dynamically
```

## Files

- **`utils.go`** - Base `CCHandler` with shared state, packet tracking, feedback logic, loss detection, and timers
- **`client_handler.go`** - Client-specific wrapper (handles REQUEST sends, RESPONSE receives)
- **`server_handler.go`** - Server-specific wrapper (handles RESPONSE sends, REQUEST receives)
- **`ccfeedback_packet.go`** - CCFeedback packet format (17 bytes header + 8 bytes per packet ID)
- **`cubic/`** - CUBIC congestion control algorithm (ported from QUIC-Go)

## How Congestion Control Works

### The Control Flow

```
Client/Server:
┌─────────────────────────────────────┐
│ 1. OnSend() - Track sent packet    │
│    Check HasPacingBudget()          │
│    Check CanSend()                  │
│    Add to SentPackets map           │
│    Call CUBIC OnPacketSent()        │
│                                     │
│ 2. OnReceive(DataPacket)            │
│    Track received packet            │
│    Add to ReceivedPackets map       │
│    If count >= feedbackInterval:    │
│      Send CCFeedback                │
│                                     │
│ 3. OnReceive(CCFeedback)            │
│    Process acked packets            │
│    Detect lost packets (gap-based)  │
│    Call OnPacketAcked() per packet │
│    Call OnCongestionEvent() per loss│
│    → cwnd grows/shrinks            │
│                                     │
│ 4. Periodic Timer (100ms)           │
│    Check for timeout packets        │
│    If timeout → assume loss         │
│    Call OnRetransmissionTimeout()   │
│    → cwnd shrinks                   │
└─────────────────────────────────────┘
```

### Symmetric Architecture

```
Client:                          Server:
┌──────────┐                    ┌──────────┐
│OnSend    │ ──(REQUEST)──────> │OnReceive │
│(track)   │                    │(track)   │
│          │                    │          │
│          │ <─(CCFeedback)──── │OnSend    │
│OnReceive │                    │(feedback)│
│(process) │                    │          │
│          │                    │          │
│OnReceive │ <─(RESPONSE)────── │OnSend    │
│(track)   │                    │(track)   │
│          │                    │          │
│OnSend    │ ──(CCFeedback)────> │OnReceive│
│(feedback)│                   │(process) │
└──────────┘                    └──────────┘
```

Both sides independently manage CUBIC algorithms and track their own connections.

## CCFeedback Packet Format

**Structure (17 bytes header + 8 bytes per packet ID):**
```go
type CCFeedbackPacket struct {
    PacketTypeID packet.PacketTypeID // 1 byte
    AckedCount   uint32              // 4 bytes - number of packets acked
    AckedBytes   uint64              // 8 bytes - total bytes acked
    PacketIDs    []uint64            // Variable length - array of packet IDs
}
```

**Serialization:**
- Header: 1 + 4 + 8 + 4 = 17 bytes
- PacketIDCount: 4 bytes
- PacketIDs: 8 bytes per packet ID
- Total: 17 + 4 + (8 * N) bytes for N packets

**Sending:**
- CCFeedback packets are sent directly via UDP (bypass handler chain)
- Address is derived from `ConnID` (IP and Port)
- Similar to how ACK packets are sent in reliable layer

## Loss Detection Details

**Two mechanisms:**

1. **Gap-based detection (from feedback):**
   - When feedback arrives, find smallest acked packet ID
   - Any sent packet with ID < smallest acked → assumed lost
   - Triggers `OnCongestionEvent()` for each lost packet

2. **Timeout-based detection (periodic timer):**
   - Timer runs every 100ms
   - Check each sent packet: `now - sendTime > packetTimeout`
   - If timeout → assume loss
   - Triggers `OnRetransmissionTimeout()` (conservative)

**Example scenarios:**

```
Scenario 1: All packets delivered
  - Send packets 1, 2, 3, 4, 5
  - Receive feedback: [1, 2, 3, 4, 5]
  - All packets ACKed ✓
  
Scenario 2: Some packets lost (gap-based)
  - Send packets 1, 2, 3, 4, 5
  - Receive feedback: [3, 4, 5] (smallest = 3)
  - Packets 1, 2 < 3 → assumed lost
  - CUBIC reduces congestion window
  
Scenario 3: Timeout-based loss
  - Send packet 1 at t0
  - No feedback received
  - Timer checks at t0 + 2s → timeout
  - Assume packet 1 lost
  - CUBIC reduces congestion window
```

## Connection Management

**Automatic cleanup:**
- Periodic timer (every 1 second) checks for inactive connections
- Connections inactive for > 30 seconds are removed
- Prevents memory leaks from stale connections

**Per-connection state:**
- Each connection maintains:
  - `SentPackets` map - tracks sent packets (for bytes-in-flight and loss detection)
  - `ReceivedPackets` map - tracks received packets (for feedback)
  - `feedbackCount` - counter for feedback triggering

## Future Work

1. **Enable blocking on congestion checks**: Currently `HasPacingBudget()` and `CanSend()` only log warnings. Could return errors to block sending.
2. **Adaptive Intervals**: Adjust feedback frequency based on RTT/cwnd
3. **BBR Support**: Add interfaces for delay-based CC algorithms
4. **RTT Measurement**: Add accurate RTT calculation from packet timestamps
