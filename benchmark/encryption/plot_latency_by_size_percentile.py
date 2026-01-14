#!/usr/bin/env python3
"""
Encryption Latency by Message Size Percentile

This script shows latency values for messages at different size percentiles.
Instead of showing P50/P95/P99 of latency distribution, it shows:
- What is the average latency for messages at the P50 size?
- What is the average latency for messages at the P95 size?
- etc.

This helps understand how latency scales with message size by looking at
specific size percentiles from the trace distribution.

Usage:
    python plot_latency_by_size_percentile.py

Input Files:
    CSV timing data files in 'profile_data/' directory with format:
        latency_ns,message_size
        1234,256
        ...

Output:
    - latency_by_size_percentile.pdf: Table showing latency at each size percentile
"""
import matplotlib.pyplot as plt
import numpy as np
import matplotlib
import os

# --- Global Style Settings ---
matplotlib.rcParams['pdf.fonttype'] = 42
matplotlib.rcParams['ps.fonttype'] = 42
matplotlib.rcParams.update({'font.size': 12})

PROFILE_DATA_DIR = "profile_data"
OUTPUT_FILE = "latency_by_size_percentile.pdf"

# Format labels and file prefixes
FORMATS = {
    "Whole": "encryption_whole",
    "Random Split": "encryption_random_split",
    "Equal Split": "encryption_equal_split",
    "Key-Value Split": "encryption_key_value_split",
}

# Size percentiles to analyze
SIZE_PERCENTILES = [50, 75, 95, 99]
SIZE_PERCENTILE_LABELS = ['P50', 'P75', 'P95', 'P99']

# Bin width as percentage of size (messages within ±BIN_WIDTH% are grouped)
BIN_WIDTH_PERCENT = 5


def load_timing_data(filename):
    """Load timing data with message sizes from a CSV file.
    
    Returns:
        tuple: (latencies, sizes) as numpy arrays
    """
    filepath = os.path.join(PROFILE_DATA_DIR, filename)
    latencies = []
    sizes = []
    
    with open(filepath, "r") as f:
        header = f.readline()  # Skip CSV header
        for line in f:
            line = line.strip()
            if line:
                try:
                    parts = line.split(',')
                    latency_ns = int(parts[0])
                    message_size = int(parts[1])
                    latencies.append(latency_ns)
                    sizes.append(message_size)
                except (ValueError, IndexError):
                    continue
    
    return np.array(latencies), np.array(sizes)


def compute_latency_at_size_percentiles(latencies, sizes, percentiles=SIZE_PERCENTILES):
    """Compute mean latency for messages at each size percentile.
    
    For each size percentile, we find messages within a small bin around that size
    and compute their mean latency.
    
    Returns:
        dict: {percentile: {'size': size_value, 'latency_mean': mean, 'latency_median': median, 'count': n}}
    """
    results = {}
    
    for p in percentiles:
        # Get the size value at this percentile
        size_at_p = np.percentile(sizes, p)
        
        # Find messages within ±BIN_WIDTH_PERCENT of this size
        bin_width = max(size_at_p * BIN_WIDTH_PERCENT / 100, 1)  # At least 1 byte
        lower = size_at_p - bin_width
        upper = size_at_p + bin_width
        
        mask = (sizes >= lower) & (sizes <= upper)
        count = mask.sum()
        
        if count > 0:
            latencies_in_bin = latencies[mask]
            results[p] = {
                'size': size_at_p,
                'latency_mean': latencies_in_bin.mean(),
                'latency_median': np.median(latencies_in_bin),
                'latency_std': latencies_in_bin.std(),
                'count': count,
            }
        else:
            # Fallback: use exact percentile match if bin is empty
            # Find the closest message to the target size
            closest_idx = np.argmin(np.abs(sizes - size_at_p))
            results[p] = {
                'size': sizes[closest_idx],
                'latency_mean': latencies[closest_idx],
                'latency_median': latencies[closest_idx],
                'latency_std': 0,
                'count': 1,
            }
    
    return results


def plot_percentile_table(encrypt_data, decrypt_data, output_filename=OUTPUT_FILE):
    """
    Create a table figure showing latency at each size percentile.
    """
    fig, ax = plt.subplots(figsize=(12, 4))
    ax.axis('off')
    
    strategies = list(FORMATS.keys())
    num_percs = len(SIZE_PERCENTILES)
    
    # Build table data
    # Header row: Strategy | P50 (size) | P75 (size) | ... for Encrypt, then Decrypt
    # First, compute size percentiles from any available data to show in header
    all_sizes = []
    for data_dict in [encrypt_data, decrypt_data]:
        for strategy, (latencies, sizes) in data_dict.items():
            all_sizes.extend(sizes)
    all_sizes = np.array(all_sizes) if all_sizes else np.array([0])
    
    # Create column labels with size values
    col_labels = ['Strategy']
    for p in SIZE_PERCENTILES:
        size_val = np.percentile(all_sizes, p) if len(all_sizes) > 1 else 0
        col_labels.append(f'P{p}\n({int(size_val):,}B)')
    for p in SIZE_PERCENTILES:
        size_val = np.percentile(all_sizes, p) if len(all_sizes) > 1 else 0
        col_labels.append(f'P{p}\n({int(size_val):,}B)')
    
    num_cols = len(col_labels)
    
    # Data rows
    table_data = []
    for strategy in strategies:
        row = [strategy]
        
        # Encrypt latencies at size percentiles
        if strategy in encrypt_data:
            latencies, sizes = encrypt_data[strategy]
            percs = compute_latency_at_size_percentiles(latencies, sizes)
            row.extend([f"{percs[p]['latency_mean']:,.0f}" for p in SIZE_PERCENTILES])
        else:
            row.extend(['-'] * num_percs)
        
        # Decrypt latencies at size percentiles
        if strategy in decrypt_data:
            latencies, sizes = decrypt_data[strategy]
            percs = compute_latency_at_size_percentiles(latencies, sizes)
            row.extend([f"{percs[p]['latency_mean']:,.0f}" for p in SIZE_PERCENTILES])
        else:
            row.extend(['-'] * num_percs)
        
        table_data.append(row)
    
    # Create table - dynamic column widths
    strategy_width = 0.14
    perc_width = (1.0 - strategy_width) / (num_percs * 2)
    col_widths = [strategy_width] + [perc_width] * (num_percs * 2)
    
    table = ax.table(
        cellText=table_data,
        colLabels=col_labels,
        cellLoc='center',
        loc='center',
        colWidths=col_widths
    )
    
    # Style the table
    table.auto_set_font_size(False)
    table.set_fontsize(10)
    table.scale(1.2, 2.0)
    
    # Color header cells
    header_color = '#4878d0'
    encrypt_color = '#e8f4e8'  # Light green
    decrypt_color = '#e8e8f4'  # Light blue
    
    for j in range(num_cols):
        cell = table._cells[(0, j)]
        cell.set_facecolor(header_color)
        cell.set_text_props(color='white', fontweight='bold')
    
    # Color data cells
    for i in range(len(strategies)):
        row_idx = i + 1
        # Strategy column
        table._cells[(row_idx, 0)].set_facecolor('#f5f5f5')
        table._cells[(row_idx, 0)].set_text_props(fontweight='bold')
        # Encrypt columns
        for j in range(1, 1 + num_percs):
            table._cells[(row_idx, j)].set_facecolor(encrypt_color)
        # Decrypt columns
        for j in range(1 + num_percs, num_cols):
            table._cells[(row_idx, j)].set_facecolor(decrypt_color)
    
    # Add column group headers
    encrypt_center = strategy_width + (num_percs * perc_width) / 2
    decrypt_center = strategy_width + num_percs * perc_width + (num_percs * perc_width) / 2
    ax.text(encrypt_center, 0.95, 'Encrypt Latency (ns)', ha='center', va='bottom', 
            fontsize=12, fontweight='bold', transform=ax.transAxes)
    ax.text(decrypt_center, 0.95, 'Decrypt Latency (ns)', ha='center', va='bottom',
            fontsize=12, fontweight='bold', transform=ax.transAxes)
    
    # Add title
    ax.set_title('Mean Latency at Message Size Percentiles', fontsize=14, fontweight='bold', pad=30)
    
    # Add subtitle explaining the table
    ax.text(0.5, -0.05, 'Column headers show size percentile and corresponding message size in bytes',
            ha='center', va='top', fontsize=10, style='italic', transform=ax.transAxes)
    
    plt.tight_layout()
    
    print(f"Saving percentile table to {output_filename}...")
    plt.savefig(output_filename, bbox_inches='tight', dpi=150)
    plt.close()
    print(f"Saved percentile table to {output_filename}")


def print_percentile_table(encrypt_data, decrypt_data):
    """Print a formatted table of latency at size percentiles."""
    strategies = list(FORMATS.keys())
    
    # Compute global size percentiles for header
    all_sizes = []
    for data_dict in [encrypt_data, decrypt_data]:
        for strategy, (latencies, sizes) in data_dict.items():
            all_sizes.extend(sizes)
    all_sizes = np.array(all_sizes) if all_sizes else np.array([0])
    
    print("\n" + "="*90)
    print("LATENCY AT MESSAGE SIZE PERCENTILES (nanoseconds)")
    print("="*90)
    print("\nThis table shows the mean latency for messages at each size percentile.")
    print("For example, P95 column shows latency for messages at the 95th percentile of size.\n")
    
    # Print size reference
    print("Size Percentile Reference:")
    print("-" * 50)
    for p in SIZE_PERCENTILES:
        size_val = np.percentile(all_sizes, p) if len(all_sizes) > 1 else 0
        print(f"  P{p}: {int(size_val):,} bytes")
    print()
    
    # Build header
    header = f"{'Strategy':<20}" + "".join(f"{'P'+str(p):>14}" for p in SIZE_PERCENTILES)
    separator = "-" * (20 + 14 * len(SIZE_PERCENTILES))
    
    for op_name, data_dict in [("ENCRYPT", encrypt_data), ("DECRYPT", decrypt_data)]:
        print(f"\n{op_name}:")
        print(separator)
        print(header)
        print(separator)
        
        for strategy in strategies:
            if strategy in data_dict:
                latencies, sizes = data_dict[strategy]
                percs = compute_latency_at_size_percentiles(latencies, sizes)
                row = f"{strategy:<20}" + "".join(f"{percs[p]['latency_mean']:>14,.0f}" for p in SIZE_PERCENTILES)
                print(row)
        print(separator)
    
    print()


def main():
    # Load encrypt data
    encrypt_data = {}
    for label, prefix in FORMATS.items():
        filename = f"{prefix}_encrypt_times.csv"
        try:
            latencies, sizes = load_timing_data(filename)
            if len(latencies) > 0:
                encrypt_data[label] = (latencies, sizes)
                print(f"Loaded {len(latencies)} encrypt samples for {label}")
        except FileNotFoundError:
            print(f"Warning: {filename} not found, skipping...")
    
    # Load decrypt data
    decrypt_data = {}
    for label, prefix in FORMATS.items():
        filename = f"{prefix}_decrypt_times.csv"
        try:
            latencies, sizes = load_timing_data(filename)
            if len(latencies) > 0:
                decrypt_data[label] = (latencies, sizes)
                print(f"Loaded {len(latencies)} decrypt samples for {label}")
        except FileNotFoundError:
            print(f"Warning: {filename} not found, skipping...")
    
    if not encrypt_data and not decrypt_data:
        print("Error: No timing data found. Please run benchmarks first.")
        return
    
    # Print summary table
    print_percentile_table(encrypt_data, decrypt_data)
    
    # Generate table figure
    plot_percentile_table(encrypt_data, decrypt_data)


if __name__ == "__main__":
    main()

