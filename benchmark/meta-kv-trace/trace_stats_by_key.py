# Computes and saves log-scale CDFs of key/value/total sizes from SET ops in a large cache trace CSV, processed in chunks.
import pandas as pd
import matplotlib.pyplot as plt
import matplotlib
import numpy as np

# --- Global Style Settings ---
matplotlib.rcParams['pdf.fonttype'] = 42
matplotlib.rcParams['ps.fonttype'] = 42
matplotlib.rcParams.update({'font.size': 14})

TRACE_FILE = "kvcache_traces_1.csv.zst"
PLOT_FILE = "set_size_cdf.pdf"
MAX_ROWS = 5_000_000  # limit processed rows
# MAX_ROWS = 5_000  # limit processed rows

# Standard SIGCOMM Color Palette
COLORS = ['#4878d0', '#ee854a', '#6acc64', '#d65f5f', '#956cb4']

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

    # Print percentiles for key+value sizes
    p90, p95, p99 = np.percentile(total_sizes, [90, 95, 99])
    print(f"\nKey + Value Size Statistics:")
    print(f"  Min: {total_sizes.min()} bytes")
    print(f"  Max: {total_sizes.max()} bytes")
    print(f"  P90: {p90:.0f} bytes")
    print(f"  P95: {p95:.0f} bytes")
    print(f"  P99: {p99:.0f} bytes\n")

    # # Remove top and bottom x%
    # def trim_outliers(data):
    #     low, high = np.percentile(data, [0, 100])
    #     trimmed = data[(data >= low) & (data <= high)]
    #     print(f"Trimmed to {len(trimmed)} values (5th-95th percentile range: {low:.2f}-{high:.2f})")
    #     return trimmed

    # key_sizes = trim_outliers(key_sizes)
    # value_sizes = trim_outliers(value_sizes)
    # total_sizes = trim_outliers(total_sizes)

    # Compute CDFs
    def compute_cdf(data):
        sorted_data = np.sort(data)
        yvals = np.arange(1, len(sorted_data) + 1) / len(sorted_data)
        return sorted_data, yvals

    key_x, key_y = compute_cdf(key_sizes)
    val_x, val_y = compute_cdf(value_sizes)
    tot_x, tot_y = compute_cdf(total_sizes)

    # Plot CDFs (log-scale x-axis)
    fig, ax = plt.subplots(1, 1, figsize=(5, 3))
    
    ax.plot(key_x, key_y, label="Key size", color=COLORS[0], linestyle='-', linewidth=2.5)
    ax.plot(val_x, val_y, label="Value size", color=COLORS[1], linestyle='-', linewidth=2.5)
    ax.plot(tot_x, tot_y, label="Key + Value size", color=COLORS[2], linestyle='--', linewidth=2.5)

    ax.set_xscale("log")
    ax.set_xlabel("Size (bytes)", fontsize=14)
    
    # Styling y-axis as percentage
    ax.set_yticks([0, 0.25, 0.50, 0.75, 1.0])
    ax.set_yticklabels(['0', '25', '50', '75', '100'])
    ax.set_ylabel('CDF (%)')
    
    ax.legend(loc='lower right', frameon=True)
    ax.grid(True, which="major", ls="-", alpha=0.3)
    
    plt.tight_layout()
    plt.savefig(PLOT_FILE, bbox_inches='tight')
    plt.close()
    print(f"Saved log-scale CDF plot with 1400-byte marker to {PLOT_FILE}")

if __name__ == "__main__":
    main()
