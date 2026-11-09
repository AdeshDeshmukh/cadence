from __future__ import annotations

import os
import sys
import wave
from pathlib import Path
from typing import Any

# Ensure local worker directory is in sys.path
_WORKER_DIR = Path(__file__).resolve().parent
if str(_WORKER_DIR) not in sys.path:
    sys.path.insert(0, str(_WORKER_DIR))

try:
    from ml.workers.base_worker import Worker
except ImportError:
    from base_worker import Worker

_PIPELINE: Any = None


def get_pipeline(hf_token: str | None = None) -> Any:
    global _PIPELINE
    if _PIPELINE is not None:
        return _PIPELINE

    token = hf_token or os.environ.get("HF_TOKEN") or os.environ.get("HUGGINGFACE_TOKEN")
    try:
        from pyannote.audio import Pipeline
        import torch

        pipeline = Pipeline.from_pretrained(
            "pyannote/speaker-diarization-3.1",
            use_auth_token=token,
        )
        if torch.cuda.is_available():
            pipeline.to(torch.device("cuda"))
        elif hasattr(torch.backends, "mps") and torch.backends.mps.is_available():
            pipeline.to(torch.device("mps"))

        _PIPELINE = pipeline
    except Exception:
        _PIPELINE = False

    return _PIPELINE


def get_wav_duration_ms(wav_path: str) -> int:
    """Read duration in milliseconds using the standard library wave module."""
    try:
        with wave.open(wav_path, "rb") as wf:
            frames = wf.getnframes()
            rate = wf.getframerate()
            if rate <= 0:
                return 0
            return int((frames / float(rate)) * 1000)
    except Exception:
        return 0


def fallback_diarize(wav_path: str, num_speakers: int = 2) -> list[dict[str, Any]]:
    """Heuristic / simulated speaker segmentation when pyannote is not installed or offline."""
    duration_ms = get_wav_duration_ms(wav_path)
    if duration_ms <= 0:
        duration_ms = 15000  # Default fallback 15s

    k = max(1, min(num_speakers, 10))
    turn_ms = 10000  # 10s turns
    segments: list[dict[str, Any]] = []

    curr_start = 0
    idx = 0
    while curr_start < duration_ms:
        curr_end = min(curr_start + turn_ms, duration_ms)
        spk_idx = idx % k
        segments.append({
            "start_ms": curr_start,
            "end_ms": curr_end,
            "speaker": f"SPEAKER_{spk_idx:02d}",
            "confidence": 0.85,
        })
        curr_start = curr_end
        idx += 1

    return segments


def diarize(request: dict[str, Any]) -> dict[str, Any]:
    wav_path = request.get("audio_path") or request.get("wav_path") or ""
    if not wav_path:
        raise ValueError("missing audio_path parameter")

    num_speakers = request.get("num_speakers")
    min_speakers = request.get("min_speakers")
    max_speakers = request.get("max_speakers")
    hf_token = request.get("hf_token")

    pipeline = get_pipeline(hf_token)
    if pipeline and pipeline is not False:
        try:
            kwargs: dict[str, Any] = {}
            if num_speakers is not None and int(num_speakers) > 0:
                kwargs["num_speakers"] = int(num_speakers)
            if min_speakers is not None and int(min_speakers) > 0:
                kwargs["min_speakers"] = int(min_speakers)
            if max_speakers is not None and int(max_speakers) > 0:
                kwargs["max_speakers"] = int(max_speakers)

            diarization = pipeline(wav_path, **kwargs)

            segments: list[dict[str, Any]] = []
            for turn, _, speaker in diarization.itertracks(yield_label=True):
                segments.append({
                    "start_ms": int(turn.start * 1000),
                    "end_ms": int(turn.end * 1000),
                    "speaker": str(speaker),
                    "confidence": 0.90,
                })
            return {"segments": segments, "backend": "pyannote"}
        except Exception:
            pass

    # Offline / simulated fallback
    speakers_count = 2
    if num_speakers is not None and int(num_speakers) > 0:
        speakers_count = int(num_speakers)

    segs = fallback_diarize(wav_path, speakers_count)
    return {"segments": segs, "backend": "fallback"}


def main() -> None:
    worker = Worker()
    worker.register("diarize", diarize)
    worker.run()


if __name__ == "__main__":
    main()
