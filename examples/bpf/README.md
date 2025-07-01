# aRPC UDP Interceptor (eBPF/BCC)

This tool uses eBPF (via BCC) to trace and decode aRPC UDP packets sent from your system, printing out service, method, and header information for each packet.

## Prerequisites

- **Ubuntu 20.04+** (other distros may work but are untested)
- **Python 3**
- **BCC (BPF Compiler Collection) and Python bindings**

## Installation

1. **Install BCC and dependencies**

   You can use the provided script:

   ```bash
   ./install.sh
   ```

   This will install all required packages and build BCC from source.

## Usage

1. **Edit the UDP port if needed**

   By default, the script traces UDP port `9000`. If your aRPC server uses a different port, edit the `UDP_PORT` variable at the top of `intercept_arpc.py`:

   ```python
   UDP_PORT = 9000  # change to match your aRPC server port
   ```

2. **Run the script with root privileges**

   eBPF tracing requires root:

   ```bash
   sudo python3 intercept_arpc.py
   ```

3. **Send some aRPC UDP traffic**

   The script will print decoded information for each UDP packet sent to the specified port.

4. **Stop tracing**

   Press `Ctrl+C` to exit.
