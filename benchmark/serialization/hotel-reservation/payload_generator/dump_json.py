"""Utilities for writing generated payloads."""

import json
import os
from typing import Any, Dict, List


def ensure_dir(path: str) -> None:
    """Create directory if it doesn't exist."""
    os.makedirs(path, exist_ok=True)


def write_jsonl(path: str, objs: List[Dict[str, Any]]) -> None:
    """Write objects as JSONL (one JSON object per line)."""
    with open(path, "w", encoding="utf-8") as f:
        for o in objs:
            f.write(json.dumps(o, separators=(",", ":"), ensure_ascii=False))
            f.write("\n")


def write_pretty_json_array(path: str, objs: List[Dict[str, Any]]) -> None:
    """Write objects as a pretty-printed JSON array."""
    with open(path, "w", encoding="utf-8") as f:
        json.dump(objs, f, indent=2, ensure_ascii=False)
        f.write("\n")
