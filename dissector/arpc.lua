-- arpc.lua

local arpc_proto = Proto("arpc", "aRPC Protocol")

-- Define fields
local f_packet_type   = ProtoField.uint8("arpc.packet_type", "Packet Type", base.DEC, {[1]="Data", [2]="Ack"})
local f_rpc_id        = ProtoField.uint64("arpc.rpc_id", "RPC ID", base.DEC)
local f_total_packets = ProtoField.uint16("arpc.total_packets", "Total Packets", base.DEC)
local f_seq_number    = ProtoField.uint16("arpc.seq_number", "Sequence Number", base.DEC)
local f_payload       = ProtoField.bytes("arpc.payload", "Payload")

arpc_proto.fields = {
  f_packet_type, f_rpc_id, f_total_packets, f_seq_number, f_payload
}

function arpc_proto.dissector(buffer, pinfo, tree)
    
    pinfo.cols.protocol = "aRPC"
    local subtree = tree:add(arpc_proto, buffer(), "aRPC Packet")

    local offset = 0

    subtree:add_le(f_packet_type,   buffer(offset, 1)); offset = offset + 1
    subtree:add_le(f_rpc_id,        buffer(offset, 8)); offset = offset + 8
    subtree:add_le(f_total_packets, buffer(offset, 2)); offset = offset + 2
    subtree:add_le(f_seq_number,    buffer(offset, 2)); offset = offset + 2

    if buffer:len() > offset then
        subtree:add(f_payload, buffer(offset))
    end
end

-- Register to a specific port (e.g., 9090)
local udp_port = DissectorTable.get("udp.port")
udp_port:add(9090, arpc_proto)
