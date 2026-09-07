"""Independent ORT checks for the graphs emitted by export.py.

The report distinguishes a real CPU/CUDA execution from an unavailable provider.  It
also executes alternate sequence lengths, so a dynamic_axes declaration alone cannot
make a check pass.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import shutil
import sys
import tempfile
import time
from pathlib import Path
from typing import Any

import numpy as np
import onnx
import onnxruntime as ort
import torch

sys.path.insert(0, r'E:\programming\AI_generative\fm-live-radio\tools\irodori_export')
from export import (_load_model, _samples, CodecDecoderGraph, CodecEncoderGraph,
                    DurationGraph, DitStepGraph, SpeakerGraph, TextCaptionGraph,
                    CODEC_REPO, SOURCE_REVISION, EXPECTED_CHECKPOINT_SHA256,
                    EXPECTED_TOKENIZER_SHA256, _validate_fixed_inputs, _module_state_sha256,
                    _source_tree_sha256)


ATOL = 1e-4
RTOL = 1e-3


def _np(x: torch.Tensor) -> np.ndarray:
    return x.detach().cpu().numpy()


def _provider_report() -> dict[str, Any]:
    available = ort.get_available_providers()
    return {
        "available": available,
        "cpu": "CPUExecutionProvider" in available,
        "cuda": "CUDAExecutionProvider" in available,
    }


def _session(path: Path, provider: str, *, profile: bool = False) -> ort.InferenceSession:
    options = ort.SessionOptions()
    options.enable_profiling = bool(profile)
    if profile:
        options.profile_file_prefix = f"irodori_{path.stem}_{provider[:3]}_{time.time_ns()}"
    if provider == "CUDAExecutionProvider":
        # Keep ORT CUDA math aligned with the FP32 PyTorch reference.  In
        # particular, TensorCore TF32 would make otherwise valid comparisons
        # fail or hide precision regressions.
        return ort.InferenceSession(
            str(path), sess_options=options,
            providers=[("CUDAExecutionProvider", {"device_id": 0, "use_tf32": 0})]
        )
    return ort.InferenceSession(str(path), sess_options=options, providers=[provider])


def _inputs(session: ort.InferenceSession, tensors: tuple[torch.Tensor, ...], names: list[str]) -> dict[str, np.ndarray]:
    return {n: _np(t) for n, t in zip(names, tensors)}


def _compare_outputs(expected: tuple[torch.Tensor, ...], actual: list[np.ndarray]) -> tuple[bool, list[dict[str, Any]]]:
    if len(expected) != len(actual):
        return False, [{"error": f"output count mismatch: torch={len(expected)} ort={len(actual)}"}]
    details: list[dict[str, Any]] = []
    passed = True
    for index, (torch_value, ort_value) in enumerate(zip(expected, actual)):
        expected_np = _np(torch_value)
        actual_np = np.asarray(ort_value)
        if expected_np.shape != actual_np.shape:
            details.append({"index": index, "ok": False, "error": f"shape mismatch: torch={list(expected_np.shape)} ort={list(actual_np.shape)}"})
            passed = False
            continue
        if expected_np.dtype.kind == "b" or actual_np.dtype.kind == "b":
            equal = bool(np.array_equal(expected_np.astype(bool), actual_np.astype(bool)))
            details.append({"index": index, "ok": equal, "comparison": "exact_bool"})
        else:
            expected_cmp = expected_np.astype(np.float32, copy=False)
            actual_cmp = actual_np.astype(np.float32, copy=False)
            finite = bool(np.isfinite(expected_cmp).all() and np.isfinite(actual_cmp).all())
            delta = np.abs(expected_cmp - actual_cmp)
            max_abs = float(np.max(delta)) if delta.size else 0.0
            max_rel = float(np.max(delta / np.maximum(np.abs(expected_cmp), 1e-12))) if delta.size else 0.0
            equal = finite and bool(np.allclose(expected_cmp, actual_cmp, atol=ATOL, rtol=RTOL))
            details.append({"index": index, "ok": equal, "max_abs": max_abs, "max_rel": max_rel, "comparison": "allclose"})
        passed = passed and equal
    return passed, details


def _run_graph(name: str, path: Path, model: torch.nn.Module, tensors: tuple[torch.Tensor, ...], names: list[str], provider: str) -> dict[str, Any]:
    result: dict[str, Any] = {"provider": provider, "graph": name, "path": path.name}
    try:
        session = _session(path, provider)
        ort_out = session.run(None, _inputs(session, tensors, names))
        with torch.inference_mode():
            torch_out = model(*tensors)
        if not isinstance(torch_out, tuple):
            torch_out = (torch_out,)
        ok, errors = _compare_outputs(torch_out, ort_out)
        result.update({"ok": ok, "errors": errors, "output_shapes": [list(np.asarray(x).shape) for x in ort_out]})
    except Exception as exc:
        result.update({"ok": False, "error": repr(exc), "kind": type(exc).__name__})
    return result


def _missing_external_check(path: Path) -> dict[str, Any]:
    """Copy a graph to a temporary folder and remove its external data if present."""
    with tempfile.TemporaryDirectory(prefix="irodori-ort-missing-") as temp:
        dst = Path(temp) / path.name
        shutil.copy2(path, dst)
        graph = onnx.load(str(path), load_external_data=False)
        locations = {str(item.value) for init in graph.graph.initializer for item in init.external_data if item.key == "location"}
        data = [path.parent / location for location in locations if (path.parent / location).is_file()]
        if not locations:
            return {"ok": False, "status": "no_external_data", "reason": "graph has no external initializers"}
        if not data:
            return {"ok": False, "status": "missing_external_data", "reason": "manifest/graph declares external data but no sidecar exists"}
        for item in data:
            shutil.copy2(item, Path(temp) / item.name)
        (Path(temp) / data[0].name).unlink()
        try:
            ort.InferenceSession(str(dst), providers=["CPUExecutionProvider"])
        except Exception as exc:
            return {"ok": True, "status": "rejected", "error": repr(exc)}
        return {"ok": False, "status": "accepted_missing_external_data"}


def _process_rss_mib() -> float | None:
    """Read Windows process RSS without adding a non-pinned psutil dependency."""
    try:
        import psutil
        return float(psutil.Process().memory_info().rss) / (1024.0 * 1024.0)
    except Exception:
        pass
    if os.name != "nt":
        return None
    import ctypes
    from ctypes import wintypes

    class Counters(ctypes.Structure):
        _fields_ = [("cb", wintypes.DWORD), ("page_fault_count", wintypes.DWORD),
                    ("peak_working_set", ctypes.c_size_t), ("working_set", ctypes.c_size_t),
                    ("quota_peak_paged_pool", ctypes.c_size_t), ("quota_paged_pool", ctypes.c_size_t),
                    ("quota_peak_nonpaged_pool", ctypes.c_size_t), ("quota_nonpaged_pool", ctypes.c_size_t),
                    ("pagefile_usage", ctypes.c_size_t), ("peak_pagefile_usage", ctypes.c_size_t)]
    counters = Counters()
    counters.cb = ctypes.sizeof(counters)
    handle = ctypes.windll.kernel32.GetCurrentProcess()
    if ctypes.windll.psapi.GetProcessMemoryInfo(handle, ctypes.byref(counters), counters.cb):
        return float(counters.working_set) / (1024.0 * 1024.0)
    return None


def _resource_report(model_dir: Path) -> dict[str, Any]:
    files = [p for p in model_dir.iterdir() if p.is_file() and p.suffix in {".onnx", ".data"}]
    by_hash: dict[str, int] = {}
    hash_counts: dict[str, int] = {}
    for item in files:
        digest = hashlib.sha256(item.read_bytes()).hexdigest() if item.stat().st_size < 10_000_000 else None
        if digest:
            by_hash[digest] = item.stat().st_size
            hash_counts[digest] = hash_counts.get(digest, 0) + 1
    report: dict[str, Any] = {
        "bundle_bytes": sum(item.stat().st_size for item in files),
        "graph_bytes": sum(item.stat().st_size for item in files if item.suffix == ".onnx"),
        "external_data_bytes": sum(item.stat().st_size for item in files if item.suffix == ".data"),
        "unique_small_file_bytes": sum(by_hash.values()),
        "shared_weight_hashes": [digest for digest, count in hash_counts.items() if count > 1],
        "rss_mib": _process_rss_mib(),
    }
    if torch.cuda.is_available():
        report.update({
            "cuda_device": torch.cuda.get_device_name(0),
            "cuda_allocated_peak_mib": float(torch.cuda.max_memory_allocated() / 2**20),
            "cuda_reserved_peak_mib": float(torch.cuda.max_memory_reserved() / 2**20),
        })
    else:
        report["cuda_device"] = None
    return report


REQUIRED_GRAPHS = ("text_caption_encoder", "speaker_encoder", "duration_predictor", "dit_step", "codec_encoder", "codec_decoder")


def _validate_manifest(manifest: dict[str, Any], model_dir: Path) -> list[str]:
    errors: list[str] = []
    if manifest.get("schema_version") != 2:
        errors.append(f"unsupported manifest schema: {manifest.get('schema_version')!r}")
    for key in ("official_source", "tokenizer", "codec", "checkpoint", "graphs", "external_data", "licenses"):
        if key not in manifest:
            errors.append(f"manifest missing required section: {key}")
    checkpoint = Path(manifest.get("checkpoint", {}).get("path", ""))
    if not checkpoint.is_file():
        errors.append(f"checkpoint missing: {checkpoint}")
    elif manifest.get("checkpoint", {}).get("sha256") != hashlib.sha256(checkpoint.read_bytes()).hexdigest():
        errors.append("checkpoint hash differs from manifest")
    tokenizer = model_dir / str(manifest.get("tokenizer", {}).get("path", "tokenizer/tokenizer.json"))
    if not tokenizer.is_file():
        errors.append(f"tokenizer missing: {tokenizer}")
    elif manifest.get("tokenizer", {}).get("sha256") != hashlib.sha256(tokenizer.read_bytes()).hexdigest():
        errors.append("tokenizer hash differs from manifest")
    declared_external = {str(row.get("path")): row for row in manifest.get("external_data", [])}
    for name in REQUIRED_GRAPHS:
        graph = manifest.get("graphs", {}).get(name)
        path = model_dir / str(graph.get("path", f"{name}.onnx")) if graph else model_dir / f"{name}.onnx"
        if not path.is_file():
            errors.append(f"missing graph: {name} ({path.name})")
            continue
        if graph and graph.get("sha256") != hashlib.sha256(path.read_bytes()).hexdigest():
            errors.append(f"graph hash differs from manifest: {name}")
        if not graph:
            errors.append(f"manifest missing graph entry: {name}")
        model = onnx.load(str(path), load_external_data=False)
        locations = {str(item.value) for init in model.graph.initializer for item in init.external_data if item.key == "location"}
        if not locations:
            errors.append(f"graph has no external data: {name}")
        for location in locations:
            data_path = model_dir / location
            row = declared_external.get(location)
            if not data_path.is_file():
                errors.append(f"missing external data: {location}")
            elif row and row.get("sha256") != hashlib.sha256(data_path.read_bytes()).hexdigest():
                errors.append(f"external data hash differs from manifest: {location}")
            elif not row:
                errors.append(f"external data missing manifest entry: {location}")
    return errors


def _ort_full_smoke(
    root: Path,
    source: Path,
    tokenizer_path: Path,
    model_dir: Path,
    cfg: Any,
    manifest: dict[str, Any],
    graph_specs: dict[str, Any],
) -> dict[str, Any]:
    """Run the complete natural-text request with every neural stage in ORT.

    Tokenization, duration feature construction and the Euler arithmetic are the
    official Python utilities.  All neural calls (including reference encode and
    final decode) go through the exported ONNX graphs.  This deliberately does not
    call ``InferenceRuntime.synthesize``: that method is a useful oracle, but it
    executes the PyTorch networks and therefore cannot prove the ORT request path.
    """
    ref = root / "narrator" / "narrator_01.wav"
    if not ref.is_file():
        return {"status": "failed", "error": f"reference narrator missing: {ref}"}
    try:
        _source = source.resolve()
        if str(_source) not in sys.path:
            sys.path.insert(0, str(_source))
        from irodori_tts.duration import build_duration_features
        from irodori_tts.tokenizer import PretrainedTextTokenizer
        import soundfile as sf
        import torchaudio

        if not torch.cuda.is_available():
            return {"status": "failed", "error": "CUDA unavailable for ORT smoke"}

        text = "こんにちは。今日はラジオの音声合成を確認します。"
        text_max_len = 256
        sample_rate = int(manifest.get("codec", {}).get("sample_rate", 48000))
        hop_length = int(manifest.get("codec", {}).get("hop_length", 480))
        started = time.perf_counter()
        tok = PretrainedTextTokenizer.from_pretrained(
            str(tokenizer_path.parent), add_bos=True, local_files_only=True
        )
        text_ids_t, text_mask_t = tok.batch_encode([text], max_length=text_max_len)
        # An empty caption is the official default.  Keep its IDs (the graph input
        # remains exercised) while making its mask explicitly null as synthesize does.
        caption_ids_t, caption_mask_t = tok.batch_encode([""], max_length=text_max_len)
        caption_mask_t.zero_()
        text_ids = text_ids_t.numpy().astype(np.int64, copy=False)
        text_mask = text_mask_t.numpy().astype(np.bool_, copy=False)
        caption_ids = caption_ids_t.numpy().astype(np.int64, copy=False)
        caption_mask = caption_mask_t.numpy().astype(np.bool_, copy=False)

        wav, sr = sf.read(str(ref), dtype="float32", always_2d=True)
        waveform = torch.from_numpy(wav.T).mean(dim=0, keepdim=True)
        if int(sr) != sample_rate:
            waveform = torchaudio.functional.resample(waveform, int(sr), sample_rate)
        # Match the explicit smoke request: max_ref_seconds=120, normalize_db=None,
        # ensure_max=False.  The checked-in narrator is about ten seconds, so the
        # complete reference is retained after mono/resampling.
        waveform = waveform.contiguous().unsqueeze(0).numpy()

        paths = {name: model_dir / f"{name}.onnx" for name in REQUIRED_GRAPHS}
        sessions = {name: _session(path, "CUDAExecutionProvider", profile=True) for name, path in paths.items()}
        calls: dict[str, int] = {name: 0 for name in REQUIRED_GRAPHS}
        oracle_checks: dict[str, Any] = {}

        def oracle(name: str, values: dict[str, np.ndarray], actual: list[np.ndarray]) -> None:
            wrapper = graph_specs[name][0]
            tensors = tuple(torch.from_numpy(np.asarray(values[key])).to("cpu") for key in graph_specs[name][2])
            with torch.inference_mode():
                expected = wrapper(*tensors)
            if not isinstance(expected, tuple):
                expected = (expected,)
            ok, details = _compare_outputs(expected, actual)
            oracle_checks[name + ("_step_" + str(calls[name]) if name == "dit_step" else "")] = {"ok": ok, "details": details, "input_sha256": hashlib.sha256(b"".join(np.asarray(values[key]).tobytes() for key in graph_specs[name][2])).hexdigest()}

        def run(name: str, values: dict[str, np.ndarray]) -> list[np.ndarray]:
            calls[name] += 1
            return sessions[name].run(None, values)

        text_values = {
            "text_input_ids": text_ids, "text_mask": text_mask,
            "caption_input_ids": caption_ids, "caption_mask": caption_mask,
        }
        text_out = run("text_caption_encoder", text_values)
        oracle("text_caption_encoder", text_values, text_out)
        text_state, caption_state = text_out
        codec_values = {"waveform": waveform}
        codec_out = run("codec_encoder", codec_values)
        oracle("codec_encoder", codec_values, codec_out)
        ref_latent = codec_out[0].astype(np.float32, copy=False)
        ref_mask = np.ones((1, ref_latent.shape[1]), dtype=np.bool_)
        speaker_values = {"ref_latent": ref_latent, "ref_mask": ref_mask}
        speaker_out = run("speaker_encoder", speaker_values)
        oracle("speaker_encoder", speaker_values, speaker_out)
        speaker_state, speaker_mask = speaker_out
        duration_features = build_duration_features(
            [text], token_counts=text_mask.sum(axis=1), max_text_len=text_max_len,
            has_speaker=np.array([True], dtype=np.bool_),
        ).numpy().astype(np.float32, copy=False)
        has_speaker = np.array([True], dtype=np.bool_)
        has_caption = np.array([False], dtype=np.bool_)
        duration_values = {
            "text_state": text_state, "text_mask": text_mask,
            "speaker_state": speaker_state, "speaker_mask": speaker_mask,
            "caption_state": caption_state, "caption_mask": caption_mask,
            "duration_features": duration_features, "has_speaker": has_speaker,
            "has_caption": has_caption,
        }
        duration_out = run("duration_predictor", duration_values)
        oracle("duration_predictor", duration_values, duration_out)
        log_frames = duration_out[0]
        pred_frames = float(np.expm1(np.asarray(log_frames, dtype=np.float32)).mean())
        latent_steps = int(round(pred_frames))
        min_frames = max(1, math.ceil(0.5 * sample_rate / hop_length))
        max_frames = max(1, math.floor(30.0 * sample_rate / hop_length))
        latent_steps = max(min_frames, min(max_frames, latent_steps))
        expected_log = graph_specs['duration_predictor'][0](*(torch.from_numpy(np.asarray(duration_values[k])).to('cpu') for k in graph_specs['duration_predictor'][2])).detach().numpy()
        expected_frames = max(min_frames, min(max_frames, int(round(float(np.expm1(expected_log).mean())))))
        oracle_checks['duration_final_frames'] = {'ok': abs(expected_frames-latent_steps) <= 1, 'torch_final_frames': expected_frames, 'ort_final_frames': latent_steps, 'delta': abs(expected_frames-latent_steps)}
        patched_steps = math.ceil(latent_steps / int(cfg.latent_patch_size))

        # Match the official FP32 CUDA generator and independent CFG schedule.  The
        # empty caption is not an enabled guidance branch because its mask is null.
        generator = torch.Generator(device="cuda").manual_seed(0)
        x = torch.randn((1, patched_steps, int(cfg.patched_latent_dim)), device="cuda", dtype=torch.float32, generator=generator).cpu().numpy()
        text_zero = np.zeros_like(text_state)
        text_mask_zero = np.zeros_like(text_mask)
        speaker_zero = np.zeros_like(speaker_state)
        speaker_mask_zero = np.zeros_like(speaker_mask)
        schedule = (1.0 - np.linspace(0.0, 1.0, 41, dtype=np.float32)) * 0.999
        trajectory_hash = hashlib.sha256()
        for index in range(40):
            t = float(schedule[index])
            t_next = float(schedule[index + 1])
            if 0.5 <= t <= 1.0:
                # cond, text-uncond, speaker-uncond; one ORT batch call is exactly
                # the official independent CFG bundle and keeps noise identical.
                batch_x = np.concatenate([x, x, x], axis=0).astype(np.float32, copy=False)
                dit_values = {
                    "x_t": batch_x, "t": np.full((3,), t, dtype=np.float32),
                    "text_state": np.concatenate([text_state, text_zero, text_state]),
                    "text_mask": np.concatenate([text_mask, text_mask_zero, text_mask]),
                    "speaker_state": np.concatenate([speaker_state, speaker_state, speaker_zero]),
                    "speaker_mask": np.concatenate([speaker_mask, speaker_mask, speaker_mask_zero]),
                    "caption_state": np.concatenate([caption_state] * 3),
                    "caption_mask": np.concatenate([caption_mask] * 3),
                    "latent_mask": np.ones((3, patched_steps), dtype=np.bool_),
                }
                out = run("dit_step", dit_values)
                oracle("dit_step", dit_values, out)
                cond, text_u, speaker_u = np.asarray(out[0], dtype=np.float32).reshape(3, patched_steps, -1)
                velocity = cond + 3.0 * (cond - text_u) + 5.0 * (cond - speaker_u)
            else:
                dit_values = {
                    "x_t": x, "t": np.array([t], dtype=np.float32),
                    "text_state": text_state, "text_mask": text_mask,
                    "speaker_state": speaker_state, "speaker_mask": speaker_mask,
                    "caption_state": caption_state, "caption_mask": caption_mask,
                    "latent_mask": np.ones((1, patched_steps), dtype=np.bool_),
                }
                out = run("dit_step", dit_values)
                oracle('dit_step', dit_values, out)
                velocity = out[0]
            trajectory_hash.update(np.asarray(x, dtype=np.float32).tobytes())
            x = x + np.float32(t_next - t) * np.asarray(velocity, dtype=np.float32)
        latent = x[:, :latent_steps, :].astype(np.float32, copy=False)
        decode_values = {"latent": latent}
        decode_out = run("codec_decoder", decode_values)
        oracle("codec_decoder", decode_values, decode_out)
        audio = np.asarray(decode_out[0], dtype=np.float32)
        audio_1d = audio[0, 0]
        output = model_dir / "ort-full-smoke.wav"
        sf.write(str(output), audio_1d, sample_rate, subtype="PCM_16")
        profiles: dict[str, Any] = {}
        for name, session in sessions.items():
            try:
                profile_path = session.end_profiling()
                profile = json.loads(Path(profile_path).read_text(encoding="utf-8"))
                providers = sorted({str(item.get("args", {}).get("provider")) for item in profile if item.get("args", {}).get("provider")})
                profiles[name] = {"path": Path(profile_path).name, "providers": providers, "events": len(profile)}
            except Exception as exc:
                profiles[name] = {"error": repr(exc)}
        elapsed = time.perf_counter() - started
        oracle_ok = bool(oracle_checks) and all(row.get("ok") is True for row in oracle_checks.values())
        return {
            "status": "generated",
            "path": output.name, "method": "official tokenizer+duration features+Euler arithmetic; text_caption_encoder->codec_encoder->speaker_encoder->duration_predictor->40-step independent CFG dit_step->codec_decoder (all ORT)",
            "all_neural_stages_onnx": True,
            "reference_options": {"max_ref_seconds": 120.0, "normalize_db": None, "ensure_max": False, "tail_trim_samples": 0},
            "elapsed_seconds": round(elapsed, 3), "sample_rate": sample_rate,
            "samples": int(audio_1d.size), "seconds": float(audio_1d.size / sample_rate),
            "finite": bool(np.isfinite(audio_1d).all()), "peak": float(np.abs(audio_1d).max()),
            "rms": float(np.sqrt(np.mean(audio_1d * audio_1d))), "used_seed": 0,
            "predicted_frames": pred_frames, "latent_steps": latent_steps,
            "final_frame_delta": abs(float(audio_1d.size / hop_length) - latent_steps),
            "reference_latent_steps": int(ref_latent.shape[1]),
            "dit_steps": 40, "trajectory_sha256": trajectory_hash.hexdigest(),
            "ort_session_runs": calls, "oracle_checks": oracle_checks, "cuda_profiles": profiles,
            "oracle_pass": oracle_ok,
            "sha256": hashlib.sha256(output.read_bytes()).hexdigest(),
        }
    except Exception as exc:
        return {"status": "failed", "error": repr(exc), "kind": type(exc).__name__}


def parse_args() -> argparse.Namespace:
    root = Path(r'E:\programming\AI_generative\fm-live-radio')
    p = argparse.ArgumentParser()
    p.add_argument("--model-dir", type=Path, default=root / "model/irodori-v4.1")
    p.add_argument("--checkpoint", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/model.safetensors")
    p.add_argument("--source", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-8224dafb46d0aba89209a8f905f1cb7e3299d9c1")
    p.add_argument("--tokenizer", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/tokenizer/tokenizer.json")
    p.add_argument("--codec", default=CODEC_REPO)
    p.add_argument("--device", choices=("cpu", "cuda"), default="cpu")
    p.add_argument("--graphs", nargs="*", default=["all"], choices=("all", "text_caption_encoder", "speaker_encoder", "duration_predictor", "dit_step", "codec_encoder", "codec_decoder"))
    p.add_argument("--report", type=Path, default=None)
    p.add_argument("--cpu-only", action="store_true", help="run only CPUExecutionProvider; CUDA results remain explicitly unverified")
    return p.parse_args()


def main() -> int:
    args = parse_args()
    if "all" in args.graphs:
        args.graphs = ["text_caption_encoder", "speaker_encoder", "duration_predictor", "dit_step", "codec_encoder", "codec_decoder"]
    manifest_path = args.model_dir / "manifest.json"
    # Disable TF32 in the PyTorch reference before creating any CUDA tensors.
    if torch.cuda.is_available():
        torch.backends.cuda.matmul.allow_tf32 = False
        torch.backends.cudnn.allow_tf32 = False
    report: dict[str, Any] = {"schema_version": 2, "started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "threshold": {"atol": ATOL, "rtol": RTOL}, "providers": _provider_report(), "math": {"pytorch_tf32": False, "ort_cuda_use_tf32": 0}, "graphs": {}, "dynamic_length": {}, "speaker_ref_boundary": {}, "codec_waveform_boundary": {}, "external_data_missing": {}, "four_conditions": {}}
    if not manifest_path.exists():
        report["error"] = f"missing manifest: {manifest_path}"
        print(json.dumps(report, ensure_ascii=False, indent=2))
        return 2
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    report["manifest_sha256"] = hashlib.sha256(manifest_path.read_bytes()).hexdigest()
    try:
        _validate_fixed_inputs(args.source, args.checkpoint, args.tokenizer, args.codec)
    except Exception as exc:
        report["input_validation"] = {"ok": False, "error": repr(exc)}
        report["error"] = "fixed input validation failed"
        report["finished_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        report["resources"] = _resource_report(args.model_dir)
        (args.report or (args.model_dir / "parity.json")).write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
        print(json.dumps(report, ensure_ascii=False, indent=2))
        return 2
    manifest_errors = _validate_manifest(manifest, args.model_dir)
    declared_source_hash = manifest.get("official_source", {}).get("content_sha256")
    if declared_source_hash and declared_source_hash != _source_tree_sha256(args.source):
        manifest_errors.append("official source content hash differs from manifest")
    report["manifest_validation"] = {"ok": not manifest_errors, "errors": manifest_errors}
    if manifest_errors:
        report["error"] = "manifest validation failed"
    try:
        model, cfg, _inf, _backbone = _load_model(args.source, args.checkpoint)
        device = torch.device(args.device)
        model.to(device).eval()
        samples = _samples(cfg, device)
        graph_specs = {
            "text_caption_encoder": (TextCaptionGraph(model), samples["text_caption_encoder"], ["text_input_ids", "text_mask", "caption_input_ids", "caption_mask"]),
            "speaker_encoder": (SpeakerGraph(model, cfg.speaker_patch_size), samples["speaker_encoder"], ["ref_latent", "ref_mask"]),
            "duration_predictor": (DurationGraph(model), samples["duration_predictor"], ["text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "duration_features", "has_speaker", "has_caption"]),
            "dit_step": (DitStepGraph(model), samples["dit_step"], ["x_t", "t", "text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "latent_mask"]),
        }
        official_site = os.environ.get("IRODORI_OFFICIAL_SITE_PACKAGES")
        if official_site and official_site not in sys.path:
            sys.path.append(official_site)
        try:
            from irodori_tts.codec import DACVAECodec

            codec = DACVAECodec.load(repo_id=manifest.get("codec", {}).get("repository", "Aratako/Semantic-DACVAE-Japanese-32dim"), device=str(device), deterministic_encode=True, deterministic_decode=True, normalize_db=None)
            codec_hash = _module_state_sha256(codec.model)
            declared_codec_hash = manifest.get("codec", {}).get("weights_sha256")
            report["codec_load"] = {"status": "loaded", "repository": manifest.get("codec", {}).get("repository"), "weights_sha256": codec_hash, "hash_match": declared_codec_hash is None or declared_codec_hash == codec_hash}
            if declared_codec_hash is not None and declared_codec_hash != codec_hash:
                report["error"] = "codec weight hash differs from manifest"
            graph_specs["codec_encoder"] = (CodecEncoderGraph(codec.model), (torch.zeros((1, 1, 2048), device=device),), ["waveform"])
            graph_specs["codec_decoder"] = (CodecDecoderGraph(codec.model), (torch.zeros((1, 8, cfg.latent_dim), device=device),), ["latent"])
        except Exception as exc:
            report["codec_load"] = {"status": "failed", "error": repr(exc)}
        if "codec_load" not in report:
            report["codec_load"] = {"status": "loaded", "repository": manifest.get("codec", {}).get("repository")}
        for required in args.graphs:
            if required not in graph_specs:
                report["graphs"][required] = [{"provider": "unavailable", "graph": required, "ok": False, "error": "graph wrapper unavailable (codec load failed or missing implementation)"}]
                report["external_data_missing"][required] = {"ok": False, "status": "not_checked", "reason": "graph wrapper unavailable"}
        for name, (wrapper, tensors, names) in graph_specs.items():
            if name not in args.graphs:
                continue
            path = args.model_dir / f"{name}.onnx"
            if not path.exists():
                report["graphs"][name] = {"status": "missing"}
                continue
            providers = ["CPUExecutionProvider"]
            if report["providers"]["cuda"] and not args.cpu_only:
                providers.append("CUDAExecutionProvider")
            report["graphs"][name] = [_run_graph(name, path, wrapper, tensors, names, provider) for provider in providers]
            report["external_data_missing"][name] = _missing_external_check(path)
        if "codec_decoder" in graph_specs and "codec_decoder" in args.graphs and (args.model_dir / "codec_decoder.onnx").exists():
            try:
                sess = _session(args.model_dir / "codec_decoder.onnx", "CPUExecutionProvider")
                latent = np.zeros((1, 8, cfg.latent_dim), dtype=np.float32)
                audio = np.asarray(sess.run(None, {"latent": latent})[0])
                import soundfile as sf

                wav_path = args.model_dir / "ort-codec-smoke.wav"
                sf.write(str(wav_path), audio[0, 0], int(manifest.get("codec", {}).get("sample_rate", 48000)))
                report["short_ort_wav"] = {"status": "generated", "path": wav_path.name, "sample_rate": int(manifest.get("codec", {}).get("sample_rate", 48000)), "samples": int(audio.shape[-1]), "peak": float(np.max(np.abs(audio))), "rms": float(np.sqrt(np.mean(audio * audio))), "method": "codec_decoder_random_latent"}
            except Exception as exc:
                report["short_ort_wav"] = {"status": "failed", "error": repr(exc)}
        # Dynamic execution uses distinct text/ref/latent lengths from export samples.
        dyn = {
            "text_caption_encoder": (torch.ones((1, 5), dtype=torch.long, device=device), torch.ones((1, 5), dtype=torch.bool, device=device), torch.ones((1, 9), dtype=torch.long, device=device), torch.ones((1, 9), dtype=torch.bool, device=device)),
            "speaker_encoder": (torch.zeros((1, 9, cfg.latent_dim * cfg.latent_patch_size), device=device), torch.ones((1, 9), dtype=torch.bool, device=device)),
            "dit_step": (torch.zeros((1, 13, cfg.patched_latent_dim), device=device), torch.full((1,), 0.25, device=device), torch.zeros((1, 5, cfg.text_dim), device=device), torch.ones((1, 5), dtype=torch.bool, device=device), torch.zeros((1, 3, cfg.speaker_dim), device=device), torch.ones((1, 3), dtype=torch.bool, device=device), torch.zeros((1, 9, cfg.caption_dim_resolved), device=device), torch.ones((1, 9), dtype=torch.bool, device=device), torch.ones((1, 13), dtype=torch.bool, device=device)),
            "duration_predictor": (torch.zeros((1, 5, cfg.text_dim), device=device), torch.ones((1, 5), dtype=torch.bool, device=device), torch.zeros((1, 3, cfg.speaker_dim), device=device), torch.ones((1, 3), dtype=torch.bool, device=device), torch.zeros((1, 9, cfg.caption_dim_resolved), device=device), torch.ones((1, 9), dtype=torch.bool, device=device), torch.zeros((1, cfg.duration_aux_dim), device=device), torch.zeros((1,), dtype=torch.bool, device=device), torch.zeros((1,), dtype=torch.bool, device=device)),
        }
        for name, tensors in dyn.items():
            if name not in args.graphs:
                continue
            path = args.model_dir / f"{name}.onnx"
            if not path.exists():
                report["dynamic_length"][name] = {"status": "missing"}
                continue
            spec = graph_specs[name]
            try:
                provider_results = []
                for provider in ["CPUExecutionProvider"] + (["CUDAExecutionProvider"] if report["providers"]["cuda"] and not args.cpu_only else []):
                    sess = _session(path, provider)
                    out = sess.run(None, _inputs(sess, tensors, spec[2]))
                    with torch.inference_mode():
                        expected = spec[0](*tensors)
                    if not isinstance(expected, tuple):
                        expected = (expected,)
                    ok, errors = _compare_outputs(expected, out)
                    provider_results.append({"provider": provider, "status": "executed", "ok": ok, "errors": errors, "shapes": [list(np.asarray(x).shape) for x in out]})
                report["dynamic_length"][name] = provider_results
            except Exception as exc:
                report["dynamic_length"][name] = {"status": "failed", "error": repr(exc)}
        # The official patcher uses floor/truncate and rejects lengths below one
        # complete speaker patch.  Exercise the invalid minimums (1,3) and the
        # valid patch/endpoint cases (4,5,7,8,9,17).
        if "speaker_encoder" in args.graphs and (args.model_dir / "speaker_encoder.onnx").exists():
            try:
                boundary_rows: dict[str, Any] = {}
                for length in (1, 3, 4, 5, 7, 8, 9, 17):
                    rows: dict[str, Any] = {}
                    tensors = (torch.zeros((1, length, cfg.latent_dim * cfg.latent_patch_size), device=device), torch.ones((1, length), dtype=torch.bool, device=device))
                    for provider in ["CPUExecutionProvider"] + (["CUDAExecutionProvider"] if report["providers"]["cuda"] and not args.cpu_only else []):
                        path = args.model_dir / "speaker_encoder.onnx"
                        sess = _session(path, provider)
                        if length < int(cfg.speaker_patch_size):
                            ort_error = None
                            torch_error = None
                            try:
                                sess.run(None, _inputs(sess, tensors, ["ref_latent", "ref_mask"]))
                            except Exception as exc:
                                ort_error = repr(exc)
                            try:
                                with torch.inference_mode():
                                    graph_specs["speaker_encoder"][0](*tensors)
                            except Exception as exc:
                                torch_error = repr(exc)
                            rows[provider] = {"ok": ort_error is not None and torch_error is not None, "status": "expected_rejection", "ort_error": ort_error, "torch_error": torch_error}
                        else:
                            actual = sess.run(None, _inputs(sess, tensors, ["ref_latent", "ref_mask"]))
                            with torch.inference_mode():
                                expected = graph_specs["speaker_encoder"][0](*tensors)
                            if not isinstance(expected, tuple):
                                expected = (expected,)
                            ok, errors = _compare_outputs(expected, actual)
                            rows[provider] = {"ok": ok, "status": "executed", "errors": errors, "output_shapes": [list(np.asarray(x).shape) for x in actual], "expected_patched_length": int(length // cfg.speaker_patch_size)}
                    boundary_rows[str(length)] = rows
                report["speaker_ref_boundary"] = boundary_rows
            except Exception as exc:
                report["speaker_ref_boundary"] = {"status": "failed", "error": repr(exc)}
        # Exercise all four independent condition combinations through the exported
        # duration graph.  The state tensors are produced by the exported encoders;
        # masks and has_* flags are changed per case, never inferred from names.
        if "duration_predictor" in args.graphs and (args.model_dir / "duration_predictor.onnx").exists():
            try:
                dit_graph = DitStepGraph(model)
                dit_x = np.zeros((1, 8, cfg.patched_latent_dim), dtype=np.float32)
                dit_t = np.array([0.5], dtype=np.float32)
                cases = {}
                for label, use_speaker, use_caption in (("null", False, False), ("speaker", True, False), ("caption", False, True), ("speaker+caption", True, True)):
                    provider_rows: dict[str, Any] = {}
                    for provider in ["CPUExecutionProvider"] + (["CUDAExecutionProvider"] if report["providers"]["cuda"] and not args.cpu_only else []):
                        text_sess = _session(args.model_dir / "text_caption_encoder.onnx", provider)
                        text_state, caption_state = text_sess.run(None, _inputs(text_sess, samples["text_caption_encoder"], graph_specs["text_caption_encoder"][2]))
                        speaker_sess = _session(args.model_dir / "speaker_encoder.onnx", provider)
                        speaker_state, speaker_mask_np = speaker_sess.run(None, _inputs(speaker_sess, samples["speaker_encoder"], graph_specs["speaker_encoder"][2]))
                        duration_sess = _session(args.model_dir / "duration_predictor.onnx", provider)
                        dit_sess = _session(args.model_dir / "dit_step.onnx", provider)
                        text_mask_np = np.ones((1, text_state.shape[1]), dtype=np.bool_)
                        caption_mask_np = np.ones((1, caption_state.shape[1]), dtype=np.bool_) if use_caption else np.zeros((1, caption_state.shape[1]), dtype=np.bool_)
                        speaker_mask_case = speaker_mask_np if use_speaker else np.zeros_like(speaker_mask_np)
                        features_np = np.zeros((1, cfg.duration_aux_dim), dtype=np.float32)
                        args_np = {"text_state": text_state, "text_mask": text_mask_np, "speaker_state": speaker_state, "speaker_mask": speaker_mask_case, "caption_state": caption_state, "caption_mask": caption_mask_np, "duration_features": features_np, "has_speaker": np.array([use_speaker], dtype=np.bool_), "has_caption": np.array([use_caption], dtype=np.bool_)}
                        ort_frames = float(duration_sess.run(None, args_np)[0][0])
                        torch_args = tuple(torch.from_numpy(v).to(device) for v in (text_state, text_mask_np, speaker_state, speaker_mask_case, caption_state, caption_mask_np, features_np, np.array([use_speaker], dtype=np.bool_), np.array([use_caption], dtype=np.bool_)))
                        with torch.inference_mode():
                            torch_frames = float(DurationGraph(model)(*torch_args)[0])
                        dit_inputs = {"x_t": dit_x, "t": dit_t, "text_state": text_state, "text_mask": text_mask_np, "speaker_state": speaker_state, "speaker_mask": speaker_mask_case, "caption_state": caption_state, "caption_mask": caption_mask_np, "latent_mask": np.ones((1, 8), dtype=np.bool_)}
                        dit_ort = dit_sess.run(None, dit_inputs)
                        with torch.inference_mode():
                            dit_torch = dit_graph(*(torch.from_numpy(dit_inputs[k]).to(device) for k in ("x_t", "t", "text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "latent_mask")))
                        dit_ok, dit_errors = _compare_outputs((dit_torch,), dit_ort)
                        duration_ok = bool(np.allclose(ort_frames, torch_frames, atol=ATOL, rtol=RTOL))
                        frame_delta = abs(float(np.expm1(ort_frames)) - float(np.expm1(torch_frames)))
                        provider_rows[provider] = {"ort_log_frames": ort_frames, "torch_log_frames": torch_frames, "frame_delta": frame_delta, "duration_pass": duration_ok, "dit_pass": dit_ok, "dit_errors": dit_errors, "pass": duration_ok and dit_ok and frame_delta <= 1.0}
                    cases[label] = {"has_speaker": use_speaker, "has_caption": use_caption, "providers": provider_rows, "frame_delta": max((float(row["frame_delta"]) for row in provider_rows.values()), default=999.0), "pass": bool(provider_rows) and all(row.get("pass") is True for row in provider_rows.values())}
                report["four_conditions"] = cases
            except Exception as exc:
                report["four_conditions"] = {"status": "failed", "error": repr(exc)}
        # Dynamic codec lengths are executed explicitly, and a one-step text->DiT->
        # codec chain produces a short non-silent WAV for boundary smoke coverage.
        if "codec_encoder" in args.graphs and (args.model_dir / "codec_encoder.onnx").exists():
            try:
                provider_results = []
                for provider in ["CPUExecutionProvider"] + (["CUDAExecutionProvider"] if report["providers"]["cuda"] and not args.cpu_only else []):
                    enc = _session(args.model_dir / "codec_encoder.onnx", provider)
                    waveform = np.zeros((1, 1, 2400), dtype=np.float32)
                    z = enc.run(None, {"waveform": waveform})[0]
                    with torch.inference_mode():
                        expected_z = graph_specs["codec_encoder"][0](torch.from_numpy(waveform).to(device))
                    ok, errors = _compare_outputs((expected_z,), [z])
                    provider_results.append({"provider": provider, "status": "executed", "ok": ok, "errors": errors, "shapes": [list(np.asarray(z).shape)]})
                report["dynamic_length"]["codec_encoder"] = provider_results
            except Exception as exc:
                report["dynamic_length"]["codec_encoder"] = {"status": "failed", "error": repr(exc)}
        if "codec_decoder" in args.graphs and (args.model_dir / "codec_decoder.onnx").exists():
            try:
                latent_dynamic = np.zeros((1, 5, cfg.latent_dim), dtype=np.float32)
                provider_results = []
                for provider in ["CPUExecutionProvider"] + (["CUDAExecutionProvider"] if report["providers"]["cuda"] and not args.cpu_only else []):
                    dec = _session(args.model_dir / "codec_decoder.onnx", provider)
                    audio_dynamic = dec.run(None, {"latent": latent_dynamic})[0]
                    with torch.inference_mode():
                        expected_audio = graph_specs["codec_decoder"][0](torch.from_numpy(latent_dynamic).to(device))
                    ok, errors = _compare_outputs((expected_audio,), [audio_dynamic])
                    provider_results.append({"provider": provider, "status": "executed", "ok": ok, "errors": errors, "shapes": [list(np.asarray(audio_dynamic).shape)]})
                report["dynamic_length"]["codec_decoder"] = provider_results
            except Exception as exc:
                report["dynamic_length"]["codec_decoder"] = {"status": "failed", "error": repr(exc)}
        # Codec padding is a dynamic convolution boundary.  Check both sides of a
        # hop multiple, including the complete narrator length, on CPU and CUDA.
        if "codec_encoder" in args.graphs and (args.model_dir / "codec_encoder.onnx").exists():
            try:
                codec_boundary: dict[str, Any] = {}
                path = args.model_dir / "codec_encoder.onnx"
                for length in (1919, 1920, 1921, 479999, 480000, 480001):
                    tensors = (torch.zeros((1, 1, length), device=device),)
                    rows: dict[str, Any] = {}
                    for provider in ["CPUExecutionProvider"] + (["CUDAExecutionProvider"] if report["providers"]["cuda"] and not args.cpu_only else []):
                        sess = _session(path, provider)
                        actual = sess.run(None, {"waveform": np.zeros((1, 1, length), dtype=np.float32)})
                        with torch.inference_mode():
                            expected = graph_specs["codec_encoder"][0](*tensors)
                        if not isinstance(expected, tuple):
                            expected = (expected,)
                        ok, errors = _compare_outputs(expected, actual)
                        rows[provider] = {"ok": ok, "errors": errors, "torch_shape": [list(x.shape) for x in expected], "ort_shape": [list(np.asarray(x).shape) for x in actual]}
                    codec_boundary[str(length)] = rows
                report["codec_waveform_boundary"] = codec_boundary
            except Exception as exc:
                report["codec_waveform_boundary"] = {"status": "failed", "error": repr(exc)}
        if all(name in args.graphs for name in ("text_caption_encoder", "speaker_encoder", "dit_step", "codec_decoder")):
            try:
                txt_sess = _session(args.model_dir / "text_caption_encoder.onnx", "CPUExecutionProvider")
                txt, cap = txt_sess.run(None, _inputs(txt_sess, samples["text_caption_encoder"], graph_specs["text_caption_encoder"][2]))
                sp_sess = _session(args.model_dir / "speaker_encoder.onnx", "CPUExecutionProvider")
                sp, spm = sp_sess.run(None, _inputs(sp_sess, samples["speaker_encoder"], graph_specs["speaker_encoder"][2]))
                d_sess = _session(args.model_dir / "dit_step.onnx", "CPUExecutionProvider")
                latent = np.zeros((1, 8, cfg.patched_latent_dim), dtype=np.float32)
                dit = d_sess.run(None, {"x_t": latent, "t": np.array([0.5], dtype=np.float32), "text_state": txt, "text_mask": np.ones((1, txt.shape[1]), dtype=np.bool_), "speaker_state": sp, "speaker_mask": spm, "caption_state": cap, "caption_mask": np.ones((1, cap.shape[1]), dtype=np.bool_), "latent_mask": np.ones((1, latent.shape[1]), dtype=np.bool_)})[0]
                dec_sess = _session(args.model_dir / "codec_decoder.onnx", "CPUExecutionProvider")
                audio = np.asarray(dec_sess.run(None, {"latent": dit.astype(np.float32)})[0])
                import soundfile as sf

                wav_path = args.model_dir / "ort-text-dit-codec-smoke.wav"
                sf.write(str(wav_path), audio[0, 0], int(manifest.get("codec", {}).get("sample_rate", 48000)))
                report["short_ort_wav"] = {"status": "generated", "path": wav_path.name, "sample_rate": int(manifest.get("codec", {}).get("sample_rate", 48000)), "samples": int(audio.shape[-1]), "peak": float(np.max(np.abs(audio))), "rms": float(np.sqrt(np.mean(audio * audio))), "method": "text_caption_encoder->dit_step(one_step)->codec_decoder"}
            except Exception as exc:
                report["short_ort_wav"] = {"status": "failed", "error": repr(exc)}
        report["ort_smoke"] = _ort_full_smoke(
            args.model_dir.parents[1], args.source, args.tokenizer, args.model_dir,
            cfg, manifest, graph_specs,
        )
    except Exception as exc:
        report["error"] = repr(exc)
    report["finished_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    report_path = args.report or (args.model_dir / "parity.json")
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))
    graph_results = [r for rows in report["graphs"].values() if isinstance(rows, list) for r in rows]
    all_graphs_ok = len(report["graphs"]) == len(REQUIRED_GRAPHS) and all(r.get("ok") is True for r in graph_results)
    dynamic_results = [r for rows in report["dynamic_length"].values() if isinstance(rows, list) for r in rows]
    dynamic_ok = len(report["dynamic_length"]) == len(REQUIRED_GRAPHS) and bool(dynamic_results) and all(r.get("status") == "executed" and r.get("ok") is True for r in dynamic_results)
    conditions = report.get("four_conditions", {})
    conditions_ok = isinstance(conditions, dict) and all(
        isinstance(row, dict) and row.get("pass") is True and float(row.get("frame_delta", 999.0)) <= 1.0
        for row in conditions.values()
    ) and len(conditions) == 4
    boundary = report.get("speaker_ref_boundary", {})
    boundary_ok = isinstance(boundary, dict) and all(
        isinstance(rows, dict) and rows and all(item.get("ok") is True for item in rows.values())
        for rows in boundary.values()
    ) and set(boundary) == {"1", "3", "4", "5", "7", "8", "9", "17"}
    codec_boundary = report.get("codec_waveform_boundary", {})
    codec_boundary_ok = isinstance(codec_boundary, dict) and all(
        isinstance(rows, dict) and rows and all(item.get("ok") is True for item in rows.values())
        for rows in codec_boundary.values()
    ) and set(codec_boundary) == {"1919", "1920", "1921", "479999", "480000", "480001"}
    external_ok = len(report["external_data_missing"]) == len(REQUIRED_GRAPHS) and all(
        row.get("status") == "rejected" and row.get("ok") is True
        for row in report["external_data_missing"].values() if isinstance(row, dict)
    )
    smoke = report.get("ort_smoke", {})
    smoke_ok = smoke.get("status") == "generated" and smoke.get("finite") is True and smoke.get("peak", 0.0) > 0 and smoke.get("rms", 0.0) > 0 and float(smoke.get("final_frame_delta", 999.0)) <= 1.0 and smoke.get("all_neural_stages_onnx", True) is True and smoke.get("oracle_pass") is True
    required_cuda = report["providers"].get("cuda") is True and not args.cpu_only
    if not required_cuda:
        report["cuda_unverified"] = "CUDAExecutionProvider is unavailable in this locked ORT environment; CUDA parity remains unverified"
    report["checks"] = {"graphs": all_graphs_ok, "dynamic_length": dynamic_ok, "speaker_ref_boundary": boundary_ok, "codec_waveform_boundary": codec_boundary_ok, "four_conditions": conditions_ok, "external_data_missing": external_ok, "ort_smoke": smoke_ok, "cuda_required": required_cuda}
    report["resources"] = _resource_report(args.model_dir)
    report_path = args.report or (args.model_dir / "parity.json")
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0 if not report.get("error") and all(report["checks"].values()) and report.get("manifest_validation", {}).get("ok") is True else 2


if __name__ == "__main__":
    raise SystemExit(main())

