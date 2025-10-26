## Running the Transparent UDP Proxy

This guide walks you through building and running the Symphony proxy locally, configuring `iptables` for transparent redirection, and debugging the setup.

---

### 1. Build and Run the Proxy

Build the proxy binary and run it under a dedicated user (e.g., `proxyuser`) to prevent redirect loops.

```bash
go build -o myproxy main.go buffer.go
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

```bash
sudo bash apply_symphony_iptables_local.sh
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