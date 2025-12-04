package server_test

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"example.com/echo-server/server"
)

func TestHandleConn_STATS_ReturnsServerStats(t *testing.T) {
	ctx := t.Context()

	cs := server.NewEchoServer("127.0.0.1:0", 10)
	addr, err := cs.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer conn.Close()

	// Send a line to generate some stats.
	_, _ = conn.Write([]byte("hello\n"))
	reader := bufio.NewReader(conn)
	_, _ = reader.ReadString('\n') // ignore echo

	// Request stats
	_, _ = conn.Write([]byte("STATS\n"))
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read stats: %v", err)
	}

	if !strings.Contains(line, "bytes=") {
		t.Fatalf("expected stats to contain bytes=, got: %q", line)
	}
}

func TestHandleConn_CLOSE_ClosesClientConnection(t *testing.T) {
	ctx := t.Context()

	cs := server.NewEchoServer("127.0.0.1:0", 10)
	addr, err := cs.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Request close
	_, _ = conn.Write([]byte("CLOSE\n"))

	// Connection should close soon after.
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, err = reader.ReadByte()
	if err == nil {
		t.Fatalf("expected connection to close after CLOSE command, but read succeeded")
	}
}

func TestHandleConn_EchoStillWorksWithUnknownCommands(t *testing.T) {
	ctx := t.Context()

	cs := server.NewEchoServer("127.0.0.1:0", 10)
	addr, err := cs.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	_, _ = conn.Write([]byte("xyz\n"))
	resp, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read echo: %v", err)
	}

	if resp != "xyz\n" {
		t.Fatalf("expected echo of xyz, got %q", resp)
	}
}

func TestHandleConn_CommandsAreCaseInsensitive(t *testing.T) {
	ctx := t.Context()

	cs := server.NewEchoServer("127.0.0.1:0", 10)
	addr, err := cs.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	_, _ = conn.Write([]byte("stAtS\n"))
	resp, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read stats: %v", err)
	}

	if !strings.Contains(resp, "bytes=") {
		t.Fatalf("expected stats output, got %q", resp)
	}
}
