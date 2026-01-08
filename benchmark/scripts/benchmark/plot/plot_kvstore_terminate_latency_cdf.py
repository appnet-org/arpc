#!/usr/bin/env python3
"""
KV-Store Latency CDF Plotter

This script computes and plots Cumulative Distribution Functions (CDFs) of latency
for kv-store operations comparing gRPC and gRPC-istio systems.

Usage:
    python plot_kvstore_latency_cdf.py

Prerequisites:
    - Python 3.x
    - matplotlib
    - numpy

Input Files:
    The script expects latency data files in the 'logs/' directory:
        - kv-store-grpc_latency.txt
        - kv-store-grpc-istio_latency.txt

    Each file should contain one timing value (in nanoseconds) per line.

Output:
    - kv-store-latency_cdf.pdf: A PDF file containing a CDF plot
      comparing gRPC and gRPC-istio latencies.
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

# Path to logs directory (relative to script location)
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
LOGS_DIR = os.path.join(SCRIPT_DIR, "..", "latency", "logs")
OUTPUT_FILE = "kv_store_termination_latency_overhead_cdf.pdf"

# System labels and file names
SYSTEMS = {
    "gRPC": "kv-store-grpc_latency.txt",
    "gRPC+Envoy": "kv-store-grpc-istio_latency.txt",
}

def load_timings(filename):
    """Load timing data from a file in nanoseconds."""
    filepath = os.path.join(LOGS_DIR, filename)
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

def plot_latency_cdf(data_dict, x_label="Latency (ns)", 
                     output_filename="latency_cdf.pdf", 
                     system_order=None):
    """
    Plots a CDF comparing multiple systems.
    """
    
    # 1. Setup Figure
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
    
    ax.set_ylabel('CDF (%)')
    ax.set_xlabel(x_label, fontsize=14)
    
    ax.set_xscale('log')
    ax.grid(True, which="major", ls="-", alpha=0.3)
    
    # 4. Legend
    ax.legend(loc='lower right', frameon=True)

    # 5. Adjust Layout
    plt.tight_layout()

    print(f"Saving plot to {output_filename}...")
    plt.savefig(output_filename, bbox_inches='tight')
    plt.close()
    print(f"Saved plot to {output_filename}")

def main():
    parser = argparse.ArgumentParser(description='Plot CDF of kv-store latency')
    args = parser.parse_args()
    
    # Load latency timings
    latency_timings = {}
    for label, filename in SYSTEMS.items():
        filepath = os.path.join(LOGS_DIR, filename)
        if not os.path.exists(filepath):
            print(f"Warning: {filepath} not found, skipping {label}")
            continue
        print(f"Loading {filename}...")
        timings = load_timings(filename)
        if len(timings) > 0:
            latency_timings[label] = timings
            print(f"  Loaded {len(timings)} samples")
            print(f"  Statistics: min={timings.min():.2f}ns, max={timings.max():.2f}ns, "
                  f"mean={timings.mean():.2f}ns, median={np.median(timings):.2f}ns")
    
    # Plot CDF
    if latency_timings:
        system_order = list(SYSTEMS.keys())
        output_path = os.path.join(SCRIPT_DIR, OUTPUT_FILE)
        plot_latency_cdf(latency_timings,
                        x_label="Latency (ns)",
                        output_filename=output_path,
                        system_order=system_order)
    else:
        print("Error: No latency data loaded")

if __name__ == "__main__":
    main()

