
## 🛠 Running the Transparent UDP Proxy


### 1.  Build and Run the Proxy

First, build the binary and run it under a dedicated user (e.g., `proxyuser`) to avoid redirect loops:

```bash
go build -o myproxy main.go
sudo -u proxyuser ./myproxy
```

> 💡 Make sure `proxyuser` exists:
>
> ```bash
> sudo useradd -r -s /sbin/nologin proxyuser
> ```

---

### 2. Set Up `iptables` Rules

To transparently intercept both directions of UDP communication:

```bash
# Intercept server → client responses (PREROUTING for incoming packets)
sudo iptables -t nat -A PREROUTING -p udp --sport 10000:65535 -j REDIRECT --to-ports 15002

# Intercept client → server requests (OUTPUT for local packets)
sudo iptables -t nat -A OUTPUT -p udp --dport 10000:65535 -m owner ! --uid-owner proxyuser -j REDIRECT --to-ports 15002
```

These rules ensure:

* Packets from the **frontend to the server** are redirected to the proxy (except those sent by the proxy itself).
* Packets from the **server to the frontend** are also redirected to the proxy before reaching the frontend.

---

### 3. Confirm Rules Are Installed

Run the following to verify the rules are active and matching packets:

```bash
sudo iptables -t nat -L -n -v
```

You should see non-zero packet and byte counts for the two rules after sending traffic.

---

### 4. Resetting Rules (Cleanup)

To remove the rules if needed:

```bash
sudo iptables -t nat -D PREROUTING -p udp --sport 9000 -j REDIRECT --to-ports 15002
sudo iptables -t nat -D OUTPUT -p udp --dport 9000 -m owner ! --uid-owner proxyuser -j REDIRECT --to-ports 15002
```



# Flush old rules
sudo iptables -t nat -F
sudo iptables -t mangle -F
sudo iptables -t nat -L -n -v
sudo iptables -t mangle -L -n -v

# -- MANGLE TABLE: MARK outbound requests --
# Mark all outgoing traffic to server:11000
sudo iptables -t mangle -A OUTPUT -p udp --dport 10000:65535 -j CONNMARK --set-mark 0x1
sudo iptables -t mangle -A PREROUTING -p udp -j CONNMARK --restore-mark
sudo iptables -t nat -A PREROUTING -p udp -m connmark --mark 0x1 -j REDIRECT --to-port 15002
sudo iptables -t nat -A OUTPUT -p udp --dport 10000:65535 -m owner ! --uid-owner proxyuser -j REDIRECT --to-ports 15002


# 1. Mark inbound UDP traffic in PREROUTING (except from proxy itself)
sudo iptables -t mangle -A PREROUTING -p udp --dport 10000:65535 -j CONNMARK --set-mark 0x2

# 2. Restore connmark (for use in NAT PREROUTING)
sudo iptables -t mangle -A PREROUTING -p udp -j CONNMARK --restore-mark

# 3. NAT PREROUTING: Redirect marked traffic to 15006
sudo iptables -t nat -A PREROUTING -p udp -m connmark --mark 0x2 -j REDIRECT --to-port 15006

# 4. NAT OUTPUT: Redirect responses from app back through 15006 (unless it's the proxy itself)
sudo iptables -t nat -A OUTPUT -p udp -m connmark --mark 0x2 -m owner ! --uid-owner proxyuser -j REDIRECT --to-port 15006


Debugging Tips
Dump conntrack marks:


sudo conntrack -L -p udp
Monitor proxy ports:

sudo tcpdump -n -i any udp port 15002 or port 15006