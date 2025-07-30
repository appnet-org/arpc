## Running the Transparent UDP Proxy

This guide walks you through building and running the UDP proxy, configuring `iptables` for transparent redirection, and debugging the setup.

---

### 1. Build and Run the Proxy

Build the proxy binary and run it under a dedicated user (e.g., `proxyuser`) to prevent redirect loops.

```bash
go build -o myproxy main.go
sudo -u proxyuser ./myproxy
```

> 💡 Create the `proxyuser` account if it doesn't exist:
>
> ```bash
> sudo useradd -r -s /sbin/nologin proxyuser
> ```

---

### 2. Set Up `iptables` Rules

These rules transparently intercept both **outbound** and **inbound** UDP traffic:

#### Outbound Traffic (e.g., app → external server)

```bash
# Mark outbound UDP traffic from the app
sudo iptables -t mangle -A OUTPUT -p udp --dport 10000:65535 -j CONNMARK --set-mark 0x1

# Restore the connmark for incoming responses
sudo iptables -t mangle -A PREROUTING -p udp -j CONNMARK --restore-mark

# Redirect inbound response packets to proxy (15002)
sudo iptables -t nat -A PREROUTING -p udp -m connmark --mark 0x1 -j REDIRECT --to-port 15002

# Redirect app-generated outbound packets to proxy (15002)
sudo iptables -t nat -A OUTPUT -p udp --dport 10000:65535 -m owner ! --uid-owner proxyuser -j REDIRECT --to-ports 15002
```

#### Inbound Traffic (e.g., external client → app)

```bash
# Mark inbound UDP traffic to the app
sudo iptables -t mangle -A PREROUTING -p udp --dport 10000:65535 -j CONNMARK --set-mark 0x2

# Restore the connmark for outbound responses
sudo iptables -t mangle -A PREROUTING -p udp -j CONNMARK --restore-mark

# Redirect inbound packets to proxy (15006)
sudo iptables -t nat -A PREROUTING -p udp -m connmark --mark 0x2 -j REDIRECT --to-port 15006

# Redirect app-generated responses to proxy (15006)
sudo iptables -t nat -A OUTPUT -p udp -m connmark --mark 0x2 -m owner ! --uid-owner proxyuser -j REDIRECT --to-port 15006
```

---

### 3. Confirm Rules Are Active

Check whether the `iptables` rules are installed and processing packets:

```bash
sudo iptables -t nat -L -n -v
sudo iptables -t mangle -L -n -v
```

Look for non-zero **pkts** and **bytes** columns on relevant rules after generating traffic.

---

### 4. Reset Rules (Cleanup)

To flush all rules in the `nat` and `mangle` tables:

```bash
sudo iptables -t nat -F
sudo iptables -t mangle -F
```

---

### Debugging Tips

#### Dump conntrack entries (look for marks):

```bash
sudo conntrack -L -p udp
```

#### Monitor proxy ports (to verify redirection):

```bash
sudo tcpdump -n -i any udp port 15002 or port 15006
```