FROM ubuntu:22.04

# Install iptables (and clean up to reduce image size)
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends iptables && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY proxy-tcp/init_container/apply_tcp_iptables.sh /apply_symphony_iptables.sh

