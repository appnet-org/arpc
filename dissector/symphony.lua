-- symphony.lua

local symphony_proto = Proto("Symphony", "Symphony RPC Protocol")

-- Define fields

local f_method_type   = ProtoField.uint8("symphony.method_type", "Method Type", base.DEC)
local f_rpc_id        = ProtoField.uint32("symphony.rpc_id", "RPC ID", base.DEC)
local f_num_of_pkts   = ProtoField.uint8("symphony.num_of_pkts", "Num of Packets", base.DEC)
local f_seq_num       = ProtoField.uint8("symphony.seq_num", "Seq Number", base.DEC)

symphony_proto.fields = {
  f_method_type, f_rpc_id, f_num_of_pkts, f_seq_num,
  f_session_id, f_username, f_cache_control
}

-- Dissector function
function symphony_proto.dissector(buffer, pinfo, tree)
    pinfo.cols.protocol = "Symphony"
    local subtree = tree:add(symphony_proto, buffer(), "Symphony Protocol Data")

    subtree:add(f_method_type, buffer(0,1))
    subtree:add(f_rpc_id, buffer(1,4))
    subtree:add(f_num_of_pkts, buffer(5,1))
    subtree:add(f_seq_num, buffer(6,1))
    subtree:add(f_session_id, buffer(7,2))
    subtree:add(f_username, buffer(9,32))
    subtree:add(f_cache_control, buffer(41,1))
end

-- Register dissector to a specific UDP port (adjust as needed)
local udp_port = DissectorTable.get("udp.port")
udp_port:add(9090, symphony_proto)
