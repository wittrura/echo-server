package chat_test

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"

	. "example.com/echo-server/chat"
)

func dialChat(t *testing.T, addr string, timeout time.Duration) net.Conn {
	t.Helper()

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial chat server %s: %v", addr, err)
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("failed to set deadline: %v", err)
	}
	return conn
}

func TestChatServer_BroadcastsMessageToOtherClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer("127.0.0.1:0")
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start chat server: %v", err)
	}

	// Two clients connect.
	c1 := dialChat(t, addr.String(), 2*time.Second)
	defer c1.Close()
	c2 := dialChat(t, addr.String(), 2*time.Second)
	defer c2.Close()

	r1 := bufio.NewReader(c1)
	r2 := bufio.NewReader(c2)

	// Client 1 sends a message.
	msg := "hello-from-c1\n"
	if _, err := c1.Write([]byte(msg)); err != nil {
		t.Fatalf("client1 failed to write: %v", err)
	}

	// Client 2 should receive that line.
	resp, err := r2.ReadString('\n')
	if err != nil {
		t.Fatalf("client2 failed to read broadcast: %v", err)
	}
	if resp != msg {
		t.Fatalf("client2 expected %q, got %q", msg, resp)
	}

	// Client 1 should receive that line.
	resp, err = r1.ReadString('\n')
	if err != nil {
		t.Fatalf("client1 failed to read broadcast: %v", err)
	}
	if resp != msg {
		t.Fatalf("client1 expected %q, got %q", msg, resp)
	}

	// Client 2 sends a message.
	msg = "hello-from-c2\n"
	if _, err := c2.Write([]byte(msg)); err != nil {
		t.Fatalf("client2 failed to write: %v", err)
	}

	// Client 1 should receive that line.
	resp, err = r1.ReadString('\n')
	if err != nil {
		t.Fatalf("client1 failed to read broadcast: %v", err)
	}
	if resp != msg {
		t.Fatalf("client1 expected %q, got %q", msg, resp)
	}

	// Client 2 should receive that line.
	resp, err = r2.ReadString('\n')
	if err != nil {
		t.Fatalf("client2 failed to read broadcast: %v", err)
	}
	if resp != msg {
		t.Fatalf("client2 expected %q, got %q", msg, resp)
	}

	// Shutdown server so goroutines can exit cleanly.
	cancel()
	select {
	case <-srv.Done():
	case <-time.After(3 * time.Second):
		t.Fatalf("chat server did not shut down after cancel")
	}

	// After shutdown, new connections should fail.
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	if _, err := dialer.Dial("tcp", addr.String()); err == nil {
		t.Fatalf("expected dialing after shutdown to fail, but it succeeded")
	}
}

func TestChatServer_BroadcastsToMultipleClients(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer("127.0.0.1:0")
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start chat server: %v", err)
	}

	// Three clients connect.
	const clientCount = 3
	conns := make([]net.Conn, 0, clientCount)
	readers := make([]*bufio.Reader, 0, clientCount)
	for i := 0; i < clientCount; i++ {
		conn := dialChat(t, addr.String(), 2*time.Second)
		conns = append(conns, conn)
		readers = append(readers, bufio.NewReader(conn))
	}
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()

	// Have client 0 send a message.
	senderIdx := 0
	msg := "broadcast-to-all-others\n"
	if _, err := conns[senderIdx].Write([]byte(msg)); err != nil {
		t.Fatalf("sender failed to write: %v", err)
	}

	// All *other* clients should receive that line.
	for i := 0; i < clientCount; i++ {
		if i == senderIdx {
			continue
		}
		resp, err := readers[i].ReadString('\n')
		if err != nil {
			t.Fatalf("client %d failed to read broadcast: %v", i, err)
		}
		if resp != msg {
			t.Fatalf("client %d expected %q, got %q", i, msg, resp)
		}
	}

	// Shutdown server.
	cancel()
	select {
	case <-srv.Done():
	case <-time.After(3 * time.Second):
		t.Fatalf("chat server did not shut down after cancel in multi-client test")
	}
}

func TestChatServer_SlowClientDoesNotBlockOthers(t *testing.T) {
	ctx := t.Context()

	srv := NewServer("127.0.0.1:0")
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start chat server: %v", err)
	}

	// Fast receiver
	fast := dialChat(t, addr.String(), 2*time.Second)
	defer fast.Close()
	fastR := bufio.NewReader(fast)

	// Slow receiver: we won't read from it at all.
	slow := dialChat(t, addr.String(), 2*time.Second)
	defer slow.Close()

	// Sender
	sender := dialChat(t, addr.String(), 2*time.Second)
	defer sender.Close()

	// Send message from sender
	msg := "hello-broadcast\n"
	if _, err := sender.Write([]byte(msg)); err != nil {
		t.Fatalf("sender failed to write: %v", err)
	}

	// Fast client MUST receive it quickly
	fast.SetDeadline(time.Now().Add(500 * time.Millisecond))
	resp, err := fastR.ReadString('\n')
	if err != nil {
		t.Fatalf("fast client failed to read broadcast: %v", err)
	}
	if resp != msg {
		t.Fatalf("fast client expected %q, got %q", msg, resp)
	}
}

func TestChatServer_DisconnectedClientIsRemovedCleanly(t *testing.T) {
	ctx := t.Context()

	srv := NewServer("127.0.0.1:0")
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start chat server: %v", err)
	}

	// Two clients
	c1 := dialChat(t, addr.String(), 1*time.Second)
	defer c1.Close()

	c2 := dialChat(t, addr.String(), 1*time.Second)
	defer c2.Close()
	r2 := bufio.NewReader(c2)

	// Close c1 to simulate a client dropping
	_ = c1.Close()

	// c2 should still receive broadcasts, and server should not panic.
	if _, err := c2.Write([]byte("ping\n")); err != nil {
		t.Fatalf("c2 write failed: %v", err)
	}

	resp, err := r2.ReadString('\n')
	if err != nil {
		t.Fatalf("c2 failed to read: %v", err)
	}
	if resp != "ping\n" {
		t.Fatalf("c2 expected %q, got %q", "ping\n", resp)
	}
}
