from __future__ import annotations

import os
from typing import Any

try:
    from base_worker import Worker
except ImportError:
    from ml.workers.base_worker import Worker

_MODEL = None

def model():
    global _MODEL
    if _MODEL is None:
        from faster_whisper import WhisperModel
        _MODEL = WhisperModel(
            os.environ.get(
                "CADENCE_WHISPER_MODEL",
                "large-v3-turbo",
            ),
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

    import numpy as np
    audio_data = np.array(audio, dtype=np.float32)

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
