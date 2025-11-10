# Bytes Relay Proxy

This directory contains a simple **transparent TCP proxy** that relays traffic bidirectionally without modification. The proxy intercepts connections using iptables rules and forwards them to their original destinations.

## What It Does

The proxy (`simple-proxy.go`) is a minimal transparent relay that:

- Listens on port **15002** by default (configurable via command-line argument)
- Accepts connections redirected by iptables
- Retrieves the original destination using `SO_ORIGINAL_DST` socket option
- Establishes a connection to the original destination
- Relays data **bidirectionally** between client and destination
- Logs all connection activity for monitoring

**Key Point**: This proxy does **not** modify, inspect, or transform the traffic—it simply relays bytes between endpoints.

## Usage

```bash
# Build the proxy
go build simple-proxy.go

# Run with default port (15002)
sudo ./simple-proxy

# Run with custom port
sudo ./simple-proxy 8888
```

Note: The proxy requires root privileges to read `SO_ORIGINAL_DST` from socket file descriptors.

## iptables Rules Explained

The `apply_iptables.sh` script configures transparent traffic interception using iptables NAT rules:

### Inbound Traffic Interception

```bash
-A PREROUTING -p tcp -m multiport --dports 8080,11000 -j REDIRECT --to-ports 15002
```

**What it does**: Redirects all **incoming** TCP connections destined for ports **8080** or **11000** to the proxy on port **15002**.

- Traffic intended for these ports is transparently captured before reaching the application
- The proxy retrieves the original destination (8080 or 11000) and forwards traffic there
- Useful for intercepting/monitoring inbound service traffic

### Outbound Traffic Interception

```bash
# Allow loopback traffic to bypass the proxy
-A OUTPUT -o lo -j RETURN

# Redirect TCP traffic from 'appuser' to the proxy
-A OUTPUT -p tcp -m owner --uid-owner appuser -j REDIRECT --to-ports 15002
```

**What it does**: Redirects all **outgoing** TCP connections made by the user `appuser` to the proxy on port **15002**, except for loopback traffic.

- Only traffic from processes running as user `appuser` is intercepted
- Loopback connections (localhost/127.0.0.1) bypass the proxy to avoid redirection loops
- The proxy retrieves the original destination and connects to it on behalf of `appuser`
- Useful for monitoring/logging all outbound connections from a specific application user

### Rule Application

```bash
# Apply the rules
sudo ./apply_iptables.sh

# Verify rules
sudo iptables -t nat -L -n -v
```

The script safely flushes existing rules and sets default policies to `ACCEPT` to prevent SSH lockouts.