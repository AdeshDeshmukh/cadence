from __future__ import annotations

import os
from typing import Any

from base_worker import Worker

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

    segments, info = model().transcribe(
        audio,
        beam_size=5,
        word_timestamps=True,
    )

    texts = []
    for segment in segments:
        texts.append(segment.text)

    return {
        "text": "".join(texts).strip(),
        "language": info.language,
        "confidence": info.language_probability,
    }

def main() -> None:
    worker = Worker()
    worker.register("transcribe", transcribe)
    worker.run()

if __name__ == "__main__":
    main()
