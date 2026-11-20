package ipc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// NamedPipePrefix is the standard Windows named pipe prefix.
const NamedPipePrefix = `\\.\pipe\`

// DefaultPipeName is the default pipe name on Windows.
const DefaultPipeName = `\\.\pipe\cadence`

// DefaultSocketPath returns the standard IPC endpoint for the current OS.
func DefaultSocketPath() string {
	if runtime.GOOS == "windows" {
		return DefaultPipeName
	}
	// On Unix, use /tmp or user cache
	tmp := os.TempDir()
	return filepath.Join(tmp, "cadence.sock")
}

// IsNamedPipe checks whether the given path represents a Windows named pipe.
func IsNamedPipe(path string) bool {
	return strings.HasPrefix(path, NamedPipePrefix)
}

// Listen creates a network or named-pipe listener depending on the platform and address.
func Listen(network, address string) (net.Listener, error) {
	if network == "pipe" || IsNamedPipe(address) {
		return listenPipe(address)
	}
	return net.Listen(network, address)
}

// Dial connects to an IPC server via network or named pipe.
func Dial(ctx context.Context, network, address string) (net.Conn, error) {
	if network == "pipe" || IsNamedPipe(address) {
		return dialPipe(ctx, address)
	}
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}
