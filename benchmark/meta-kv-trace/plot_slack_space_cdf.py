#!/usr/bin/env python3
"""
Slack Space CDF Plotter for Meta KV Trace

This script computes and plots a Cumulative Distribution Function (CDF) of the
remaining slack space for each key-value pair in the trace.

Slack space is calculated as: (key_size + value_size) % MTU
This represents the number of bytes that don't fill a complete MTU packet.

Usage:
    python plot_slack_space_cdf.py

Prerequisites:
    - Python 3.x
    - matplotlib
    - numpy

Input Files:
    - trace.req: Key-value trace data

Output:
    - slack_space_cdf.pdf: A PDF file containing the CDF plot of slack space
"""
import matplotlib.pyplot as plt
import numpy as np
import matplotlib
from urllib.parse import urlparse, parse_qs

# --- Global Style Settings ---
matplotlib.rcParams['pdf.fonttype'] = 42
matplotlib.rcParams['ps.fonttype'] = 42
matplotlib.rcParams.update({'font.size': 14})

TRACE_FILE = "trace.req"
OUTPUT_FILE = "slack_space_cdf.pdf"
MTU = 1500  # Standard Ethernet MTU in bytes


def parse_line(line):
    """Parse a line from trace.req and extract key_size, value_size."""
    line = line.strip()
    if not line:
        return None
    
    # Parse the URL query string
    parsed = urlparse(line)
    params = parse_qs(parsed.query)
    
    key_size = int(params.get("key_size", ["0"])[0])
    value_size = int(params.get("value_size", ["0"])[0])
    
    return key_size, value_size


def load_slack_space(filename, mtu=MTU):
    """Load trace data and compute slack space for each key-value pair.
    
    Slack space = (key_size + value_size) % MTU
    
    Returns:
        numpy array of slack space values
    """
    slack_values = []
    total = 0
    
    with open(filename, "r") as f:
        for line in f:
            result = parse_line(line)
            if result is None:
                continue
            
            key_size, value_size = result
            total_size = key_size + value_size
            
            # Calculate slack space (remainder after dividing by MTU)
            slack = total_size % mtu
            slack_values.append(slack)
            
            total += 1
            if total % 100000 == 0:
                print(f"Processed {total} requests...")
    
    print(f"Loaded {total} key-value pairs")
    return np.array(slack_values)


def plot_slack_space_cdf(slack_data, output_filename=OUTPUT_FILE, mtu=MTU, show_legend=False):
    """
    Plots a CDF of slack space values.
    
    Args:
        slack_data: numpy array of slack space values
        output_filename: output PDF filename
        mtu: MTU value used (for legend label)
        show_legend: whether to show the legend (default: False)
    """
    # Setup Figure
    fig, ax = plt.subplots(1, 1, figsize=(5, 2.5))
    
    # Standard SIGCOMM Color Palette
    color = '#4878d0'
    
    # Compute CDF
    sorted_data = np.sort(slack_data)
    yvals = np.arange(1, len(sorted_data) + 1) / len(sorted_data)
    
    ax.plot(sorted_data, yvals, 
            label=f"Slack Space (MTU={mtu})", 
            color=color, 
            linestyle='-', 
            linewidth=2.5)

    # Styling
    ax.set_yticks([0, 0.25, 0.50, 0.75, 1.0])
    ax.set_yticklabels(['0', '25', '50', '75', '100'])
    ax.set_ylabel('CDF (%)')
    ax.set_xlabel('Slack Space (bytes)', fontsize=14)
    
    # Set x-axis limits from 0 to MTU
    ax.set_xlim(0, mtu)
    
    ax.grid(True, which="major", ls="-", alpha=0.3)
    
    if show_legend:
        ax.legend(loc='lower right', frameon=True)

    plt.tight_layout()

    print(f"Saving plot to {output_filename}...")
    plt.savefig(output_filename, bbox_inches='tight')
    plt.close()
    print(f"Saved CDF plot to {output_filename}")


def main():
    print(f"Loading trace data from {TRACE_FILE}...")
    slack_data = load_slack_space(TRACE_FILE, MTU)
    
    if len(slack_data) > 0:
        print(f"\nSlack Space Statistics (MTU={MTU}):")
        print(f"  Min: {slack_data.min()} bytes")
        print(f"  Max: {slack_data.max()} bytes")
        print(f"  Mean: {slack_data.mean():.2f} bytes")
        print(f"  Median: {np.median(slack_data):.2f} bytes")
        print(f"  Std Dev: {slack_data.std():.2f} bytes")
        
        # Calculate percentage that exactly fills MTU boundaries
        zero_slack = np.sum(slack_data == 0)
        print(f"  Zero slack (exact MTU fit): {zero_slack} ({100*zero_slack/len(slack_data):.2f}%)")
        
        plot_slack_space_cdf(slack_data, OUTPUT_FILE, MTU)
    else:
        print("Error: No data found in trace file.")


if __name__ == "__main__":
    main()
