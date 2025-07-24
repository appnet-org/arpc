```bash
# ssh into frontend pod and run:
iptables -t nat -A PREROUTING -p udp --sport 9000 -j REDIRECT --to-ports 15002
iptables -t nat -A OUTPUT -p udp --dport 9000 -m owner ! --uid-owner 1337 -j REDIRECT --to-ports 15002
```