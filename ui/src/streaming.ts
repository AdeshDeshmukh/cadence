import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";

export interface PartialTextPayload {
  session_id: string;
  text: string;
  is_final: boolean;
  word_count: number;
}

/**
 * StreamingAccumulator tracks incremental transcription tokens as the user speaks.
 * It formats text for live display, distinguishing committed tokens from speculative partials
 * and animating an active speech cursor.
 */
export class StreamingAccumulator {
  private rawText = "";
  private isFinal = false;
  private sessionId = "";

  constructor(initialText = "") {
    this.rawText = initialText;
  }

  public update(text: string, isFinal = false, sessionId = ""): void {
    this.rawText = text.trim();
    this.isFinal = isFinal;
    if (sessionId) {
      this.sessionId = sessionId;
    }
  }

  public reset(): void {
    this.rawText = "";
    this.isFinal = false;
    this.sessionId = "";
  }

  public getText(): string {
    return this.rawText;
  }

  public getSessionId(): string {
    return this.sessionId;
  }

  public getWordCount(): number {
    if (!this.rawText) return 0;
    return this.rawText.split(/\s+/).filter(Boolean).length;
  }

  public getIsFinal(): boolean {
    return this.isFinal;
  }

  /**
   * Generates formatted HTML with high-visibility streaming caret
   * and subtle styling for the latest speculative word tokens.
   */
  public getFormattedHTML(): string {
    if (!this.rawText) {
      return `<span class="streaming-placeholder">Listening...</span><span class="streaming-caret"></span>`;
    }

    const words = this.rawText.split(/\s+/);
    if (words.length <= 1) {
      return `<span class="streaming-token latest">${this.rawText}</span><span class="streaming-caret"></span>`;
    }

    const committed = words.slice(0, -1).join(" ");
    const latest = words[words.length - 1];

    return `<span class="streaming-token committed">${committed}</span> <span class="streaming-token latest">${latest}</span><span class="streaming-caret"></span>`;
  }
}

/**
 * Fetch current active session partial transcription from Cadence daemon.
 */
export async function getDaemonPartial(): Promise<PartialTextPayload> {
  return await invoke<PartialTextPayload>("cadence_get_partial");
}

/**
 * Broadcast partial text update across all Tauri windows.
 */
export async function broadcastPartialText(payload: PartialTextPayload): Promise<void> {
  await invoke("cadence_emit_partial", { payload });
}

/**
 * Subscribe to streaming partial text events in a webview window.
 */
export async function subscribePartialText(
  callback: (payload: PartialTextPayload) => void
): Promise<() => void> {
  const unlisten = await listen<PartialTextPayload>("stream-partial-text", (event) => {
    if (event.payload) {
      callback(event.payload);
    }
  });
  return unlisten;
}
