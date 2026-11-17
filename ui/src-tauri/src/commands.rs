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

/// Fetch snippet triggers and templates from daemon.
#[tauri::command]
pub fn cadence_get_snippets() -> Result<serde_json::Value, String> {
    ipc::call_daemon("snippet_list", None)
}

/// Generic proxy method to invoke any supported daemon method over IPC.
#[tauri::command]
pub fn cadence_call(
    method: String,
    params: Option<serde_json::Value>,
) -> Result<serde_json::Value, String> {
    ipc::call_daemon(&method, params)
}
