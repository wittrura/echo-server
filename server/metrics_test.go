package server_test

import (
	"bufio"
	"context"
	"testing"
	"time"

	. "example.com/echo-server/server"
)

// Assumes dialWithTimeout(t, addr, timeout) already exists in this package.
// Assumes busyMessage const exists: const busyMessage = "server busy, try again later\n"

// Test that a single successful connection is reflected in server stats:
// - TotalConnections increments
// - TotalBytesEchoed >= payload size
// - One ConnStats entry with non-empty RemoteAddr and correct BytesEchoed.
func TestEchoServer_RecordsPerConnectionStats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewEchoServer("127.0.0.1:0", 10)

	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	conn := dialWithTimeout(t, addr.String(), 2*time.Second)
	reader := bufio.NewReader(conn)

	payload := "hello\nworld\n"
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("failed to write payload: %v", err)
	}

	// Read echoed data back (we know HandleConn echoes line-by-line).
	out1, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read first line: %v", err)
	}
	out2, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read second line: %v", err)
	}

	if out1+out2 != payload {
		t.Fatalf("expected echoed %q, got %q", payload, out1+out2)
	}

	// Close client so the handler can exit.
	if err := conn.Close(); err != nil {
		t.Fatalf("failed to close conn: %v", err)
	}

	// Trigger shutdown and wait for graceful completion.
	cancel()
	select {
	case <-srv.Done():
	case <-time.After(3 * time.Second):
		t.Fatalf("server did not shut down in stats test")
	}

	stats := srv.Stats()

	if stats.TotalConnections != 1 {
		t.Fatalf("expected TotalConnections=1, got %d", stats.TotalConnections)
	}

	if stats.TotalBytesEchoed < int64(len(payload)) {
		t.Fatalf("expected TotalBytesEchoed >= %d, got %d", len(payload), stats.TotalBytesEchoed)
	}

	if len(stats.Connections) != 1 {
		t.Fatalf("expected 1 ConnStats entry, got %d", len(stats.Connections))
	}

	connStats := stats.Connections[0]
	if connStats.BytesEchoed < int64(len(payload)) {
		t.Fatalf("expected ConnStats.BytesEchoed >= %d, got %d", len(payload), connStats.BytesEchoed)
	}
	if connStats.RemoteAddr == "" {
		t.Fatalf("expected non-empty RemoteAddr in ConnStats")
	}
}

// Test that the server updates stats for both accepted and rejected connections:
// - 1 allowed connection → TotalConnections=1
// - 1 rejected connection → TotalRejected=1
// - TotalBytesEchoed reflects the payload sent on the allowed connection.
func TestEchoServer_UpdatesStatsForAcceptedAndRejected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const maxConns = 1

	srv := NewEchoServer("127.0.0.1:0", maxConns)

	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Open the allowed connection and send a message.
	conn1 := dialWithTimeout(t, addr.String(), 2*time.Second)
	reader1 := bufio.NewReader(conn1)

	msg := "metrics-test-message\n"
	if _, err := conn1.Write([]byte(msg)); err != nil {
		t.Fatalf("failed to write on allowed connection: %v", err)
	}

	resp, err := reader1.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read echo on allowed connection: %v", err)
	}
	if resp != msg {
		t.Fatalf("expected %q on allowed connection, got %q", msg, resp)
	}

	// Open a second connection to trigger rejection.
	conn2 := dialWithTimeout(t, addr.String(), 2*time.Second)
	rejectedReader := bufio.NewReader(conn2)

	line, err := rejectedReader.ReadString('\n')
	if err != nil {
		t.Fatalf("expected to read rejection message, got error: %v", err)
	}
	if line != busyMessage {
		t.Fatalf("expected busy message %q, got %q", busyMessage, line)
	}
	_ = conn2.Close()

	// Close allowed connection so its handler can exit.
	if err := conn1.Close(); err != nil {
		t.Fatalf("failed to close allowed connection: %v", err)
	}

	// Initiate shutdown and wait for it to complete.
	cancel()
	select {
	case <-srv.Done():
	case <-time.After(3 * time.Second):
		t.Fatalf("server did not shut down in metrics rejection test")
	}

	stats := srv.Stats()

	if stats.TotalConnections != 1 {
		t.Fatalf("expected TotalConnections=1, got %d", stats.TotalConnections)
	}

	if stats.TotalRejected != 1 {
		t.Fatalf("expected TotalRejected=1, got %d", stats.TotalRejected)
	}

	if stats.TotalBytesEchoed < int64(len(msg)) {
		t.Fatalf("expected TotalBytesEchoed >= %d, got %d", len(msg), stats.TotalBytesEchoed)
	}
}
