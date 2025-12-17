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

func TestChatServer_NickCommandPrefixesMessages(t *testing.T) {
	ctx := t.Context()

	srv := NewServer("127.0.0.1:0")
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start chat server: %v", err)
	}

	// Two clients: c1 will set a nick and send, c2 will observe.
	c1 := dialChat(t, addr.String(), 2*time.Second)
	defer c1.Close()
	c2 := dialChat(t, addr.String(), 2*time.Second)
	defer c2.Close()

	r2 := bufio.NewReader(c2)

	// c1 sets nickname.
	if _, err := c1.Write([]byte("/nick alice\n")); err != nil {
		t.Fatalf("client1 failed to write /nick: %v", err)
	}

	// We do not expect /nick itself to be broadcast, so we don't read here.

	// c1 sends a normal chat line.
	if _, err := c1.Write([]byte("hello there\n")); err != nil {
		t.Fatalf("client1 failed to write chat line: %v", err)
	}

	// c2 should see the nick-prefixed message.
	resp, err := r2.ReadString('\n')
	if err != nil {
		t.Fatalf("client2 failed to read nick-prefixed message: %v", err)
	}
	expected := "alice: hello there\n"
	if resp != expected {
		t.Fatalf("client2 expected %q, got %q", expected, resp)
	}
}

func TestChatServer_QuitClosesClientButKeepsOthersAlive(t *testing.T) {
	ctx := t.Context()

	srv := NewServer("127.0.0.1:0")
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start chat server: %v", err)
	}

	// Two clients.
	c1 := dialChat(t, addr.String(), 2*time.Second)
	c2 := dialChat(t, addr.String(), 2*time.Second)
	defer c2.Close()

	r2 := bufio.NewReader(c2)

	// c1 issues /quit.
	if _, err := c1.Write([]byte("/quit\n")); err != nil {
		t.Fatalf("client1 failed to write /quit: %v", err)
	}

	// c1's connection should be closed shortly.
	c1.SetDeadline(time.Now().Add(300 * time.Millisecond))
	_, err = bufio.NewReader(c1).ReadByte()
	if err == nil {
		t.Fatalf("expected client1 connection to be closed after /quit, but read succeeded")
	}
	_ = c1.Close() // ensure it's closed on our side too

	// c2 should still be able to send and receive messages without panic.
	msg := "still-here\n"
	if _, err := c2.Write([]byte(msg)); err != nil {
		t.Fatalf("client2 failed to write after client1 quit: %v", err)
	}

	resp, err := r2.ReadString('\n')
	if err != nil {
		t.Fatalf("client2 failed to read its own broadcast after client1 quit: %v", err)
	}
	if resp != msg {
		t.Fatalf("client2 expected %q, got %q", msg, resp)
	}
}

func TestChatServer_JoinMovesClientAndBroadcastIsRoomScoped(t *testing.T) {
	ctx := t.Context()

	srv := NewServer("127.0.0.1:0")
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start chat server: %v", err)
	}

	// c1 + c2 start in lobby
	c1 := dialChat(t, addr.String(), 2*time.Second)
	defer c1.Close()
	c2 := dialChat(t, addr.String(), 2*time.Second)
	defer c2.Close()
	r2 := bufio.NewReader(c2)

	// c3 will join a different room
	c3 := dialChat(t, addr.String(), 2*time.Second)
	defer c3.Close()
	r3 := bufio.NewReader(c3)

	// Move c3 to room "blue"
	if _, err := c3.Write([]byte("/join blue\n")); err != nil {
		t.Fatalf("c3 failed to write /join: %v", err)
	}

	// Wait for server to process /join so the test isn't racy.
	ack, err := r3.ReadString('\n')
	if err != nil {
		t.Fatalf("c3 failed to read join ack: %v", err)
	}
	if ack != "joined blue\n" {
		t.Fatalf("expected join ack %q, got %q", "joined blue\n", ack)
	}

	// c1 sends a lobby message
	msg := "hello-lobby\n"
	if _, err := c1.Write([]byte(msg)); err != nil {
		t.Fatalf("c1 failed to write: %v", err)
	}

	// Current implementation broadcasts to the sender as well; drain c1's own lobby message
	// so it doesn't interfere with the later "should not receive" assertion.
	r1 := bufio.NewReader(c1)
	got1, err := r1.ReadString('\n')
	if err != nil {
		t.Fatalf("c1 failed to read its own lobby broadcast: %v", err)
	}
	if got1 != msg {
		t.Fatalf("c1 expected %q, got %q", msg, got1)
	}

	// c2 should receive it (still in lobby)
	got2, err := r2.ReadString('\n')
	if err != nil {
		t.Fatalf("c2 failed to read lobby broadcast: %v", err)
	}
	if got2 != msg {
		t.Fatalf("c2 expected %q, got %q", msg, got2)
	}

	// c3 should NOT receive lobby messages (deadline to prove absence)
	_ = c3.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, err = r3.ReadString('\n')
	if err == nil {
		t.Fatalf("expected c3 to NOT receive lobby message after joining blue, but read succeeded")
	}

	// Now c3 sends in blue
	msgBlue := "hello-blue\n"
	_ = c3.SetReadDeadline(time.Time{}) // clear deadline
	if _, err := c3.Write([]byte(msgBlue)); err != nil {
		t.Fatalf("c3 failed to write: %v", err)
	}

	// c1 should NOT receive blue messages
	_ = c1.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, err = r1.ReadString('\n')
	if err == nil {
		t.Fatalf("expected c1 to NOT receive blue message, but read succeeded")
	}
}
