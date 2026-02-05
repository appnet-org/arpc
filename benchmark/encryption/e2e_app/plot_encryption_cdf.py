#!/usr/bin/env python3
"""
E2E Application Encryption Benchmark Latency CDF Plotter

This script computes and plots Cumulative Distribution Functions (CDFs) of latency
for encrypt and decrypt operations using Online Boutique and Hotel message sizes.

Generates two PDFs:
    - e2e_encryption_latency_cdf.pdf: Encryption latency (Boutique left, Hotel right)
    - e2e_decryption_latency_cdf.pdf: Decryption latency (Boutique left, Hotel right)

Usage:
    python plot_encryption_cdf.py

Prerequisites:
    - Python 3.x
    - matplotlib
    - numpy

Input Files:
    The script expects CSV timing data files in the 'profile_data/' directory:
        - boutique_triple_encryption_whole_encrypt_times.csv
        - boutique_triple_encryption_whole_decrypt_times.csv
        - boutique_triple_encryption_fixed_split_encrypt_times.csv
        - boutique_triple_encryption_fixed_split_decrypt_times.csv
        - hotel_triple_encryption_whole_encrypt_times.csv
        - hotel_triple_encryption_whole_decrypt_times.csv
        - hotel_triple_encryption_fixed_split_encrypt_times.csv
        - hotel_triple_encryption_fixed_split_decrypt_times.csv

    CSV format:
        latency_ns,message_size
        1234,256
        ...
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
ENCRYPT_OUTPUT_FILE = "e2e_encryption_latency_cdf.pdf"
DECRYPT_OUTPUT_FILE = "e2e_decryption_latency_cdf.pdf"

# Strategy labels and file suffixes (order matters for legend)
STRATEGIES = {
    "Baseline": "whole",
    "fRPC": "fixed_split",
}

# Applications
APPLICATIONS = {
    "boutique": "Online Boutique",
    "hotel": "Hotel Reservation",
}


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


def plot_app_comparison_cdfs(data_left, data_right, 
                              x_labels=('Online Boutique', 'Hotel Reservation'),
                              y_label='CDF (%)',
                              output_filename="latency_cdf.pdf", 
                              system_order=None):
    """
    Plots two CDFs side-by-side with shared legend at bottom.
    Left: Online Boutique, Right: Hotel Reservation
    Each subplot shows different encryption strategies.
    """
    
    # 1. Setup Figure (1 row, 2 columns)
    fig, axes = plt.subplots(1, 2, figsize=(8, 3))
    
    # Standard SIGCOMM Color Palette & Styles
    colors = ['#6acc64', '#4878d0', '#e6a04e', '#d65f5f', '#82c6e2'] 
    linestyles = ['-', '--', '-.', ':', '-']
    
    if system_order is None:
        system_order = list(data_left.keys())

    datasets = [data_left, data_right]

    # Compute global min/max for consistent x-axis across both plots
    all_values = []
    for data_dict in datasets:
        for system in system_order:
            if system in data_dict:
                all_values.extend(data_dict[system])
    global_min = min(all_values)
    global_max = max(all_values)
    
    # Add some padding for log scale
    x_min = global_min * 0.8
    x_max = global_max * 1.2

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
        ax.set_ylabel(y_label if idx == 0 else "") 
        
        # X-LABELS CUSTOMIZED (application name)
        ax.set_xlabel(x_labels[idx], fontsize=14)
        
        ax.set_xscale('log')
        ax.set_xlim(x_min, x_max)  # Use consistent x-axis limits
        ax.grid(True, which="major", ls="-", alpha=0.3)

    # 4. Legend on the right figure only (bottom right)
    axes[1].legend(loc='lower right', frameon=True)

    # 5. Adjust Layout
    plt.tight_layout() 

    print(f"Saving plot to {output_filename}...")
    plt.savefig(output_filename, bbox_inches='tight')
    plt.close()
    print(f"Saved plot to {output_filename}")


def load_app_timings(app_prefix, operation):
    """Load timing data for a specific application and operation (encrypt/decrypt).
    
    Args:
        app_prefix: 'boutique' or 'hotel'
        operation: 'encrypt' or 'decrypt'
    
    Returns:
        Dictionary mapping strategy labels to timing arrays
    """
    timings = {}
    
    for label, strategy_suffix in STRATEGIES.items():
        filename = f"{app_prefix}_triple_encryption_{strategy_suffix}_{operation}_times.csv"
        print(f"Loading {filename}...")
        try:
            data = load_timings(filename)
            if len(data) > 0:
                timings[label] = data
                print(f"  Loaded {len(data)} samples")
                print(f"  Statistics: min={data.min():.2f}ns, max={data.max():.2f}ns, "
                      f"mean={data.mean():.2f}ns, median={np.median(data):.2f}ns")
        except FileNotFoundError:
            print(f"  Warning: {filename} not found, skipping...")
    
    return timings


def main():
    print("=" * 60)
    print("Loading Encryption Timing Data")
    print("=" * 60)
    
    # Load encryption timings for both applications
    boutique_encrypt = load_app_timings("boutique", "encrypt")
    hotel_encrypt = load_app_timings("hotel", "encrypt")
    
    print("\n" + "=" * 60)
    print("Loading Decryption Timing Data")
    print("=" * 60)
    
    # Load decryption timings for both applications
    boutique_decrypt = load_app_timings("boutique", "decrypt")
    hotel_decrypt = load_app_timings("hotel", "decrypt")
    
    system_order = list(STRATEGIES.keys())
    
    # Plot encryption latency CDF (Boutique left, Hotel right)
    print("\n" + "=" * 60)
    print("Generating Encryption Latency CDF")
    print("=" * 60)
    
    if boutique_encrypt and hotel_encrypt:
        plot_app_comparison_cdfs(
            boutique_encrypt, 
            hotel_encrypt,
            x_labels=('Online Boutique\nEncrypt Latency (ns)', 'Hotel Reservation\nEncrypt Latency (ns)'),
            y_label='CDF (%)',
            output_filename=ENCRYPT_OUTPUT_FILE,
            system_order=system_order
        )
    else:
        print("Error: Missing encryption timing data. Please run benchmarks first.")
    
    # Plot decryption latency CDF (Boutique left, Hotel right)
    print("\n" + "=" * 60)
    print("Generating Decryption Latency CDF")
    print("=" * 60)
    
    if boutique_decrypt and hotel_decrypt:
        plot_app_comparison_cdfs(
            boutique_decrypt, 
            hotel_decrypt,
            x_labels=('Online Boutique\nDecrypt Latency (ns)', 'Hotel Reservation\nDecrypt Latency (ns)'),
            y_label='CDF (%)',
            output_filename=DECRYPT_OUTPUT_FILE,
            system_order=system_order
        )
    else:
        print("Error: Missing decryption timing data. Please run benchmarks first.")
    
    print("\n" + "=" * 60)
    print("Done!")
    print("=" * 60)


if __name__ == "__main__":
    main()
