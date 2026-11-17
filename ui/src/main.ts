// Cadence Desktop UI - Main entry point
import { CadenceIPC, type CadenceStatus } from "./ipc";
import {
  initOverlay,
  toggleOverlayWindow,
  broadcastOverlayState,
  type OverlayState,
} from "./overlay";
import {
  getDaemonPartial,
  broadcastPartialText,
} from "./streaming";
import {
  loadSettings,
  renderSettingsPanel,
} from "./settings";
import {
  fetchHistory,
  renderHistoryView,
} from "./history";
import {
  fetchVocabulary,
  renderVocabularyManager,
} from "./vocab";

export type UIState = {
  mode: "overlay" | "dashboard";
  activeTab: "overview" | "history" | "vocabulary" | "settings";
  version: string;
  daemonConnected: boolean;
  daemonStatus: CadenceStatus | null;
  lastPingResult: string;
  overlayVisible: boolean;
  activeDictating: boolean;
  streamingSimulationActive: boolean;
};

const state: UIState = {
  mode: window.location.search.includes("window=overlay") ? "overlay" : "dashboard",
  activeTab: "overview",
  version: "0.1.0",
  daemonConnected: false,
  daemonStatus: null,
  lastPingResult: "",
  overlayVisible: false,
  activeDictating: false,
  streamingSimulationActive: false,
};

let streamingPollTimer: number | null = null;

export function renderDashboard(el: HTMLElement) {
  const isConn = state.daemonConnected;
  const modeName = state.daemonStatus?.mode || "Dictation";
  const engineName =
    state.daemonStatus?.engine || state.daemonStatus?.asr_engine || "Whisper (local)";
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

      <!-- Navigation Tabs -->
      <nav class="nav-tabs">
        <button id="tab-overview" class="nav-tab ${state.activeTab === "overview" ? "active" : ""}">Overview & Overlay</button>
        <button id="tab-history" class="nav-tab ${state.activeTab === "history" ? "active" : ""}">History</button>
        <button id="tab-vocabulary" class="nav-tab ${state.activeTab === "vocabulary" ? "active" : ""}">Vocabulary</button>
        <button id="tab-settings" class="nav-tab ${state.activeTab === "settings" ? "active" : ""}">Settings</button>
      </nav>

      <div id="tab-content" style="flex: 1; display: flex; flex-direction: column;">
        <!-- Tab Content dynamically injected below -->
      </div>
    </div>
  `;

  // Attach tab navigation listeners
  const overviewTab = el.querySelector("#tab-overview") as HTMLButtonElement | null;
  if (overviewTab) {
    overviewTab.onclick = () => {
      state.activeTab = "overview";
      renderDashboard(el);
    };
  }

  const historyTab = el.querySelector("#tab-history") as HTMLButtonElement | null;
  if (historyTab) {
    historyTab.onclick = async () => {
      state.activeTab = "history";
      await fetchHistory();
      renderDashboard(el);
    };
  }

  const vocabTab = el.querySelector("#tab-vocabulary") as HTMLButtonElement | null;
  if (vocabTab) {
    vocabTab.onclick = async () => {
      state.activeTab = "vocabulary";
      await fetchVocabulary();
      renderDashboard(el);
    };
  }

  const settingsTab = el.querySelector("#tab-settings") as HTMLButtonElement | null;
  if (settingsTab) {
    settingsTab.onclick = async () => {
      state.activeTab = "settings";
      await loadSettings();
      renderDashboard(el);
    };
  }

  const tabContent = el.querySelector("#tab-content") as HTMLElement | null;
  if (!tabContent) return;

  if (state.activeTab === "history") {
    renderHistoryView(tabContent, async (pastedText) => {
      state.lastPingResult = `Re-pasted: "${pastedText.slice(0, 30)}..."`;
      await broadcastOverlayState({
        state: "done",
        text: "Re-pasted text",
      });
    });
  } else if (state.activeTab === "vocabulary") {
    renderVocabularyManager(tabContent);
  } else if (state.activeTab === "settings") {
    renderSettingsPanel(tabContent, async () => {
      await checkDaemonConnectivity(el);
    });
  } else {
    // Render Overview Tab
    tabContent.innerHTML = `
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
          <div class="card-title">Streaming Engine</div>
          <div class="card-value" id="card-pill">Real-time ASR</div>
        </div>
      </section>

      <!-- Overlay & Dictation Controls -->
      <section style="background: rgba(255, 255, 255, 0.6); border: 1px solid #e2e8f0; border-radius: 12px; padding: 16px; margin-bottom: 20px;">
        <h4 style="margin-bottom: 12px; color: #334155;">Pill Overlay & Streaming Dictation Controls</h4>
        <div style="display: flex; flex-wrap: wrap; gap: 10px; align-items: center; margin-bottom: 12px;">
          <button id="btn-toggle-overlay" class="action-btn" style="background: #475569;">Toggle Overlay Window</button>
          <button id="btn-start-dictation" class="action-btn" style="background: #ef4444;" ${!isConn ? "disabled" : ""}>Start Dictation</button>
          <button id="btn-stop-dictation" class="action-btn" style="background: #10b981;" ${!isConn ? "disabled" : ""}>Stop Dictation</button>
          <button id="btn-stream-sim" class="action-btn" style="background: #8b5cf6;" ${state.streamingSimulationActive ? "disabled" : ""}>
            ${state.streamingSimulationActive ? "Streaming..." : "Simulate Live Streaming"}
          </button>
        </div>
        <div style="display: flex; flex-wrap: wrap; gap: 8px; align-items: center;">
          <span style="font-size: 12px; color: #64748b; font-weight: 500;">Simulate States:</span>
          <button id="sim-rec" class="action-btn" style="background: #ef4444; font-size: 11px;">Rec</button>
          <button id="sim-asr" class="action-btn" style="background: #3b82f6; font-size: 11px;">ASR</button>
          <button id="sim-ai" class="action-btn" style="background: #a855f7; font-size: 11px;">Polish</button>
          <button id="sim-done" class="action-btn" style="background: #10b981; font-size: 11px;">Done</button>
          <button id="sim-idle" class="action-btn" style="background: #64748b; font-size: 11px;">Idle</button>
        </div>
      </section>

      <div style="flex: 1; display: flex; flex-direction: column; justify-content: center; align-items: center; border: 1px dashed rgba(148, 163, 184, 0.4); border-radius: 12px; padding: 24px; text-align: center;">
        <h3 style="margin-bottom: 8px;">Cadence Desktop Engine</h3>
        <p style="color: #64748b; max-width: 480px; margin-bottom: 12px;">
          The native desktop application renders real-time streaming speech partials directly onto the floating pill overlay.
        </p>
        <div id="ipc-feedback" style="font-family: monospace; font-size: 12px; color: #3b82f6; min-height: 20px;">
          ${state.lastPingResult ? `IPC: ${state.lastPingResult}` : ""}
        </div>
      </div>
    `;

    // Attach button event listeners in Overview
    const toggleOverlayBtn = tabContent.querySelector("#btn-toggle-overlay") as HTMLButtonElement | null;
    if (toggleOverlayBtn) {
      toggleOverlayBtn.onclick = async () => {
        try {
          const visible = await toggleOverlayWindow();
          state.overlayVisible = visible;
          state.lastPingResult = `Overlay window ${visible ? "visible" : "hidden"}`;
          renderDashboard(el);
        } catch (err: any) {
          state.lastPingResult = `Toggle overlay error: ${err?.message || err}`;
          renderDashboard(el);
        }
      };
    }

    const startDictBtn = tabContent.querySelector("#btn-start-dictation") as HTMLButtonElement | null;
    if (startDictBtn) {
      startDictBtn.onclick = async () => {
        try {
          await CadenceIPC.dictateStart();
          await broadcastOverlayState({
            state: "recording",
            text: "Listening...",
          });
          state.activeDictating = true;
          startStreamingPoller();
          state.lastPingResult = "Dictation recording started";
          renderDashboard(el);
        } catch (err: any) {
          state.lastPingResult = `Dictate start error: ${err?.message || err}`;
          renderDashboard(el);
        }
      };
    }

    const stopDictBtn = tabContent.querySelector("#btn-stop-dictation") as HTMLButtonElement | null;
    if (stopDictBtn) {
      stopDictBtn.onclick = async () => {
        stopStreamingPoller();
        try {
          await broadcastOverlayState({
            state: "transcribing",
            text: "Transcribing audio...",
          });
          await CadenceIPC.dictateStop();
          await broadcastOverlayState({
            state: "done",
            text: "Pasted text",
          });
          state.activeDictating = false;
          state.lastPingResult = "Dictation stopped and processed";
          renderDashboard(el);
        } catch (err: any) {
          state.lastPingResult = `Dictate stop error: ${err?.message || err}`;
          renderDashboard(el);
        }
      };
    }

    const streamSimBtn = tabContent.querySelector("#btn-stream-sim") as HTMLButtonElement | null;
    if (streamSimBtn) {
      streamSimBtn.onclick = async () => {
        state.streamingSimulationActive = true;
        renderDashboard(el);

        await broadcastOverlayState({
          state: "recording",
          text: "Listening...",
        });

        const sentence = "Cadence dictation engine streams words in real-time as you speak.";
        const words = sentence.split(" ");
        let accumulated = "";

        for (let i = 0; i < words.length; i++) {
          accumulated += (accumulated ? " " : "") + words[i];
          await broadcastPartialText({
            session_id: "sim-sess-1",
            text: accumulated,
            is_final: i === words.length - 1,
            word_count: i + 1,
          });
          await new Promise((resolve) => window.setTimeout(resolve, 140));
        }

        await new Promise((resolve) => window.setTimeout(resolve, 600));
        await broadcastOverlayState({
          state: "polishing",
          text: "Polishing with AI...",
        });

        await new Promise((resolve) => window.setTimeout(resolve, 800));
        await broadcastOverlayState({
          state: "done",
          text: "Pasted: " + sentence,
        });

        state.streamingSimulationActive = false;
        state.lastPingResult = "Completed streaming simulation";
        renderDashboard(el);
      };
    }

    const attachSim = (id: string, s: OverlayState, text: string) => {
      const btn = tabContent.querySelector(id) as HTMLButtonElement | null;
      if (btn) {
        btn.onclick = async () => {
          try {
            await broadcastOverlayState({ state: s, text });
            state.lastPingResult = `Simulated overlay state: ${s}`;
            renderDashboard(el);
          } catch (err: any) {
            state.lastPingResult = `Sim error: ${err?.message || err}`;
            renderDashboard(el);
          }
        };
      }
    };

    attachSim("#sim-rec", "recording", "Listening...");
    attachSim("#sim-asr", "transcribing", "Transcribing audio...");
    attachSim("#sim-ai", "polishing", "Polishing with AI...");
    attachSim("#sim-done", "done", "Pasted into document");
    attachSim("#sim-idle", "idle", "Ready (Cmd+Shift+Space)");
  }

  // Header Ping Button
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
        renderDashboard(el);
      }
    };
  }
}

function startStreamingPoller() {
  if (streamingPollTimer !== null) return;
  streamingPollTimer = window.setInterval(async () => {
    if (!state.activeDictating) {
      stopStreamingPoller();
      return;
    }
    try {
      const partial = await getDaemonPartial();
      if (partial && partial.text) {
        await broadcastPartialText(partial);
      }
    } catch (_e) {
      // ignore transient poll errors while recording
    }
  }, 100);
}

function stopStreamingPoller() {
  if (streamingPollTimer !== null) {
    window.clearInterval(streamingPollTimer);
    streamingPollTimer = null;
  }
}

async function checkDaemonConnectivity(el: HTMLElement) {
  try {
    const status = await CadenceIPC.getStatus();
    state.daemonConnected = true;
    state.daemonStatus = status;

    if (status.recording && !state.activeDictating) {
      state.activeDictating = true;
      startStreamingPoller();
      await broadcastOverlayState({
        state: "recording",
        text: "Listening...",
      });
    } else if (!status.recording && state.activeDictating) {
      state.activeDictating = false;
      stopStreamingPoller();
    }
  } catch (_e) {
    state.daemonConnected = false;
    state.daemonStatus = null;
  }

  if (state.mode === "dashboard") {
    renderDashboard(el);
  }
}

const appEl = document.getElementById("app");
if (appEl) {
  if (state.mode === "overlay") {
    initOverlay(appEl);
  } else {
    renderDashboard(appEl);
    checkDaemonConnectivity(appEl);
    window.setInterval(() => checkDaemonConnectivity(appEl), 2000);
  }
}
