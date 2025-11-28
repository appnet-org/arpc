#!/bin/bash

echo "Applying sidecar proxy rules..."

iptables-restore <<'EOF'
*nat
:PREROUTING ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]

### --- EXCLUSIONS (prevent infinite loops & preserve system traffic) ---

# 1. Do NOT intercept proxy’s own traffic
-A OUTPUT -m owner --uid-owner 1337 -j RETURN

# 2. Do NOT intercept loopback (prevents hairpin loops)
-A OUTPUT -o lo -j RETURN

# 3. Do NOT intercept DNS (recommended)
-A OUTPUT -p udp --dport 53 -j RETURN
-A OUTPUT -p tcp --dport 53 -j RETURN

# 4. Preserve existing connections
-A PREROUTING -m conntrack --ctstate RELATED,ESTABLISHED -j RETURN
-A OUTPUT     -m conntrack --ctstate RELATED,ESTABLISHED -j RETURN


### --- INBOUND INTERCEPTION (for traffic *into* the app) ---

# Redirect client → app incoming traffic to inbound proxy at 15006
-A PREROUTING -p tcp --dport 11000:11100 -j REDIRECT --to-ports 15006


### --- OUTBOUND INTERCEPTION (for app-initiated connections) ---

# Redirect app outbound traffic → outbound proxy at 15002
-A OUTPUT -p tcp --dport 11000:11100 -m owner ! --uid-owner 1337 -j REDIRECT --to-ports 15002

COMMIT
EOF

echo "Sidecar proxy rules applied successfully."