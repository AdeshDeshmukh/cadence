pub mod commands;
pub mod ipc;
pub mod overlay;
pub mod streaming;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .invoke_handler(tauri::generate_handler![
            commands::get_status,
            commands::cadence_ping,
            commands::cadence_get_status,
            commands::cadence_dictate_start,
            commands::cadence_dictate_stop,
            commands::cadence_get_history,
            commands::cadence_get_vocabulary,
            commands::cadence_get_snippets,
            commands::cadence_get_config,
            commands::cadence_set_config,
            commands::cadence_get_audio_devices,
            commands::cadence_get_polish_profiles,
            commands::cadence_call,
            overlay::show_overlay,
            overlay::hide_overlay,
            overlay::toggle_overlay,
            overlay::emit_overlay_state,
            streaming::cadence_get_partial,
            streaming::cadence_emit_partial,
        ])
        .run(tauri::generate_context!())
        .expect("error while running cadence desktop app");
}

#[cfg(test)]
mod tests {
    use super::commands::*;

    #[test]
    fn test_get_status_fallback() {
        let status = get_status().expect("get_status should succeed with status struct");
        assert_eq!(status.version, "0.1.0");
    }
}
