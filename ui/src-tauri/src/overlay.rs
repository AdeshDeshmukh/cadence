use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter, Manager};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum OverlayState {
    Idle,
    Recording,
    Transcribing,
    Polishing,
    Done,
    Error,
    Offline,
}

impl Default for OverlayState {
    fn default() -> Self {
        OverlayState::Idle
    }
}

impl OverlayState {
    pub fn as_str(&self) -> &'static str {
        match self {
            OverlayState::Idle => "idle",
            OverlayState::Recording => "recording",
            OverlayState::Transcribing => "transcribing",
            OverlayState::Polishing => "polishing",
            OverlayState::Done => "done",
            OverlayState::Error => "error",
            OverlayState::Offline => "offline",
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OverlayPayload {
    pub state: OverlayState,
    pub text: Option<String>,
    pub app_name: Option<String>,
    pub duration_ms: Option<u64>,
}

#[tauri::command]
pub fn show_overlay(app: AppHandle) -> Result<(), String> {
    if let Some(win) = app.get_webview_window("overlay") {
        win.show().map_err(|e| e.to_string())?;
        win.set_focus().map_err(|e| e.to_string())?;
        Ok(())
    } else {
        Err("overlay window not found".to_string())
    }
}

#[tauri::command]
pub fn hide_overlay(app: AppHandle) -> Result<(), String> {
    if let Some(win) = app.get_webview_window("overlay") {
        win.hide().map_err(|e| e.to_string())?;
        Ok(())
    } else {
        Err("overlay window not found".to_string())
    }
}

#[tauri::command]
pub fn toggle_overlay(app: AppHandle) -> Result<bool, String> {
    if let Some(win) = app.get_webview_window("overlay") {
        let is_visible = win.is_visible().unwrap_or(false);
        if is_visible {
            win.hide().map_err(|e| e.to_string())?;
            Ok(false)
        } else {
            win.show().map_err(|e| e.to_string())?;
            Ok(true)
        }
    } else {
        Err("overlay window not found".to_string())
    }
}

#[tauri::command]
pub fn emit_overlay_state(app: AppHandle, payload: OverlayPayload) -> Result<(), String> {
    if let Some(win) = app.get_webview_window("overlay") {
        // Automatically make window visible when entering active dictation states
        if matches!(
            payload.state,
            OverlayState::Recording | OverlayState::Transcribing | OverlayState::Polishing
        ) {
            let _ = win.show();
        }
    }
    app.emit("overlay-state-changed", payload)
        .map_err(|e| e.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_overlay_state_serialization() {
        let payload = OverlayPayload {
            state: OverlayState::Recording,
            text: Some("Listening...".into()),
            app_name: Some("VS Code".into()),
            duration_ms: Some(1500),
        };

        let json = serde_json::to_string(&payload).expect("serialize");
        assert!(json.contains("\"state\":\"recording\""));
        assert!(json.contains("\"text\":\"Listening...\""));
        assert!(json.contains("\"app_name\":\"VS Code\""));

        let deserialized: OverlayPayload = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(deserialized.state, OverlayState::Recording);
        assert_eq!(deserialized.text.as_deref(), Some("Listening..."));
    }

    #[test]
    fn test_overlay_state_strings() {
        assert_eq!(OverlayState::Idle.as_str(), "idle");
        assert_eq!(OverlayState::Recording.as_str(), "recording");
        assert_eq!(OverlayState::Transcribing.as_str(), "transcribing");
        assert_eq!(OverlayState::Polishing.as_str(), "polishing");
        assert_eq!(OverlayState::Done.as_str(), "done");
        assert_eq!(OverlayState::Error.as_str(), "error");
        assert_eq!(OverlayState::Offline.as_str(), "offline");
    }
}
