"""Emit value-bearing, official-source v4 fixtures for the Go parity harness.

The fixture contains the exact tensors passed to the official PyTorch graph
wrappers and their outputs.  It is intentionally JSON (rather than hashes) so
the Go harness cannot report success without actually executing and comparing
numeric values.
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import numpy as np
import torch

from export import (
    CODEC_CODE_REVISION,
    CODEC_REPO,
    CODEC_WEIGHTS_REVISION,
    EXPECTED_CHECKPOINT_SHA256,
    EXPECTED_CODEC_WEIGHTS_FILE_SHA256,
    EXPECTED_SOURCE_CONTENT_SHA256,
    EXPECTED_TOKENIZER_SHA256,
    MODEL_REVISION,
    SOURCE_REVISION,
    TOKENIZER_REVISION,
    DitStepGraph,
    DurationGraph,
    SpeakerGraph,
    TextCaptionGraph,
    _load_model,
    _resolve_codec_weights,
    _source_tree_sha256,
    sha256,
)


def arr(value: torch.Tensor | np.ndarray) -> dict:
    if isinstance(value, torch.Tensor):
        value = value.detach().cpu().numpy()
    value = np.asarray(value)
    return {"dtype": str(value.dtype), "shape": list(value.shape), "values": value.reshape(-1).tolist()}


def tuple_arrays(values):
    if not isinstance(values, tuple):
        values = (values,)
    return [arr(value) for value in values]


def run_graph(wrapper, tensors):
    with torch.inference_mode():
        return tuple_arrays(wrapper(*tensors))


def linear_schedule(steps: int) -> np.ndarray:
    return (1.0 - np.linspace(0.0, 1.0, steps + 1, dtype=np.float32)) * np.float32(0.999)


def final_latent(dit, x0, text_state, text_mask, speaker_state, speaker_mask, caption_state, caption_mask, cfg_scales):
    x = np.array(x0, dtype=np.float32, copy=True)
    schedule = linear_schedule(40)
    scale_text, scale_speaker, scale_caption = cfg_scales
    for i in range(40):
        t = np.array([schedule[i]], dtype=np.float32)
        dt = np.float32(schedule[i + 1] - schedule[i])
        if 0.5 <= float(t[0]) <= 1.0 and any(v > 0 for v in cfg_scales):
            bundles = [(text_state, text_mask, speaker_state, speaker_mask, caption_state, caption_mask)]
            if scale_text > 0:
                bundles.append((np.zeros_like(text_state), np.zeros_like(text_mask), speaker_state, speaker_mask, caption_state, caption_mask))
            if scale_speaker > 0:
                bundles.append((text_state, text_mask, np.zeros_like(speaker_state), np.zeros_like(speaker_mask), caption_state, caption_mask))
            if scale_caption > 0:
                bundles.append((text_state, text_mask, speaker_state, speaker_mask, np.zeros_like(caption_state), np.zeros_like(caption_mask)))
            xs = np.concatenate([x] * len(bundles), axis=0)
            vals = {
                "x_t": xs, "t": np.full((len(bundles),), t[0], np.float32),
                "text_state": np.concatenate([b[0] for b in bundles]), "text_mask": np.concatenate([b[1] for b in bundles]),
                "speaker_state": np.concatenate([b[2] for b in bundles]), "speaker_mask": np.concatenate([b[3] for b in bundles]),
                "caption_state": np.concatenate([b[4] for b in bundles]), "caption_mask": np.concatenate([b[5] for b in bundles]),
                "latent_mask": np.ones((len(bundles), x.shape[1]), dtype=np.bool_),
            }
            with torch.inference_mode():
                out = dit(*(torch.from_numpy(vals[k]) for k in ("x_t", "t", "text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "latent_mask")))
            v = out.detach().cpu().numpy().reshape(len(bundles), *x.shape)
            velocity = v[0]
            offset = 1
            if scale_text > 0:
                velocity = velocity + np.float32(scale_text) * (v[0] - v[offset]); offset += 1
            if scale_speaker > 0:
                velocity = velocity + np.float32(scale_speaker) * (v[0] - v[offset]); offset += 1
            if scale_caption > 0:
                velocity = velocity + np.float32(scale_caption) * (v[0] - v[offset])
        else:
            values = {
                "x_t": x, "t": t, "text_state": text_state, "text_mask": text_mask,
                "speaker_state": speaker_state, "speaker_mask": speaker_mask,
                "caption_state": caption_state, "caption_mask": caption_mask,
                "latent_mask": np.ones((1, x.shape[1]), dtype=np.bool_),
            }
            with torch.inference_mode():
                out = dit(*(torch.from_numpy(values[k]) for k in ("x_t", "t", "text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "latent_mask")))
            velocity = out.detach().cpu().numpy()
        x = x + dt * velocity
    return arr(x)


def main() -> int:
    root = Path(__file__).resolve().parents[2]
    p = argparse.ArgumentParser()
    p.add_argument("--source", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-8224dafb46d0aba89209a8f905f1cb7e3299d9c1")
    p.add_argument("--checkpoint", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/model.safetensors")
    p.add_argument("--tokenizer", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/tokenizer/tokenizer.json")
    p.add_argument("--codec", default=CODEC_REPO, help="fixed codec repo id or local weights.pth")
    p.add_argument("--out", type=Path, default=root / "model/irodori-v4.1/go-parity-fixture.json")
    args = p.parse_args()
    if sha256(args.checkpoint) != EXPECTED_CHECKPOINT_SHA256:
        raise ValueError("checkpoint SHA256 mismatch")
    if sha256(args.tokenizer) != EXPECTED_TOKENIZER_SHA256:
        raise ValueError("tokenizer SHA256 mismatch")
    source_hash = _source_tree_sha256(args.source)
    if source_hash != EXPECTED_SOURCE_CONTENT_SHA256:
        raise ValueError("official source content SHA256 mismatch")
    codec_weights = _resolve_codec_weights(args.codec)
    codec_hash = sha256(codec_weights)
    if codec_hash != EXPECTED_CODEC_WEIGHTS_FILE_SHA256:
        raise ValueError("codec weights SHA256 mismatch")
    model, cfg, _, _ = _load_model(args.source, args.checkpoint)
    model.eval()
    device = torch.device("cpu")
    model.to(device)
    # Fixed values are official _samples() values, not a manifest/stat transfer.
    from parity import CodecDecoderGraph, CodecEncoderGraph, _samples
    if str(args.source.resolve()) not in sys.path:
        sys.path.insert(0, str(args.source.resolve()))
    from irodori_tts.duration import build_duration_features
    samples = _samples(cfg, device)
    text = TextCaptionGraph(model)
    speaker = SpeakerGraph(model, cfg.speaker_patch_size)
    duration = DurationGraph(model)
    dit = DitStepGraph(model)
    from irodori_tts.codec import DACVAECodec
    codec = DACVAECodec.load(repo_id=str(codec_weights), device="cpu", deterministic_encode=True, deterministic_decode=True, normalize_db=None)
    codec_encoder = CodecEncoderGraph(codec.model)
    codec_decoder = CodecDecoderGraph(codec.model)
    text_inputs = tuple(x.detach().cpu().numpy() for x in samples["text_caption_encoder"])
    speaker_inputs = tuple(x.detach().cpu().numpy() for x in samples["speaker_encoder"])
    text_out = tuple(x.detach().cpu().numpy() for x in text(*samples["text_caption_encoder"]))
    speaker_out = tuple(x.detach().cpu().numpy() for x in speaker(*samples["speaker_encoder"]))
    graph_inputs = {
        "text_caption_encoder": {"text_input_ids": arr(text_inputs[0]), "text_mask": arr(text_inputs[1]), "caption_input_ids": arr(text_inputs[2]), "caption_mask": arr(text_inputs[3])},
        "speaker_encoder": {"ref_latent": arr(speaker_inputs[0]), "ref_mask": arr(speaker_inputs[1])},
        "codec_encoder": {"waveform": arr(np.zeros((1, 1, 2048), dtype=np.float32))},
        "codec_decoder": {"latent": arr(np.zeros((1, 8, cfg.latent_dim), dtype=np.float32))},
        "dit_step": {name: arr(value.detach().cpu().numpy()) for name, value in zip(("x_t", "t", "text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "latent_mask"), samples["dit_step"])},
        "duration_predictor": {name: arr(value.detach().cpu().numpy()) for name, value in zip(("text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "duration_features", "has_speaker", "has_caption"), samples["duration_predictor"])},
    }
    graph_outputs = {
        "text_caption_encoder": [arr(x) for x in text_out],
        "speaker_encoder": [arr(x) for x in speaker_out],
        "duration_predictor": run_graph(duration, samples["duration_predictor"]),
        "codec_encoder": run_graph(codec_encoder, (torch.zeros((1, 1, 2048), dtype=torch.float32),)),
        "codec_decoder": run_graph(codec_decoder, (torch.zeros((1, 8, cfg.latent_dim), dtype=torch.float32),)),
        "dit_step": run_graph(dit, samples["dit_step"]),
    }
    # Exercise every official duration feature with a stable, value-bearing
    # multilingual case.  The speaker+caption row has all 14 values non-zero;
    # the other rows intentionally differ only in the official has_speaker
    # flag.  Raw outputs and scale-specific frame expectations are stored so
    # Go verifies the complete raw -> expm1 -> scale -> round/clamp contract.
    feature_text = "日本語かなA1、。ー…!？😮‍💨𠀀"
    min_frames = int(np.ceil(0.5 * 48000 / 1920))
    max_frames = int(np.floor(30.0 * 48000 / 1920))

    def expected_frames(raw_value: float, scale: float) -> int:
        frames = int(np.round(np.expm1(float(raw_value)) * scale))
        return max(min_frames, min(max_frames, frames))

    conditions = {}
    for name, use_speaker, use_caption in (("null", False, False), ("speaker", True, False), ("caption", False, True), ("speaker+caption", True, True)):
        tm = np.ones((1, text_out[0].shape[1]), np.bool_)
        sm = speaker_out[1] if use_speaker else np.zeros_like(speaker_out[1])
        cm = np.ones((1, text_out[1].shape[1]), np.bool_) if use_caption else np.zeros((1, text_out[1].shape[1]), np.bool_)
        features = build_duration_features(
            [feature_text], token_counts=np.array([int(tm.sum())], dtype=np.int64),
            max_text_len=256, has_speaker=np.array([use_speaker], dtype=np.bool_),
        ).numpy().astype(np.float32, copy=False)
        args_np = (text_out[0], tm, speaker_out[0], sm, text_out[1], cm, features, np.array([use_speaker]), np.array([use_caption]))
        duration_output = run_graph(duration, tuple(torch.from_numpy(x) for x in args_np))
        raw_value = float(duration_output[0]["values"][0])
        conditions[name] = {"inputs": {"text_state": arr(args_np[0]), "text_mask": arr(args_np[1]), "speaker_state": arr(args_np[2]), "speaker_mask": arr(args_np[3]), "caption_state": arr(args_np[4]), "caption_mask": arr(args_np[5]), "duration_features": arr(args_np[6]), "has_speaker": arr(args_np[7]), "has_caption": arr(args_np[8])}, "duration_features": arr(args_np[6]), "duration_token_count": int(tm.sum()), "duration_max_text_len": 256, "duration_has_speaker": bool(use_speaker), "duration_has_caption": bool(use_caption), "duration_output": duration_output, "duration_raw": duration_output, "duration_frames": {"0.5": expected_frames(raw_value, 0.5), "1": expected_frames(raw_value, 1.0), "2": expected_frames(raw_value, 2.0)}, "dit_output": run_graph(dit, tuple(torch.from_numpy(x) for x in (np.zeros((1, 8, cfg.patched_latent_dim), np.float32), np.array([0.5], np.float32), *args_np[:6], np.ones((1, 8), np.bool_))))}
    x0 = np.random.default_rng(0).standard_normal((1, 8, cfg.patched_latent_dim), dtype=np.float32)
    conditions["speaker+caption"]["initial_noise"] = arr(x0)
    conditions["speaker+caption"]["trajectory_cfg1"] = final_latent(dit, x0, text_out[0], np.ones((1, text_out[0].shape[1]), np.bool_), speaker_out[0], speaker_out[1], text_out[1], np.ones((1, text_out[1].shape[1]), np.bool_), (1.0, 1.0, 1.0))
    conditions["speaker+caption"]["trajectory_default"] = final_latent(dit, x0, text_out[0], np.ones((1, text_out[0].shape[1]), np.bool_), speaker_out[0], speaker_out[1], text_out[1], np.ones((1, text_out[1].shape[1]), np.bool_), (3.0, 5.0, 3.0))
    payload = {
        "schema_version": 1,
        "source": "official Irodori-TTS fixed source + PyTorch graph wrappers",
        "seed": 0,
        "threshold": {"atol": 1e-4, "rtol": 1e-3},
        "provenance": {
            "source_repository": "Aratako/Irodori-TTS",
            "source_revision": SOURCE_REVISION,
            "source_tree_sha256": source_hash,
            "model_revision": MODEL_REVISION,
            "model_file_sha256": sha256(args.checkpoint),
            "tokenizer_revision": TOKENIZER_REVISION,
            "tokenizer_file_sha256": sha256(args.tokenizer),
            "codec_repository": CODEC_REPO,
            "codec_weights_revision": CODEC_WEIGHTS_REVISION,
            "codec_code_revision": CODEC_CODE_REVISION,
            "codec_weights_file_sha256": codec_hash,
            "lock_path": "tools/irodori_export/uv.lock",
            "lock_sha256": sha256(root / "tools/irodori_export/uv.lock"),
            "generator_path": "tools/irodori_export/generate_go_fixture.py",
            "generator_sha256": sha256(Path(__file__).resolve()),
        },
        "graphs": graph_outputs,
        "graph_inputs": graph_inputs,
        "conditions": conditions,
        "duration_feature_text": feature_text,
    }
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(payload, ensure_ascii=False, separators=(",", ":")), encoding="utf-8")
    print(f"wrote {args.out} ({args.out.stat().st_size} bytes)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
