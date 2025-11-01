"""Utilities to extract keys with large values from kvstore traces and
write all lines for those keys to a separate trace file."""

def extract_large_value_keys(filename, threshold=1400):
    large_keys = []
    seen_keys = set()
    keys_with_set = set()
    with open(filename, 'r') as f:
        for line in f:
            line = line.strip()
            if not line or not line.startswith("/?op="):
                continue

            # Extract fields
            parts = line.split("&")
            kv = {}
            for part in parts:
                if "=" in part:
                    key, value = part.split("=", 1)
                    kv[key] = value

            # Track keys that have been SET
            op = kv.get("/?op") or kv.get("op")
            key = kv.get("key")
            if op == "SET" and key is not None:
                keys_with_set.add(key)

            # Check large value condition only if we've seen a SET for this key
            try:
                value_size = int(kv.get("value_size", 0))
                if value_size > threshold and key is not None and key in keys_with_set:
                    if key not in seen_keys:
                        seen_keys.add(key)
                        large_keys.append(key)
            except ValueError:
                continue  # skip malformed lines

    return large_keys


def write_lines_with_keys(input_filename, output_filename, keys):
    keys_set = set(keys)
    with open(input_filename, 'r') as fin, open(output_filename, 'w') as fout:
        for line in fin:
            raw_line = line.rstrip('\n')
            line = raw_line.strip()
            if not line or not line.startswith("/?op="):
                continue

            parts = line.split("&")
            kv = {}
            for part in parts:
                if "=" in part:
                    key, value = part.split("=", 1)
                    kv[key] = value

            if kv.get("key") in keys_set:
                fout.write(raw_line + '\n')


if __name__ == "__main__":
    filename = "trace.req"
    keys = extract_large_value_keys(filename)
    print("Keys with value_size > 1400:")
    print(len(keys))
    write_lines_with_keys("trace.req", "trace_large.req", keys)
