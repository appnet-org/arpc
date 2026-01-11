# Payload Generator

Generates JSON payloads for Online Boutique message types. Useful for testing and benchmarking serialization formats.

## Running

From this directory:

```bash
python -m payload_generator.main
```

Or:

```bash
python main.py
```

## Configuration

Edit `config.py` to customize:

- **`seed`**: Random seed for deterministic generation (default: `1`)
- **`counts`**: Number of payloads per message type (default: ~100k total)
- **`output.out_dir`**: Output directory (default: `../payloads/`)
- **`output.pretty`**: Pretty-print JSON instead of JSONL (default: `False`)

Other settings control field generation (prices, addresses, product names, etc.).

## Output

Generates one file per message type in the output directory:
- JSONL format (one JSON object per line) by default
- Pretty JSON array if `output.pretty = True`
