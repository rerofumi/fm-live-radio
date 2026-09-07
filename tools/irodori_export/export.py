"""Export the official Irodori v4.1-small components to ONNX.

The exporter deliberately keeps the graph boundary explicit.  The shared ModernBERT
backbone is emitted once by ``text_caption_encoder`` and is reused for both conditions.
No product package is imported or modified by this tool.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import shutil
import sys
import time
from pathlib import Path
from typing import Any

import torch
from torch import nn
import numpy as np
import onnx
from huggingface_hub import hf_hub_download


MODEL_REVISION = "2b28324dc263ed5e6638b3cf3dd94c82ead07b4b"
SOURCE_REVISION = "8224dafb46d0aba89209a8f905f1cb7e3299d9c1"
TOKENIZER_REVISION = "77675fc96a7e445e982e2ba90246b816efc74ec6"
CODEC_REPO = "Aratako/Semantic-DACVAE-Japanese-32dim"
CODEC_WEIGHTS_REVISION = "47376ee24834d7a05a48ebabfe3cde29b3c5e214"
CODEC_CODE_REVISION = "414c20785fc3a28373073ea8ef7a1316eeeaca6e"
OPSET = 18
EXPECTED_CHECKPOINT_SHA256 = "c85de88c01700cb53538e706f128ebcb1b8513ad21d7d0e75f58bc82cdbf89f6"
EXPECTED_TOKENIZER_SHA256 = "6a0734cf21c802169defaffe719bc2ef12bb9d0be37e54b61ed27aa89394723d"
EXPECTED_SOURCE_CONTENT_SHA256 = "f159d56bdbde0e6ada8d8fec875a4c66c540baff23425fe661ae000f6a7b437f"
EXPECTED_SOURCE_ARCHIVE_SHA256 = "e80f64d4a4b1a4db186003bc8f3416f5ecfc2e32b1b73b8b967465fba5c27c9c"
EXPECTED_CODEC_WEIGHTS_FILE_SHA256 = "db120339c5ee7eca1912cdf29bc612b947a0808e69c3cebfb4936b45a762c1d5"


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def _source_import(source: Path) -> None:
    source = source.resolve()
    if str(source) not in sys.path:
        sys.path.insert(0, str(source))


def _resolve_codec_weights(codec: str) -> Path:
    """Resolve the exact codec snapshot before DACVAE's revisionless loader runs."""
    location = str(codec).strip()
    if location == CODEC_REPO:
        return Path(hf_hub_download(
            repo_id=CODEC_REPO,
            filename="weights.pth",
            revision=CODEC_WEIGHTS_REVISION,
        ))
    path = Path(location).expanduser()
    if path.is_dir():
        path = path / "weights.pth"
    if not path.is_file():
        raise ValueError(f"codec local path must be a weights.pth file or directory: {codec}")
    return path


def _validate_fixed_inputs(
    source: Path,
    checkpoint: Path,
    tokenizer: Path,
    codec: str,
    source_archive: Path | None = None,
) -> Path:
    """Reject inputs outside the revision-pinned export contract."""
    if not source.name.endswith(SOURCE_REVISION):
        raise ValueError(f"source directory must end with fixed revision {SOURCE_REVISION}: {source}")
    source_hash = _source_tree_sha256(source)
    if source_hash != EXPECTED_SOURCE_CONTENT_SHA256:
        raise ValueError(
            "official source content SHA256 mismatch: "
            f"expected={EXPECTED_SOURCE_CONTENT_SHA256} actual={source_hash}"
        )
    if source_archive is not None:
        if not source_archive.is_file():
            raise ValueError(f"fixed source archive is missing: {source_archive}")
        archive_hash = sha256(source_archive)
        if archive_hash != EXPECTED_SOURCE_ARCHIVE_SHA256:
            raise ValueError(
                "official source archive SHA256 mismatch: "
                f"expected={EXPECTED_SOURCE_ARCHIVE_SHA256} actual={archive_hash}"
            )
    commit_metadata = source.parent / "upstream-commit.json"
    if commit_metadata.is_file():
        metadata = json.loads(commit_metadata.read_text(encoding="utf-8-sig"))
        if metadata.get("sha") != SOURCE_REVISION:
            raise ValueError(f"source metadata revision mismatch: expected={SOURCE_REVISION} actual={metadata.get('sha')!r}")
    checkpoint_hash = sha256(checkpoint)
    if checkpoint_hash != EXPECTED_CHECKPOINT_SHA256:
        raise ValueError(
            "checkpoint SHA256 mismatch for fixed v4.1-small input: "
            f"expected={EXPECTED_CHECKPOINT_SHA256} actual={checkpoint_hash}"
        )
    tokenizer_hash = sha256(tokenizer)
    if tokenizer_hash != EXPECTED_TOKENIZER_SHA256:
        raise ValueError(
            "tokenizer SHA256 mismatch for fixed ModernBERT-ja input: "
            f"expected={EXPECTED_TOKENIZER_SHA256} actual={tokenizer_hash}"
        )
    codec_weights = _resolve_codec_weights(codec)
    codec_hash = sha256(codec_weights)
    if codec_hash != EXPECTED_CODEC_WEIGHTS_FILE_SHA256:
        raise ValueError(
            "codec weights SHA256 mismatch for fixed DACVAE snapshot: "
            f"expected={EXPECTED_CODEC_WEIGHTS_FILE_SHA256} actual={codec_hash}"
        )
    return codec_weights


def _exporter_files() -> dict[str, str]:
    root = Path(__file__).resolve().parent
    names = ("export.py", "parity.py", "externalize.py", "merge_manifest.py", "pyproject.toml", "uv.lock", "mise.toml")
    return {name: sha256(root / name) for name in names if (root / name).is_file()}


def _module_state_sha256(module: nn.Module) -> str:
    h = hashlib.sha256()
    with torch.no_grad():
        for name, value in sorted(module.state_dict().items()):
            h.update(name.encode("utf-8"))
            h.update(np.ascontiguousarray(value.detach().cpu().numpy()).tobytes())
    return h.hexdigest()


def _source_tree_sha256(source: Path) -> str:
    """Hash source code/config files while excluding caches and downloaded weights."""
    extensions = {".py", ".toml", ".yaml", ".yml", ".md", ".txt", ".cfg", ".ini"}
    files = sorted(
        p for p in source.rglob("*")
        if p.is_file() and p.suffix.lower() in extensions and p.stat().st_size <= 1_000_000
        and ".cache" not in p.parts and "__pycache__" not in p.parts and ".git" not in p.parts
        and (len(p.relative_to(source).parts) == 1 or p.relative_to(source).parts[0] in {"irodori_tts", "configs", "docs"})
    )
    h = hashlib.sha256()
    for path in files:
        h.update(path.relative_to(source).as_posix().encode("utf-8"))
        h.update(b"\0")
        with path.open("rb") as stream:
            for block in iter(lambda: stream.read(1024 * 1024), b""):
                h.update(block)
    return h.hexdigest()


def _install_onnx_rope(source_module: Any) -> None:
    """Replace complex-valued RoPE with algebraically identical real ops.

    ONNX has no portable complex tensor operator.  The official implementation stores
    cos/sin as a complex tensor; packing the same pairs into a real tensor preserves the
    formula while making speaker and DiT graphs exportable.
    """
    def real_freqs(dim: int, end: int, theta: float = 10000.0) -> torch.Tensor:
        freq = 1.0 / (theta ** (torch.arange(0, dim, 2, dtype=torch.float32) / dim))
        angles = torch.outer(torch.arange(end, dtype=torch.float32), freq)
        packed = torch.empty((end, dim), dtype=torch.float32)
        packed[:, 0::2] = torch.cos(angles)
        packed[:, 1::2] = torch.sin(angles)
        return packed

    def real_apply(x: torch.Tensor, freqs: torch.Tensor) -> torch.Tensor:
        x_f = x.float()
        cos = freqs[None, :, None, 0::2]
        sin = freqs[None, :, None, 1::2]
        even = x_f[..., 0::2]
        odd = x_f[..., 1::2]
        out = torch.empty_like(x_f)
        out[..., 0::2] = even * cos - odd * sin
        out[..., 1::2] = even * sin + odd * cos
        return out.type_as(x)

    source_module.precompute_freqs_cis = real_freqs
    source_module.apply_rotary_emb = real_apply


def _load_model(source: Path, checkpoint: Path):
    _source_import(source)
    from irodori_tts.inference_runtime import _load_checkpoint_for_inference
    from irodori_tts.config import ModelConfig
    import irodori_tts.model as model_module
    from irodori_tts.model import TextToLatentRFDiT

    _install_onnx_rope(model_module)

    state, config, inference, backbone_config = _load_checkpoint_for_inference(checkpoint)
    cfg = ModelConfig(**config)
    model = TextToLatentRFDiT(
        cfg,
        pretrained_backbone_config=backbone_config,
        load_pretrained_backbone_weights=False,
    )
    missing, unexpected = model.load_state_dict(state, strict=False)
    if missing or unexpected:
        raise RuntimeError(
            "checkpoint/model state mismatch: "
            f"missing={list(missing)[:8]} unexpected={list(unexpected)[:8]}"
        )
    model.eval()
    return model, cfg, inference, backbone_config


class TextCaptionGraph(nn.Module):
    def __init__(self, model: nn.Module):
        super().__init__()
        self.model = model

    def forward(self, text_input_ids, text_mask, caption_input_ids, caption_mask):
        backbone = self.model.pretrained_text_backbone
        if backbone is None:
            raise RuntimeError("v4.1 export requires the shared pretrained text backbone")
        text = self.model.text_encoder(backbone, text_input_ids, text_mask)
        text = self.model.text_norm(text)
        caption = self.model.caption_encoder(backbone, caption_input_ids, caption_mask)
        caption = self.model.caption_norm(caption)
        return text, caption


class SpeakerGraph(nn.Module):
    def __init__(self, model: nn.Module, patch_size: int):
        super().__init__()
        self.model = model
        self.patch_size = int(patch_size)

    def forward(self, ref_latent, ref_mask):
        from irodori_tts.model import patch_sequence_with_mask

        latent, mask = patch_sequence_with_mask(ref_latent, ref_mask, self.patch_size)
        state = self.model.speaker_encoder(latent, mask)
        state = self.model.speaker_norm(state)
        return self.model._prepend_masked_mean_token(state, mask)


class DurationGraph(nn.Module):
    def __init__(self, model: nn.Module):
        super().__init__()
        self.model = model

    def forward(
        self,
        text_state,
        text_mask,
        speaker_state,
        speaker_mask,
        caption_state,
        caption_mask,
        duration_features,
        has_speaker,
        has_caption,
    ):
        # Keep every contract input alive in the traced graph.  Without this anchor,
        # an all-ones fixture lets TorchScript constant-fold masks/flags away and the
        # resulting ONNX graph cannot represent null or partial conditions.
        anchor = (
            speaker_mask.to(dtype=speaker_state.dtype).sum()
            + caption_mask.to(dtype=caption_state.dtype).sum()
            + duration_features.sum()
            + has_speaker.to(dtype=text_state.dtype).sum()
            + has_caption.to(dtype=text_state.dtype).sum()
        ) * 0.0
        text_state = text_state + anchor
        return self.model.predict_duration_log_frames(
            text_state=text_state,
            text_mask=text_mask,
            speaker_state=speaker_state,
            speaker_mask=speaker_mask,
            caption_state=caption_state,
            caption_mask=caption_mask,
            duration_features=duration_features,
            has_speaker=has_speaker,
            has_caption=has_caption,
            detach_condition=False,
        )


class DitStepGraph(nn.Module):
    def __init__(self, model: nn.Module):
        super().__init__()
        self.model = model

    def forward(
        self,
        x_t,
        t,
        text_state,
        text_mask,
        speaker_state,
        speaker_mask,
        caption_state,
        caption_mask,
        latent_mask,
    ):
        anchor = (
            text_mask.to(dtype=x_t.dtype).sum()
            + speaker_mask.to(dtype=x_t.dtype).sum()
            + caption_mask.to(dtype=x_t.dtype).sum()
            + latent_mask.to(dtype=x_t.dtype).sum()
        ) * 0.0
        x_t = x_t + anchor
        return self.model.forward_with_encoded_conditions(
            x_t=x_t,
            t=t,
            text_state=text_state,
            text_mask=text_mask,
            speaker_state=speaker_state,
            speaker_mask=speaker_mask,
            caption_state=caption_state,
            caption_mask=caption_mask,
            latent_mask=latent_mask,
        )


class CodecEncoderGraph(nn.Module):
    def __init__(self, codec: nn.Module):
        super().__init__()
        self.codec = codec

    def forward(self, waveform):
        # dacvae._pad has a data-dependent branch (and the legacy exporter would
        # trace the 2048-sample fixture's non-multiple path as a constant).  Express
        # the same right-reflect padding with Shape/Arange/IndexSelect so multiples
        # such as the 480000-sample narrator remain unpadded in ONNX too.
        hop = int(self.codec.hop_length)
        length = torch.onnx.operators.shape_as_tensor(waveform)[-1]
        remainder = length % hop
        pad = (hop - remainder) % hop
        # Flip the whole signal and remove its endpoint; taking the first ``pad``
        # samples via the dynamic index below reproduces F.pad(..., "reflect")
        # without a traced negative slice tied to the 2048-sample fixture.
        reflected = torch.flip(waveform, dims=(-1,))[..., 1:]
        padded = torch.cat((waveform, reflected), dim=-1)
        indices = torch.arange(length + pad, device=waveform.device)
        padded = torch.index_select(padded, -1, indices)
        z = self.codec.encoder(padded)
        mean, _scale = self.codec.quantizer.in_proj(z).chunk(2, dim=1)
        return mean.transpose(1, 2).contiguous()


class CodecDecoderGraph(nn.Module):
    def __init__(self, codec: nn.Module):
        super().__init__()
        self.codec = codec

    def forward(self, latent):
        return self.codec.decode(latent.transpose(1, 2).contiguous())


def _axes(names: list[str], dynamic: dict[str, dict[int, str]]) -> dict[str, dict[int, str]]:
    return {name: dims for name, dims in dynamic.items() if name in names}


def _export(
    module: nn.Module,
    inputs: tuple[torch.Tensor, ...],
    output_path: Path,
    input_names: list[str],
    output_names: list[str],
    dynamic_axes: dict[str, dict[int, str]],
) -> dict[str, Any]:
    output_path.parent.mkdir(parents=True, exist_ok=True)
    started = time.perf_counter()
    # dynamo=False is intentional: the official model contains custom attention and
    # exporting through the classic tracer gives a stable opset-18 graph on torch 2.10.
    torch.onnx.export(
        module,
        inputs,
        str(output_path),
        input_names=input_names,
        output_names=output_names,
        dynamic_axes=_axes(input_names + output_names, dynamic_axes),
        opset_version=OPSET,
        do_constant_folding=True,
        dynamo=False,
        external_data=True,
    )
    return {
        "path": output_path.name,
        "inputs": input_names,
        "outputs": output_names,
        "dynamic_axes": _axes(input_names + output_names, dynamic_axes),
        "opset": OPSET,
        "bytes": output_path.stat().st_size,
        "sha256": sha256(output_path),
        "seconds": round(time.perf_counter() - started, 3),
        "io": _onnx_io(output_path),
    }


def _onnx_io(path: Path) -> dict[str, list[dict[str, Any]]]:
    graph = onnx.load(str(path), load_external_data=False).graph

    def describe(value: Any) -> dict[str, Any]:
        tensor = value.type.tensor_type
        dims = [dim.dim_param if dim.dim_param else int(dim.dim_value) for dim in tensor.shape.dim]
        return {"name": value.name, "dtype": onnx.TensorProto.DataType.Name(tensor.elem_type), "shape": dims}

    return {"inputs": [describe(x) for x in graph.input], "outputs": [describe(x) for x in graph.output]}


def _samples(cfg, device: torch.device):
    b = 1
    ids = torch.ones((b, 8), dtype=torch.long, device=device)
    mask = torch.ones((b, 8), dtype=torch.bool, device=device)
    cids = torch.ones((b, 12), dtype=torch.long, device=device)
    cmask = torch.ones((b, 12), dtype=torch.bool, device=device)
    ref = torch.zeros((b, 17, cfg.latent_dim * cfg.latent_patch_size), dtype=torch.float32, device=device)
    rmask = torch.ones((b, 17), dtype=torch.bool, device=device)
    text_state = torch.zeros((b, 8, cfg.text_dim), dtype=torch.float32, device=device)
    cap_state = torch.zeros((b, 12, cfg.caption_dim_resolved), dtype=torch.float32, device=device)
    speaker_state = torch.zeros((b, 5, cfg.speaker_dim), dtype=torch.float32, device=device)
    speaker_mask = torch.ones((b, 5), dtype=torch.bool, device=device)
    features = torch.zeros((b, cfg.duration_aux_dim), dtype=torch.float32, device=device)
    flags = torch.ones((b,), dtype=torch.bool, device=device)
    x = torch.zeros((b, 21, cfg.patched_latent_dim), dtype=torch.float32, device=device)
    t = torch.full((b,), 0.5, dtype=torch.float32, device=device)
    lmask = torch.ones((b, 21), dtype=torch.bool, device=device)
    return {
        "text_caption_encoder": (ids, mask, cids, cmask),
        "speaker_encoder": (ref, rmask),
        "duration_predictor": (text_state, mask, speaker_state, speaker_mask, cap_state, cmask, features, flags, flags),
        "dit_step": (x, t, text_state, mask, speaker_state, speaker_mask, cap_state, cmask, lmask),
    }


def _manifest_base(args, checkpoint: Path, tokenizer: Path, cfg, inference, backbone_config) -> dict[str, Any]:
    return {
        "schema_version": 2,
        "model_family": "irodori-v4",
        "model_release": "v4.1-small",
        "model_revision": MODEL_REVISION,
        "official_source": {
            "repository": "Aratako/Irodori-TTS",
            "revision": SOURCE_REVISION,
            "content_sha256": _source_tree_sha256(args.source),
            "archive": "third_party/irodori-v4-research/upstream.zip",
            "archive_sha256": EXPECTED_SOURCE_ARCHIVE_SHA256,
        },
        "exporter": {
            "repository": "fm-live-radio/tools/irodori_export",
            "python": platform.python_version(),
            "torch": torch.__version__,
            "opset": OPSET,
            "device": str(args.device),
            "precision": "float32",
            "files_sha256": _exporter_files(),
        },
        "tokenizer": {
            "repository": cfg.text_tokenizer_repo,
            "revision": TOKENIZER_REVISION,
            "path": "tokenizer/tokenizer.json",
            "sha256": sha256(tokenizer),
            "special_ids": {
                "bos": int(backbone_config.get("bos_token_id", 1)),
                "eos": int(backbone_config.get("eos_token_id", 2)),
                "pad": int(backbone_config.get("pad_token_id", 3)),
            },
            "text_max_length": int(inference["max_text_len"]),
            "caption_max_length": int(inference["max_caption_len"]),
            "max_ref_seconds": float(inference["ref_max_seconds"]),
            "license": {
                "spdx": "MIT",
                "source": "https://huggingface.co/sbintuitions/modernbert-ja-310m/blob/77675fc96a7e445e982e2ba90246b816efc74ec6/LICENSE",
                "copyright": "Copyright (c) 2025 SB Intuitions",
                "note": "配布するtokenizer.jsonはこのModernBERT-ja checkpoint由来。原文license/NOTICEを上記revisionで参照。",
            },
        },
        "codec": {
            "repository": CODEC_REPO,
            "weights_revision": CODEC_WEIGHTS_REVISION,
            "code_revision": CODEC_CODE_REVISION,
            "weights_sha256": EXPECTED_CODEC_WEIGHTS_FILE_SHA256,
            "sample_rate": 48000,
            "latent_dim": int(cfg.latent_dim),
            # Semantic DACVAE v1 uses 48 kHz / 25 Hz latent frames.
            "hop_length": 1920,
            "deterministic_encode": True,
            "deterministic_decode": True,
        },
        "licenses": {
            "official_code": {"spdx": "MIT", "source": "https://github.com/Aratako/Irodori-TTS/blob/8224dafb46d0aba89209a8f905f1cb7e3299d9c1/LICENSE"},
            "model": {"spdx": "MIT", "source": "https://huggingface.co/Aratako/Irodori-TTS-v4.1-Small"},
            "codec_code": {"spdx": "Apache-2.0", "source": "https://github.com/facebookresearch/dacvae/blob/414c20785fc3a28373073ea8ef7a1316eeeaca6e/LICENSE"},
            "codec_model": {"spdx": "MIT", "source": "https://huggingface.co/Aratako/Semantic-DACVAE-Japanese-32dim"},
            "tokenizer_modernbert": {"spdx": "MIT", "source": "https://huggingface.co/sbintuitions/modernbert-ja-310m/blob/77675fc96a7e445e982e2ba90246b816efc74ec6/LICENSE", "copyright": "Copyright (c) 2025 SB Intuitions", "distribution_target": "model/irodori-v4.1/tokenizer/tokenizer.json"},
            "distribution_targets": ["model/irodori-v4.1/*.onnx", "model/irodori-v4.1/*.onnx.data", "model/irodori-v4.1/tokenizer/tokenizer.json"],
            "development_only": ["tools/irodori_export/.venv", "tools/irodori_export/uv.lock"],
            "notice_sources": ["https://github.com/Aratako/Irodori-TTS/blob/8224dafb46d0aba89209a8f905f1cb7e3299d9c1/LICENSE", "https://github.com/facebookresearch/dacvae/blob/414c20785fc3a28373073ea8ef7a1316eeeaca6e/LICENSE"],
            "notice_files": [
                {"component": "ModernBERT-ja tokenizer", "path": "docs/plan_20260907_irodori_v4/evidence/licenses/modernbert-ja-LICENSE.txt", "sha256": "284353c80e0d52c06e97af62fd40e4dd1d253fa28f5d1e756121ea1d5d441509"},
                {"component": "DACVAE code", "path": "docs/plan_20260907_irodori_v4/evidence/licenses/dacvae-LICENSE.txt", "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30"},
            ],
        },
        "conditions": {
            "text_dim": int(cfg.text_dim),
            "caption_dim": int(cfg.caption_dim_resolved),
            "speaker_dim": int(cfg.speaker_dim),
            "speaker_patch_size": int(cfg.speaker_patch_size),
            "duration_aux_dim": int(cfg.duration_aux_dim),
            "duration_architecture": cfg.duration_architecture,
            "duration_token_init_frames": float(cfg.duration_token_init_frames),
            "latent_dim": int(cfg.latent_dim),
            "latent_patch_size": int(cfg.latent_patch_size),
        },
        "checkpoint": {
            "path": str(checkpoint),
            "sha256": sha256(checkpoint),
            "bytes": checkpoint.stat().st_size,
        },
        "graphs": {},
        "export": {"started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "errors": []},
    }


def parse_args() -> argparse.Namespace:
    root = Path(__file__).resolve().parents[2]
    p = argparse.ArgumentParser()
    p.add_argument("--checkpoint", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/model.safetensors")
    p.add_argument("--source", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-8224dafb46d0aba89209a8f905f1cb7e3299d9c1")
    p.add_argument("--source-archive", type=Path, default=root / "third_party/irodori-v4-research/upstream.zip")
    p.add_argument("--tokenizer", type=Path, default=root / "third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/tokenizer/tokenizer.json")
    p.add_argument("--output", type=Path, default=root / "model/irodori-v4.1")
    p.add_argument("--codec", default=CODEC_REPO, help="local DACVAE path or fixed Hugging Face repo")
    p.add_argument("--device", choices=("cpu", "cuda"), default="cpu")
    p.add_argument("--graphs", nargs="*", default=["all"], choices=("all", "text_caption_encoder", "speaker_encoder", "duration_predictor", "dit_step", "codec"))
    p.add_argument("--strict", action=argparse.BooleanOptionalAction, default=True)
    return p.parse_args()


def main() -> int:
    args = parse_args()
    if "all" in args.graphs:
        args.graphs = ["text_caption_encoder", "speaker_encoder", "duration_predictor", "dit_step", "codec"]
    for path in (args.checkpoint, args.source, args.tokenizer):
        if not path.exists():
            raise FileNotFoundError(f"required fixed input is missing: {path}")
    codec_weights = _validate_fixed_inputs(args.source, args.checkpoint, args.tokenizer, args.codec, args.source_archive)
    if args.device == "cuda" and not torch.cuda.is_available():
        raise RuntimeError("CUDA requested but torch.cuda.is_available() is false; refusing CPU fallback")
    device = torch.device(args.device)
    args.output.mkdir(parents=True, exist_ok=True)
    model, cfg, inference, backbone_config = _load_model(args.source, args.checkpoint)
    model.to(device)
    tokenizer_out = args.output / "tokenizer" / "tokenizer.json"
    tokenizer_out.parent.mkdir(parents=True, exist_ok=True)
    if tokenizer_out.resolve() != args.tokenizer.resolve():
        shutil.copy2(args.tokenizer, tokenizer_out)
    manifest = _manifest_base(args, args.checkpoint, tokenizer_out, cfg, inference, backbone_config)
    samples = _samples(cfg, device)
    fixtures: dict[str, Any] = {}
    for graph_name, tensors in samples.items():
        fixtures[graph_name] = [
            {
                "dtype": str(t.dtype),
                "shape": list(t.shape),
                "sha256": hashlib.sha256(
                    np.ascontiguousarray(t.detach().cpu().numpy()).tobytes()
                ).hexdigest(),
            }
            for t in tensors
        ]
    fixture_path = args.output / "fixtures.json"
    fixture_path.write_text(json.dumps(fixtures, ensure_ascii=False, indent=2), encoding="utf-8")
    manifest["fixtures"] = {
        "path": fixture_path.name,
        "sha256": sha256(fixture_path),
        "seed": 0,
        "conditions": ["reference", "caption", "reference+caption", "null"],
    }
    common_bool = {"text_mask": {0: "batch", 1: "text_seq"}, "caption_mask": {0: "batch", 1: "caption_seq"}}

    jobs: dict[str, tuple[nn.Module, tuple[torch.Tensor, ...], list[str], list[str], dict[str, dict[int, str]]]] = {
        "text_caption_encoder": (
            TextCaptionGraph(model), samples["text_caption_encoder"],
            ["text_input_ids", "text_mask", "caption_input_ids", "caption_mask"], ["text_state", "caption_state"],
            {"text_input_ids": {0: "batch", 1: "text_seq"}, "text_mask": {0: "batch", 1: "text_seq"}, "caption_input_ids": {0: "batch", 1: "caption_seq"}, "caption_mask": {0: "batch", 1: "caption_seq"}, "text_state": {0: "batch", 1: "text_seq"}, "caption_state": {0: "batch", 1: "caption_seq"}},
        ),
        "speaker_encoder": (
            SpeakerGraph(model, cfg.speaker_patch_size), samples["speaker_encoder"], ["ref_latent", "ref_mask"], ["speaker_state", "speaker_mask"],
            {"ref_latent": {0: "batch", 1: "ref_seq"}, "ref_mask": {0: "batch", 1: "ref_seq"}, "speaker_state": {0: "batch", 1: "speaker_seq"}, "speaker_mask": {0: "batch", 1: "speaker_seq"}},
        ),
        "duration_predictor": (
            DurationGraph(model), samples["duration_predictor"], ["text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "duration_features", "has_speaker", "has_caption"], ["log_frames"],
            {"text_state": {0: "batch", 1: "text_seq"}, "text_mask": {0: "batch", 1: "text_seq"}, "speaker_state": {0: "batch", 1: "speaker_seq"}, "speaker_mask": {0: "batch", 1: "speaker_seq"}, "caption_state": {0: "batch", 1: "caption_seq"}, "caption_mask": {0: "batch", 1: "caption_seq"}, "duration_features": {0: "batch"}, "has_speaker": {0: "batch"}, "has_caption": {0: "batch"}, "log_frames": {0: "batch"}},
        ),
        "dit_step": (
            DitStepGraph(model), samples["dit_step"], ["x_t", "t", "text_state", "text_mask", "speaker_state", "speaker_mask", "caption_state", "caption_mask", "latent_mask"], ["v_pred"],
            {"x_t": {0: "batch", 1: "latent_seq"}, "t": {0: "batch"}, "text_state": {0: "batch", 1: "text_seq"}, "text_mask": {0: "batch", 1: "text_seq"}, "speaker_state": {0: "batch", 1: "speaker_seq"}, "speaker_mask": {0: "batch", 1: "speaker_seq"}, "caption_state": {0: "batch", 1: "caption_seq"}, "caption_mask": {0: "batch", 1: "caption_seq"}, "latent_mask": {0: "batch", 1: "latent_seq"}, "v_pred": {0: "batch", 1: "latent_seq"}},
        ),
    }
    # Codec is loaded before the export loop so both codec graphs are actually emitted.
    export_names = [name for name in args.graphs if name != "codec"]
    if "codec" in args.graphs:
        try:
            # dacvae pins protobuf<3.20 while ONNX needs a modern protobuf.  Keep
            # the exporter lock authoritative and append only the official source
            # environment for its codec module when requested by the task runner.
            official_site = os.environ.get("IRODORI_OFFICIAL_SITE_PACKAGES")
            if official_site and official_site not in sys.path:
                sys.path.append(official_site)
            from irodori_tts.codec import DACVAECodec

            # DACVAECodec.load accepts a local path but downloads a repo id without
            # a revision. Resolve and hash the pinned snapshot above, then pass the
            # resulting file so the loader cannot silently select a moving HEAD.
            codec = DACVAECodec.load(repo_id=str(codec_weights), device=args.device, deterministic_encode=True, deterministic_decode=True, normalize_db=None)
            jobs["codec_encoder"] = (CodecEncoderGraph(codec.model), (torch.zeros((1, 1, 2048), device=device),), ["waveform"], ["latent"], {"waveform": {0: "batch", 2: "samples"}, "latent": {0: "batch", 1: "latent_seq"}})
            jobs["codec_decoder"] = (CodecDecoderGraph(codec.model), (torch.zeros((1, 8, cfg.latent_dim), device=device),), ["latent"], ["waveform"], {"latent": {0: "batch", 1: "latent_seq"}, "waveform": {0: "batch", 2: "samples"}})
            manifest["codec"].update({"sample_rate": int(codec.sample_rate), "latent_dim": int(codec.latent_dim), "device": str(codec.device), "dtype": str(codec.dtype), "source": str(args.codec), "weights_path": str(codec_weights), "weights_file_sha256": sha256(codec_weights), "weights_sha256": _module_state_sha256(codec.model)})
            export_names.extend(["codec_encoder", "codec_decoder"])
        except Exception as exc:
            detail = {"graph": "codec", "error": repr(exc), "kind": type(exc).__name__}
            manifest["export"]["errors"].append(detail)
            if args.strict:
                raise
    for name in export_names:
        module, inputs, input_names, output_names, axes = jobs[name]
        path = args.output / f"{name}.onnx"
        try:
            with torch.inference_mode():
                manifest["graphs"][name] = _export(module, inputs, path, input_names, output_names, axes)
        except Exception as exc:
            detail = {"graph": name, "error": repr(exc), "kind": type(exc).__name__}
            manifest["export"]["errors"].append(detail)
            if args.strict:
                manifest["export"]["finished_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
                (args.output / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
                raise
    # Record external tensor-data files after export. ORT needs each file beside its graph.
    external: list[dict[str, Any]] = []
    for item in args.output.iterdir():
        if item.is_file() and item.name.endswith(".data"):
            external.append({"path": item.name, "bytes": item.stat().st_size, "sha256": sha256(item)})
    manifest["external_data"] = external
    manifest["export"]["finished_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    (args.output / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(manifest, ensure_ascii=False, indent=2))
    return 0 if not manifest["export"]["errors"] else 2


if __name__ == "__main__":
    raise SystemExit(main())
