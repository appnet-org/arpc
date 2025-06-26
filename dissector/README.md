# aRPC Packet Dissector

## Usage

**Step 1: Capture some aRPC packets**  
Use tools like `tcpdump` or Wireshark to capture some aRPC traffic.

**Step 2: Open the packet capture in Wireshark**  
Launch Wireshark and open the `.pcap` file containing the aRPC packets.

**Step 3: Load the Lua plugin**  
Copy `arpc.lua` to your personal Wireshark plugin directory  
(it can be found in **Help → About Wireshark → Folders**).

Click **Analyze → Reload Lua Plugins** and decode the packet with `arpc`.


## Example

A sample aRPC message can be found in `samples/echo_capnp.pcap`.