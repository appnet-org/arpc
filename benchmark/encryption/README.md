# Encryption Benchmark

Benchmark measuring AES-GCM encryption and decryption performance for different data splitting strategies using real trace data from KV-store operations.

## Overview

This benchmark measures encryption and decryption times for three different scenarios:
1. **Whole String Encryption**: Encrypt and decrypt entire data strings using a public key
2. **Random Split Encryption**: Split data at a random point, encrypt public and private parts separately with different keys
3. **Equal Substrings Encryption**: Split data into equal-length substrings, then split each substring into public/private parts for encryption

The benchmark uses the size distribution from `benchmark/meta-kv-trace/trace.req` (sum of `key_size + value_size` for each entry) to generate test data that matches real-world usage patterns.

## Prerequisites

- Go 1.24+
- Access to `benchmark/meta-kv-trace/trace.req` file

## Running Benchmarks

Run all benchmarks:

```bash
go test -bench=. -benchmem
```

Run specific benchmark:

```bash
go test -bench=BenchmarkEncryption_Whole -benchmem
go test -bench=BenchmarkEncryption_RandomSplit -benchmem
go test -bench=BenchmarkEncryption_EqualSubstrings -benchmem
```

Run with custom benchmark time:

```bash
go test -bench=. -benchmem -benchtime=10s ./benchmark/encryption
```

## Benchmarks

- `BenchmarkEncryption_Whole`: Encrypts and decrypts entire strings using the public key
- `BenchmarkEncryption_RandomSplit`: Splits data at random points, encrypts public part with public key and private part with private key
- `BenchmarkEncryption_EqualSubstrings`: Splits data into equal-length substrings, then splits each substring into public/private parts for encryption

Each benchmark measures encryption and decryption times separately.

## Output Files

Benchmark results are written to `profile_data/` directory:

- `encryption_whole_encrypt_times.txt` - Encryption times for whole string encryption (nanoseconds)
- `encryption_whole_decrypt_times.txt` - Decryption times for whole string encryption (nanoseconds)
- `encryption_random_split_encrypt_times.txt` - Total encryption times for random split encryption (nanoseconds)
- `encryption_random_split_decrypt_times.txt` - Total decryption times for random split encryption (nanoseconds)
- `encryption_equal_substrings_encrypt_times.txt` - Total encryption times for equal substrings encryption (nanoseconds)
- `encryption_equal_substrings_decrypt_times.txt` - Total decryption times for equal substrings encryption (nanoseconds)

Each file contains one timing value (in nanoseconds) per line, suitable for CDF plotting.

## Encryption Details

- **Algorithm**: AES-256-GCM
- **Key Size**: 32 bytes (256 bits)
- **Nonce Size**: 12 bytes
- **Authentication Tag**: 16 bytes
- **Public Key**: `27e1fa17d72b1faf722362deb1974a7675058db98843705124a074c61172f796`
- **Private Key**: `9b5300678420678a3157a4bcacdc3e864693971f8a3fab05b06913fb43c7ebf9`

The encryption implementation matches `pkg/transport/encryption.go` using the same `encryptSegment` and `decryptSegment` functions.

## Plotting Results

The output files can be used with plotting scripts similar to the kv-store serialization benchmark to generate CDF (Cumulative Distribution Function) graphs comparing encryption and decryption performance across the three strategies.
