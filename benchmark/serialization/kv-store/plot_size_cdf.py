#!/usr/bin/env python3
"""
Serialization Message Size CDF Plotter

This script computes and plots Cumulative Distribution Functions (CDFs) of message sizes
for serialized data across different serialization formats.

Supported Formats:
    - FlatBuffers
    - Cap'n Proto
    - Protobuf
    - fRPC (Symphony)
    - fRPC Hybrid (optional, requires --include-hybrid flag)

Usage:
    # Plot base formats only (default)
    python plot_size_cdf.py

    # Include fRPC Hybrid in the plot
    python plot_size_cdf.py --include-hybrid

    # Show help
    python plot_size_cdf.py --help

Prerequisites:
    - Python 3.x
    - matplotlib
    - numpy

Input Files:
    The script expects size data files in the 'profile_data/' directory:
        - flatbuffers_write_sizes.txt
        - capnp_write_sizes.txt
        - protobuf_write_sizes.txt
        - symphony_write_sizes.txt
        - symphony_hybrid_write_sizes.txt (if using --include-hybrid)

    Each file should contain one size value (in bytes) per line.

Output:
    - serialization_size_cdf.pdf: A PDF file containing CDF plot
      for message sizes with a shared legend at the bottom (base formats only).
    - serialization_size_cdf_hybrid.pdf: A PDF file when --include-hybrid is used,
      containing all formats including fRPC Hybrid.

Command-line Options:
    --include-hybrid    Include fRPC Hybrid format in the plot (default: False)
    --help, -h          Show this help message and exit
"""
import argparse
import matplotlib.pyplot as plt
import numpy as np
import matplotlib
import os

# --- Global Style Settings ---
matplotlib.rcParams['pdf.fonttype'] = 42
matplotlib.rcParams['ps.fonttype'] = 42
matplotlib.rcParams.update({'font.size': 14})

PROFILE_DATA_DIR = "profile_data"
OUTPUT_FILE_BASE = "kv_store_serialization_size_cdf.pdf"
OUTPUT_FILE_HYBRID = "kv_stor_serialization_size_cdf_hybrid.pdf"

# Base format labels and file names (order matters for legend)
BASE_FORMATS = {
    "FlatBuffers": "flatbuffers",
    "Cap'n Proto": "capnp",
    "Protobuf": "protobuf",
    "fRPC": "symphony",
}

# Hybrid format
HYBRID_FORMAT = {
    "fRPC (B-Opt)": "symphony_hybrid",
}

def load_sizes(filename):
    """Load size data from a file in bytes."""
    filepath = os.path.join(PROFILE_DATA_DIR, filename)
    sizes = []
    
    with open(filepath, "r") as f:
        for line in f:
            line = line.strip()
            if line:
                try:
                    bytes_val = int(line)
                    sizes.append(bytes_val)
                except ValueError:
                    continue
    
    return np.array(sizes)

def plot_size_cdf(data_dict, 
                  x_label='Message Size (bytes)', 
                  output_filename="size_cdf.pdf", 
                  system_order=None):
    """
    Plots a CDF of message sizes with shared legend at bottom.
    """
    
    # 1. Setup Figure (single plot)
    fig, ax = plt.subplots(1, 1, figsize=(6, 4))
    
    # Standard SIGCOMM Color Palette & Styles
    colors = ['#6acc64', '#4878d0', '#82c6e2', '#e6a04e', '#d65f5f'] 
    linestyles = ['-', '--', '-.', ':', '-']
    
    if system_order is None:
        system_order = list(data_dict.keys())

    # 2. Plot each system
    for i, system in enumerate(system_order):
        if system not in data_dict:
            continue
        
        sorted_data = np.sort(data_dict[system])
        yvals = np.arange(1, len(sorted_data) + 1) / len(sorted_data)
        
        ax.plot(sorted_data, yvals, 
                 label=system, 
                 color=colors[i % len(colors)], 
                 linestyle=linestyles[i % len(linestyles)], 
                 linewidth=2.5)

    # 3. Styling
    ax.set_yticks([0, 0.25, 0.50, 0.75, 1.0])
    ax.set_yticklabels(['0', '25', '50', '75', '100'])
    
    ax.set_ylabel('CDF (%)', fontsize=14)
    ax.set_xlabel(x_label, fontsize=14)
    
    ax.set_xscale('log')
    ax.grid(True, which="major", ls="-", alpha=0.3)

    # 4. Shared Legend at Bottom
    handles, labels = ax.get_legend_handles_labels()
    
    # Adjust ncol based on number of systems
    ncol = len(labels)
    
    fig.legend(handles, labels, 
               loc='lower center', 
               bbox_to_anchor=(0.5, -0.15), 
               ncol=ncol, 
               frameon=True,
               columnspacing=1.5)

    # 5. Adjust Layout
    plt.tight_layout()
    plt.subplots_adjust(bottom=0.25) 

    print(f"Saving plot to {output_filename}...")
    plt.savefig(output_filename, bbox_inches='tight')
    plt.close()
    print(f"Saved plot to {output_filename}")

def main():
    parser = argparse.ArgumentParser(description='Plot CDF of serialization message sizes')
    parser.add_argument('--include-hybrid', action='store_true',
                        help='Include fRPC Hybrid in the plot (default: False)')
    args = parser.parse_args()
    
    # Build FORMATS dict based on flag
    FORMATS = BASE_FORMATS.copy()
    if args.include_hybrid:
        FORMATS.update(HYBRID_FORMAT)
    
    # Determine output filename based on whether hybrid is included
    output_file = OUTPUT_FILE_HYBRID if args.include_hybrid else OUTPUT_FILE_BASE
    
    # Load write sizes
    write_sizes = {}
    for label, prefix in FORMATS.items():
        filename = f"{prefix}_write_sizes.txt"
        print(f"Loading {filename}...")
        sizes = load_sizes(filename)
        if len(sizes) > 0:
            write_sizes[label] = sizes
            print(f"  Loaded {len(sizes)} samples")
            print(f"  Statistics: min={sizes.min()} bytes, max={sizes.max()} bytes, "
                  f"mean={sizes.mean():.2f} bytes, median={np.median(sizes):.2f} bytes")
    
    # Plot CDF
    if write_sizes:
        system_order = list(FORMATS.keys())
        plot_size_cdf(write_sizes,
                     x_label='Message Size (bytes)',
                     output_filename=output_file,
                     system_order=system_order)
    else:
        print("Error: No size data found. Please run benchmarks first to generate size data files.")

if __name__ == "__main__":
    main()

