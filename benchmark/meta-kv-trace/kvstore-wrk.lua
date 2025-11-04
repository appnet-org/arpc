-- kv_trace.lua
local lines = {}
local i = 1

-- Load the trace file once at startup
do
  local f = io.open("trace.req", "r")
  if not f then error("cannot open trace.req") end
  for line in f:lines() do
    local trimmed = line:match("^%s*(.-)%s*$")
    if trimmed ~= "" then
      table.insert(lines, trimmed)
    end
  end
  f:close()
end

print(string.format("Loaded %d requests", #lines))

-- Return one request per wrk iteration
function request()
  if i > #lines then
    wrk.thread:stop()
    return nil
  end
  print("Sending request", i)
  local line = lines[i]
  i = i + 1

  -- Always use GET for simplicity
  return wrk.format("GET", line)
end
