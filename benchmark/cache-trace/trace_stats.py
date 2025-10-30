# Computes and saves log-scale CDFs of key/value/total sizes from SET ops in a large cache trace CSV, processed in chunks.
import pandas as pd
import matplotlib.pyplot as plt
import numpy as np

TRACE_FILE = "kvcache_traces_1.csv"
PLOT_FILE = "set_size_cdf.png"
MAX_ROWS = 5_000_000  # limit processed rows

def main():
    key_sizes = []
    value_sizes = []

    chunksize = 10**5  # process in 100k-row chunks
    total = 0

    for chunk in pd.read_csv(
        TRACE_FILE,
        usecols=["key_size", "op", "size"],
        chunksize=chunksize
    ):
        print(f"Processing chunk {chunk.index.start} to {chunk.index.stop}")
        set_rows = chunk[chunk["op"] == "SET"]
        key_sizes.extend(set_rows["key_size"].astype(int).tolist())
        value_sizes.extend(set_rows["size"].astype(int).tolist())
        total += len(set_rows)

        if total >= MAX_ROWS:
            print(f"Reached {MAX_ROWS} rows. Stopping.")
            break

    print(f"Collected {total} SET requests")

    key_sizes = np.array(key_sizes)
    value_sizes = np.array(value_sizes)
    total_sizes = key_sizes + value_sizes  # combined size

    # Remove top and bottom x%
    def trim_outliers(data):
        low, high = np.percentile(data, [0, 100])
        trimmed = data[(data >= low) & (data <= high)]
        print(f"Trimmed to {len(trimmed)} values (5th–95th percentile range: {low:.2f}–{high:.2f})")
        return trimmed

    key_sizes = trim_outliers(key_sizes)
    value_sizes = trim_outliers(value_sizes)
    total_sizes = trim_outliers(total_sizes)

    # Compute CDFs
    def compute_cdf(data):
        sorted_data = np.sort(data)
        yvals = np.arange(1, len(sorted_data) + 1) / len(sorted_data)
        return sorted_data, yvals

    key_x, key_y = compute_cdf(key_sizes)
    val_x, val_y = compute_cdf(value_sizes)
    tot_x, tot_y = compute_cdf(total_sizes)

    # Plot CDFs (log-scale x-axis)
    plt.figure(figsize=(8, 5))
    plt.plot(key_x, key_y, label="Key size CDF")
    plt.plot(val_x, val_y, label="Value size CDF")
    plt.plot(tot_x, tot_y, label="Key + Value size CDF", linestyle="--", linewidth=2)

    # Vertical line at 1400 bytes (e.g., MTU boundary)
    plt.axvline(x=1400, color="red", linestyle="--", linewidth=1, label="1400 bytes")

    plt.xscale("log")
    plt.xlabel("Size (bytes, log scale)")
    plt.ylabel("CDF")
    plt.title("CDF of Key, Value, and Total Sizes")
    plt.legend(loc="upper left") 
    plt.grid(True, which="both", linestyle="--", linewidth=0.5)
    plt.tight_layout()
    plt.savefig(PLOT_FILE, dpi=200)
    plt.close()
    print(f"Saved log-scale CDF plot with 1400-byte marker to {PLOT_FILE}")

if __name__ == "__main__":
    main()
