pub mod commands {
    use serde::{Deserialize, Serialize};

    #[derive(Debug, Clone, Serialize, Deserialize)]
    pub struct DaemonStatus {
        pub ok: bool,
        pub status: String,
        pub version: String,
        pub recording: bool,
    }

    #[tauri::command]
    pub fn get_status() -> Result<DaemonStatus, String> {
        Ok(DaemonStatus {
            ok: true,
            status: "running".into(),
            version: "0.1.0".into(),
            recording: false,
        })
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .invoke_handler(tauri::generate_handler![commands::get_status])
        .run(tauri::generate_context!())
        .expect("error while running cadence desktop app");
}
#[cfg(test)]
mod tests {
    use super::commands::*;

    #[test]
    fn test_get_status() {
        let status = get_status().expect("get_status should succeed");
        assert!(status.ok);
        assert_eq!(status.status, "running");
        assert_eq!(status.version, "0.1.0");
        assert!(!status.recording);
    }
}
