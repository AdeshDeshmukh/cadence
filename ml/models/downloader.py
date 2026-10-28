from __future__ import annotations

import argparse
import hashlib
import os
import sys
from pathlib import Path
from urllib.request import urlretrieve

SUPPORTED_MODELS: dict[str, dict[str, str]] = {
    "tiny": {"repo": "Systran/faster-whisper-tiny"},
    "tiny.en": {"repo": "Systran/faster-whisper-tiny.en"},
    "base": {"repo": "Systran/faster-whisper-base"},
    "base.en": {"repo": "Systran/faster-whisper-base.en"},
    "small": {"repo": "Systran/faster-whisper-small"},
    "small.en": {"repo": "Systran/faster-whisper-small.en"},
    "medium": {"repo": "Systran/faster-whisper-medium"},
    "medium.en": {"repo": "Systran/faster-whisper-medium.en"},
    "large-v3-turbo": {"repo": "deepdml/faster-whisper-large-v3-turbo-ct2"},
}

DEFAULT_CACHE_DIR = Path.home() / ".cache" / "cadence" / "models"

def verify_sha256(file_path: Path, expected_hash: str) -> bool:
    hasher = hashlib.sha256()
    with open(file_path, "rb") as f:
        while chunk := f.read(65536):
            hasher.update(chunk)
    return hasher.hexdigest().lower() == expected_hash.lower()

def download_model(
    model_name: str,
    output_dir: str | Path | None = None,
) -> Path:
    if model_name not in SUPPORTED_MODELS:
        raise ValueError(
            f"Unsupported model: {model_name}. Supported: {list(SUPPORTED_MODELS.keys())}"
        )

    target_dir = Path(output_dir) if output_dir else DEFAULT_CACHE_DIR
    target_dir.mkdir(parents=True, exist_ok=True)
    model_dir = target_dir / model_name

    try:
        from faster_whisper import download_model as fw_download  # type: ignore[import-untyped, import-not-found]
    except ImportError as exc:
        raise RuntimeError(
            "faster-whisper is required for model downloading: pip install faster-whisper"
        ) from exc

    model_path = fw_download(
        SUPPORTED_MODELS[model_name]["repo"],
        output_dir=str(model_dir),
    )
    result_path = Path(model_path)
    if not result_path.exists():
        raise FileNotFoundError(f"Model download failed: directory not found at {result_path}")

    return result_path

def main() -> None:
    parser = argparse.ArgumentParser(
        description="Cadence Whisper Model Downloader and Cache Manager"
    )
    parser.add_argument(
        "--model",
        choices=list(SUPPORTED_MODELS.keys()),
        default="base.en",
        help="Whisper model to download (default: base.en)",
    )
    parser.add_argument(
        "--dest",
        type=str,
        default=str(DEFAULT_CACHE_DIR),
        help=f"Target directory for model cache (default: {DEFAULT_CACHE_DIR})",
    )
    parser.add_argument(
        "--list",
        action="store_true",
        help="List all supported models",
    )

    args = parser.parse_args()

    if args.list:
        print("Supported Cadence Whisper Models:")
        for name, meta in SUPPORTED_MODELS.items():
            print(f"  - {name:<16} (HuggingFace repo: {meta['repo']})")
        return

    path = download_model(args.model, args.dest)
    print(f"Model ready at: {path}")

if __name__ == "__main__":
    main()
