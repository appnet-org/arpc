#!/usr/bin/env python3

from bcc import BPF
from datetime import datetime
import argparse
from pyroute2 import IPRoute # python3.8 -m pip install "pyroute2<0.7.0"
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

def decode_rpc_frame(data):
    try:
        offset = 0
        
        # Message ID (8 bytes)
        if len(data) < offset + 8:
            return f"Too short for message_id (offset={offset}, len={len(data)})", {}
        message_id = struct.unpack("<Q", data[offset:offset+8])[0]
        offset += 8
        
        # Protocol version (4 bytes)
        if len(data) < offset + 4:
            return f"Too short for protocol_version (offset={offset}, len={len(data)})", {}
        protocol_version = struct.unpack("<I", data[offset:offset+4])[0]
        offset += 4
        
        # Service name
        if len(data) < offset + 2:
            return f"Too short for service_len (offset={offset}, len={len(data)})", {}
        service_len = struct.unpack("<H", data[offset:offset+2])[0]
        offset += 2
        
        if len(data) < offset + service_len:
            return f"Too short for service (offset={offset}, service_len={service_len}, len={len(data)})", {}
        service = data[offset:offset+service_len].decode("utf-8", errors="ignore")
        offset += service_len
        
        # Method name
        if len(data) < offset + 2:
            return f"Too short for method_len (offset={offset}, len={len(data)})", {}
        method_len = struct.unpack("<H", data[offset:offset+2])[0]
        offset += 2
        
        if len(data) < offset + method_len:
            return f"Too short for method (offset={offset}, method_len={method_len}, len={len(data)})", {}
        method = data[offset:offset+method_len].decode("utf-8", errors="ignore")
        offset += method_len
        
        # Message type
        if len(data) < offset + 4:
            return f"Too short for message_type (offset={offset}, len={len(data)})", {}
        message_type = struct.unpack("<I", data[offset:offset+4])[0]
        offset += 4
        
        # Parameters
        params = {}
        while offset < len(data):
            if len(data) < offset + 2:
                break
            param_len = struct.unpack("<H", data[offset:offset+2])[0]
            offset += 2
            
            if len(data) < offset + param_len:
                break
            param_name = data[offset:offset+param_len].decode("utf-8", errors="ignore")
            offset += param_len
            
            if len(data) < offset + 2:
                break
            value_len = struct.unpack("<H", data[offset:offset+2])[0]
            offset += 2
            
            if len(data) < offset + value_len:
                break
            param_value = data[offset:offset+value_len].decode("utf-8", errors="ignore")
            offset += value_len
            
            params[param_name] = param_value
        
        return "OK", {
            "message_id": message_id,
            "protocol_version": protocol_version,
            "service": service,
            "method": method,
            "message_type": message_type,
            "params": params
        }
        
    except Exception as e:
        return f"Error: {e}", {}

class EventHandler:
    def process_event(self, cpu, data, size):
        print(f"Processing event on CPU {cpu}")
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
                payload = bytes(event.payload[:event.payload_len])
                status, decoded = decode_rpc_frame(payload)
                if status == "OK":
                    print(f"RPC Message:")
                    print(f"  Message ID: {decoded['message_id']}")
                    print(f"  Service: {decoded['service']}")
                    print(f"  Method: {decoded['method']}")
                    print(f"  Message Type: {decoded['message_type']}")
                    print(f"  Parameters:")
                    for name, value in decoded['params'].items():
                        print(f"    {name}: {value}")
                else:
                    print(f"Decode Error: {status}")
                    print(f"Raw Payload: {payload.hex()}")
            except Exception as e:
                print(f"Error decoding payload: {e}")
                print(f"Raw Payload: {payload.hex()}")
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
