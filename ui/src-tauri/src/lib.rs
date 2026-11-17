pub mod commands;
pub mod ipc;
pub mod overlay;
pub mod streaming;
pub mod tray;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .setup(|app| {
            if let Err(e) = tray::setup_tray(app.handle()) {
                eprintln!("Warning: failed to setup status bar tray icon: {}", e);
            }
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            commands::get_status,
            commands::cadence_ping,
            commands::cadence_get_status,
            commands::cadence_dictate_start,
            commands::cadence_dictate_stop,
            commands::cadence_get_history,
            commands::cadence_get_vocabulary,
            commands::cadence_add_vocabulary,
            commands::cadence_delete_vocabulary,
            commands::cadence_import_vocabulary,
            commands::cadence_export_vocabulary,
            commands::cadence_get_snippets,
            commands::cadence_set_snippet,
            commands::cadence_delete_snippet,
            commands::cadence_expand_snippet,
            commands::cadence_get_config,
            commands::cadence_set_config,
            commands::cadence_get_audio_devices,
            commands::cadence_get_polish_profiles,
            commands::cadence_paste,
            commands::cadence_get_privacy_stats,
            commands::cadence_clear_recoveries,
            commands::cadence_get_benchmark_stats,
            commands::cadence_run_benchmark,
            commands::cadence_search,
            commands::cadence_call,
            overlay::show_overlay,
            overlay::hide_overlay,
            overlay::toggle_overlay,
            overlay::emit_overlay_state,
            streaming::cadence_get_partial,
            streaming::cadence_emit_partial,
            tray::cadence_update_tray,
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
