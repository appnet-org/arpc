import subprocess
wrk_path = "/users/xzhu/arpc/benchmark/scripts/wrk/wrk"

for i in range(0, 51, 5): 
    
    
    # First generates the kv.lua file
    number = 1400 * i + 1
    with open("kv.lua", "w") as f:
        f.write(f'wrk.method = "GET"\n')
        f.write(f'wrk.path = "/?op=SET&key=82131353f9ddc8c6&key_size=48&value_size={number}"\n')
    
    num_of_packet = number // 1400 + 1
    print(f"Running wrk for (num_of_packet={num_of_packet})")
    # Run wrk for latency test
    cmd =[wrk_path, "-d", "60s", "-t", "1", "-c", "1", "http://10.96.88.88:80", "-s", "kv.lua", "-L"]
    result = subprocess.run(" ".join(cmd), shell=True, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE).stdout.decode("utf-8").split('\n')
    
    # Extract 50% latency and convert to milliseconds
    for line in result:
        if "50%" in line:
            # Parse the line like "     50%  330.00us"
            parts = line.strip().split()
            if len(parts) >= 2:
                latency_str = parts[1]
                # Extract number and unit
                latency_value = float(latency_str.rstrip('us').rstrip('s').rstrip('ms'))
                unit = latency_str[-2:] if len(latency_str) >= 2 else 'ms'
                
                # Convert to milliseconds
                if unit == 'us':
                    latency_ms = latency_value / 1000
                elif unit == 's':
                    latency_ms = latency_value * 1000
                else:  # already ms
                    latency_ms = latency_value
                
                
                print(f"50% Latency: {latency_ms:.2f}ms (num_of_packet={num_of_packet})")
                break