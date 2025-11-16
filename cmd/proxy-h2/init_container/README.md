# Symphony Proxy-h2 Init Container

A lightweight init container that configures iptables rules for UDP traffic routing in Symphony proxy deployments.

## Purpose

This container runs before the main proxy container to set up network rules that:
- Mark UDP traffic on ports 10000-65535 with connection marks
- Redirect traffic to appropriate proxy ports (15002, 15006)
- Handle traffic routing based on connection marks and user ownership

## Usage

```bash
# Build the image
./build_images.sh
```

## Iptables Rules

The container applies rules that:
- Mark UDP traffic with connection marks (0x1, 0x2)
- Redirect traffic based on marks and user ownership
- Route traffic to proxy ports 15002 and 15006 