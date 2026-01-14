#!/usr/bin/env python3
"""
Encryption Benchmark Percentile Bar Chart

This script creates bar charts showing key percentiles (P50, P95, P99) 
for encrypt and decrypt operations across different encryption strategies.

Usage:
    python plot_encryption_percentiles.py

Input Files:
    CSV timing data files in 'profile_data/' directory with format:
        latency_ns,message_size
        1234,256
        ...

Output:
    - encryption_latency_percentiles.pdf: Bar chart comparing percentiles
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
OUTPUT_FILE = "encryption_latency_percentiles.pdf"

# Format labels and file prefixes
FORMATS = {
    "Whole": "encryption_whole",
    "Random Split": "encryption_random_split",
    "Equal Split": "encryption_equal_split",
    "Key-Value Split": "encryption_key_value_split",
}

# Percentiles to display
PERCENTILES = [50, 75, 95, 99]
PERCENTILE_LABELS = ['P50', 'P75', 'P95', 'P99']


def load_timings(filename):
    """Load timing data from a CSV file in nanoseconds.
    
    Expected CSV format:
        latency_ns,message_size
        1234,256
        5678,512
        ...
    
    Returns only the latency values (message_size is ignored).
    """
    filepath = os.path.join(PROFILE_DATA_DIR, filename)
    timings = []
    
    with open(filepath, "r") as f:
        header = f.readline()  # Skip CSV header
        for line in f:
            line = line.strip()
            if line:
                try:
                    # CSV format: latency_ns,message_size
                    parts = line.split(',')
                    ns = int(parts[0])
                    timings.append(ns)
                except (ValueError, IndexError):
                    continue
    
    return np.array(timings)


def compute_percentiles(timings, percentiles=PERCENTILES):
    """Compute specified percentiles from timing data."""
    return {p: np.percentile(timings, p) for p in percentiles}


def plot_percentile_table(encrypt_data, decrypt_data, output_filename=OUTPUT_FILE):
    """
    Create a clean table figure showing percentiles for each strategy.
    """
    fig, ax = plt.subplots(figsize=(10, 3))
    ax.axis('off')
    
    strategies = list(FORMATS.keys())
    
    # Build table data
    # Header row - dynamically build based on PERCENTILE_LABELS
    col_labels = ['Strategy'] + PERCENTILE_LABELS + PERCENTILE_LABELS
    num_cols = len(col_labels)
    num_percs = len(PERCENTILES)
    
    # Data rows
    table_data = []
    for strategy in strategies:
        row = [strategy]
        # Encrypt percentiles
        if strategy in encrypt_data:
            percs = compute_percentiles(encrypt_data[strategy])
            row.extend([f"{percs[p]:,.0f}" for p in PERCENTILES])
        else:
            row.extend(['-'] * num_percs)
        # Decrypt percentiles
        if strategy in decrypt_data:
            percs = compute_percentiles(decrypt_data[strategy])
            row.extend([f"{percs[p]:,.0f}" for p in PERCENTILES])
        else:
            row.extend(['-'] * num_percs)
        table_data.append(row)
    
    # Create table - dynamic column widths
    strategy_width = 0.18
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
    table.set_fontsize(11)
    table.scale(1.2, 1.8)
    
    # Color header cells
    header_color = '#4878d0'
    encrypt_color = '#e8f4e8'  # Light green
    decrypt_color = '#e8e8f4'  # Light blue
    
    for j in range(num_cols):
        cell = table._cells[(0, j)]
        cell.set_facecolor(header_color)
        cell.set_text_props(color='white', fontweight='bold')
    
    # Color data cells alternating by row
    for i in range(len(strategies)):
        row_idx = i + 1
        # Strategy column
        table._cells[(row_idx, 0)].set_facecolor('#f5f5f5')
        table._cells[(row_idx, 0)].set_text_props(fontweight='bold')
        # Encrypt columns (1 to num_percs)
        for j in range(1, 1 + num_percs):
            table._cells[(row_idx, j)].set_facecolor(encrypt_color)
        # Decrypt columns (1 + num_percs to end)
        for j in range(1 + num_percs, num_cols):
            table._cells[(row_idx, j)].set_facecolor(decrypt_color)
    
    # Add column group headers - position based on number of percentiles
    # Encrypt header centered over encrypt columns, decrypt over decrypt columns
    encrypt_center = strategy_width + (num_percs * perc_width) / 2
    decrypt_center = strategy_width + num_percs * perc_width + (num_percs * perc_width) / 2
    ax.text(encrypt_center, 0.92, 'Encrypt (ns)', ha='center', va='bottom', 
            fontsize=12, fontweight='bold', transform=ax.transAxes)
    ax.text(decrypt_center, 0.92, 'Decrypt (ns)', ha='center', va='bottom',
            fontsize=12, fontweight='bold', transform=ax.transAxes)
    
    # Add title
    ax.set_title('Encryption Latency Percentiles', fontsize=14, fontweight='bold', pad=20)
    
    plt.tight_layout()
    
    print(f"Saving percentile table to {output_filename}...")
    plt.savefig(output_filename, bbox_inches='tight', dpi=150)
    plt.close()
    print(f"Saved percentile table to {output_filename}")


def print_percentile_table(encrypt_data, decrypt_data):
    """Print a formatted table of percentiles for quick reference."""
    strategies = list(FORMATS.keys())
    
    # Build header dynamically
    header = f"{'Strategy':<20}" + "".join(f"{'P'+str(p):>12}" for p in PERCENTILES)
    separator = "-" * (20 + 12 * len(PERCENTILES))
    
    print("\n" + "="*80)
    print("ENCRYPTION LATENCY PERCENTILES (nanoseconds)")
    print("="*80)
    
    for op_name, data_dict in [("ENCRYPT", encrypt_data), ("DECRYPT", decrypt_data)]:
        print(f"\n{op_name}:")
        print(separator)
        print(header)
        print(separator)
        
        for strategy in strategies:
            if strategy in data_dict:
                percs = compute_percentiles(data_dict[strategy])
                row = f"{strategy:<20}" + "".join(f"{percs[p]:>12.0f}" for p in PERCENTILES)
                print(row)
        print(separator)
    
    print()


def main():
    # Load encrypt timings
    encrypt_timings = {}
    for label, prefix in FORMATS.items():
        filename = f"{prefix}_encrypt_times.csv"
        try:
            timings = load_timings(filename)
            if len(timings) > 0:
                encrypt_timings[label] = timings
                print(f"Loaded {len(timings)} encrypt samples for {label}")
        except FileNotFoundError:
            print(f"Warning: {filename} not found, skipping...")
    
    # Load decrypt timings
    decrypt_timings = {}
    for label, prefix in FORMATS.items():
        filename = f"{prefix}_decrypt_times.csv"
        try:
            timings = load_timings(filename)
            if len(timings) > 0:
                decrypt_timings[label] = timings
                print(f"Loaded {len(timings)} decrypt samples for {label}")
        except FileNotFoundError:
            print(f"Warning: {filename} not found, skipping...")
    
    if not encrypt_timings and not decrypt_timings:
        print("Error: No timing data found. Please run benchmarks first.")
        return
    
    # Print summary table
    print_percentile_table(encrypt_timings, decrypt_timings)
    
    # Generate table figure
    plot_percentile_table(encrypt_timings, decrypt_timings)


if __name__ == "__main__":
    main()

