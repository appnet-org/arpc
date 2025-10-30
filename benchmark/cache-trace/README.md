# Cache Trace Analysis

This directory contains tools and scripts for analyzing cache traces, specifically Meta's KV cache traces from 2024.

## Download the Trace

Download the compressed trace file from the Meta [cache dataset](https://github.com/cacheMon/cache_dataset?tab=readme-ov-file#meta-key-value-cache-traces):

```bash
wget https://cache-datasets.s3.amazonaws.com/cache_dataset_txt/2022_metaKV/kvcache_202401/kvcache_traces_1.csv.zst
```

Decompress the trace file:

```bash
unzstd kvcache_traces_1.csv.zst
```

## Read the Trace

Preview the first 20 lines of the trace data:

```bash
zstdcat kvcache_traces_1.csv.zst | head -n 20
```

## Creating Our Trace Based on Cachelib Trace

When processing the trace data, the following fields are of interest:

- **op**: Operation (GET, SET, DELETE)
- **key**: The cache key identifier
- **Key_size**: Size of the key in bytes
- **size**: Size of the cached value in bytes


## Using Trace

```bash
./wrk -d 20s -t 1 -c 1 http://10.96.88.88:80 -s kv-store.lua -L
```