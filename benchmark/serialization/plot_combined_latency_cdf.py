#!/usr/bin/env python3
"""
Combined Serialization Latency CDF Plotter

This script computes and plots Cumulative Distribution Functions (CDFs) of latency
for read and write operations across different serialization formats, comparing
Online Boutique and Hotel Reservation benchmarks side-by-side.

Output:
    - serialization_write_latency_cdf.pdf: Write latency CDFs with Online Boutique (left) and Hotel Reservation (right)
    - serialization_read_latency_cdf.pdf: Read latency CDFs with Online Boutique (left) and Hotel Reservation (right)
    - With --include-hybrid flag, outputs files with _hybrid suffix

Usage:
    python plot_combined_latency_cdf.py
    python plot_combined_latency_cdf.py --include-hybrid
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

# Paths to profile data directories
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
ONLINE_BOUTIQUE_DIR = os.path.join(SCRIPT_DIR, "online-boutique", "profile_data")
HOTEL_RESERVATION_DIR = os.path.join(SCRIPT_DIR, "hotel-reservation", "profile_data")

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


def load_timings(profile_dir, filename):
    """Load timing data from a file in nanoseconds."""
    filepath = os.path.join(profile_dir, filename)
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


def load_all_timings(profile_dir, formats, operation):
    """Load all timings for a given operation (read/write) from a profile directory."""
    timings = {}
    for label, prefix in formats.items():
        filename = f"{prefix}_{operation}_times.txt"
        try:
            data = load_timings(profile_dir, filename)
            if len(data) > 0:
                timings[label] = data
                print(f"  Loaded {len(data)} samples from {filename}")
        except FileNotFoundError:
            print(f"  Warning: {filename} not found in {profile_dir}, skipping...")
    return timings


def plot_combined_latency_cdfs(data_left, data_right, 
                                subplot_titles=('Online Boutique', 'Hotel Reservation'),
                                xlabel='Latency (ns)',
                                output_filename="latency_cdf.pdf", 
                                system_order=None):
    """
    Plots two CDFs side-by-side with shared legend at bottom.
    Left: Online Boutique, Right: Hotel Reservation
    """
    
    # 1. Setup Figure (1 row, 2 columns)
    fig, axes = plt.subplots(1, 2, figsize=(8, 3))
    
    # Standard SIGCOMM Color Palette & Styles
    colors = ['#6acc64', '#4878d0', '#82c6e2', '#e6a04e', '#d65f5f'] 
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
        
        # Set subplot title and x-label
        ax.set_title(subplot_titles[idx], fontsize=14)
        ax.set_xlabel(xlabel, fontsize=14)
        
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
    parser = argparse.ArgumentParser(
        description='Plot combined CDF of serialization latency for Online Boutique and Hotel Reservation benchmarks')
    parser.add_argument('--include-hybrid', action='store_true',
                        help='Include fRPC Hybrid in the plot (default: False)')
    args = parser.parse_args()
    
    # Build FORMATS dict based on flag
    FORMATS = BASE_FORMATS.copy()
    if args.include_hybrid:
        FORMATS.update(HYBRID_FORMAT)
    
    system_order = list(FORMATS.keys())
    
    # Determine output filenames based on whether hybrid is included
    suffix = "_hybrid" if args.include_hybrid else ""
    write_output = f"serialization_write_latency_cdf{suffix}.pdf"
    read_output = f"serialization_read_latency_cdf{suffix}.pdf"
    
    # Load write timings from both benchmarks
    print("Loading Online Boutique write timings...")
    ob_write_timings = load_all_timings(ONLINE_BOUTIQUE_DIR, FORMATS, "write")
    
    print("Loading Hotel Reservation write timings...")
    hr_write_timings = load_all_timings(HOTEL_RESERVATION_DIR, FORMATS, "write")
    
    # Load read timings from both benchmarks
    print("Loading Online Boutique read timings...")
    ob_read_timings = load_all_timings(ONLINE_BOUTIQUE_DIR, FORMATS, "read")
    
    print("Loading Hotel Reservation read timings...")
    hr_read_timings = load_all_timings(HOTEL_RESERVATION_DIR, FORMATS, "read")
    
    # Plot write latency CDFs (Online Boutique left, Hotel Reservation right)
    if ob_write_timings and hr_write_timings:
        plot_combined_latency_cdfs(
            ob_write_timings, 
            hr_write_timings,
            subplot_titles=('Online Boutique', 'Hotel Reservation'),
            xlabel='Write Latency (ns)',
            output_filename=write_output,
            system_order=system_order
        )
    else:
        print("Error: Missing write timing data. Please run benchmarks first.")
    
    # Plot read latency CDFs (Online Boutique left, Hotel Reservation right)
    if ob_read_timings and hr_read_timings:
        plot_combined_latency_cdfs(
            ob_read_timings, 
            hr_read_timings,
            subplot_titles=('Online Boutique', 'Hotel Reservation'),
            xlabel='Read Latency (ns)',
            output_filename=read_output,
            system_order=system_order
        )
    else:
        print("Error: Missing read timing data. Please run benchmarks first.")


if __name__ == "__main__":
    main()
