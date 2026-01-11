"""
Online Boutique (.proto) JSON payload generator

- Generates reasonable JSON payloads for each message type in the provided schema.
- Outputs one file per message type (JSONL: one JSON object per line).
- Keeps logic simple: field-level generators + message builders.
- Deterministic with seed.

Configuration is in config.py.
"""

import os

from config import Config
from field_generator import FieldGenerator
from message_generators import message_generators
from dump_json import ensure_dir, write_jsonl, write_pretty_json_array


def main() -> None:
    cfg = Config()

    ensure_dir(cfg.output.out_dir)
    g = FieldGenerator(cfg)

    for msg_type, count in cfg.counts.items():
        gen_fn = message_generators.get(msg_type)
        if gen_fn is None:
            raise KeyError(f"No generator registered for message type: {msg_type}")

        payloads = [gen_fn(g) for _ in range(count)]
        out_path = os.path.join(cfg.output.out_dir, f"{msg_type}.jsonl" if not cfg.output.pretty else f"{msg_type}.json")

        if cfg.output.pretty:
            write_pretty_json_array(out_path, payloads)
        else:
            write_jsonl(out_path, payloads)

        print(f"Wrote {count:>7} payloads -> {out_path}")

    print("Done.")


if __name__ == "__main__":
    main()
