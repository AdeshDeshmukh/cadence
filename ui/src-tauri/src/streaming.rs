use crate::ipc;
use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct PartialTextPayload {
    pub session_id: String,
    pub text: String,
    pub is_final: bool,
    pub word_count: usize,
}

impl PartialTextPayload {
    pub fn new(session_id: impl Into<String>, text: impl Into<String>, is_final: bool) -> Self {
        let t = text.into();
        let words = t.split_whitespace().count();
        Self {
            session_id: session_id.into(),
            text: t,
            is_final,
            word_count: words,
        }
    }
}

/// Fetches current active transcription partial from the Cadence daemon over IPC.
#[tauri::command]
pub fn cadence_get_partial() -> Result<PartialTextPayload, String> {
    let res = ipc::call_daemon("partial", None)?;
    let session = res
        .get("session")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    let partial = res
        .get("partial")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    let recording = res
        .get("recording")
        .and_then(|v| v.as_bool())
        .unwrap_or(false);

    Ok(PartialTextPayload::new(session, partial, !recording))
}

/// Emits a streaming partial text event across Tauri windows.
#[tauri::command]
pub fn cadence_emit_partial(
    app: AppHandle,
    payload: PartialTextPayload,
) -> Result<(), String> {
    app.emit("stream-partial-text", payload)
        .map_err(|e| e.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_partial_payload_word_count() {
        let payload = PartialTextPayload::new("sess-1", "hello world from cadence", false);
        assert_eq!(payload.session_id, "sess-1");
        assert_eq!(payload.text, "hello world from cadence");
        assert_eq!(payload.word_count, 4);
        assert!(!payload.is_final);

        let empty = PartialTextPayload::new("sess-2", "", true);
        assert_eq!(empty.word_count, 0);
        assert!(empty.is_final);
    }

    #[test]
    fn test_partial_payload_serialization() {
        let payload = PartialTextPayload::new("sess-123", "testing partial stream", false);
        let serialized = serde_json::to_string(&payload).expect("serialize");
        assert!(serialized.contains("\"session_id\":\"sess-123\""));
        assert!(serialized.contains("\"word_count\":3"));

        let deserialized: PartialTextPayload =
            serde_json::from_str(&serialized).expect("deserialize");
        assert_eq!(deserialized, payload);
    }
}
