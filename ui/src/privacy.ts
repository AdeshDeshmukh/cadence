import { CadenceIPC, type PrivacyStats } from "./ipc";

export interface PrivacyDashboardState {
  stats: PrivacyStats | null;
  loading: boolean;
  statusMessage: string;
  selectedNetworkMode: string;
  telemetryEnabled: boolean;
}

const state: PrivacyDashboardState = {
  stats: {
    transcripts_count: 0,
    vocabulary_count: 0,
    snippets_count: 0,
    recoveries_count: 0,
    notes_count: 0,
    meetings_count: 0,
    db_path: "~/.cadence/cadence.db",
    db_size_bytes: 40960,
    network_mode: "local",
    telemetry: false,
  },
  loading: false,
  statusMessage: "",
  selectedNetworkMode: "local",
  telemetryEnabled: false,
};

export async function fetchPrivacyStats(): Promise<PrivacyStats | null> {
  state.loading = true;
  try {
    const s = await CadenceIPC.getPrivacyStats();
    if (s) {
      state.stats = s;
      state.selectedNetworkMode = s.network_mode || "local";
      state.telemetryEnabled = !!s.telemetry;
    }
  } catch (err) {
    console.warn("Failed to fetch privacy stats via IPC:", err);
  } finally {
    state.loading = false;
  }
  return state.stats;
}

export function renderPrivacyDashboard(container: HTMLElement): void {
  const stats = state.stats || {
    transcripts_count: 0,
    vocabulary_count: 0,
    snippets_count: 0,
    recoveries_count: 0,
    notes_count: 0,
    meetings_count: 0,
    db_path: "~/.cadence/cadence.db",
    db_size_bytes: 0,
    network_mode: "local",
    telemetry: false,
  };

  const isLocalMode = state.selectedNetworkMode === "local";
  const sizeFormatted = formatBytes(stats.db_size_bytes || 0);

  container.innerHTML = `
    <div class="privacy-panel">
      <div class="vocab-header">
        <div>
          <div style="display: flex; align-items: center; gap: 8px;">
            <h3>Privacy & Data Dashboard</h3>
            <span class="badge ${isLocalMode ? "badge-connected" : "badge-offline"}" style="font-size: 11px;">
              ${isLocalMode ? "🔒 100% On-Device" : "☁️ Network Active"}
            </span>
          </div>
          <p class="vocab-subtitle">Audit all locally stored data, configure strict network isolation, and manage telemetry.</p>
        </div>
        <div class="vocab-actions-top">
          <button id="btn-refresh-privacy" class="action-btn" style="background: #3b82f6;">Refresh</button>
        </div>
      </div>

      ${state.statusMessage ? `<div class="save-banner">${state.statusMessage}</div>` : ""}

      <!-- Privacy Guarantee Banner -->
      <div class="privacy-guarantee-banner">
        <div class="privacy-guarantee-icon">🛡️</div>
        <div class="privacy-guarantee-content">
          <h4>Zero Cloud Data Leakage Guarantee</h4>
          <p>Cadence is built from the ground up for developer privacy. When operating in <strong>Local Mode</strong>, your audio streams, transcription logs, and custom terms never leave your computer.</p>
        </div>
      </div>

      <!-- Stored Data Breakdown -->
      <div class="privacy-section">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
          <h4 style="color: #1e293b;">Stored Local Data Overview</h4>
          <button id="btn-purge-recoveries" class="action-btn" style="background: #ef4444; font-size: 11px; padding: 4px 10px;">
            Purge Temporary Recoveries (${stats.recoveries_count})
          </button>
        </div>

        <div class="cards-grid" style="margin-bottom: 16px;">
          <div class="card">
            <div class="card-title">Stored Transcripts</div>
            <div class="card-value">${stats.transcripts_count}</div>
            <div style="font-size: 11px; color: #64748b; margin-top: 4px;">Searchable dictation history</div>
          </div>
          <div class="card">
            <div class="card-title">Custom Vocabulary</div>
            <div class="card-value">${stats.vocabulary_count}</div>
            <div style="font-size: 11px; color: #64748b; margin-top: 4px;">Technical terms & acronyms</div>
          </div>
          <div class="card">
            <div class="card-title">Voice Snippets</div>
            <div class="card-value">${stats.snippets_count}</div>
            <div style="font-size: 11px; color: #64748b; margin-top: 4px;">Dynamic templates</div>
          </div>
          <div class="card">
            <div class="card-title">Meeting Records</div>
            <div class="card-value">${stats.meetings_count}</div>
            <div style="font-size: 11px; color: #64748b; margin-top: 4px;">Notes & transcripts</div>
          </div>
        </div>

        <!-- Database Storage Details -->
        <div class="privacy-storage-info">
          <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px;">
            <div>
              <span style="font-size: 12px; font-weight: 600; color: #475569;">Local SQLite Database:</span>
              <code style="margin-left: 8px; font-size: 12px; background: rgba(148, 163, 184, 0.15); padding: 2px 6px; border-radius: 4px;">
                ${stats.db_path || "~/.cadence/cadence.db"}
              </code>
            </div>
            <div style="font-size: 12px; color: #64748b;">
              Database Size on Disk: <strong>${sizeFormatted}</strong>
            </div>
          </div>
        </div>
      </div>

      <!-- Network Mode & Privacy Controls -->
      <div class="privacy-controls-grid">
        <div class="privacy-card">
          <h4 style="margin-bottom: 8px; color: #1e293b;">Network Isolation Mode</h4>
          <p style="font-size: 12px; color: #64748b; margin-bottom: 12px;">Control outbound network connectivity for AI transcription and polishing.</p>

          <form id="network-mode-form" style="display: flex; flex-direction: column; gap: 10px;">
            <label class="privacy-radio-label">
              <input type="radio" name="network_mode" value="local" ${state.selectedNetworkMode === "local" ? "checked" : ""} />
              <div>
                <strong>Local Only (Zero Cloud)</strong>
                <p>100% on-device Whisper & Ollama. Sockets and network adapters strictly isolated.</p>
              </div>
            </label>

            <label class="privacy-radio-label">
              <input type="radio" name="network_mode" value="hybrid" ${state.selectedNetworkMode === "hybrid" ? "checked" : ""} />
              <div>
                <strong>Hybrid (Local Whisper + Cloud LLM)</strong>
                <p>Audio is transcribed locally; optional cloud LLMs used only for text polishing if configured.</p>
              </div>
            </label>

            <label class="privacy-radio-label">
              <input type="radio" name="network_mode" value="cloud" ${state.selectedNetworkMode === "cloud" ? "checked" : ""} />
              <div>
                <strong>Cloud Mode</strong>
                <p>Connects to remote transcription and completion endpoints.</p>
              </div>
            </label>

            <button type="submit" class="action-btn" style="background: #16a34a; align-self: flex-start; margin-top: 8px;">
              Apply Network Mode
            </button>
          </form>
        </div>

        <div class="privacy-card">
          <h4 style="margin-bottom: 8px; color: #1e293b;">Telemetry & Diagnostics</h4>
          <p style="font-size: 12px; color: #64748b; margin-bottom: 12px;">Help improve Cadence with anonymous crash and latency reports.</p>

          <div style="display: flex; flex-direction: column; gap: 14px;">
            <label class="form-toggle" style="align-items: flex-start;">
              <input id="toggle-telemetry" type="checkbox" ${state.telemetryEnabled ? "checked" : ""} />
              <div>
                <span class="toggle-label" style="font-weight: 500;">Enable Anonymous Diagnostics</span>
                <p style="font-size: 11px; color: #64748b; margin-top: 2px;">
                  Transmits anonymous latency percentiles (P50/P95) and crash traces. Audio, text, and keys are NEVER collected.
                </p>
              </div>
            </label>

            <div class="privacy-checklist">
              <div class="check-item">✅ Zero voice audio is ever uploaded</div>
              <div class="check-item">✅ Zero transcribed text or clipboard content is logged</div>
              <div class="check-item">✅ Full user ownership over SQLite database</div>
              <div class="check-item">✅ Open-source auditability across all code</div>
            </div>

            <button id="btn-save-telemetry" class="action-btn" style="background: #3b82f6; align-self: flex-start;">
              Save Telemetry Setting
            </button>
          </div>
        </div>
      </div>
    </div>
  `;

  // Network mode radio form submit
  const netForm = container.querySelector("#network-mode-form") as HTMLFormElement | null;
  if (netForm) {
    netForm.onsubmit = async (e) => {
      e.preventDefault();
      const selected = (netForm.querySelector("input[name='network_mode']:checked") as HTMLInputElement)?.value || "local";
      try {
        await CadenceIPC.setConfig({
          privacy: {
            network_mode: selected,
            telemetry: state.telemetryEnabled,
          },
        });
        state.selectedNetworkMode = selected;
        if (state.stats) state.stats.network_mode = selected;
        state.statusMessage = `Network mode updated to "${selected}".`;
      } catch (err: any) {
        state.statusMessage = `Failed to update network mode: ${err?.message || err}`;
      } finally {
        renderPrivacyDashboard(container);
      }
    };
  }

  // Telemetry save button
  const saveTelBtn = container.querySelector("#btn-save-telemetry") as HTMLButtonElement | null;
  const telToggle = container.querySelector("#toggle-telemetry") as HTMLInputElement | null;
  if (saveTelBtn && telToggle) {
    saveTelBtn.onclick = async () => {
      const enabled = telToggle.checked;
      try {
        await CadenceIPC.setConfig({
          privacy: {
            network_mode: state.selectedNetworkMode,
            telemetry: enabled,
          },
        });
        state.telemetryEnabled = enabled;
        if (state.stats) state.stats.telemetry = enabled;
        state.statusMessage = `Telemetry preference saved: ${enabled ? "Opted In" : "Disabled"}.`;
      } catch (err: any) {
        state.statusMessage = `Failed to save telemetry: ${err?.message || err}`;
      } finally {
        renderPrivacyDashboard(container);
      }
    };
  }

  // Purge recoveries button
  const purgeBtn = container.querySelector("#btn-purge-recoveries") as HTMLButtonElement | null;
  if (purgeBtn) {
    purgeBtn.onclick = async () => {
      purgeBtn.disabled = true;
      purgeBtn.textContent = "Purging...";
      try {
        await CadenceIPC.clearRecoveries();
        state.statusMessage = "All cached session recoveries purged successfully!";
        if (state.stats) state.stats.recoveries_count = 0;
      } catch (err: any) {
        state.statusMessage = `Failed to purge: ${err?.message || err}`;
      } finally {
        renderPrivacyDashboard(container);
      }
    };
  }

  // Refresh button
  const refreshBtn = container.querySelector("#btn-refresh-privacy") as HTMLButtonElement | null;
  if (refreshBtn) {
    refreshBtn.onclick = async () => {
      refreshBtn.disabled = true;
      refreshBtn.textContent = "Refreshing...";
      await fetchPrivacyStats();
      state.statusMessage = "Privacy statistics refreshed.";
      renderPrivacyDashboard(container);
    };
  }
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
}
