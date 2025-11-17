# TCP Proxy with iptables Interception

This proxy implements a transparent TCP proxy similar to Envoy that can intercept packets redirected by iptables.

## Features

- **iptables REDIRECT Support**: Automatically retrieves the original destination using `SO_ORIGINAL_DST` socket option
- **TCP Stream Interception**: Handles raw TCP connections and can process data through element chains
- **Transparent Proxying**: Works with iptables REDIRECT rules to intercept traffic transparently
- **Fallback Support**: Can use `TARGET_ADDR` environment variable if iptables interception is not available
- **Element Chain Processing**: Supports processing data through RPC element chains (similar to Envoy WASM filters)

## How It Works

The proxy listens on configured ports (default: 15002, 15006) and handles connections as follows:

1. **Connection Acceptance**: Accepts TCP connections on the configured ports
2. **Original Destination Retrieval**: Uses `SO_ORIGINAL_DST` socket option to get the original destination address set by iptables REDIRECT
3. **Connection Forwarding**: Connects to the original destination and forwards traffic bidirectionally
4. **Data Processing**: Processes data chunks through element chains before forwarding

## Architecture

The proxy follows the same architecture as `proxy-h2` but operates on raw TCP streams:

- **ProxyState**: Manages the element chain and connection state
- **Element Chain**: Processes data through configurable RPC elements
- **Bidirectional Forwarding**: Handles client->target and target->client streams independently
- **Connection Management**: Tracks connections with unique IDs

## iptables Configuration

To use with iptables REDIRECT, configure rules like this:

```bash
# Redirect incoming TCP traffic on port 8080 to proxy port 15002
iptables -t nat -A PREROUTING -p tcp --dport 8080 -j REDIRECT --to-port 15002

# Redirect outgoing TCP traffic on port 8080 to proxy port 15002
iptables -t nat -A OUTPUT -p tcp --dport 8080 -j REDIRECT --to-port 15002
```

## Configuration

### Environment Variables

- `LOG_LEVEL`: Logging level (default: `debug`)
- `LOG_FORMAT`: Logging format (default: `console`)
- `TARGET_ADDR`: Fallback target address if SO_ORIGINAL_DST is unavailable (default: empty, uses iptables)

### Default Ports

The proxy listens on ports: `15002`, `15006`

## Building

```bash
go build -o proxy-tcp .
```

## Running

```bash
# Run with iptables interception (recommended)
./proxy-tcp

# Run with fallback target address
TARGET_ADDR=localhost:8080 ./proxy-tcp
```

## Element Chain

The proxy supports processing data through RPC element chains. Elements can:

- Inspect and modify TCP data
- Drop data (return `VerdictDrop`)
- Pass data through (return `VerdictPass`)

Elements are processed in forward order for requests and reverse order for responses, similar to Envoy WASM filters.

## Technical Details

### SO_ORIGINAL_DST Implementation

The proxy retrieves the original destination using the Linux `SO_ORIGINAL_DST` socket option:

1. Converts the TCP connection to a file descriptor
2. Calls `getsockopt` with `SO_ORIGINAL_DST` at the `IPPROTO_IP` level
3. Parses the returned `sockaddr_in` structure to extract IP and port
4. Returns the original destination address

This approach is similar to how Envoy handles transparent proxying.

### Data Processing

- Data is read in chunks (default: 4096 bytes)
- Each chunk is processed through the element chain
- Processed data is forwarded to the destination
- Dropped data is not forwarded

## Limitations

- Currently supports IPv4 only (IPv6 support can be added)
- Requires appropriate permissions to use socket options
- Works best with iptables REDIRECT (not TPROXY)
- TCP is a stream protocol, so message boundaries are not preserved

## Comparison with proxy-h2

- **proxy-h2**: Handles HTTP/2 frames, parses headers, buffers per-stream
- **proxy-tcp**: Handles raw TCP streams, processes data chunks, simpler architecture

Both proxies share the same core architecture:
- iptables interception via SO_ORIGINAL_DST
- Element chain processing
- Bidirectional forwarding
- Connection management

