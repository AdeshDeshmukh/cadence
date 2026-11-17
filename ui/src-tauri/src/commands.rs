use crate::ipc;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DaemonStatus {
    pub ok: bool,
    pub status: String,
    pub version: String,
    pub recording: bool,
}

/// Fallback / legacy status command for Tauri.
#[tauri::command]
pub fn get_status() -> Result<DaemonStatus, String> {
    match ipc::call_daemon("status", None) {
        Ok(val) => {
            let recording = val.get("recording").and_then(|v| v.as_bool()).unwrap_or(false);
            let mode = val.get("mode").and_then(|v| v.as_str()).unwrap_or("idle");
            Ok(DaemonStatus {
                ok: true,
                status: mode.to_string(),
                version: "0.1.0".to_string(),
                recording,
            })
        }
        Err(_) => Ok(DaemonStatus {
            ok: false,
            status: "offline".into(),
            version: "0.1.0".into(),
            recording: false,
        }),
    }
}

/// Ping the Cadence daemon over UNIX socket.
#[tauri::command]
pub fn cadence_ping() -> Result<String, String> {
    let res = ipc::call_daemon("ping", None)?;
    if let Some(s) = res.as_str() {
        Ok(s.to_string())
    } else {
        Ok(res.to_string())
    }
}

/// Query full daemon status over IPC.
#[tauri::command]
pub fn cadence_get_status() -> Result<serde_json::Value, String> {
    ipc::call_daemon("status", None)
}

/// Start dictation recording session via daemon IPC.
#[tauri::command]
pub fn cadence_dictate_start() -> Result<serde_json::Value, String> {
    ipc::call_daemon("dictate_start", None)
}

/// Stop dictation recording session and process text via daemon IPC.
#[tauri::command]
pub fn cadence_dictate_stop() -> Result<serde_json::Value, String> {
    ipc::call_daemon("dictate_stop", None)
}

/// Fetch dictation history records from daemon.
#[tauri::command]
pub fn cadence_get_history(limit: Option<usize>) -> Result<serde_json::Value, String> {
    let params = limit.map(|l| serde_json::json!({ "limit": l }));
    ipc::call_daemon("history", params)
}

/// Fetch vocabulary entries from daemon.
#[tauri::command]
pub fn cadence_get_vocabulary() -> Result<serde_json::Value, String> {
    ipc::call_daemon("vocabulary", None)
}

/// Add custom vocabulary spoken -> canonical pair via daemon IPC.
#[tauri::command]
pub fn cadence_add_vocabulary(
    spoken: String,
    canonical: String,
) -> Result<serde_json::Value, String> {
    ipc::call_daemon(
        "vocab_add",
        Some(serde_json::json!({
            "spoken": spoken,
            "canonical": canonical
        })),
    )
}

/// Delete custom vocabulary term via daemon IPC.
#[tauri::command]
pub fn cadence_delete_vocabulary(spoken: String) -> Result<serde_json::Value, String> {
    ipc::call_daemon(
        "vocab_remove",
        Some(serde_json::json!({
            "spoken": spoken
        })),
    )
}

/// Import vocabulary batch via daemon IPC.
#[tauri::command]
pub fn cadence_import_vocabulary(
    content: String,
    format: Option<String>,
) -> Result<serde_json::Value, String> {
    ipc::call_daemon(
        "vocab_import",
        Some(serde_json::json!({
            "content": content,
            "format": format.unwrap_or_else(|| "json".to_string())
        })),
    )
}

/// Export vocabulary entries via daemon IPC.
#[tauri::command]
pub fn cadence_export_vocabulary(
    format: Option<String>,
) -> Result<serde_json::Value, String> {
    ipc::call_daemon(
        "vocab_export",
        Some(serde_json::json!({
            "format": format.unwrap_or_else(|| "json".to_string())
        })),
    )
}

/// Fetch snippet triggers and templates from daemon.
#[tauri::command]
pub fn cadence_get_snippets() -> Result<serde_json::Value, String> {
    ipc::call_daemon("snippet_list", None)
}

/// Fetch full daemon configuration via IPC.
#[tauri::command]
pub fn cadence_get_config() -> Result<serde_json::Value, String> {
    ipc::call_daemon("config_get", None)
}

/// Update daemon configuration fields via IPC.
#[tauri::command]
pub fn cadence_set_config(config: serde_json::Value) -> Result<serde_json::Value, String> {
    ipc::call_daemon("config_set", Some(config))
}

/// Query available audio input devices from daemon.
#[tauri::command]
pub fn cadence_get_audio_devices() -> Result<serde_json::Value, String> {
    ipc::call_daemon("audio_devices", None)
}

/// Fetch list of available polish profiles from daemon.
#[tauri::command]
pub fn cadence_get_polish_profiles() -> Result<serde_json::Value, String> {
    ipc::call_daemon("polish_profile_list", None)
}

/// Trigger re-paste of text into the active application via daemon IPC.
#[tauri::command]
pub fn cadence_paste(text: String) -> Result<serde_json::Value, String> {
    ipc::call_daemon("paste", Some(serde_json::json!({ "text": text })))
}

/// Perform full-text search across past dictations and notes via daemon IPC.
#[tauri::command]
pub fn cadence_search(query: String, limit: Option<usize>) -> Result<serde_json::Value, String> {
    let params = serde_json::json!({
        "query": query,
        "limit": limit.unwrap_or(30)
    });
    ipc::call_daemon("search", Some(params))
}

/// Generic proxy method to invoke any supported daemon method over IPC.
#[tauri::command]
pub fn cadence_call(
    method: String,
    params: Option<serde_json::Value>,
) -> Result<serde_json::Value, String> {
    ipc::call_daemon(&method, params)
}
