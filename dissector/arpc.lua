-- arpc.lua

local arpc_proto = Proto("arpc", "aRPC Protocol")

-- Packet header fields
local f_packet_type   = ProtoField.uint8("arpc.packet_type", "Packet Type", base.DEC, {[1]="Data", [2]="Ack"})
local f_rpc_id        = ProtoField.uint64("arpc.rpc_id", "RPC ID", base.DEC)
local f_total_packets = ProtoField.uint16("arpc.total_packets", "Total Packets", base.DEC)
local f_seq_number    = ProtoField.uint16("arpc.seq_number", "Sequence Number", base.DEC)

-- Framed request fields
local f_service_len   = ProtoField.uint16("arpc.service_len", "Service Name Length", base.DEC)
local f_service       = ProtoField.string("arpc.service", "Service Name")
local f_method_len    = ProtoField.uint16("arpc.method_len", "Method Name Length", base.DEC)
local f_method        = ProtoField.string("arpc.method", "Method Name")
local f_header_len    = ProtoField.uint16("arpc.header_len", "Header Length", base.DEC)
local f_headers       = ProtoField.bytes("arpc.headers", "aRPC Headers")
local f_header_count  = ProtoField.uint16("arpc.header_count", "Header Count", base.DEC)
local f_header_key    = ProtoField.string("arpc.header.key", "Key")
local f_header_value  = ProtoField.string("arpc.header.value", "Value")
local f_payload       = ProtoField.bytes("arpc.payload", "Payload")

arpc_proto.fields = {
  f_packet_type, f_rpc_id, f_total_packets, f_seq_number,
  f_service_len, f_service, f_method_len, f_method,
  f_header_len, f_headers, f_header_count, f_header_key, f_header_value,
  f_payload
}

function arpc_proto.dissector(buffer, pinfo, tree)
    pinfo.cols.protocol = "aRPC"
    local subtree = tree:add(arpc_proto, buffer(), "aRPC Packet")

    local offset = 0

    subtree:add_le(f_packet_type,   buffer(offset, 1)); offset = offset + 1
    subtree:add_le(f_rpc_id,        buffer(offset, 8)); offset = offset + 8
    subtree:add_le(f_total_packets, buffer(offset, 2)); offset = offset + 2
    subtree:add_le(f_seq_number,    buffer(offset, 2)); offset = offset + 2

    -- Only parse payload if Data packet
    local pkt_type = buffer(0, 1):uint()
    if pkt_type ~= 1 then return end

    if buffer:len() <= offset + 2 then return end
    local payload_tree = subtree:add(arpc_proto, buffer(offset), "aRPC Message")

    -- Service
    local service_len = buffer(offset, 2):le_uint()
    payload_tree:add_le(f_service_len, buffer(offset, 2))
    offset = offset + 2
    payload_tree:add(f_service, buffer(offset, service_len))
    offset = offset + service_len

    -- Method
    local method_len = buffer(offset, 2):le_uint()
    payload_tree:add_le(f_method_len, buffer(offset, 2))
    offset = offset + 2
    payload_tree:add(f_method, buffer(offset, method_len))
    offset = offset + method_len

    -- Headers
    local header_len = buffer(offset, 2):le_uint()
    payload_tree:add_le(f_header_len, buffer(offset, 2))
    offset = offset + 2

    local headers_buf = buffer(offset, header_len)
    local headers_tree = payload_tree:add(f_headers, headers_buf)
    local headers_offset = 0

    if header_len >= 2 then
        local count = headers_buf(headers_offset, 2):le_uint()
        headers_tree:add_le(f_header_count, headers_buf(headers_offset, 2))
        headers_offset = headers_offset + 2

        for i = 1, count do
            if headers_offset + 2 > header_len then break end
            local klen = headers_buf(headers_offset, 2):le_uint()
            headers_offset = headers_offset + 2

            if headers_offset + klen + 2 > header_len then break end
            local key = headers_buf(headers_offset, klen):string()
            headers_tree:add(f_header_key, headers_buf(headers_offset, klen))
            headers_offset = headers_offset + klen

            local vlen = headers_buf(headers_offset, 2):le_uint()
            headers_offset = headers_offset + 2

            if headers_offset + vlen > header_len then break end
            local value = headers_buf(headers_offset, vlen):string()
            headers_tree:add(f_header_value, headers_buf(headers_offset, vlen))
            headers_offset = headers_offset + vlen
        end
    end

    offset = offset + header_len

    -- Payload
    if buffer:len() > offset then
      local symphony_buf = buffer(offset)
      local symphony_tree = payload_tree:add(arpc_proto, symphony_buf, "Symphony Payload")

      local sym_offset = 0

      -- Layout Header (1 byte)
      symphony_tree:add(symphony_buf(sym_offset, 1), "Layout Header: " .. symphony_buf(sym_offset, 1):uint())
      sym_offset = sym_offset + 1

      -- Field Ordering Section
      local ordering_tree = symphony_tree:add(symphony_buf(sym_offset, 4), "Field Ordering")
      local field_order = {}
      for i = 1, 4 do
          local fid = symphony_buf(sym_offset, 1):uint()
          table.insert(field_order, fid)
          ordering_tree:add(symphony_buf(sym_offset, 1), string.format("Position %d: Field ID %d", i, fid))
          sym_offset = sym_offset + 1
      end

      -- Offset Table Section (2 var-length fields)
      local offset_table_tree = symphony_tree:add(symphony_buf(sym_offset, 10), "Offset Table")
      local var_fields = {}
      for i = 1, 2 do
          local fid = symphony_buf(sym_offset, 1):uint()
          local off = symphony_buf(sym_offset + 1, 2):le_uint()
          local len = symphony_buf(sym_offset + 3, 2):le_uint()
          local label = (fid == 3 and "Username") or (fid == 4 and "Content") or ("Field " .. fid)
          table.insert(var_fields, {fid=fid, offset=off, len=len})
          offset_table_tree:add(symphony_buf(sym_offset, 5), string.format("%s → offset=%d len=%d", label, off, len))
          sym_offset = sym_offset + 5
      end

      -- Data region starts at sym_offset
      local data_region = symphony_buf(sym_offset):tvb()
      local data_tree = symphony_tree:add(symphony_buf(sym_offset), "Data Region")

      local fixed_fields = {
          [1] = {name="Id", size=4},
          [2] = {name="Score", size=4}
      }

      local cursor = 0
      for _, fid in ipairs(field_order) do
          local meta = fixed_fields[fid]
          if meta then
              data_tree:add(data_region(cursor, meta.size), meta.name .. ": " .. data_region(cursor, meta.size):le_uint())
              cursor = cursor + meta.size
          else
              for _, entry in ipairs(var_fields) do
                  if entry.fid == fid then
                      local label = (fid == 3 and "Username") or (fid == 4 and "Content") or ("Field " .. fid)
                      data_tree:add(data_region(entry.offset, entry.len), string.format("%s: \"%s\"", label, data_region(entry.offset, entry.len):string()))
                  end
              end
          end
      end
    end

end

-- Register to UDP port 9090
local udp_port = DissectorTable.get("udp.port")
udp_port:add(9000, arpc_proto)
