#!/usr/bin/env python3
"""
Fragmentation Probability Plotter for Meta KV Trace

This script computes and plots the probability of fragmentation based on
expansion size for multi-packet messages.

For a given expansion size X, the probability of fragmentation is the
percentage of packets where Slack Space < X. This means if we add X bytes
to the message, it would require an additional packet.

Usage:
    python plot_fragmentation_probability.py

Prerequisites:
    - Python 3.x
    - matplotlib
    - numpy
    - zstandard (for reading .zst files)

Input Files:
    - kvcache_traces_1.csv.zst: Compressed CSV trace data

Output:
    - fragmentation_probability.pdf: A PDF file containing the probability plot
"""
import matplotlib.pyplot as plt
import numpy as np
import matplotlib
import csv
import zstandard as zstd
import io

# --- Global Style Settings ---
matplotlib.rcParams['pdf.fonttype'] = 42
matplotlib.rcParams['ps.fonttype'] = 42
matplotlib.rcParams.update({'font.size': 14})

TRACE_FILE = "kvcache_traces_1.csv.zst"
OUTPUT_FILE = "fragmentation_probability.pdf"
MTU = 1500  # Standard Ethernet MTU in bytes
MAX_LINES = 1000000  # Maximum number of lines to process

# Expansion use cases to annotate
ANNOTATIONS = [
    {"name": "GPS", "size": 16, "color": "#e41a1c"},
    {"name": "Tracing ID", "size": 55, "color": "#377eb8"},
    {"name": "JWT", "size": 500, "color": "#4daf4a"},
]


def load_slack_space(filename, mtu=MTU, max_lines=MAX_LINES):
    """Load trace data and compute slack space for each key-value pair.
    
    Slack space = (key_size + value_size) % MTU
    
    Only includes key-value pairs that exceed a single MTU (require fragmentation).
    
    Args:
        filename: Path to the compressed CSV file
        mtu: MTU size in bytes
        max_lines: Maximum number of lines to process (None for no limit)
    
    Returns:
        numpy array of slack space values
    """
    slack_values = []
    total = 0
    filtered = 0
    
    # Open and decompress the zstd file
    dctx = zstd.ZstdDecompressor()
    with open(filename, "rb") as f:
        with dctx.stream_reader(f) as reader:
            text_stream = io.TextIOWrapper(reader, encoding='utf-8')
            csv_reader = csv.DictReader(text_stream)
            
            for row in csv_reader:
                key_size = int(row['key_size'])
                value_size = int(row['size'])
                total_size = key_size + value_size
                
                total += 1
                
                # Filter out data points that fit in a single MTU
                if total_size <= mtu:
                    filtered += 1
                    continue
                
                # Calculate slack space (remainder after dividing by MTU)
                slack = total_size % mtu
                slack_values.append(slack)
                
                if total % 10000 == 0:
                    print(f"Processed {total} requests...", flush=True)
                
                # Stop if we've reached the max lines limit
                if max_lines is not None and total >= max_lines:
                    print(f"Reached max_lines limit ({max_lines}), stopping.", flush=True)
                    break
    
    print(f"Loaded {total} key-value pairs")
    print(f"Filtered out {filtered} pairs that fit in a single MTU ({100*filtered/total:.2f}%)")
    print(f"Remaining {len(slack_values)} pairs requiring fragmentation")
    return np.array(slack_values)


def compute_fragmentation_probability(slack_data, expansion_sizes):
    """
    Compute the probability of fragmentation for each expansion size.
    
    For a given expansion size X, the probability of fragmentation is the
    percentage of packets where Slack Space < X.
    
    Args:
        slack_data: numpy array of slack space values
        expansion_sizes: array of expansion sizes to evaluate
    
    Returns:
        numpy array of fragmentation probabilities (0-100%)
    """
    probabilities = []
    n = len(slack_data)
    
    for size in expansion_sizes:
        # Count packets where slack < expansion size (would cause fragmentation)
        fragmented = np.sum(slack_data < size)
        probability = 100.0 * fragmented / n
        probabilities.append(probability)
    
    return np.array(probabilities)


def plot_fragmentation_probability(slack_data, output_filename=OUTPUT_FILE, mtu=MTU):
    """
    Plots the probability of fragmentation based on expansion size.
    
    Args:
        slack_data: numpy array of slack space values
        output_filename: output PDF filename
        mtu: MTU value (for x-axis range)
    """
    # Setup Figure
    fig, ax = plt.subplots(1, 1, figsize=(6, 3.5))
    
    # Main curve color
    curve_color = '#4878d0'
    
    # Compute probabilities for all expansion sizes from 0 to MTU
    expansion_sizes = np.arange(0, mtu + 1)
    probabilities = compute_fragmentation_probability(slack_data, expansion_sizes)
    
    # Plot the main curve
    ax.plot(expansion_sizes, probabilities, 
            color=curve_color, 
            linestyle='-', 
            linewidth=2.5,
            label='Fragmentation Probability')

    # Add vertical dashed lines for annotations
    for annotation in ANNOTATIONS:
        size = annotation["size"]
        name = annotation["name"]
        color = annotation["color"]
        
        # Get the probability at this expansion size
        prob = probabilities[size]
        
        # Draw vertical dashed line
        ax.axvline(x=size, color=color, linestyle='--', linewidth=1.5, alpha=0.8)
        
        # Add label with name and probability
        # Position the label to avoid overlap
        y_offset = 5 if size < 300 else -10
        ax.annotate(f'{name}\n({size}B, {prob:.1f}%)', 
                    xy=(size, prob),
                    xytext=(size + 30, prob + y_offset),
                    fontsize=10,
                    color=color,
                    ha='left',
                    arrowprops=dict(arrowstyle='->', color=color, lw=1))

    # Styling
    ax.set_ylabel('Probability of Fragmentation (%)')
    ax.set_xlabel('Expansion Size (Bytes)', fontsize=14)
    
    # Set axis limits
    ax.set_xlim(0, mtu)
    ax.set_ylim(0, 100)
    
    # Set y-axis ticks
    ax.set_yticks([0, 20, 40, 60, 80, 100])
    
    ax.grid(True, which="major", ls="-", alpha=0.3)

    plt.tight_layout()

    print(f"Saving plot to {output_filename}...")
    plt.savefig(output_filename, bbox_inches='tight')
    plt.close()
    print(f"Saved fragmentation probability plot to {output_filename}")
    
    # Print probabilities for annotated expansion sizes
    print("\nFragmentation probabilities for annotated expansion sizes:")
    for annotation in ANNOTATIONS:
        size = annotation["size"]
        name = annotation["name"]
        prob = probabilities[size]
        print(f"  {name} ({size} bytes): {prob:.2f}%")


def main():
    print(f"Loading trace data from {TRACE_FILE}...")
    slack_data = load_slack_space(TRACE_FILE, MTU)
    
    if len(slack_data) > 0:
        print(f"\nSlack Space Statistics (MTU={MTU}):")
        print(f"  Min: {slack_data.min()} bytes")
        print(f"  Max: {slack_data.max()} bytes")
        print(f"  Mean: {slack_data.mean():.2f} bytes")
        print(f"  Median: {np.median(slack_data):.2f} bytes")
        
        plot_fragmentation_probability(slack_data, OUTPUT_FILE, MTU)
    else:
        print("Error: No data found in trace file.")


if __name__ == "__main__":
    main()
