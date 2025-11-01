"""Parse a CSV trace and emit wrk-style HTTP request lines.

Reads `kvcache_traces_1.csv` in chunks and writes up to `MAX_ROWS` lines to
`trace.req`. Each output line corresponds to a GET or SET operation in the form:

  /?op=<GET|SET>&key=<key>&key_size=<bytes>&value_size=<bytes>

This format is used by the Lua wrk scripts in `benchmark/cache-trace/`.
"""

import pandas as pd

TRACE_FILE = "kvcache_traces_1.csv"
REQ_FILE = "trace.req"
# MAX_ROWS = 1_000_000   # stop after this many rows
MAX_ROWS = 500_000 

def main():
    written = 0
    with open(REQ_FILE, "w") as out:
        chunksize = 10**5  # 100k rows at a time
        for chunk in pd.read_csv(
            TRACE_FILE,
            usecols=["key", "key_size", "op", "size"],
            chunksize=chunksize
        ):
            print(f"Processing chunk {chunk.index.start} to {chunk.index.stop}")

            for _, row in chunk.iterrows():
                if written >= MAX_ROWS:
                    print(f"Reached {MAX_ROWS} rows. Stopping.")
                    return

                key, key_size, op, size = row["key"], row["key_size"], row["op"], row["size"]
                if op in ("GET", "SET"):
                    out.write(f"/?op={op}&key={key}&key_size={key_size}&value_size={size}\n")
                    written += 1

    print(f"Wrote {written} requests to {REQ_FILE}")

if __name__ == "__main__":
    main()
