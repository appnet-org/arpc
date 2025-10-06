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
