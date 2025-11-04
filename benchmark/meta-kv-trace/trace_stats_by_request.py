# Computes and saves log-scale CDF of byte size per request from trace_large.req
# For GET operations: uses value_size only
# For SET operations: uses key_size + value_size
import matplotlib.pyplot as plt
import numpy as np
from urllib.parse import urlparse, parse_qs

TRACE_FILE = "trace.req"
PLOT_FILE = "request_size_cdf.png"

def parse_line(line):
    """Parse a line from trace_large.req and extract op, key_size, value_size."""
    line = line.strip()
    if not line:
        return None
    
    # Parse the URL query string
    parsed = urlparse(line)
    params = parse_qs(parsed.query)
    
    op = params.get("op", [None])[0]
    key_size = int(params.get("key_size", ["0"])[0])
    value_size = int(params.get("value_size", ["0"])[0])
    
    return op, key_size, value_size

def main():
    request_sizes = []
    total = 0

    with open(TRACE_FILE, "r") as f:
        for line in f:
            result = parse_line(line)
            if result is None:
                continue
            
            op, key_size, value_size = result
            
            if op == "GET":
                # For GET, only value size matters
                request_sizes.append(value_size)
            elif op == "SET":
                # For SET, take the sum of key_size + value_size
                request_sizes.append(key_size + value_size)
            else:
                # Skip unknown operations
                continue
            
            total += 1
            
            if total % 100000 == 0:
                print(f"Processed {total} requests...")

    print(f"Collected {total} requests")
    
    request_sizes = np.array(request_sizes)
    print(f"Size statistics: min={request_sizes.min()}, max={request_sizes.max()}, mean={request_sizes.mean():.2f}, median={np.median(request_sizes):.2f}")

    # Remove top and bottom x%
    def trim_outliers(data):
        low, high = np.percentile(data, [0, 100])
        trimmed = data[(data >= low) & (data <= high)]
        print(f"Trimmed to {len(trimmed)} values (range: {low:.2f}-{high:.2f})")
        return trimmed

    request_sizes = trim_outliers(request_sizes)

    # Compute CDF
    def compute_cdf(data):
        sorted_data = np.sort(data)
        yvals = np.arange(1, len(sorted_data) + 1) / len(sorted_data)
        return sorted_data, yvals

    x, y = compute_cdf(request_sizes)

    # Plot CDF (log-scale x-axis)
    plt.figure(figsize=(8, 5))
    plt.plot(x, y, label="Message size CDF", linewidth=2)

    # Vertical line at 1400 bytes (e.g., MTU boundary)
    plt.axvline(x=1400, color="red", linestyle="--", linewidth=1, label="MTU")

    plt.xscale("log")
    plt.xlabel("Size (bytes, log scale)")
    plt.ylabel("CDF")
    plt.title("CDF of Byte Size Per Request")
    plt.legend(loc="upper left") 
    plt.grid(True, which="both", linestyle="--", linewidth=0.5)
    plt.tight_layout()
    plt.savefig(PLOT_FILE, dpi=200)
    plt.close()
    print(f"Saved log-scale CDF plot with MTU marker to {PLOT_FILE}")

if __name__ == "__main__":
    main()
