# Computes and saves CDF plots of latency for read and write operations
# for different serialization formats (Protobuf, FlatBuffers, Cap'n Proto, fRPC)
import matplotlib.pyplot as plt
import numpy as np
import matplotlib
import os

# --- Global Style Settings ---
matplotlib.rcParams['pdf.fonttype'] = 42
matplotlib.rcParams['ps.fonttype'] = 42
matplotlib.rcParams.update({'font.size': 14})

PROFILE_DATA_DIR = "profile_data"
OUTPUT_FILE = "serialization_latency_cdf.pdf"

# Format labels and file names (order matters for legend)
FORMATS = {
    "FlatBuffers": "flatbuffers",
    "Cap'n Proto": "capnp",
    "Protobuf": "protobuf",
    "fRPC": "symphony",
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
                             x_labels=('Write Latency (ns)', 'Read Latency (ns)'), 
                             output_filename="latency_cdf.pdf", 
                             system_order=None):
    """
    Plots two CDFs side-by-side with shared legend at bottom.
    Titles are removed; X-axis labels differentiate the plots.
    """
    
    # 1. Setup Figure (1 row, 2 columns)
    fig, axes = plt.subplots(1, 2, figsize=(8, 3))
    
    # Standard SIGCOMM Color Palette & Styles
    colors = ['#6acc64', '#4878d0', '#82c6e2', '#e6a04e'] 
    linestyles = ['-', '--', '-.', ':']
    
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
        
        # TITLES REMOVED, X-LABELS CUSTOMIZED
        ax.set_xlabel(x_labels[idx], fontsize=14)
        
        ax.set_xscale('log')
        ax.grid(True, which="major", ls="-", alpha=0.3)

    # 4. Shared Legend at Bottom
    handles, labels = axes[0].get_legend_handles_labels()
    
    fig.legend(handles, labels, 
               loc='lower center', 
               bbox_to_anchor=(0.5, -0.15), 
               ncol=4, 
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
    # Load write timings
    write_timings = {}
    for label, prefix in FORMATS.items():
        filename = f"{prefix}_write_times.txt"
        print(f"Loading {filename}...")
        timings = load_timings(filename)
        if len(timings) > 0:
            write_timings[label] = timings
            print(f"  Loaded {len(timings)} samples")
            print(f"  Statistics: min={timings.min():.2f}ns, max={timings.max():.2f}ns, "
                  f"mean={timings.mean():.2f}ns, median={np.median(timings):.2f}ns")
    
    # Load read timings
    read_timings = {}
    for label, prefix in FORMATS.items():
        filename = f"{prefix}_read_times.txt"
        print(f"Loading {filename}...")
        timings = load_timings(filename)
        if len(timings) > 0:
            read_timings[label] = timings
            print(f"  Loaded {len(timings)} samples")
            print(f"  Statistics: min={timings.min():.2f}ns, max={timings.max():.2f}ns, "
                  f"mean={timings.mean():.2f}ns, median={np.median(timings):.2f}ns")
    
    # Plot merged CDFs
    if write_timings and read_timings:
        system_order = list(FORMATS.keys())
        plot_merged_latency_cdfs(write_timings, read_timings,
                                x_labels=('Write Latency (ns)', 'Read Latency (ns)'),
                                output_filename=OUTPUT_FILE,
                                system_order=system_order)

if __name__ == "__main__":
    main()

