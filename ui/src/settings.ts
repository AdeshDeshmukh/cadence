import { CadenceIPC, type CadenceConfig, type AudioDevice } from "./ipc";

export interface SettingsState {
  config: CadenceConfig;
  devices: AudioDevice[];
  profiles: string[];
  loading: boolean;
  saveStatus: string;
}

let state: SettingsState = {
  config: {
    hotkey: { shortcut: "Cmd+Shift+Space", undo_shortcut: "Cmd+Shift+U", mode: "push_to_talk" },
    asr: { engine: "whisper", model: "base.en", language: "auto", streaming: true },
    polish: { enabled: true, backend: "ollama", model: "llama3.2", default_profile: "general", endpoint: "http://127.0.0.1:11434" },
    privacy: { network_mode: "local_only", telemetry: false },
  },
  devices: [{ id: "default", name: "Default System Microphone", is_default: true }],
  profiles: ["general", "code", "email", "chat", "technical"],
  loading: false,
  saveStatus: "",
};

export async function loadSettings(): Promise<void> {
  state.loading = true;
  try {
    const [cfg, devs, profs] = await Promise.all([
      CadenceIPC.getConfig().catch(() => state.config),
      CadenceIPC.getAudioDevices().catch(() => state.devices),
      CadenceIPC.getPolishProfiles().catch(() => state.profiles),
    ]);
    if (cfg && cfg.hotkey) {
      state.config = cfg;
    }
    if (devs && devs.length > 0) {
      state.devices = devs;
    }
    if (profs && profs.length > 0) {
      state.profiles = profs;
    }
  } catch (e: any) {
    console.warn("Failed to load settings via IPC:", e);
  } finally {
    state.loading = false;
  }
}

export function renderSettingsPanel(container: HTMLElement, onSaveSuccess?: () => void): void {
  const cfg = state.config;
  const hk = cfg.hotkey || {};
  const asr = cfg.asr || {};
  const pol = cfg.polish || {};
  const priv = cfg.privacy || {};

  container.innerHTML = `
    <div class="settings-panel">
      <div class="settings-header">
        <h3>Cadence Settings</h3>
        <p class="settings-subtitle">Configure audio capture, keyboard triggers, ASR models, and LLM text polishing.</p>
      </div>

      ${state.saveStatus ? `<div class="save-banner">${state.saveStatus}</div>` : ""}

      <form id="settings-form" class="settings-form">
        <!-- 1. Microphone & Audio -->
        <fieldset class="settings-section">
          <legend class="section-title">Microphone & Audio</legend>
          <div class="form-row">
            <label for="setting-mic">Input Device</label>
            <select id="setting-mic" class="form-input">
              ${state.devices
                .map(
                  (d) => `<option value="${d.id}" ${d.is_default ? "selected" : ""}>${d.name}</option>`
                )
                .join("")}
            </select>
          </div>
          <div class="form-row">
            <label>Sample Rate</label>
            <span class="form-readonly">16,000 Hz (Optimal for Whisper & Silero VAD)</span>
          </div>
        </fieldset>

        <!-- 2. Hotkeys -->
        <fieldset class="settings-section">
          <legend class="section-title">Hotkeys & Trigger Mode</legend>
          <div class="form-row">
            <label for="setting-shortcut">Dictation Shortcut</label>
            <input id="setting-shortcut" type="text" class="form-input" value="${hk.shortcut || "Cmd+Shift+Space"}" placeholder="e.g. Cmd+Shift+Space" />
          </div>
          <div class="form-row">
            <label for="setting-undo-shortcut">Undo Shortcut</label>
            <input id="setting-undo-shortcut" type="text" class="form-input" value="${hk.undo_shortcut || "Cmd+Shift+U"}" placeholder="e.g. Cmd+Shift+U" />
          </div>
          <div class="form-row">
            <label for="setting-mode">Activation Mode</label>
            <select id="setting-mode" class="form-input">
              <option value="push_to_talk" ${hk.mode === "push_to_talk" ? "selected" : ""}>Push-to-Talk (Hold while speaking)</option>
              <option value="toggle" ${hk.mode === "toggle" ? "selected" : ""}>Toggle (Press once to start/stop)</option>
            </select>
          </div>
        </fieldset>

        <!-- 3. ASR Engine -->
        <fieldset class="settings-section">
          <legend class="section-title">Speech Recognition (ASR)</legend>
          <div class="form-row">
            <label for="setting-asr-engine">Engine</label>
            <select id="setting-asr-engine" class="form-input">
              <option value="whisper" ${asr.engine === "whisper" ? "selected" : ""}>Whisper (On-Device local)</option>
              <option value="mock" ${asr.engine === "mock" ? "selected" : ""}>Mock Engine (Test / CI)</option>
            </select>
          </div>
          <div class="form-row">
            <label for="setting-asr-model">Model</label>
            <select id="setting-asr-model" class="form-input">
              <option value="base.en" ${asr.model === "base.en" ? "selected" : ""}>base.en (~142 MB, fast)</option>
              <option value="small.en" ${asr.model === "small.en" ? "selected" : ""}>small.en (~466 MB, balanced)</option>
              <option value="medium.en" ${asr.model === "medium.en" ? "selected" : ""}>medium.en (~1.5 GB, high accuracy)</option>
            </select>
          </div>
          <div class="form-row">
            <label for="setting-asr-lang">Language</label>
            <select id="setting-asr-lang" class="form-input">
              <option value="auto" ${asr.language === "auto" ? "selected" : ""}>Auto-Detect</option>
              <option value="en" ${asr.language === "en" ? "selected" : ""}>English (en)</option>
              <option value="es" ${asr.language === "es" ? "selected" : ""}>Spanish (es)</option>
              <option value="fr" ${asr.language === "fr" ? "selected" : ""}>French (fr)</option>
              <option value="de" ${asr.language === "de" ? "selected" : ""}>German (de)</option>
            </select>
          </div>
          <div class="form-row checkbox-row">
            <input type="checkbox" id="setting-asr-stream" ${asr.streaming ? "checked" : ""} />
            <label for="setting-asr-stream">Enable real-time partial streaming hypothesis</label>
          </div>
        </fieldset>

        <!-- 4. AI Polish Profiles -->
        <fieldset class="settings-section">
          <legend class="section-title">AI Text Polish & LLM Profiles</legend>
          <div class="form-row checkbox-row">
            <input type="checkbox" id="setting-polish-enable" ${pol.enabled ? "checked" : ""} />
            <label for="setting-polish-enable">Enable AI text transformation and polishing</label>
          </div>
          <div class="form-row">
            <label for="setting-polish-backend">Backend Provider</label>
            <select id="setting-polish-backend" class="form-input">
              <option value="ollama" ${pol.backend === "ollama" ? "selected" : ""}>Ollama (Local Private LLM)</option>
              <option value="openai" ${pol.backend === "openai" ? "selected" : ""}>OpenAI (Cloud API)</option>
              <option value="anthropic" ${pol.backend === "anthropic" ? "selected" : ""}>Anthropic (Claude)</option>
              <option value="mock" ${pol.backend === "mock" ? "selected" : ""}>Mock Polisher</option>
            </select>
          </div>
          <div class="form-row">
            <label for="setting-polish-model">Model Name</label>
            <input id="setting-polish-model" type="text" class="form-input" value="${pol.model || "llama3.2"}" placeholder="e.g. llama3.2, mistral" />
          </div>
          <div class="form-row">
            <label for="setting-polish-endpoint">Endpoint URL</label>
            <input id="setting-polish-endpoint" type="text" class="form-input" value="${pol.endpoint || "http://127.0.0.1:11434"}" />
          </div>
          <div class="form-row">
            <label for="setting-polish-profile">Default Profile</label>
            <select id="setting-polish-profile" class="form-input">
              ${state.profiles
                .map(
                  (p) => `<option value="${p}" ${pol.default_profile === p ? "selected" : ""}>${p.toUpperCase()}</option>`
                )
                .join("")}
            </select>
          </div>
        </fieldset>

        <!-- 5. Privacy & Telemetry -->
        <fieldset class="settings-section">
          <legend class="section-title">Privacy & Network</legend>
          <div class="form-row">
            <label for="setting-priv-mode">Network Mode</label>
            <select id="setting-priv-mode" class="form-input">
              <option value="local_only" ${priv.network_mode === "local_only" ? "selected" : ""}>Strict Local Only (Zero telemetry)</option>
              <option value="hybrid" ${priv.network_mode === "hybrid" ? "selected" : ""}>Hybrid (Local Whisper + Cloud LLM)</option>
            </select>
          </div>
        </fieldset>

        <div class="settings-actions">
          <button type="submit" id="btn-save-settings" class="action-btn save-btn">Save Changes</button>
        </div>
      </form>
    </div>
  `;

  const form = container.querySelector("#settings-form") as HTMLFormElement | null;
  if (form) {
    form.onsubmit = async (e) => {
      e.preventDefault();
      const saveBtn = container.querySelector("#btn-save-settings") as HTMLButtonElement | null;
      if (saveBtn) {
        saveBtn.disabled = true;
        saveBtn.textContent = "Saving...";
      }

      const updatedConfig: CadenceConfig = {
        hotkey: {
          shortcut: (container.querySelector("#setting-shortcut") as HTMLInputElement).value,
          undo_shortcut: (container.querySelector("#setting-undo-shortcut") as HTMLInputElement).value,
          mode: (container.querySelector("#setting-mode") as HTMLSelectElement).value,
        },
        asr: {
          engine: (container.querySelector("#setting-asr-engine") as HTMLSelectElement).value,
          model: (container.querySelector("#setting-asr-model") as HTMLSelectElement).value,
          language: (container.querySelector("#setting-asr-lang") as HTMLSelectElement).value,
          streaming: (container.querySelector("#setting-asr-stream") as HTMLInputElement).checked,
        },
        polish: {
          enabled: (container.querySelector("#setting-polish-enable") as HTMLInputElement).checked,
          backend: (container.querySelector("#setting-polish-backend") as HTMLSelectElement).value,
          model: (container.querySelector("#setting-polish-model") as HTMLInputElement).value,
          endpoint: (container.querySelector("#setting-polish-endpoint") as HTMLInputElement).value,
          default_profile: (container.querySelector("#setting-polish-profile") as HTMLSelectElement).value,
        },
        privacy: {
          network_mode: (container.querySelector("#setting-priv-mode") as HTMLSelectElement).value,
          telemetry: false,
        },
      };

      try {
        await CadenceIPC.setConfig(updatedConfig);
        state.config = updatedConfig;
        state.saveStatus = `Settings successfully saved at ${new Date().toLocaleTimeString()}!`;
        if (onSaveSuccess) onSaveSuccess();
      } catch (err: any) {
        state.saveStatus = `Save failed: ${err?.message || err}`;
      } finally {
        if (saveBtn) {
          saveBtn.disabled = false;
          saveBtn.textContent = "Save Changes";
        }
        renderSettingsPanel(container, onSaveSuccess);
      }
    };
  }
}
