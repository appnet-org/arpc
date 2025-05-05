#!/usr/bin/env python3

from bcc import BPF
from datetime import datetime
import argparse
from pyroute2 import IPRoute
import time
import socket
import struct
import ctypes

# Compile and load BPF program
b = BPF(src_file="tc.c", debug=0)
ipr = IPRoute()

def print_ip(ip):
    return socket.inet_ntoa(struct.pack("<I", ip))

class Data(ctypes.Structure):
    _fields_ = [
        ("saddr", ctypes.c_uint32),
        ("daddr", ctypes.c_uint32),
        ("sport", ctypes.c_uint16),
        ("dport", ctypes.c_uint16),
        ("protocol", ctypes.c_ubyte),
        ("payload", ctypes.c_ubyte * 64),
        ("payload_len", ctypes.c_uint32),
    ]

class EventHandler:
    def process_event(self, cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(Data)).contents
        proto = 'TCP' if event.protocol == 6 else 'UDP' if event.protocol == 17 else str(event.protocol)
        print(f"{datetime.now().strftime('%H:%M:%S')} | "
              f"SRC: {print_ip(event.saddr):15} | "
              f"DST: {print_ip(event.daddr):15} | "
              f"SPORT: {event.sport:5} | "
              f"DPORT: {event.dport:5} | "
              f"PROTO: {proto}")
        if event.payload_len > 0:
            try:
                payload = bytes(event.payload[:event.payload_len]).hex()
            except Exception as e:
                payload = "<decode error>"
            print(f"PAYLOAD: {payload}")
        print("-" * 80)

def main():
    parser = argparse.ArgumentParser(description="traffic monitor")
    parser.add_argument("-i", "--interface", default="enp24s0f0", help="network interface to monitor")
    args = parser.parse_args()

    idx = None
    try:
        ingress_fn = b.load_func("tc_ingress", BPF.SCHED_CLS)
        egress_fn = b.load_func("tc_egress", BPF.SCHED_CLS)

        idx = ipr.link_lookup(ifname=args.interface)[0]

        try: ipr.tc("del", "ingress", idx, "ffff:")
        except: pass
        try: ipr.tc("del", "htb", idx, "1:")
        except: pass

        ipr.tc("add", "ingress", idx, "ffff:")
        ipr.tc("add-filter", "bpf", idx, ":1",
               fd=ingress_fn.fd, name=ingress_fn.name,
               parent="ffff:", action="ok", classid=1)

        ipr.tc("add", "htb", idx, "1:")
        ipr.tc("add-filter", "bpf", idx, ":2",
               fd=egress_fn.fd, name=egress_fn.name,
               parent="1:", action="ok", classid=1)

        print(f"BPF attached to {args.interface}. Press Ctrl+C to exit.")

        handler = EventHandler()
        b["events"].open_perf_buffer(handler.process_event)

        while True:
            b.perf_buffer_poll()
    except KeyboardInterrupt:
        print("Detaching BPF program...")
    finally:
        if idx is not None:
            try: ipr.tc("del", "ingress", idx, "ffff:")
            except: pass
            try: ipr.tc("del", "htb", idx, "1:")
            except: pass
        print("BPF detached.")

if __name__ == "__main__":
    main()
