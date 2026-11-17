import { listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";
import { StreamingAccumulator, subscribePartialText, type PartialTextPayload } from "./streaming";

export type OverlayState =
  | "idle"
  | "recording"
  | "transcribing"
  | "polishing"
  | "done"
  | "error"
  | "offline";

export interface OverlayPayload {
  state: OverlayState;
  text?: string;
  app_name?: string;
  duration_ms?: number;
}

let currentState: OverlayPayload = {
  state: "idle",
  text: "Ready (Cmd+Shift+Space)",
};

const accumulator = new StreamingAccumulator();
let autoHideTimer: number | null = null;

const STATE_CONFIG: Record<
  OverlayState,
  { label: string; badge: string; dotClass: string; defaultText: string }
> = {
  idle: {
    label: "IDLE",
    badge: "badge-idle",
    dotClass: "idle",
    defaultText: "Ready (Cmd+Shift+Space)",
  },
  recording: {
    label: "REC",
    badge: "badge-recording",
    dotClass: "recording",
    defaultText: "Listening...",
  },
  transcribing: {
    label: "ASR",
    badge: "badge-transcribing",
    dotClass: "transcribing",
    defaultText: "Transcribing audio...",
  },
  polishing: {
    label: "AI",
    badge: "badge-polishing",
    dotClass: "polishing",
    defaultText: "Polishing with AI...",
  },
  done: {
    label: "PASTED",
    badge: "badge-done",
    dotClass: "done",
    defaultText: "Pasted text",
  },
  error: {
    label: "ERR",
    badge: "badge-error",
    dotClass: "error",
    defaultText: "Transcription failed",
  },
  offline: {
    label: "OFFLINE",
    badge: "badge-offline",
    dotClass: "offline",
    defaultText: "Cadence daemon offline",
  },
};

export function renderOverlay(container: HTMLElement, payload: OverlayPayload) {
  const conf = STATE_CONFIG[payload.state] || STATE_CONFIG.idle;
  const appBadge = payload.app_name ? `<span class="pill-app">${payload.app_name}</span>` : "";

  let contentHtml = "";
  if (payload.state === "recording" && accumulator.getText()) {
    contentHtml = accumulator.getFormattedHTML();
  } else {
    const displayText = payload.text || conf.defaultText;
    contentHtml = `<span>${displayText}</span>`;
  }

  container.innerHTML = `
    <div class="pill-container" id="pill" data-tauri-drag-region>
      <div class="pill-drag-handle" data-tauri-drag-region>
        <div class="pill-dot ${conf.dotClass}" id="pill-dot"></div>
      </div>
      <span class="pill-badge ${conf.badge}">${conf.label}</span>
      <div class="pill-text streaming-container" id="pill-text">
        ${contentHtml}
      </div>
      ${appBadge}
    </div>
  `;

  // Auto-scroll pill text container to end of speech
  const textEl = container.querySelector("#pill-text");
  if (textEl) {
    textEl.scrollLeft = textEl.scrollWidth;
  }
}

export function setOverlayState(
  container: HTMLElement,
  newState: OverlayState,
  text?: string,
  appName?: string,
  autoDismissMs = 2000
) {
  if (autoHideTimer !== null) {
    window.clearTimeout(autoHideTimer);
    autoHideTimer = null;
  }

  if (newState === "idle" || newState === "done") {
    accumulator.reset();
  }

  currentState = {
    state: newState,
    text: text || STATE_CONFIG[newState]?.defaultText,
    app_name: appName,
  };

  renderOverlay(container, currentState);

  // Auto-dismiss or transition back to idle for "done" state
  if (newState === "done" && autoDismissMs > 0) {
    autoHideTimer = window.setTimeout(() => {
      setOverlayState(container, "idle", "Ready (Cmd+Shift+Space)");
    }, autoDismissMs);
  }
}

export function updateStreamingText(
  container: HTMLElement,
  text: string,
  isFinal = false,
  sessionId = ""
) {
  accumulator.update(text, isFinal, sessionId);
  if (currentState.state !== "recording") {
    currentState.state = "recording";
  }
  renderOverlay(container, currentState);
}

export function getOverlayState(): OverlayPayload {
  return currentState;
}

export async function initOverlay(container: HTMLElement) {
  renderOverlay(container, currentState);

  // Listen to cross-window state change events
  try {
    await listen<OverlayPayload>("overlay-state-changed", (event) => {
      if (event.payload) {
        setOverlayState(
          container,
          event.payload.state,
          event.payload.text,
          event.payload.app_name
        );
      }
    });

    // Listen to real-time streaming partials
    await subscribePartialText((payload: PartialTextPayload) => {
      if (payload && payload.text) {
        updateStreamingText(container, payload.text, payload.is_final, payload.session_id);
      }
    });
  } catch (err) {
    console.warn("Overlay event listener initialization error:", err);
  }
}

export async function toggleOverlayWindow(): Promise<boolean> {
  return await invoke<boolean>("toggle_overlay");
}

export async function broadcastOverlayState(payload: OverlayPayload): Promise<void> {
  await invoke("emit_overlay_state", { payload });
}
