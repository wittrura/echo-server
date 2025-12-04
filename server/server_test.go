package server_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	. "example.com/echo-server/server"
)

func TestEchoServer_EchoesSingleMessage(t *testing.T) {
	tests := []struct {
		name  string
		start func(t *testing.T) (addr string, cleanup func())
	}{
		{
			name: "RunEchoServer",
			start: func(t *testing.T) (string, func()) {
				addr, _, err := RunEchoServer("127.0.0.1:0")
				if err != nil {
					t.Fatalf("RunEchoServer failed: %v", err)
				}
				// No cleanup hook for this basic helper; process exit will clean up.
				return addr.String(), nil
			},
		},
		{
			name: "RunEchoServerWithContext",
			start: func(t *testing.T) (string, func()) {
				ctx, cancel := context.WithCancel(context.Background())
				addr, done, err := RunEchoServerWithContext(ctx, "127.0.0.1:0")
				if err != nil {
					cancel()
					t.Fatalf("RunEchoServerWithContext failed: %v", err)
				}
				cleanup := func() {
					cancel()
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Fatalf("server did not shut down in EchoesSingleMessage test")
					}
				}
				return addr.String(), cleanup
			},
		},
		{
			name: "RunEchoServerWithLimits",
			start: func(t *testing.T) (string, func()) {
				ctx, cancel := context.WithCancel(context.Background())
				addr, done, err := RunEchoServerWithLimits(ctx, "127.0.0.1:0", 10)
				if err != nil {
					cancel()
					t.Fatalf("RunEchoServerWithLimits failed: %v", err)
				}
				cleanup := func() {
					cancel()
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Fatalf("server did not shut down in EchoesSingleMessage limits test")
					}
				}
				return addr.String(), cleanup
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, cleanup := tt.start(t)
			if cleanup != nil {
				defer cleanup()
			}

			conn := dialWithTimeout(t, addr, 2*time.Second)
			defer conn.Close()

			reader := bufio.NewReader(conn)
			msg := "single-message\n"
			if _, err := conn.Write([]byte(msg)); err != nil {
				t.Fatalf("failed to write message: %v", err)
			}

			resp, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("failed to read echo: %v", err)
			}
			if resp != msg {
				t.Fatalf("expected %q, got %q", msg, resp)
			}
		})
	}
}

func TestEchoServer_EchoesMultipleMessagesOnSingleConnection(t *testing.T) {
	tests := []struct {
		name  string
		start func(t *testing.T) (addr string, cleanup func())
	}{
		{
			name: "RunEchoServer",
			start: func(t *testing.T) (string, func()) {
				addr, _, err := RunEchoServer("127.0.0.1:0")
				if err != nil {
					t.Fatalf("RunEchoServer failed: %v", err)
				}
				return addr.String(), nil
			},
		},
		{
			name: "RunEchoServerWithContext",
			start: func(t *testing.T) (string, func()) {
				ctx, cancel := context.WithCancel(context.Background())
				addr, done, err := RunEchoServerWithContext(ctx, "127.0.0.1:0")
				if err != nil {
					cancel()
					t.Fatalf("RunEchoServerWithContext failed: %v", err)
				}
				cleanup := func() {
					cancel()
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Fatalf("server did not shut down in MultiMessage test")
					}
				}
				return addr.String(), cleanup
			},
		},
		{
			name: "RunEchoServerWithLimits",
			start: func(t *testing.T) (string, func()) {
				ctx, cancel := context.WithCancel(context.Background())
				addr, done, err := RunEchoServerWithLimits(ctx, "127.0.0.1:0", 10)
				if err != nil {
					cancel()
					t.Fatalf("RunEchoServerWithLimits failed: %v", err)
				}
				cleanup := func() {
					cancel()
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Fatalf("server did not shut down in MultiMessage limits test")
					}
				}
				return addr.String(), cleanup
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, cleanup := tt.start(t)
			if cleanup != nil {
				defer cleanup()
			}

			conn := dialWithTimeout(t, addr, 2*time.Second)
			defer conn.Close()

			reader := bufio.NewReader(conn)
			messages := []string{"first\n", "second\n", "third\n"}

			for _, msg := range messages {
				if _, err := conn.Write([]byte(msg)); err != nil {
					t.Fatalf("failed to write message %q: %v", msg, err)
				}

				resp, err := reader.ReadString('\n')
				if err != nil {
					t.Fatalf("failed to read echo for %q: %v", msg, err)
				}
				if resp != msg {
					t.Fatalf("expected %q, got %q", msg, resp)
				}
			}
		})
	}
}

func TestEchoServer_HandlesMultipleClients(t *testing.T) {
	tests := []struct {
		name  string
		start func(t *testing.T) (addr string, cleanup func())
	}{
		{
			name: "RunEchoServer",
			start: func(t *testing.T) (string, func()) {
				addr, _, err := RunEchoServer("127.0.0.1:0")
				if err != nil {
					t.Fatalf("RunEchoServer failed: %v", err)
				}
				return addr.String(), nil
			},
		},
		{
			name: "RunEchoServerWithContext",
			start: func(t *testing.T) (string, func()) {
				ctx, cancel := context.WithCancel(context.Background())
				addr, done, err := RunEchoServerWithContext(ctx, "127.0.0.1:0")
				if err != nil {
					cancel()
					t.Fatalf("RunEchoServerWithContext failed: %v", err)
				}
				cleanup := func() {
					cancel()
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Fatalf("server did not shut down in MultipleClients test")
					}
				}
				return addr.String(), cleanup
			},
		},
		{
			name: "RunEchoServerWithLimits",
			start: func(t *testing.T) (string, func()) {
				ctx, cancel := context.WithCancel(context.Background())
				addr, done, err := RunEchoServerWithLimits(ctx, "127.0.0.1:0", 10)
				if err != nil {
					cancel()
					t.Fatalf("RunEchoServerWithLimits failed: %v", err)
				}
				cleanup := func() {
					cancel()
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Fatalf("server did not shut down in MultipleClients limits test")
					}
				}
				return addr.String(), cleanup
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, cleanup := tt.start(t)
			if cleanup != nil {
				defer cleanup()
			}

			// Spin up multiple clients and ensure each can echo independently.
			const clientCount = 5
			conns := make([]net.Conn, 0, clientCount)
			for i := 0; i < clientCount; i++ {
				conn := dialWithTimeout(t, addr, 2*time.Second)
				conns = append(conns, conn)
			}
			defer func() {
				for _, c := range conns {
					_ = c.Close()
				}
			}()

			for i, conn := range conns {
				reader := bufio.NewReader(conn)
				msg := fmt.Sprintf("client-%d-message\n", i)
				if _, err := conn.Write([]byte(msg)); err != nil {
					t.Fatalf("client %d failed to write: %v", i, err)
				}

				resp, err := reader.ReadString('\n')
				if err != nil {
					t.Fatalf("client %d failed to read echo: %v", i, err)
				}
				if resp != msg {
					t.Fatalf("client %d expected %q, got %q", i, msg, resp)
				}
			}
		})
	}
}

// helper: dial with custom timeout
func dialWithTimeout(t *testing.T, addr string, timeout time.Duration) net.Conn {
	t.Helper()

	dialer := net.Dialer{
		Timeout: timeout,
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial server %s: %v", addr, err)
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("failed to set deadline: %v", err)
	}
	return conn
}

func TestRunEchoServerWithContext_ShutsDownOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, done, err := RunEchoServerWithContext(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("RunEchoServerWithContext failed: %v", err)
	}

	// Verify it works before cancel.
	conn := dialWithTimeout(t, addr.String(), 2*time.Second)
	reader := bufio.NewReader(conn)

	msg := "before-cancel\n"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("failed to write before cancel: %v", err)
	}

	resp, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read echo before cancel: %v", err)
	}
	if resp != msg {
		t.Fatalf("expected %q before cancel, got %q", msg, resp)
	}
	_ = conn.Close()

	// Trigger shutdown.
	cancel()

	// Server should complete shutdown (stop accepting + finish handlers) within timeout.
	select {
	case <-done:
		// ok
	case <-time.After(3 * time.Second):
		t.Fatalf("server did not shut down within timeout after context cancel")
	}

	// After shutdown, new connections should fail to establish.
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	if _, err := dialer.Dial("tcp", addr.String()); err == nil {
		t.Fatalf("expected dialing after shutdown to fail, but it succeeded")
	}
}

func TestRunEchoServerWithContext_AllowsInFlightConnAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, done, err := RunEchoServerWithContext(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("RunEchoServerWithContext failed: %v", err)
	}

	// Open a connection before cancel.
	conn := dialWithTimeout(t, addr.String(), 2*time.Second)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// Cancel context to initiate shutdown of accept loop.
	cancel()

	// In-flight connection should still be usable briefly after cancel.
	msg := "after-cancel-still-works\n"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("failed to write on in-flight conn after cancel: %v", err)
	}

	resp, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read echo on in-flight conn after cancel: %v", err)
	}
	if resp != msg {
		t.Fatalf("expected %q on in-flight conn, got %q", msg, resp)
	}

	// Close the in-flight connection so the server can fully shut down.
	if err := conn.Close(); err != nil {
		t.Fatalf("failed to close in-flight conn: %v", err)
	}

	// Server should finish shutdown once active connections are done.
	select {
	case <-done:
		// ok
	case <-time.After(3 * time.Second):
		t.Fatalf("server did not shut down after in-flight connection completed")
	}
}

func TestRunEchoServerWithLimits_AllowsUpToMaxActiveConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const maxConns = 3

	addr, done, err := RunEchoServerWithLimits(ctx, "127.0.0.1:0", maxConns)
	if err != nil {
		t.Fatalf("RunEchoServerWithLimits failed: %v", err)
	}

	var conns []net.Conn
	for range maxConns {
		conn := dialWithTimeout(t, addr.String(), 2*time.Second)
		conns = append(conns, conn)
	}

	// Each active connection should behave like a normal echo connection.
	for _, conn := range conns {
		reader := bufio.NewReader(conn)
		msg := "hello-through-limited-server\n"

		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Fatalf("failed to write on limited server connection: %v", err)
		}

		resp, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read echo on limited server connection: %v", err)
		}
		if resp != msg {
			t.Fatalf("expected %q, got %q", msg, resp)
		}
	}

	// Cleanup: close connections and shut down server.
	for _, conn := range conns {
		_ = conn.Close()
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("server did not shut down after cancel in max-conn test")
	}
}

const busyMessage = "server busy, try again later\n"

func TestRunEchoServerWithLimits_RejectsConnectionsBeyondLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const maxConns = 2

	addr, done, err := RunEchoServerWithLimits(ctx, "127.0.0.1:0", maxConns)
	if err != nil {
		t.Fatalf("RunEchoServerWithLimits failed: %v", err)
	}

	// Open maxConns connections and keep them open.
	var conns []net.Conn
	for range maxConns {
		conn := dialWithTimeout(t, addr.String(), 2*time.Second)
		conns = append(conns, conn)
	}

	// Now attempt one extra connection; it should be explicitly rejected.
	rejectedConn := dialWithTimeout(t, addr.String(), 2*time.Second)
	rejectedReader := bufio.NewReader(rejectedConn)

	line, err := rejectedReader.ReadString('\n')
	if err != nil {
		t.Fatalf("expected to read rejection message, got error: %v", err)
	}
	if line != busyMessage {
		t.Fatalf("expected busy message %q, got %q", busyMessage, line)
	}

	// After rejection message, the server should close the connection.
	_, err = rejectedReader.ReadString('\n')
	if err == nil {
		t.Fatalf("expected connection to be closed after busy message")
	}
	_ = rejectedConn.Close()

	// If we now close one existing connection, we should be able to open another "normal" one.
	_ = conns[0].Close()

	newConn := dialWithTimeout(t, addr.String(), 2*time.Second)
	defer newConn.Close()

	reader := bufio.NewReader(newConn)
	msg := "allowed-after-slot-freed\n"
	if _, err := newConn.Write([]byte(msg)); err != nil {
		t.Fatalf("failed to write on new connection after freeing slot: %v", err)
	}
	resp, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read echo on new connection: %v", err)
	}
	if resp != msg {
		t.Fatalf("expected %q on new connection, got %q", msg, resp)
	}

	// Cleanup.
	for _, conn := range conns {
		_ = conn.Close()
	}
	newConn.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("server did not shut down in rejection test")
	}
}

// Helper: small timeout dialer
func dialShort(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	return conn
}

func TestEchoServer_ReadTimeout_AllowsNormalConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewEchoServer("127.0.0.1:0", 10)
	srv.SetReadTimeout(200 * time.Millisecond) // short but workable

	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	conn := dialShort(t, addr.String())
	reader := bufio.NewReader(conn)

	msg := "hello\n"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	resp, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	if resp != msg {
		t.Fatalf("expected %q, got %q", msg, resp)
	}

	_ = conn.Close()

	cancel()
	<-srv.Done()
}

func TestEchoServer_ReadTimeout_IdleClientIsDisconnected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewEchoServer("127.0.0.1:0", 10)
	srv.SetReadTimeout(150 * time.Millisecond)

	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	conn := dialShort(t, addr.String())
	defer conn.Close()

	// Do not send anything.
	// Just wait longer than the timeout.
	time.Sleep(300 * time.Millisecond)

	// The connection should be closed by the server due to timeout.
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err == nil {
		t.Fatalf("expected idle connection to be closed by server, but read %d bytes", n)
	}

	// Shutdown
	cancel()
	<-srv.Done()
}

func TestEchoServer_ReadTimeout_PartialActivityStillTimeouts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewEchoServer("127.0.0.1:0", 10)
	srv.SetReadTimeout(150 * time.Millisecond)

	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	conn := dialShort(t, addr.String())
	defer conn.Close()

	// Send one line.
	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Read echo.
	reader := bufio.NewReader(conn)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read echo failed: %v", err)
	}

	// Now go idle past timeout.
	time.Sleep(300 * time.Millisecond)

	// Expect server to close connection.
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err == nil {
		t.Fatalf("expected timeout disconnect; got %d bytes instead", n)
	}

	cancel()
	<-srv.Done()
}
