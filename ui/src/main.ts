// Cadence Desktop UI - Main entry point
export type UIState = {
  mode: "overlay" | "dashboard";
  recordingState: "idle" | "recording" | "transcribing" | "polishing" | "done";
  statusText: string;
  version: string;
};

const state: UIState = {
  mode: window.location.search.includes("window=overlay") ? "overlay" : "dashboard",
  recordingState: "idle",
  statusText: "Ready (Cmd+Shift+Space)",
  version: "0.1.0",
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
    el.innerHTML = `
      <div class="dashboard-container">
        <header class="header">
          <div class="title">
            <span>Cadence</span>
            <span class="badge">v${state.version}</span>
          </div>
          <div>
            <span class="badge" id="daemon-status">Daemon: Connected</span>
          </div>
        </header>

        <section class="cards-grid">
          <div class="card">
            <div class="card-title">Operating Mode</div>
            <div class="card-value" id="card-mode">Dictation</div>
          </div>
          <div class="card">
            <div class="card-title">Default Shortcut</div>
            <div class="card-value" id="card-shortcut">Cmd+Shift+Space</div>
          </div>
          <div class="card">
            <div class="card-title">ASR Engine</div>
            <div class="card-value" id="card-engine">Whisper (local)</div>
          </div>
          <div class="card">
            <div class="card-title">AI Text Polish</div>
            <div class="card-value" id="card-polish">Active</div>
          </div>
        </section>

        <div style="flex: 1; display: flex; flex-direction: column; justify-content: center; align-items: center; border: 1px dashed rgba(148, 163, 184, 0.4); border-radius: 12px; padding: 32px; text-align: center;">
          <h3 style="margin-bottom: 8px;">Cadence Desktop Engine</h3>
          <p style="color: #64748b; max-width: 480px;">
            The native desktop application controls the recording pill, streaming transcript preview, knowledge notes, and system settings.
          </p>
        </div>
      </div>
    `;
  }
}

const appEl = document.getElementById("app");
if (appEl) {
  render(appEl);
}
