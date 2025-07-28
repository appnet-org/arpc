#!/bin/bash

echo "Applying Symphony iptables rules..."

iptables-restore <<EOF
*nat
:PREROUTING ACCEPT [0:0]
:INPUT ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
:POSTROUTING ACCEPT [0:0]
:SYMPHONY_INBOUND - [0:0]
:SYMPHONY_IN_REDIRECT - [0:0]
:SYMPHONY_OUTPUT - [0:0]
:SYMPHONY_REDIRECT - [0:0]

# Only redirect UDP packets with dport >= 10000
-A PREROUTING -p udp -j SYMPHONY_INBOUND
-A OUTPUT -p udp -j SYMPHONY_OUTPUT

# SYMPHONY_INBOUND chain
-A SYMPHONY_INBOUND -p udp --dport 22 -j RETURN
-A SYMPHONY_INBOUND -p udp -j SYMPHONY_IN_REDIRECT

# Redirect inbound to 15006
-A SYMPHONY_IN_REDIRECT -p udp --dport 15002 -j RETURN
-A SYMPHONY_IN_REDIRECT -p udp --dport 15006 -j RETURN
-A SYMPHONY_IN_REDIRECT -p udp -j REDIRECT --to-ports 15006

# SYMPHONY_OUTPUT chain (applies only to ports >= 10000)
-A SYMPHONY_OUTPUT -s 127.0.0.6/32 -o lo -j RETURN

# Match by UID 998
-A SYMPHONY_OUTPUT ! -d 127.0.0.1/32 -o lo -m owner --uid-owner 998 -j SYMPHONY_IN_REDIRECT
-A SYMPHONY_OUTPUT -o lo -m owner ! --uid-owner 998 -j RETURN
-A SYMPHONY_OUTPUT -m owner --uid-owner 998 -j RETURN

# Match by GID 998
-A SYMPHONY_OUTPUT ! -d 127.0.0.1/32 -o lo -m owner --gid-owner 998 -j SYMPHONY_IN_REDIRECT
-A SYMPHONY_OUTPUT -o lo -m owner ! --gid-owner 998 -j RETURN
-A SYMPHONY_OUTPUT -m owner --gid-owner 998 -j RETURN

-A SYMPHONY_OUTPUT -d 127.0.0.1/32 -j RETURN

# Final catch-all for UDP ports >= 10000
-A SYMPHONY_OUTPUT -p udp -j SYMPHONY_REDIRECT

# Redirect general outbound to 15002
-A SYMPHONY_REDIRECT -p udp -j REDIRECT --to-ports 15002

COMMIT
EOF

echo "iptables rules applied successfully."


# Chain PREROUTING (policy ACCEPT 3 packets, 144 bytes)
#  pkts bytes target     prot opt in     out     source               destination
#     0     0 REDIRECT   udp  --  *      *       0.0.0.0/0            0.0.0.0/0            udp spts:10000:65535 redir ports 15002

# Chain INPUT (policy ACCEPT 2 packets, 84 bytes)
#  pkts bytes target     prot opt in     out     source               destination

# Chain OUTPUT (policy ACCEPT 26 packets, 1732 bytes)
#  pkts bytes target     prot opt in     out     source               destination
#     1    92 REDIRECT   udp  --  *      *       0.0.0.0/0            0.0.0.0/0            udp dpts:10000:65535 ! owner UID match 998 redir ports 15002

# Chain POSTROUTING (policy ACCEPT 27 packets, 1824 bytes)
#  pkts bytes target     prot opt in     out     source               destination
#    33  2211 FLANNEL-POSTRTG  all  --  *      *       0.0.0.0/0            0.0.0.0/0            /* flanneld masq */

# Chain FLANNEL-POSTRTG (1 references)
#  pkts bytes target     prot opt in     out     source               destination
#     0     0 RETURN     all  --  *      *       0.0.0.0/0            0.0.0.0/0            mark match 0x4000/0x4000 /* flanneld masq */
#     5   300 RETURN     all  --  *      *       10.244.0.0/24        10.244.0.0/16        /* flanneld masq */
#     0     0 RETURN     all  --  *      *       10.244.0.0/16        10.244.0.0/24        /* flanneld masq */
#     0     0 RETURN     all  --  *      *      !10.244.0.0/16        10.244.0.0/24        /* flanneld masq */
#     1    60 MASQUERADE  all  --  *      *       10.244.0.0/16       !224.0.0.0/4          /* flanneld masq */ random-fully
#     0     0 MASQUERADE  all  --  *      *      !10.244.0.0/16        10.244.0.0/16        /* flanneld masq */ random-fully

# Chain SYMPHONY_INBOUND (0 references)
#  pkts bytes target     prot opt in     out     source               destination

# Chain SYMPHONY_IN_REDIRECT (0 references)
#  pkts bytes target     prot opt in     out     source               destination

# Chain SYMPHONY_OUTPUT (0 references)
#  pkts bytes target     prot opt in     out     source               destination

# Chain SYMPHONY_REDIRECT (0 references)
#  pkts bytes target     prot opt in     out     source               destination