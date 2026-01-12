#!/usr/bin/env python3
"""
Encryption Benchmark Latency CDF Plotter

This script computes and plots Cumulative Distribution Functions (CDFs) of latency
for encrypt and decrypt operations across different encryption strategies.

Supported Strategies:
    - Whole: Encrypt/decrypt entire message at once
    - Random Split: Split message at random boundaries
    - Key-Value Split: Split message at key/value boundary

Usage:
    python plot_encryption_cdf.py

Prerequisites:
    - Python 3.x
    - matplotlib
    - numpy

Input Files:
    The script expects timing data files in the 'profile_data/' directory:
        - encryption_whole_encrypt_times.txt
        - encryption_whole_decrypt_times.txt
        - encryption_random_split_encrypt_times.txt
        - encryption_random_split_decrypt_times.txt
        - encryption_key_value_split_encrypt_times.txt
        - encryption_key_value_split_decrypt_times.txt

    Each file should contain one timing value (in nanoseconds) per line.

Output:
    - encryption_latency_cdf.pdf: A PDF file containing side-by-side CDF plots
      for encrypt and decrypt latencies with a shared legend at the bottom.
"""
import matplotlib.pyplot as plt
import numpy as np
import matplotlib
import os

# --- Global Style Settings ---
matplotlib.rcParams['pdf.fonttype'] = 42
matplotlib.rcParams['ps.fonttype'] = 42
matplotlib.rcParams.update({'font.size': 14})

PROFILE_DATA_DIR = "profile_data"
OUTPUT_FILE = "encryption_latency_cdf.pdf"

# Format labels and file prefixes (order matters for legend)
FORMATS = {
    "Whole": "encryption_whole",
    "Random Split": "encryption_random_split",
    "Key-Value Split": "encryption_key_value_split",
}

def load_timings(filename):
    """Load timing data from a file in nanoseconds."""
    filepath = os.path.join(PROFILE_DATA_DIR, filename)
    timings = []
    
    with open(filepath, "r") as f:
        for line in f:
            line = line.strip()
            if line:
                try:
                    ns = int(line)
                    timings.append(ns)
                except ValueError:
                    continue
    
    return np.array(timings)

def plot_merged_latency_cdfs(data_left, data_right, 
                             x_labels=('Encrypt Latency (ns)', 'Decrypt Latency (ns)'), 
                             output_filename="latency_cdf.pdf", 
                             system_order=None):
    """
    Plots two CDFs side-by-side with shared legend at bottom.
    Titles are removed; X-axis labels differentiate the plots.
    """
    
    # 1. Setup Figure (1 row, 2 columns)
    fig, axes = plt.subplots(1, 2, figsize=(8, 3))
    
    # Standard SIGCOMM Color Palette & Styles
    colors = ['#6acc64', '#4878d0', '#e6a04e', '#d65f5f', '#82c6e2'] 
    linestyles = ['-', '--', '-.', ':', '-']
    
    if system_order is None:
        system_order = list(data_left.keys())

    datasets = [data_left, data_right]

    # 2. Loop through both subplots
    for idx, ax in enumerate(axes):
        data_dict = datasets[idx]
        
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
        
        # Y-label only on the left plot
        ax.set_ylabel('CDF (%)' if idx == 0 else "") 
        
        # X-LABELS CUSTOMIZED
        ax.set_xlabel(x_labels[idx], fontsize=14)
        
        ax.set_xscale('log')
        ax.grid(True, which="major", ls="-", alpha=0.3)

    # 4. Shared Legend at Bottom
    handles, labels = axes[0].get_legend_handles_labels()
    
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

    print(f"Saving merged plot to {output_filename}...")
    plt.savefig(output_filename, bbox_inches='tight')
    plt.close()
    print(f"Saved merged plot to {output_filename}")

def main():
    # Load encrypt timings
    encrypt_timings = {}
    for label, prefix in FORMATS.items():
        filename = f"{prefix}_encrypt_times.txt"
        print(f"Loading {filename}...")
        try:
            timings = load_timings(filename)
            if len(timings) > 0:
                encrypt_timings[label] = timings
                print(f"  Loaded {len(timings)} samples")
                print(f"  Statistics: min={timings.min():.2f}ns, max={timings.max():.2f}ns, "
                      f"mean={timings.mean():.2f}ns, median={np.median(timings):.2f}ns")
        except FileNotFoundError:
            print(f"  Warning: {filename} not found, skipping...")
    
    # Load decrypt timings
    decrypt_timings = {}
    for label, prefix in FORMATS.items():
        filename = f"{prefix}_decrypt_times.txt"
        print(f"Loading {filename}...")
        try:
            timings = load_timings(filename)
            if len(timings) > 0:
                decrypt_timings[label] = timings
                print(f"  Loaded {len(timings)} samples")
                print(f"  Statistics: min={timings.min():.2f}ns, max={timings.max():.2f}ns, "
                      f"mean={timings.mean():.2f}ns, median={np.median(timings):.2f}ns")
        except FileNotFoundError:
            print(f"  Warning: {filename} not found, skipping...")
    
    # Plot merged CDFs
    if encrypt_timings and decrypt_timings:
        system_order = list(FORMATS.keys())
        plot_merged_latency_cdfs(encrypt_timings, decrypt_timings,
                                x_labels=('Encrypt Latency (ns)', 'Decrypt Latency (ns)'),
                                output_filename=OUTPUT_FILE,
                                system_order=system_order)
    else:
        print("Error: No timing data found. Please run benchmarks first to generate timing data files.")

if __name__ == "__main__":
    main()

