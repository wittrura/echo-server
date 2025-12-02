package main_test

import (
	"bufio"
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildBinary builds the echo-server binary into a temp dir and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "echo-server")

	cmd := exec.Command("go", "build", "-o", binPath, "../../cmd/echo-server")
	cmd.Env = append(os.Environ(), "GO111MODULE=on")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to build echo-server binary: %v\nstderr:\n%s", err, stderr.String())
	}

	return binPath
}

func TestEchoServerBinary_BuildsSuccessfully(t *testing.T) {
	_ = buildBinary(t) // will fail the test if build fails
}

// startServerProcess starts the built binary with the given args and waits until it logs the listening address.
// It returns the started cmd and the parsed TCP address.
func startServerProcess(t *testing.T, binPath string, args ...string) (*exec.Cmd, string) {
	t.Helper()

	cmd := exec.Command(binPath, args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start echo-server binary: %v", err)
	}

	scanner := bufio.NewScanner(stdoutPipe)
	var addr string
	readReady := make(chan struct{})

	go func() {
		defer close(readReady)
		for scanner.Scan() {
			line := scanner.Text()
			// Expect main to log something like: "listening on 127.0.0.1:12345"
			if strings.HasPrefix(line, "listening on ") {
				addr = strings.TrimSpace(strings.TrimPrefix(line, "listening on "))
				return
			}
		}
	}()

	select {
	case <-readReady:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("timeout waiting for server to report listening address")
	}

	if addr == "" {
		_ = cmd.Process.Kill()
		t.Fatalf("server did not print listening address")
	}

	return cmd, addr
}

// waitForProcessExit sends SIGINT to the process and waits for it to exit or times out.
func waitForProcessExit(t *testing.T, cmd *exec.Cmd) {
	t.Helper()

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("failed to send SIGINT: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server process exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("server did not exit within timeout after SIGINT")
	}
}

func TestEchoServerBinary_FlagVariants(t *testing.T) {
	binPath := buildBinary(t)

	type testCase struct {
		name string
		args []string
		run  func(t *testing.T, addr string, cmd *exec.Cmd)
	}

	tests := []testCase{
		{
			name: "defaults_no_flags",
			args: []string{}, // rely on defaults from main
			run: func(t *testing.T, addr string, cmd *exec.Cmd) {
				// Simple echo then graceful shutdown using defaults.
				conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
				if err != nil {
					_ = cmd.Process.Kill()
					t.Fatalf("failed to dial server at %s: %v", addr, err)
				}
				defer conn.Close()

				msg := "hello-defaults\n"
				if _, err := conn.Write([]byte(msg)); err != nil {
					_ = cmd.Process.Kill()
					t.Fatalf("failed to write to server: %v", err)
				}

				reader := bufio.NewReader(conn)
				resp, err := reader.ReadString('\n')
				if err != nil {
					_ = cmd.Process.Kill()
					t.Fatalf("failed to read echo from server: %v", err)
				}
				if resp != msg {
					_ = cmd.Process.Kill()
					t.Fatalf("expected %q, got %q", msg, resp)
				}

				// for the default (no-timeout) case: ensure the handler can exit
				_ = conn.Close()

				waitForProcessExit(t, cmd)
			},
		},
		{
			name: "custom_read_timeout_idles_disconnect",
			args: []string{
				"-addr", "127.0.0.1:0",
				"-max-conns", "10",
				"-read-timeout", "200ms",
			},
			run: func(t *testing.T, addr string, cmd *exec.Cmd) {
				// Connect and go idle; server should close the connection due to read timeout.
				conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
				if err != nil {
					_ = cmd.Process.Kill()
					t.Fatalf("failed to dial server at %s: %v", addr, err)
				}
				defer conn.Close()

				time.Sleep(400 * time.Millisecond)

				buf := make([]byte, 1)
				n, err := conn.Read(buf)
				if err == nil {
					_ = cmd.Process.Kill()
					t.Fatalf("expected connection to be closed by server due to timeout, read %d bytes instead", n)
				}

				waitForProcessExit(t, cmd)
			},
		},
		{
			name: "custom_max_conns_rejects_extra",
			args: []string{
				"-addr", "127.0.0.1:0",
				"-max-conns", "1",
				"-read-timeout", "500ms",
			},
			run: func(t *testing.T, addr string, cmd *exec.Cmd) {
				// First connection should be accepted and echo normally.
				conn1, err := net.DialTimeout("tcp", addr, 2*time.Second)
				if err != nil {
					_ = cmd.Process.Kill()
					t.Fatalf("failed to dial first connection: %v", err)
				}
				defer conn1.Close()

				reader1 := bufio.NewReader(conn1)
				if _, err := conn1.Write([]byte("one\n")); err != nil {
					_ = cmd.Process.Kill()
					t.Fatalf("failed to write on first connection: %v", err)
				}
				if _, err := reader1.ReadString('\n'); err != nil {
					_ = cmd.Process.Kill()
					t.Fatalf("failed to read echo on first connection: %v", err)
				}

				// Second connection should be rejected with the busy message.
				conn2, err := net.DialTimeout("tcp", addr, 2*time.Second)
				if err != nil {
					_ = cmd.Process.Kill()
					t.Fatalf("failed to dial second connection: %v", err)
				}
				defer conn2.Close()

				reader2 := bufio.NewReader(conn2)
				line, err := reader2.ReadString('\n')
				if err != nil {
					_ = cmd.Process.Kill()
					t.Fatalf("expected busy message on second connection, got error: %v", err)
				}
				if line != "server busy, try again later\n" {
					_ = cmd.Process.Kill()
					t.Fatalf("expected busy message, got %q", line)
				}

				// Close first connection so server can shut down.
				_ = conn1.Close()

				waitForProcessExit(t, cmd)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, addr := startServerProcess(t, binPath, tc.args...)
			tc.run(t, addr, cmd)
		})
	}
}
