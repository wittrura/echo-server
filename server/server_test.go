package server_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	. "example.com/echo-server/server"
)

// helper: start server on an ephemeral port
func startTestServer(t *testing.T) (addr string, cleanup func()) {
	t.Helper()

	ln, err := RunEchoServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("RunEchoServer failed: %v", err)
	}

	// Extract the real address (with chosen port)
	addr = ln.Addr().String()

	cleanup = func() {
		_ = ln.Close()
	}

	return addr, cleanup
}

// helper: dial with a timeout
func dialClient(t *testing.T, addr string) net.Conn {
	t.Helper()

	dialer := net.Dialer{
		Timeout: 2 * time.Second,
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial server %s: %v", addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	return conn
}

func TestEchoServer_EchoesSingleMessage(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	conn := dialClient(t, addr)
	defer func() {
		_ = conn.Close()
	}()

	msg := "hello, echo server\n"

	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("failed to write to server: %v", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read echo from server: %v", err)
	}

	if resp != msg {
		t.Fatalf("expected %q, got %q", msg, resp)
	}
}

func TestEchoServer_EchoesMultipleMessagesOnSingleConnection(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	conn := dialClient(t, addr)
	defer conn.Close()

	reader := bufio.NewReader(conn)

	msgs := []string{
		"first message\n",
		"second message\n",
		"third message\n",
	}

	for _, msg := range msgs {
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Fatalf("failed to write %q to server: %v", msg, err)
		}

		resp, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read echo for %q: %v", msg, err)
		}

		if resp != msg {
			t.Fatalf("for message %q expected %q, got %q", msg, msg, resp)
		}
	}
}

func TestEchoServer_HandlesMultipleClients(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	const clientCount = 5

	type result struct {
		id  int
		err error
	}

	results := make(chan result, clientCount)

	for i := range clientCount {
		go func(id int) {
			conn := dialClient(t, addr)
			defer conn.Close()

			reader := bufio.NewReader(conn)
			msg := "client-message\n"

			if _, err := conn.Write([]byte(msg)); err != nil {
				results <- result{id: id, err: err}
				return
			}

			resp, err := reader.ReadString('\n')
			if err != nil {
				results <- result{id: id, err: err}
				return
			}

			if resp != msg {
				results <- result{id: id, err: fmt.Errorf("expected %q, got %q", msg, resp)}
				return
			}

			results <- result{id: id, err: nil}
		}(i)
	}

	for range clientCount {
		res := <-results
		if res.err != nil {
			t.Fatalf("client %d failed: %v", res.id, res.err)
		}
	}
}

func TestHandleConn_EchoesDataUntilEOF(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	done := make(chan struct{})
	var totalBytes int64
	var handleErr error

	go func() {
		defer close(done)
		totalBytes, handleErr = HandleConn(serverSide, nil)
	}()

	payload := []byte("hello\nworld\n")

	// Write the payload to the server.
	if _, err := clientSide.Write(payload); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	// Read the echoed payload back.
	echoed := make([]byte, len(payload))
	if _, err := io.ReadFull(clientSide, echoed); err != nil {
		t.Fatalf("client read failed: %v", err)
	}

	if string(echoed) != string(payload) {
		t.Fatalf("expected echoed %q, got %q", string(payload), string(echoed))
	}

	// Close client to signal EOF to the server side.
	if err := clientSide.Close(); err != nil {
		t.Fatalf("client close failed: %v", err)
	}

	// Wait for HandleConn to finish, but don't hang forever.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("HandleConn did not return after client closed")
	}

	if handleErr != nil {
		t.Fatalf("expected nil error on normal EOF shutdown, got: %v", handleErr)
	}

	if totalBytes != int64(len(payload)) {
		t.Fatalf("expected totalBytes=%d, got %d", len(payload), totalBytes)
	}
}

func TestHandleConn_TreatsEOFAsNormalShutdown(t *testing.T) {
	serverSide, clientSide := net.Pipe()

	done := make(chan struct{})
	var handleErr error

	go func() {
		defer close(done)
		_, handleErr = HandleConn(serverSide, nil)
	}()

	// Immediately close client; server should see EOF and exit cleanly.
	if err := clientSide.Close(); err != nil {
		t.Fatalf("client close failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("HandleConn did not return after EOF")
	}

	if handleErr != nil {
		t.Fatalf("expected nil error on EOF, got: %v", handleErr)
	}
}

// errorConn is a test double for net.Conn that always returns a non-EOF error on Read.
type errorConn struct{}

var errTestRead = errors.New("test read error")

func (e *errorConn) Read(p []byte) (int, error)       { return 0, errTestRead }
func (e *errorConn) Write(p []byte) (int, error)      { return len(p), nil }
func (e *errorConn) Close() error                     { return nil }
func (e *errorConn) LocalAddr() net.Addr              { return dummyAddr("local") }
func (e *errorConn) RemoteAddr() net.Addr             { return dummyAddr("remote") }
func (e *errorConn) SetDeadline(time.Time) error      { return nil }
func (e *errorConn) SetReadDeadline(time.Time) error  { return nil }
func (e *errorConn) SetWriteDeadline(time.Time) error { return nil }

type dummyAddr string

func (d dummyAddr) Network() string { return "test" }
func (d dummyAddr) String() string  { return string(d) }

func TestHandleConn_PropagatesNonEOFError(t *testing.T) {
	conn := &errorConn{}

	_, err := HandleConn(conn, nil)
	if err == nil {
		t.Fatalf("expected non-nil error, got nil")
	}

	if !errors.Is(err, errTestRead) {
		t.Fatalf("expected error to wrap %v, got %v", errTestRead, err)
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
