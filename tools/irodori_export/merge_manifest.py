"""Build a manifest from graph files when exports were run as separate jobs."""
from __future__ import annotations

import argparse
import hashlib
import json
import platform
from pathlib import Path

import onnx

from export import (MODEL_REVISION, SOURCE_REVISION, TOKENIZER_REVISION, CODEC_REPO,
                    CODEC_CODE_REVISION, CODEC_WEIGHTS_REVISION, OPSET,
                    EXPECTED_SOURCE_ARCHIVE_SHA256, EXPECTED_SOURCE_CONTENT_SHA256,
                    EXPECTED_CODEC_WEIGHTS_FILE_SHA256,
                    _validate_fixed_inputs, _exporter_files, _source_tree_sha256,
                    sha256, _onnx_io)

# Measured from the pinned DACVAE snapshot used by the exporter.  Keeping this in
# the merged manifest prevents a split graph export from silently dropping the
# codec weight identity.
CODEC_WEIGHTS_SHA256 = "022b6ad793fcb264e295f8773401f7a440e5b51299028a9ec5a55e8252298cb1"


def main() -> int:
    root = Path(__file__).resolve().parents[2]
    p = argparse.ArgumentParser()
    p.add_argument("--model-dir", type=Path, default=root / "model/irodori-v4.1")
    p.add_argument("--checkpoint", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/model.safetensors")
    p.add_argument("--source", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-8224dafb46d0aba89209a8f905f1cb7e3299d9c1")
    p.add_argument("--tokenizer", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/tokenizer/tokenizer.json")
    args = p.parse_args()
    source_archive = root / "third_party/irodori-v4-research/upstream.zip"
    _validate_fixed_inputs(args.source, args.checkpoint, args.tokenizer, CODEC_REPO, source_archive)
    md = args.model_dir
    graphs = {
        "text_caption_encoder": {"inputs": ["text_input_ids", "text_mask", "caption_input_ids", "caption_mask"], "outputs": ["text_state", "caption_state"]},
        "speaker_encoder": {"inputs": ["ref_latent", "ref_mask"], "outputs": ["speaker_state", "speaker_mask"]},
        "duration_predictor": {"inputs": ["text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "duration_features", "has_speaker", "has_caption"], "outputs": ["log_frames"]},
        "dit_step": {"inputs": ["x_t", "t", "text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "latent_mask"], "outputs": ["v_pred"]},
        "codec_encoder": {"inputs": ["waveform"], "outputs": ["latent"]},
        "codec_decoder": {"inputs": ["latent"], "outputs": ["waveform"]},
    }
    sidecars = [{"path": x.name, "bytes": x.stat().st_size, "sha256": sha256(x)} for x in sorted(md.glob("*.onnx.data"))]
    manifest = {
        "schema_version": 2,
        "model_family": "irodori-v4",
        "model_release": "v4.1-small",
        "model_revision": MODEL_REVISION,
        "official_source": {"repository": "Aratako/Irodori-TTS", "revision": SOURCE_REVISION, "content_sha256": EXPECTED_SOURCE_CONTENT_SHA256, "archive": "third_party/irodori-v4-research/upstream.zip", "archive_sha256": EXPECTED_SOURCE_ARCHIVE_SHA256},
        "exporter": {"repository": "fm-live-radio/tools/irodori_export", "python": platform.python_version(), "opset": OPSET, "precision": "float32", "device": "cpu", "files_sha256": _exporter_files()},
        "tokenizer": {"repository": "sbintuitions/modernbert-ja-310m", "revision": TOKENIZER_REVISION, "path": "tokenizer/tokenizer.json", "sha256": sha256(args.tokenizer), "special_ids": {"bos": 1, "eos": 2, "pad": 3}, "text_max_length": 256, "caption_max_length": 512, "max_ref_seconds": 120.0, "license": {"spdx": "MIT", "source": "https://huggingface.co/sbintuitions/modernbert-ja-310m/blob/77675fc96a7e445e982e2ba90246b816efc74ec6/LICENSE", "copyright": "Copyright (c) 2025 SB Intuitions", "note": "配布するtokenizer.jsonはこのModernBERT-ja checkpoint由来。原文license/NOTICEを上記revisionで参照。"}},
        "codec": {"repository": CODEC_REPO, "weights_revision": CODEC_WEIGHTS_REVISION, "weights_sha256": CODEC_WEIGHTS_SHA256, "weights_file_sha256": EXPECTED_CODEC_WEIGHTS_FILE_SHA256, "code_revision": CODEC_CODE_REVISION, "sample_rate": 48000, "hop_length": 1920, "latent_dim": 32, "deterministic_encode": True, "deterministic_decode": True},
        "licenses": {"official_code": {"spdx": "MIT", "source": "https://github.com/Aratako/Irodori-TTS/blob/8224dafb46d0aba89209a8f905f1cb7e3299d9c1/LICENSE"}, "model": {"spdx": "MIT", "source": "https://huggingface.co/Aratako/Irodori-TTS-v4.1-Small"}, "codec_code": {"spdx": "Apache-2.0", "source": "https://github.com/facebookresearch/dacvae/blob/414c20785fc3a28373073ea8ef7a1316eeeaca6e/LICENSE"}, "codec_model": {"spdx": "MIT", "source": "https://huggingface.co/Aratako/Semantic-DACVAE-Japanese-32dim"}, "tokenizer_modernbert": {"spdx": "MIT", "source": "https://huggingface.co/sbintuitions/modernbert-ja-310m/blob/77675fc96a7e445e982e2ba90246b816efc74ec6/LICENSE", "copyright": "Copyright (c) 2025 SB Intuitions", "distribution_target": "model/irodori-v4.1/tokenizer/tokenizer.json"}, "distribution_targets": ["model/irodori-v4.1/*.onnx", "model/irodori-v4.1/*.onnx.data", "model/irodori-v4.1/tokenizer/tokenizer.json"], "development_only": ["tools/irodori_export/.venv", "tools/irodori_export/uv.lock"], "notice_sources": ["https://github.com/Aratako/Irodori-TTS/blob/8224dafb46d0aba89209a8f905f1cb7e3299d9c1/LICENSE", "https://github.com/facebookresearch/dacvae/blob/414c20785fc3a28373073ea8ef7a1316eeeaca6e/LICENSE", "https://huggingface.co/sbintuitions/modernbert-ja-310m/blob/77675fc96a7e445e982e2ba90246b816efc74ec6/LICENSE"], "notice_files": [{"component": "ModernBERT-ja tokenizer", "path": "docs/plan_20260907_irodori_v4/evidence/licenses/modernbert-ja-LICENSE.txt", "sha256": "284353c80e0d52c06e97af62fd40e4dd1d253fa28f5d1e756121ea1d5d441509"}, {"component": "DACVAE code", "path": "docs/plan_20260907_irodori_v4/evidence/licenses/dacvae-LICENSE.txt", "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30"}]},
        "conditions": {"text_dim": 512, "caption_dim": 512, "speaker_dim": 768, "speaker_patch_size": 4, "duration_aux_dim": 14, "duration_architecture": "token_sum_dual_adarn_zero_no_aux", "latent_dim": 32, "latent_patch_size": 1},
        "checkpoint": {"path": str(args.checkpoint), "sha256": sha256(args.checkpoint), "bytes": args.checkpoint.stat().st_size},
        "graphs": {}, "external_data": sidecars,
        "resource_observations": {"gpu_peak_mib": None, "gpu_measurement": "not captured by exporter; CUDA acceptance required", "cpu_working_set": None},
        "export": {"status": "merged_from_individual_exports", "errors": []},
    }
    for name, declared in graphs.items():
        path = md / f"{name}.onnx"
        if not path.exists():
            manifest["export"]["errors"].append({"graph": name, "error": "missing graph file"})
            continue
        io = _onnx_io(path)
        axes = {x["name"]: {str(i): d for i, d in enumerate(x["shape"]) if isinstance(d, str)} for x in io["inputs"] + io["outputs"]}
        manifest["graphs"][name] = {"path": path.name, "inputs": declared["inputs"], "outputs": declared["outputs"], "io": io, "dynamic_axes": axes, "opset": OPSET, "bytes": path.stat().st_size, "sha256": sha256(path)}
    fixture = md / "fixtures.json"
    if fixture.exists():
        manifest["fixtures"] = {"path": fixture.name, "sha256": sha256(fixture), "seed": 0, "conditions": ["reference", "caption", "reference+caption", "null"]}
    out = md / "manifest.json"
    out.write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(manifest, ensure_ascii=False, indent=2))
    return 0 if not manifest["export"]["errors"] else 2


if __name__ == "__main__":
    raise SystemExit(main())
