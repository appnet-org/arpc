#!/usr/bin/env python3
"""
Message Size vs. Encryption Latency Correlation Analysis

This script analyzes the relationship between message size and encryption/decryption
latency to verify that larger messages result in higher latency.

Statistical Methods Used:
========================

1. **Pearson Correlation Coefficient (r)**
   - Measures the LINEAR relationship between two continuous variables.
   - Range: -1 to +1
     - +1: Perfect positive linear relationship
     - 0: No linear relationship
     - -1: Perfect negative linear relationship
   - Assumes: Both variables are normally distributed, relationship is linear.
   - Formula: r = Σ[(xi - x̄)(yi - ȳ)] / √[Σ(xi - x̄)² × Σ(yi - ȳ)²]

2. **Spearman Rank Correlation Coefficient (ρ)**
   - Measures the MONOTONIC relationship (not necessarily linear).
   - More robust to outliers than Pearson.
   - Works by ranking the data and computing Pearson correlation on ranks.
   - Range: -1 to +1 (same interpretation as Pearson)
   - Better for: Non-linear but monotonic relationships, ordinal data, outliers.

3. **Linear Regression (OLS - Ordinary Least Squares)**
   - Fits a line y = mx + b that minimizes the sum of squared residuals.
   - Provides:
     - Slope (m): Expected increase in latency (ns) per byte of message size.
     - Intercept (b): Base latency when message size is 0 (overhead).
     - R² (coefficient of determination): Proportion of variance explained by the model.

4. **Binned Analysis**
   - Groups messages into size bins and computes mean latency per bin.
   - Helps visualize the trend without scatter plot noise.
   - Useful when data has high variance within size groups.

Usage:
    python plot_size_vs_latency.py

Output:
    - size_vs_latency_correlation.pdf: Scatter plots with regression lines
    - Console output with correlation statistics
"""

import matplotlib.pyplot as plt
import numpy as np
import matplotlib
import os
from scipy import stats

# --- Global Style Settings ---
matplotlib.rcParams['pdf.fonttype'] = 42
matplotlib.rcParams['ps.fonttype'] = 42
matplotlib.rcParams.update({'font.size': 12})

PROFILE_DATA_DIR = "profile_data"
OUTPUT_FILE = "size_vs_latency_correlation.pdf"

# Format labels and file prefixes
FORMATS = {
    "Whole": "encryption_whole",
    "Random Split": "encryption_random_split",
    "Equal Split": "encryption_equal_split",
    "Key-Value Split": "encryption_key_value_split",
}


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


def compute_correlations(sizes, latencies):
    """Compute Pearson and Spearman correlation coefficients.
    
    Returns:
        dict: Contains pearson_r, pearson_p, spearman_r, spearman_p
    """
    # Pearson correlation (linear relationship)
    pearson_r, pearson_p = stats.pearsonr(sizes, latencies)
    
    # Spearman correlation (monotonic relationship)
    spearman_r, spearman_p = stats.spearmanr(sizes, latencies)
    
    return {
        'pearson_r': pearson_r,
        'pearson_p': pearson_p,
        'spearman_r': spearman_r,
        'spearman_p': spearman_p,
    }


def compute_linear_regression(sizes, latencies):
    """Compute linear regression (OLS).
    
    Returns:
        dict: Contains slope, intercept, r_squared, std_err
    """
    slope, intercept, r_value, p_value, std_err = stats.linregress(sizes, latencies)
    
    return {
        'slope': slope,
        'intercept': intercept,
        'r_squared': r_value ** 2,
        'p_value': p_value,
        'std_err': std_err,
    }


def compute_binned_statistics(sizes, latencies, num_bins=20):
    """Compute mean latency for each size bin.
    
    Returns:
        tuple: (bin_centers, bin_means, bin_stds, bin_counts)
    """
    # Create bins based on size range
    min_size, max_size = sizes.min(), sizes.max()
    bins = np.linspace(min_size, max_size, num_bins + 1)
    
    bin_indices = np.digitize(sizes, bins)
    
    bin_centers = []
    bin_means = []
    bin_stds = []
    bin_counts = []
    
    for i in range(1, len(bins)):
        mask = bin_indices == i
        if mask.sum() > 0:
            bin_centers.append((bins[i-1] + bins[i]) / 2)
            bin_means.append(latencies[mask].mean())
            bin_stds.append(latencies[mask].std())
            bin_counts.append(mask.sum())
    
    return np.array(bin_centers), np.array(bin_means), np.array(bin_stds), np.array(bin_counts)


def print_correlation_report(name, sizes, latencies):
    """Print a detailed correlation analysis report."""
    corr = compute_correlations(sizes, latencies)
    reg = compute_linear_regression(sizes, latencies)
    
    print(f"\n{'='*70}")
    print(f"  {name}")
    print(f"{'='*70}")
    print(f"  Data points: {len(sizes):,}")
    print(f"  Message size range: {sizes.min():,} - {sizes.max():,} bytes")
    print(f"  Latency range: {latencies.min():,} - {latencies.max():,} ns")
    print()
    print("  CORRELATION ANALYSIS:")
    print(f"  ─────────────────────────────────────────────────────────────────")
    print(f"  Pearson correlation (r):   {corr['pearson_r']:+.4f}  (p-value: {corr['pearson_p']:.2e})")
    print(f"  Spearman correlation (ρ):  {corr['spearman_r']:+.4f}  (p-value: {corr['spearman_p']:.2e})")
    print()
    print("  LINEAR REGRESSION (latency = slope × size + intercept):")
    print(f"  ─────────────────────────────────────────────────────────────────")
    print(f"  Slope:     {reg['slope']:.4f} ns/byte")
    print(f"  Intercept: {reg['intercept']:.2f} ns (base overhead)")
    print(f"  R²:        {reg['r_squared']:.4f} ({reg['r_squared']*100:.2f}% variance explained)")
    print()
    
    # Interpretation
    print("  INTERPRETATION:")
    print(f"  ─────────────────────────────────────────────────────────────────")
    if corr['spearman_r'] > 0.7:
        print("  ✓ Strong positive correlation: Larger messages → Higher latency")
    elif corr['spearman_r'] > 0.4:
        print("  ✓ Moderate positive correlation: Larger messages tend to have higher latency")
    elif corr['spearman_r'] > 0.1:
        print("  ~ Weak positive correlation: Some relationship exists")
    else:
        print("  ✗ No significant correlation found")
    
    print(f"  • Each additional byte adds ~{reg['slope']:.3f} ns to latency")
    print(f"  • Each additional KB adds ~{reg['slope']*1024:.1f} ns to latency")
    
    return corr, reg


def plot_correlation_analysis(encrypt_data, decrypt_data, output_filename=OUTPUT_FILE):
    """Create a comprehensive visualization of size vs latency correlation."""
    
    # Filter to strategies that have data
    strategies = [s for s in FORMATS.keys() if s in encrypt_data or s in decrypt_data]
    
    if not strategies:
        print("No data to plot!")
        return
    
    # Create figure with 2 columns (encrypt/decrypt) and N rows (strategies)
    n_strategies = len(strategies)
    fig, axes = plt.subplots(n_strategies, 2, figsize=(12, 3 * n_strategies))
    
    if n_strategies == 1:
        axes = axes.reshape(1, -1)
    
    colors = ['#4878d0', '#6acc64', '#e6a04e', '#d65f5f']
    
    for row_idx, strategy in enumerate(strategies):
        for col_idx, (data_dict, op_name) in enumerate([(encrypt_data, 'Encrypt'), (decrypt_data, 'Decrypt')]):
            ax = axes[row_idx, col_idx]
            
            if strategy not in data_dict:
                ax.text(0.5, 0.5, 'No data', ha='center', va='center', transform=ax.transAxes)
                ax.set_title(f"{strategy} - {op_name}")
                continue
            
            latencies, sizes = data_dict[strategy]
            
            # Scatter plot (subsample if too many points)
            max_points = 5000
            if len(sizes) > max_points:
                indices = np.random.choice(len(sizes), max_points, replace=False)
                plot_sizes = sizes[indices]
                plot_latencies = latencies[indices]
            else:
                plot_sizes = sizes
                plot_latencies = latencies
            
            ax.scatter(plot_sizes, plot_latencies, alpha=0.3, s=10, 
                      color=colors[row_idx % len(colors)], label='Data')
            
            # Linear regression line
            reg = compute_linear_regression(sizes, latencies)
            x_line = np.array([sizes.min(), sizes.max()])
            y_line = reg['slope'] * x_line + reg['intercept']
            ax.plot(x_line, y_line, 'r-', linewidth=2, 
                   label=f'Fit: {reg["slope"]:.3f} ns/byte')
            
            # Binned means with error bars
            bin_centers, bin_means, bin_stds, _ = compute_binned_statistics(sizes, latencies)
            ax.errorbar(bin_centers, bin_means, yerr=bin_stds, fmt='ko', 
                       markersize=4, capsize=3, label='Binned mean ± std')
            
            # Correlation annotation
            corr = compute_correlations(sizes, latencies)
            ax.text(0.05, 0.95, f"Spearman ρ = {corr['spearman_r']:.3f}\nR² = {reg['r_squared']:.3f}",
                   transform=ax.transAxes, fontsize=10, verticalalignment='top',
                   bbox=dict(boxstyle='round', facecolor='white', alpha=0.8))
            
            ax.set_xlabel('Message Size (bytes)')
            ax.set_ylabel('Latency (ns)')
            ax.set_title(f"{strategy} - {op_name}")
            ax.legend(loc='lower right', fontsize=8)
            ax.grid(True, alpha=0.3)
    
    plt.tight_layout()
    
    print(f"\nSaving correlation plot to {output_filename}...")
    plt.savefig(output_filename, bbox_inches='tight', dpi=150)
    plt.close()
    print(f"Saved correlation plot to {output_filename}")


def main():
    print("="*70)
    print("  MESSAGE SIZE vs LATENCY CORRELATION ANALYSIS")
    print("="*70)
    
    # Load encrypt data
    encrypt_data = {}
    for label, prefix in FORMATS.items():
        filename = f"{prefix}_encrypt_times.csv"
        try:
            latencies, sizes = load_timing_data(filename)
            if len(latencies) > 0:
                encrypt_data[label] = (latencies, sizes)
                print(f"Loaded {len(latencies):,} encrypt samples for {label}")
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
                print(f"Loaded {len(latencies):,} decrypt samples for {label}")
        except FileNotFoundError:
            print(f"Warning: {filename} not found, skipping...")
    
    if not encrypt_data and not decrypt_data:
        print("\nError: No timing data found. Please run benchmarks first.")
        return
    
    # Print detailed correlation reports
    print("\n" + "="*70)
    print("  ENCRYPTION ANALYSIS")
    print("="*70)
    for label, (latencies, sizes) in encrypt_data.items():
        print_correlation_report(f"{label} - Encrypt", sizes, latencies)
    
    print("\n" + "="*70)
    print("  DECRYPTION ANALYSIS")
    print("="*70)
    for label, (latencies, sizes) in decrypt_data.items():
        print_correlation_report(f"{label} - Decrypt", sizes, latencies)
    
    # Generate visualization
    plot_correlation_analysis(encrypt_data, decrypt_data)
    
    # Summary
    print("\n" + "="*70)
    print("  SUMMARY")
    print("="*70)
    print("\n  The analysis uses two correlation measures:")
    print("  • Pearson r: Assumes linear relationship")
    print("  • Spearman ρ: Measures monotonic relationship (more robust)")
    print("\n  A positive correlation (ρ > 0) confirms that larger messages")
    print("  tend to have higher latency. The slope from linear regression")
    print("  quantifies the cost per byte.")
    print()


if __name__ == "__main__":
    main()

