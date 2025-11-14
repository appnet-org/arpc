# TCP/HTTP Proxy with iptables Interception

This proxy implements a transparent TCP/HTTP proxy similar to Envoy that can intercept packets redirected by iptables.

## Features

- **iptables REDIRECT Support**: Automatically retrieves the original destination using `SO_ORIGINAL_DST` socket option
- **HTTP/2 gRPC Interception**: Handles HTTP/2 connections and can process gRPC messages
- **Transparent Proxying**: Works with iptables REDIRECT rules to intercept traffic transparently
- **Fallback Support**: Can use `TARGET_ADDR` environment variable if iptables interception is not available

## How It Works

The proxy listens on configured ports (default: 15002, 15006) and handles connections as follows:

1. **Connection Acceptance**: Accepts TCP connections on the configured ports
2. **Original Destination Retrieval**: Uses `SO_ORIGINAL_DST` socket option to get the original destination address set by iptables REDIRECT
3. **Connection Forwarding**: Connects to the original destination and forwards traffic
4. **Protocol Detection**: Detects HTTP/2 vs plain TCP and handles accordingly

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
go build -o proxy-http .
```

## Running

```bash
# Run with iptables interception (recommended)
./proxy-http

# Run with fallback target address
TARGET_ADDR=localhost:8080 ./proxy-http
```

## Technical Details

### SO_ORIGINAL_DST Implementation

The proxy retrieves the original destination using the Linux `SO_ORIGINAL_DST` socket option:

1. Converts the TCP connection to a file descriptor
2. Calls `getsockopt` with `SO_ORIGINAL_DST` at the `IPPROTO_IP` level
3. Parses the returned `sockaddr_in` structure to extract IP and port
4. Returns the original destination address

This approach is similar to how Envoy handles transparent proxying.

## Limitations

- Currently supports IPv4 only (IPv6 support can be added)
- Requires appropriate permissions to use socket options
- Works best with iptables REDIRECT (not TPROXY)

