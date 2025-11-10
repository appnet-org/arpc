#!/bin/bash

echo "Resetting iptables and applying proxy rules (inbound + outbound for appuser via proxy 15002)..."

# --- Minimal, safe flush ---
echo "Flushing existing iptables rules..."
sudo iptables -F                      # filter table
sudo iptables -t nat -F
sudo iptables -t mangle -F
sudo iptables -X
sudo iptables -t nat -X
sudo iptables -t mangle -X

# Safe defaults to avoid lockouts
sudo iptables -P INPUT ACCEPT
sudo iptables -P FORWARD ACCEPT
sudo iptables -P OUTPUT ACCEPT
echo "iptables flushed and reset to defaults."

# --- Apply minimal proxy rules ---
iptables-restore <<'EOF'
*nat
:PREROUTING ACCEPT [0:0]
:OUTPUT     ACCEPT [0:0]

# Inbound: redirect 8080 and 11000 to local proxy 15002
-A PREROUTING -p tcp -m multiport --dports 8080,11000 -j REDIRECT --to-ports 15002

# Outbound: let loopback traffic be local
-A OUTPUT -o lo -j RETURN

# Outbound: redirect only appuser's TCP to the proxy
-A OUTPUT -p tcp -m owner --uid-owner appuser -j REDIRECT --to-ports 15002

COMMIT
EOF

echo "iptables rules applied successfully (inbound: 8080,11000 → 15002; outbound: only appuser → 15002)."
