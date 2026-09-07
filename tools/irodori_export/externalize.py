"""Move ONNX initializers to per-graph sidecars for missing-data validation."""
from __future__ import annotations

import argparse
from pathlib import Path

import onnx
from onnx.external_data_helper import convert_model_to_external_data


def main() -> int:
    root = Path(__file__).resolve().parents[2]
    p = argparse.ArgumentParser()
    p.add_argument("--model-dir", type=Path, default=root / "model/irodori-v4.1")
    args = p.parse_args()
    for graph in sorted(args.model_dir.glob("*.onnx")):
        model = onnx.load(str(graph), load_external_data=False)
        sidecar = graph.name + ".data"
        convert_model_to_external_data(model, all_tensors_to_one_file=True, location=sidecar, size_threshold=0)
        onnx.save(model, str(graph))
        print(f"externalized {graph.name} -> {sidecar}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
