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

_MODEL: Any = None


def get_silero_model() -> Any:
    global _MODEL
    if _MODEL is None:
        try:
            import torch
            model, _ = torch.hub.load(
                repo_or_dir="snakers4/silero-vad",
                model="silero_vad",
                force_reload=False,
                onnx=os.environ.get("CADENCE_VAD_ONNX", "true").lower() == "true",
            )
            _MODEL = model
        except Exception:
            _MODEL = False
    return _MODEL


def reset_state() -> None:
    model = get_silero_model()
    if model and hasattr(model, "reset_states"):
        model.reset_states()


def detect(request: dict[str, Any]) -> dict[str, Any]:
    audio = request.get("audio", [])
    if not isinstance(audio, list):
        raise ValueError("audio must be a PCM float list")

    threshold = float(request.get("threshold", 0.5))
    sample_rate = int(request.get("sample_rate", 16000))

    if not audio:
        return {"probability": 0.0, "is_speech": False}

    import numpy as np

    audio_data = np.array(audio, dtype=np.float32)

    # Resample to 16000 if different
    if sample_rate != 16000 and len(audio_data) > 0:
        target_len = int(len(audio_data) * 16000 / sample_rate)
        if target_len > 0:
            indices = np.linspace(0, len(audio_data) - 1, target_len)
            audio_data = np.interp(
                indices, np.arange(len(audio_data)), audio_data
            ).astype(np.float32)
        sample_rate = 16000

    model = get_silero_model()
    if model:
        try:
            import torch

            tensor = torch.from_numpy(audio_data)
            if len(tensor) < 512:
                tensor = torch.nn.functional.pad(tensor, (0, 512 - len(tensor)))
            prob = float(model(tensor, sample_rate).item())
            return {
                "probability": prob,
                "is_speech": prob >= threshold,
            }
        except Exception:
            pass

    # Robust fallback for environments without silero torch/onnx dependencies
    rms = float(np.sqrt(np.mean(audio_data**2))) if len(audio_data) > 0 else 0.0
    prob = min(1.0, max(0.0, rms * 15.0))
    return {
        "probability": float(prob),
        "is_speech": prob >= threshold,
    }


def handle_reset(request: dict[str, Any]) -> dict[str, Any]:
    reset_state()
    return {"reset": True}


def main() -> None:
    worker = Worker()
    worker.register("detect", detect)
    worker.register("reset", handle_reset)
    worker.run()


if __name__ == "__main__":
    main()
