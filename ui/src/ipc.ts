import { invoke } from "@tauri-apps/api/core";

export interface CadenceStatus {
  recording?: boolean;
  is_recording?: boolean;
  mode?: string;
  engine?: string;
  asr_engine?: string;
  active_app?: string;
  profile?: string;
  version?: string;
  [key: string]: any;
}

export interface CadenceConfig {
  audio?: {
    sample_rate?: number;
    channels?: number;
  };
  hotkey?: {
    shortcut?: string;
    undo_shortcut?: string;
    mode?: string;
  };
  asr?: {
    engine?: string;
    model?: string;
    language?: string;
    streaming?: boolean;
  };
  polish?: {
    enabled?: boolean;
    backend?: string;
    model?: string;
    endpoint?: string;
    default_profile?: string;
    timeout_ms?: number;
    auto_route?: boolean;
  };
  privacy?: {
    network_mode?: string;
    telemetry?: boolean;
  };
}

export interface AudioDevice {
  id: string;
  name: string;
  is_default?: boolean;
}

export interface CadenceHistoryItem {
  id: string;
  text?: string;
  raw_text?: string;
  duration_ms?: number;
  created_at?: string;
  app?: string;
  [key: string]: any;
}

export interface CadenceVocabularyItem {
  spoken: string;
  canonical: string;
}

export interface CadenceSnippet {
  trigger: string;
  template: string;
  description?: string;
}

export interface PrivacyStats {
  ok?: boolean;
  transcripts_count: number;
  vocabulary_count: number;
  snippets_count: number;
  recoveries_count: number;
  notes_count: number;
  meetings_count: number;
  db_path?: string;
  db_size_bytes?: number;
  network_mode?: string;
  telemetry?: boolean;
}

/**
 * CadenceIPC provides type-safe IPC communication with the background
 * Cadence daemon (`cadenced`) through the native Tauri 2.0 Rust bridge.
 */
export class CadenceIPC {
  /**
   * Ping the background daemon over the UNIX domain socket.
   * Resolves with "pong" on success.
   */
  static async ping(): Promise<string> {
    return await invoke<string>("cadence_ping");
  }

  /**
   * Query full daemon status (recording state, active mode, profile, engine).
   */
  static async getStatus(): Promise<CadenceStatus> {
    return await invoke<CadenceStatus>("cadence_get_status");
  }

  /**
   * Fetch complete daemon settings and configuration.
   */
  static async getConfig(): Promise<CadenceConfig> {
    return await invoke<CadenceConfig>("cadence_get_config");
  }

  /**
   * Update daemon configuration fields via IPC.
   */
  static async setConfig(config: CadenceConfig): Promise<{ ok: boolean }> {
    return await invoke<{ ok: boolean }>("cadence_set_config", { config });
  }

  /**
   * Fetch available audio input devices.
   */
  static async getAudioDevices(): Promise<AudioDevice[]> {
    return await invoke<AudioDevice[]>("cadence_get_audio_devices");
  }

  /**
   * Fetch registered AI polish profiles list.
   */
  static async getPolishProfiles(): Promise<string[]> {
    return await invoke<string[]>("cadence_get_polish_profiles");
  }

  /**
   * Start dictation recording session via daemon IPC.
   */
  static async dictateStart(): Promise<any> {
    return await invoke<any>("cadence_dictate_start");
  }

  /**
   * Stop dictation recording session and process text via daemon IPC.
   */
  static async dictateStop(): Promise<any> {
    return await invoke<any>("cadence_dictate_stop");
  }

  /**
   * Fetch recent dictation history records.
   */
  static async getHistory(limit?: number): Promise<CadenceHistoryItem[]> {
    return await invoke<CadenceHistoryItem[]>("cadence_get_history", { limit });
  }

  /**
   * Fetch registered custom vocabulary mappings.
   */
  static async getVocabulary(): Promise<CadenceVocabularyItem[]> {
    return await invoke<CadenceVocabularyItem[]>("cadence_get_vocabulary");
  }

  /**
   * Add a new custom spoken phrase -> canonical term mapping.
   */
  static async addVocabulary(spoken: string, canonical: string): Promise<any> {
    return await invoke<any>("cadence_add_vocabulary", { spoken, canonical });
  }

  /**
   * Delete an existing vocabulary term by spoken phrase.
   */
  static async deleteVocabulary(spoken: string): Promise<any> {
    return await invoke<any>("cadence_delete_vocabulary", { spoken });
  }

  /**
   * Import batch vocabulary entries from JSON / text content.
   */
  static async importVocabulary(content: string, format = "json"): Promise<{ count: number }> {
    return await invoke<{ count: number }>("cadence_import_vocabulary", { content, format });
  }

  /**
   * Export all vocabulary entries in JSON format.
   */
  static async exportVocabulary(format = "json"): Promise<string> {
    const res = await invoke<any>("cadence_export_vocabulary", { format });
    if (res && typeof res === "object" && typeof res.content === "string") {
      return res.content;
    }
    return typeof res === "string" ? res : JSON.stringify(res, null, 2);
  }

  /**
   * Fetch registered snippets and templates.
   */
  static async getSnippets(): Promise<CadenceSnippet[]> {
    return await invoke<CadenceSnippet[]>("cadence_get_snippets");
  }

  /**
   * Create or update a snippet.
   */
  static async setSnippet(trigger: string, template: string, description?: string): Promise<any> {
    return await invoke<any>("cadence_set_snippet", { trigger, template, description });
  }

  /**
   * Delete a snippet by trigger.
   */
  static async deleteSnippet(trigger: string): Promise<any> {
    return await invoke<any>("cadence_delete_snippet", { trigger });
  }

  /**
   * Expand a snippet template with dynamic variables (e.g. {{date}}, {{time}}).
   */
  static async expandSnippet(template?: string, trigger?: string): Promise<{ ok: boolean; template: string; expanded: string }> {
    return await invoke<{ ok: boolean; template: string; expanded: string }>("cadence_expand_snippet", {
      template,
      trigger,
    });
  }

  /**
   * Re-paste specified text into the currently active target application.
   */
  static async paste(text: string): Promise<{ ok: boolean; text: string }> {
    return await invoke<{ ok: boolean; text: string }>("cadence_paste", { text });
  }

  /**
   * Fetch stored data statistics and privacy settings.
   */
  static async getPrivacyStats(): Promise<PrivacyStats> {
    return await invoke<PrivacyStats>("cadence_get_privacy_stats");
  }

  /**
   * Clear all cached recovery session records.
   */
  static async clearRecoveries(): Promise<{ ok: boolean }> {
    return await invoke<{ ok: boolean }>("cadence_clear_recoveries");
  }

  /**
   * Search past transcription history and knowledge records.
   */
  static async search(query: string, limit?: number): Promise<any[]> {
    return await invoke<any[]>("cadence_search", { query, limit });
  }

  /**
   * Generic method proxy to invoke any supported daemon method over IPC.
   */
  static async call(method: string, params?: Record<string, any>): Promise<any> {
    return await invoke<any>("cadence_call", { method, params });
  }
}
