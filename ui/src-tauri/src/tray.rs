use tauri::{
    menu::{Menu, MenuItem, PredefinedMenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    AppHandle, Manager,
};

pub const TRAY_ID: &str = "cadence_tray";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TrayStatus {
    Idle,
    Recording,
    Transcribing,
    Polishing,
    Offline,
}

impl TrayStatus {
    pub fn tooltip(&self) -> &'static str {
        match self {
            TrayStatus::Idle => "Cadence: Idle (Cmd+Shift+Space)",
            TrayStatus::Recording => "Cadence: Listening...",
            TrayStatus::Transcribing => "Cadence: Transcribing...",
            TrayStatus::Polishing => "Cadence: Polishing...",
            TrayStatus::Offline => "Cadence: Daemon Offline",
        }
    }

    pub fn menu_title(&self) -> &'static str {
        match self {
            TrayStatus::Idle => "Status: Ready",
            TrayStatus::Recording => "Status: Recording...",
            TrayStatus::Transcribing => "Status: Transcribing...",
            TrayStatus::Polishing => "Status: Polishing...",
            TrayStatus::Offline => "Status: Daemon Offline",
        }
    }
}

/// Creates the native status bar tray icon and menu for Cadence.
pub fn setup_tray(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    let status_item = MenuItem::with_id(
        app,
        "status_header",
        TrayStatus::Idle.menu_title(),
        false,
        None::<&str>,
    )?;
    let sep1 = PredefinedMenuItem::separator(app)?;
    let dictate_item = MenuItem::with_id(
        app,
        "toggle_dictate",
        "Toggle Dictation",
        true,
        Some("CmdOrCtrl+Shift+Space"),
    )?;
    let overlay_item = MenuItem::with_id(
        app,
        "toggle_overlay",
        "Toggle Overlay Pill",
        true,
        None::<&str>,
    )?;
    let dashboard_item = MenuItem::with_id(
        app,
        "open_dashboard",
        "Open Dashboard",
        true,
        None::<&str>,
    )?;
    let sep2 = PredefinedMenuItem::separator(app)?;
    let quit_item = MenuItem::with_id(
        app,
        "quit",
        "Quit Cadence",
        true,
        Some("CmdOrCtrl+Q"),
    )?;

    let menu = Menu::with_items(
        app,
        &[
            &status_item,
            &sep1,
            &dictate_item,
            &overlay_item,
            &dashboard_item,
            &sep2,
            &quit_item,
        ],
    )?;

    let mut builder = TrayIconBuilder::with_id(TRAY_ID)
        .menu(&menu)
        .show_menu_on_left_click(false)
        .tooltip(TrayStatus::Idle.tooltip());

    if let Some(icon) = app.default_window_icon() {
        builder = builder.icon(icon.clone());
    }

    builder
        .on_menu_event(|app, event| {
            match event.id.as_ref() {
                "toggle_dictate" => {
                    let _ = toggle_dictation(app);
                }
                "toggle_overlay" => {
                    let _ = crate::overlay::toggle_overlay(app.clone());
                }
                "open_dashboard" => {
                    show_main_window(app);
                }
                "quit" => {
                    app.exit(0);
                }
                _ => {}
            }
        })
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                let app = tray.app_handle();
                show_main_window(app);
            }
        })
        .build(app)?;

    Ok(())
}

/// Helper to bring the main dashboard window to the front.
pub fn show_main_window(app: &AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

/// Helper to toggle dictation via IPC.
pub fn toggle_dictation(app: &AppHandle) -> Result<(), String> {
    let status = crate::ipc::call_daemon("status", None);
    let is_rec = match status {
        Ok(val) => val
            .get("recording")
            .or_else(|| val.get("is_recording"))
            .and_then(|v| v.as_bool())
            .unwrap_or(false),
        Err(_) => false,
    };

    if is_rec {
        let _ = crate::ipc::call_daemon("dictate_stop", None);
        let _ = crate::overlay::emit_overlay_state(
            app.clone(),
            crate::overlay::OverlayPayload {
                state: crate::overlay::OverlayState::Done,
                text: Some("Dictation stopped".to_string()),
                app_name: None,
                duration_ms: None,
            },
        );
    } else {
        let _ = crate::ipc::call_daemon("dictate_start", None);
        let _ = crate::overlay::emit_overlay_state(
            app.clone(),
            crate::overlay::OverlayPayload {
                state: crate::overlay::OverlayState::Recording,
                text: Some("Listening...".to_string()),
                app_name: None,
                duration_ms: None,
            },
        );
    }
    Ok(())
}

/// Command to dynamically update the tray tooltip and menu title.
#[tauri::command]
pub fn cadence_update_tray(app: AppHandle, status: String) -> Result<(), String> {
    if let Some(tray) = app.tray_by_id(TRAY_ID) {
        let tray_status = match status.as_str() {
            "recording" => TrayStatus::Recording,
            "transcribing" => TrayStatus::Transcribing,
            "polishing" => TrayStatus::Polishing,
            "offline" => TrayStatus::Offline,
            _ => TrayStatus::Idle,
        };
        let _ = tray.set_tooltip(Some(tray_status.tooltip()));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tray_status_tooltips() {
        assert_eq!(TrayStatus::Idle.tooltip(), "Cadence: Idle (Cmd+Shift+Space)");
        assert_eq!(TrayStatus::Recording.tooltip(), "Cadence: Listening...");
        assert_eq!(TrayStatus::Transcribing.tooltip(), "Cadence: Transcribing...");
        assert_eq!(TrayStatus::Polishing.tooltip(), "Cadence: Polishing...");
        assert_eq!(TrayStatus::Offline.tooltip(), "Cadence: Daemon Offline");
    }

    #[test]
    fn test_tray_status_menu_titles() {
        assert_eq!(TrayStatus::Idle.menu_title(), "Status: Ready");
        assert_eq!(TrayStatus::Recording.menu_title(), "Status: Recording...");
        assert_eq!(TrayStatus::Transcribing.menu_title(), "Status: Transcribing...");
        assert_eq!(TrayStatus::Polishing.menu_title(), "Status: Polishing...");
        assert_eq!(TrayStatus::Offline.menu_title(), "Status: Daemon Offline");
    }
}
