from __future__ import annotations
import json
import sys
from typing import Any, Callable

Handler = Callable[[dict[str, Any]], dict[str, Any]]

class Worker:
    def __init__(self) -> None:
        self.handlers: dict[str, Handler] = {}

    def register(self, name: str, handler: Handler) -> None:
        self.handlers[name] = handler

    def run(self) -> None:
        for line in sys.stdin:
            if not line.strip():
                continue

            request: dict[str, Any] = {}
            try:
                request = json.loads(line)
                method = request.get("method", "")
                if method == "ping":
                    response = {
                        "id": request.get("id", ""),
                        "ok": True,
                        "result": "pong",
                    }
                elif method not in self.handlers:
                    response = {
                        "id": request.get("id", ""),
                        "ok": False,
                        "error": f"unknown method: {method}",
                    }
                else:
                    response = {
                        "id": request.get("id", ""),
                        "ok": True,
                        **self.handlers[method](request),
                    }
            except Exception as exc:
                response = {
                    "id": request.get("id", ""),
                    "ok": False,
                    "error": str(exc),
                }

            print(json.dumps(response), flush=True)
