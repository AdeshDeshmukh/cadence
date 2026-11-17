// Cadence Desktop UI - Main entry point
import { CadenceIPC, type CadenceStatus } from "./ipc";

export type UIState = {
  mode: "overlay" | "dashboard";
  recordingState: "idle" | "recording" | "transcribing" | "polishing" | "done";
  statusText: string;
  version: string;
  daemonConnected: boolean;
  daemonStatus: CadenceStatus | null;
  lastPingResult: string;
};

const state: UIState = {
  mode: window.location.search.includes("window=overlay") ? "overlay" : "dashboard",
  recordingState: "idle",
  statusText: "Ready (Cmd+Shift+Space)",
  version: "0.1.0",
  daemonConnected: false,
  daemonStatus: null,
  lastPingResult: "",
};

export function render(el: HTMLElement) {
  if (state.mode === "overlay") {
    el.innerHTML = `
      <div class="pill-container" id="pill">
        <div class="pill-dot ${state.recordingState}" id="pill-dot"></div>
        <span class="pill-text" id="pill-text">${state.statusText}</span>
      </div>
    `;
  } else {
    const isConn = state.daemonConnected;
    const modeName = state.daemonStatus?.mode || "Dictation";
    const engineName = state.daemonStatus?.engine || state.daemonStatus?.asr_engine || "Whisper (local)";
    const statusBadgeClass = isConn ? "badge-connected" : "badge-offline";
    const statusText = isConn ? "Daemon: Connected" : "Daemon: Offline";

    el.innerHTML = `
      <div class="dashboard-container">
        <header class="header">
          <div class="title">
            <span>Cadence</span>
            <span class="badge">v${state.version}</span>
          </div>
          <div style="display: flex; align-items: center; gap: 8px;">
            <span class="badge ${statusBadgeClass}" id="daemon-status">${statusText}</span>
            <button id="btn-ping" class="action-btn" title="Ping Daemon over UNIX Socket">Ping</button>
          </div>
        </header>

        <section class="cards-grid">
          <div class="card">
            <div class="card-title">Operating Mode</div>
            <div class="card-value" id="card-mode">${modeName}</div>
          </div>
          <div class="card">
            <div class="card-title">Default Shortcut</div>
            <div class="card-value" id="card-shortcut">Cmd+Shift+Space</div>
          </div>
          <div class="card">
            <div class="card-title">ASR Engine</div>
            <div class="card-value" id="card-engine">${engineName}</div>
          </div>
          <div class="card">
            <div class="card-title">IPC Bridge</div>
            <div class="card-value" id="card-ipc">UNIX Socket</div>
          </div>
        </section>

        <div style="flex: 1; display: flex; flex-direction: column; justify-content: center; align-items: center; border: 1px dashed rgba(148, 163, 184, 0.4); border-radius: 12px; padding: 32px; text-align: center;">
          <h3 style="margin-bottom: 8px;">Cadence Desktop Engine</h3>
          <p style="color: #64748b; max-width: 480px; margin-bottom: 16px;">
            The native desktop application communicates with the background daemon over the high-performance local UNIX domain socket bridge.
          </p>
          <div id="ipc-feedback" style="font-family: monospace; font-size: 12px; color: #3b82f6; min-height: 20px;">
            ${state.lastPingResult ? `IPC: ${state.lastPingResult}` : ""}
          </div>
        </div>
      </div>
    `;

    const pingBtn = el.querySelector("#btn-ping") as HTMLButtonElement | null;
    if (pingBtn) {
      pingBtn.onclick = async () => {
        pingBtn.disabled = true;
        pingBtn.textContent = "Pinging...";
        try {
          const res = await CadenceIPC.ping();
          state.daemonConnected = true;
          state.lastPingResult = `Ping response: ${res} at ${new Date().toLocaleTimeString()}`;
        } catch (err: any) {
          state.daemonConnected = false;
          state.lastPingResult = `Ping failed: ${err?.message || err}`;
        } finally {
          pingBtn.disabled = false;
          pingBtn.textContent = "Ping";
          render(el);
        }
      };
    }
  }
}

async function checkDaemonConnectivity() {
  try {
    const status = await CadenceIPC.getStatus();
    state.daemonConnected = true;
    state.daemonStatus = status;
    if (status.recording) {
      state.recordingState = "recording";
      state.statusText = "Listening...";
    } else {
      state.recordingState = "idle";
      state.statusText = "Ready (Cmd+Shift+Space)";
    }
  } catch (_e) {
    state.daemonConnected = false;
    state.daemonStatus = null;
    state.recordingState = "idle";
    state.statusText = "Daemon Offline";
  }

  const appEl = document.getElementById("app");
  if (appEl) {
    render(appEl);
  }
}

const appEl = document.getElementById("app");
if (appEl) {
  render(appEl);
  checkDaemonConnectivity();
}
