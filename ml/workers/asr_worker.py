from __future__ import annotations

import os
import sys
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

_MODEL = None

def model():
    global _MODEL
    if _MODEL is None:
        from faster_whisper import WhisperModel  # type: ignore[import-untyped, import-not-found]
        model_name = os.environ.get("CADENCE_WHISPER_MODEL", "large-v3-turbo")
        models_dir = Path(os.environ.get("CADENCE_MODELS_DIR", Path.home() / ".cache" / "cadence" / "models"))
        local_path = models_dir / model_name
        model_to_load = str(local_path) if local_path.is_dir() and any(local_path.iterdir()) else model_name

        _MODEL = WhisperModel(
            model_to_load,
            device=os.environ.get("CADENCE_DEVICE", "auto"),
            compute_type=os.environ.get(
                "CADENCE_COMPUTE_TYPE", "auto"
            ),
        )
    return _MODEL

def transcribe(request: dict[str, Any]) -> dict[str, Any]:
    audio = request.get("audio")
    if not isinstance(audio, list):
        raise ValueError("audio must be a PCM float list")

    if not audio:
        return {
            "text": "",
            "language": "en",
            "confidence": 1.0,
            "words": [],
        }

    sample_rate = int(request.get("sample_rate", 16000))
    import numpy as np
    audio_data = np.array(audio, dtype=np.float32)

    if sample_rate != 16000 and len(audio_data) > 0:
        target_len = int(len(audio_data) * 16000 / sample_rate)
        if target_len > 0:
            indices = np.linspace(0, len(audio_data) - 1, target_len)
            audio_data = np.interp(indices, np.arange(len(audio_data)), audio_data).astype(np.float32)

    segments, info = model().transcribe(
        audio_data,
        beam_size=5,
        word_timestamps=True,
    )

    texts = []
    words = []
    for segment in segments:
        texts.append(segment.text)
        if hasattr(segment, "words") and segment.words:
            for w in segment.words:
                words.append({
                    "text": getattr(w, "word", ""),
                    "start_ms": int(getattr(w, "start", 0) * 1000),
                    "end_ms": int(getattr(w, "end", 0) * 1000),
                    "confidence": float(getattr(w, "probability", 1.0)),
                })

    return {
        "text": "".join(texts).strip(),
        "language": info.language,
        "confidence": info.language_probability,
        "words": words,
    }

def main() -> None:
    worker = Worker()
    worker.register("transcribe", transcribe)
    worker.run()

if __name__ == "__main__":
    main()
