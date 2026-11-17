use serde::{Deserialize, Serialize};
use std::env;
use std::path::{Path, PathBuf};
use std::time::Duration;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct IpcRequest {
    pub jsonrpc: String,
    pub id: String,
    pub method: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub params: Option<serde_json::Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct IpcError {
    pub code: i32,
    pub message: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct IpcResponse {
    pub jsonrpc: String,
    pub id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub result: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<IpcError>,
}

/// Resolves the socket path for the Cadence background daemon (`cadenced`).
///
/// Priority:
/// 1. `CADENCE_SOCK_PATH` environment variable
/// 2. `CADENCE_SOCKET_PATH` environment variable
/// 3. `$HOME/.cache/cadence/cadenced.sock` (standard daemon path)
/// 4. `$HOME/.config/cadence/cadenced.sock` (alternate config path)
/// 5. `/tmp/cadenced.sock` (system fallback)
pub fn resolve_socket_path() -> PathBuf {
    if let Ok(p) = env::var("CADENCE_SOCK_PATH") {
        if !p.trim().is_empty() {
            return PathBuf::from(p.trim());
        }
    }
    if let Ok(p) = env::var("CADENCE_SOCKET_PATH") {
        if !p.trim().is_empty() {
            return PathBuf::from(p.trim());
        }
    }

    if let Ok(home) = env::var("HOME") {
        let home_path = PathBuf::from(home);
        let cache_sock = home_path.join(".cache").join("cadence").join("cadenced.sock");
        if cache_sock.exists() {
            return cache_sock;
        }
        let config_sock = home_path.join(".config").join("cadence").join("cadenced.sock");
        if config_sock.exists() {
            return config_sock;
        }
        return cache_sock;
    }

    PathBuf::from("/tmp/cadenced.sock")
}

/// Dispatches an IPC request to a UNIX domain socket with configurable timeout.
#[cfg(unix)]
pub fn call_socket(
    socket_path: &Path,
    method: &str,
    params: Option<serde_json::Value>,
    timeout: Duration,
) -> Result<serde_json::Value, String> {
    use std::io::{BufRead, BufReader, Write};
    use std::os::unix::net::UnixStream;

    let mut stream = UnixStream::connect(socket_path).map_err(|e| {
        format!(
            "daemon offline: cannot connect to socket '{}': {}",
            socket_path.display(),
            e
        )
    })?;

    stream
        .set_read_timeout(Some(timeout))
        .map_err(|e| format!("failed to set read timeout: {}", e))?;
    stream
        .set_write_timeout(Some(timeout))
        .map_err(|e| format!("failed to set write timeout: {}", e))?;

    let req = IpcRequest {
        jsonrpc: "2.0".to_string(),
        id: "tauri-1".to_string(),
        method: method.to_string(),
        params,
    };

    let mut payload =
        serde_json::to_string(&req).map_err(|e| format!("json encode error: {}", e))?;
    payload.push('\n');

    stream
        .write_all(payload.as_bytes())
        .map_err(|e| format!("socket write error: {}", e))?;
    stream
        .flush()
        .map_err(|e| format!("socket flush error: {}", e))?;

    let mut reader = BufReader::new(stream);
    let mut line = String::new();
    let bytes_read = reader
        .read_line(&mut line)
        .map_err(|e| format!("socket read error: {}", e))?;

    if bytes_read == 0 {
        return Err("daemon closed connection prematurely".to_string());
    }

    let resp: IpcResponse = serde_json::from_str(line.trim()).map_err(|e| {
        format!(
            "invalid json response from daemon: {}: raw={}",
            e,
            line.trim()
        )
    })?;

    if let Some(err) = resp.error {
        return Err(format!("daemon error {}: {}", err.code, err.message));
    }

    Ok(resp.result.unwrap_or(serde_json::Value::Null))
}

#[cfg(not(unix))]
pub fn call_socket(
    _socket_path: &Path,
    _method: &str,
    _params: Option<serde_json::Value>,
    _timeout: Duration,
) -> Result<serde_json::Value, String> {
    Err("UNIX domain socket is not supported on non-unix platforms".to_string())
}

/// Convenience helper to call the default Cadence daemon socket.
pub fn call_daemon(
    method: &str,
    params: Option<serde_json::Value>,
) -> Result<serde_json::Value, String> {
    let sock = resolve_socket_path();
    call_socket(&sock, method, params, Duration::from_secs(5))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::io::{BufRead, BufReader, Write};
    use std::os::unix::net::UnixListener;
    use std::thread;

    #[test]
    fn test_resolve_socket_path_env_override() {
        let temp_sock = "/tmp/test_cadence_override.sock";
        env::set_var("CADENCE_SOCK_PATH", temp_sock);
        let resolved = resolve_socket_path();
        assert_eq!(resolved, PathBuf::from(temp_sock));
        env::remove_var("CADENCE_SOCK_PATH");
    }

    #[test]
    fn test_ipc_serialization() {
        let req = IpcRequest {
            jsonrpc: "2.0".to_string(),
            id: "1".to_string(),
            method: "ping".to_string(),
            params: None,
        };
        let serialized = serde_json::to_string(&req).expect("serialize req");
        assert!(serialized.contains("\"method\":\"ping\""));
        assert!(!serialized.contains("\"params\""));

        let resp_json = r#"{"jsonrpc":"2.0","id":"1","result":"pong"}"#;
        let resp: IpcResponse = serde_json::from_str(resp_json).expect("deserialize resp");
        assert_eq!(resp.result, Some(serde_json::json!("pong")));
        assert!(resp.error.is_none());
    }

    #[test]
    fn test_mock_socket_ping_pong() {
        let sock_path = env::temp_dir().join(format!(
            "cadence_test_ping_{}_{}.sock",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let _ = fs::remove_file(&sock_path);
        let listener = UnixListener::bind(&sock_path).expect("bind mock unix socket");

        let handle = thread::spawn(move || {
            let (stream, _) = listener.accept().expect("accept connection");
            let mut reader = BufReader::new(stream.try_clone().expect("clone stream"));
            let mut line = String::new();
            reader.read_line(&mut line).expect("read line");

            let req: IpcRequest = serde_json::from_str(line.trim()).expect("parse request");
            assert_eq!(req.method, "ping");

            let resp = IpcResponse {
                jsonrpc: "2.0".to_string(),
                id: req.id,
                result: Some(serde_json::json!("pong")),
                error: None,
            };
            let mut payload = serde_json::to_string(&resp).unwrap();
            payload.push('\n');

            let mut writer = stream;
            writer.write_all(payload.as_bytes()).expect("write resp");
            writer.flush().expect("flush resp");
        });

        let res = call_socket(&sock_path, "ping", None, Duration::from_secs(2))
            .expect("call_socket ping should succeed");
        assert_eq!(res, serde_json::json!("pong"));

        handle.join().expect("server thread finish");
        let _ = fs::remove_file(&sock_path);
    }

    #[test]
    fn test_mock_socket_status_and_params() {
        let sock_path = env::temp_dir().join(format!(
            "cadence_test_status_{}_{}.sock",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let _ = fs::remove_file(&sock_path);
        let listener = UnixListener::bind(&sock_path).expect("bind status unix socket");

        let handle = thread::spawn(move || {
            let (stream, _) = listener.accept().expect("accept");
            let mut reader = BufReader::new(stream.try_clone().expect("clone"));
            let mut line = String::new();
            reader.read_line(&mut line).expect("read line");

            let req: IpcRequest = serde_json::from_str(line.trim()).expect("parse request");
            assert_eq!(req.method, "status");

            let resp = IpcResponse {
                jsonrpc: "2.0".to_string(),
                id: req.id,
                result: Some(serde_json::json!({
                    "recording": false,
                    "mode": "dictation",
                    "engine": "whisper",
                    "active_app": "com.apple.Terminal"
                })),
                error: None,
            };
            let mut payload = serde_json::to_string(&resp).unwrap();
            payload.push('\n');

            let mut writer = stream;
            writer.write_all(payload.as_bytes()).expect("write");
            writer.flush().expect("flush");
        });

        let res = call_socket(&sock_path, "status", None, Duration::from_secs(2))
            .expect("call_socket status should succeed");
        assert_eq!(res["mode"], "dictation");
        assert_eq!(res["engine"], "whisper");
        assert_eq!(res["recording"], false);

        handle.join().expect("thread join");
        let _ = fs::remove_file(&sock_path);
    }

    #[test]
    fn test_mock_socket_daemon_error() {
        let sock_path = env::temp_dir().join(format!(
            "cadence_test_err_{}_{}.sock",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let _ = fs::remove_file(&sock_path);
        let listener = UnixListener::bind(&sock_path).expect("bind");

        let handle = thread::spawn(move || {
            let (stream, _) = listener.accept().expect("accept");
            let mut reader = BufReader::new(stream.try_clone().expect("clone"));
            let mut line = String::new();
            let _ = reader.read_line(&mut line);

            let resp = IpcResponse {
                jsonrpc: "2.0".to_string(),
                id: "tauri-1".to_string(),
                result: None,
                error: Some(IpcError {
                    code: -32601,
                    message: "method not found".to_string(),
                }),
            };
            let mut payload = serde_json::to_string(&resp).unwrap();
            payload.push('\n');

            let mut writer = stream;
            writer.write_all(payload.as_bytes()).expect("write");
            writer.flush().expect("flush");
        });

        let err = call_socket(&sock_path, "unknown_method", None, Duration::from_secs(2))
            .expect_err("should return daemon error");
        assert!(err.contains("daemon error -32601: method not found"));

        handle.join().expect("join");
        let _ = fs::remove_file(&sock_path);
    }

    #[test]
    fn test_socket_daemon_offline() {
        let non_existent = PathBuf::from("/tmp/non_existent_cadence_socket_12345.sock");
        let err = call_socket(&non_existent, "ping", None, Duration::from_secs(1))
            .expect_err("expected offline error");
        assert!(err.contains("daemon offline"));
    }
}
