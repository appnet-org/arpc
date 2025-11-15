-- kv_trace.lua
local lines = {}
local i = 1

-- Load the trace file once at startup
do
  -- Get the directory of this script and construct relative path
  local script_path = debug.getinfo(1, "S").source:match("^@(.*)$")
  local script_dir = script_path:match("^(.*)/[^/]*$") or "."
  local trace_file = script_dir .. "/trace.req"
  
  local f = io.open(trace_file, "r")
  if not f then error("cannot open trace.req at " .. trace_file) end
  for line in f:lines() do
    local trimmed = line:match("^%s*(.-)%s*$")
    if trimmed ~= "" then
      table.insert(lines, trimmed)
    end
  end
  f:close()
end

-- print(string.format("Loaded %d requests", #lines))

-- Return one request per wrk iteration
function request()
  if i > #lines then
    wrk.thread:stop()
    return nil
  end
  -- print("Sending request", i)
  local line = lines[i]
  i = i + 1

  -- Always use GET for simplicity
  return wrk.format("GET", line)
end
