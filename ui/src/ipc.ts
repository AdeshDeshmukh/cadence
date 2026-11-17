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
   * Fetch registered snippets and templates.
   */
  static async getSnippets(): Promise<CadenceSnippet[]> {
    return await invoke<CadenceSnippet[]>("cadence_get_snippets");
  }

  /**
   * Generic method proxy to invoke any supported daemon method over IPC.
   */
  static async call(method: string, params?: Record<string, any>): Promise<any> {
    return await invoke<any>("cadence_call", { method, params });
  }
}
