package chat_test

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"strings"
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

	// Sender should NOT receive its own message.
	_ = c1.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, err = r1.ReadString('\n')
	if err == nil {
		t.Fatalf("expected client1 to NOT receive its own message, but read succeeded")
	}
	_ = c1.SetReadDeadline(time.Time{})

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

	// c3 will act as the receiver (sender should not receive its own broadcast).
	c3 := dialChat(t, addr.String(), 1*time.Second)
	defer c3.Close()
	r3 := bufio.NewReader(c3)

	// Close c1 to simulate a client dropping
	_ = c1.Close()

	// c2 should still receive broadcasts, and server should not panic.
	if _, err := c2.Write([]byte("ping\n")); err != nil {
		t.Fatalf("c2 write failed: %v", err)
	}

	resp, err := r3.ReadString('\n')
	if err != nil {
		t.Fatalf("c3 failed to read: %v", err)
	}
	if resp != "ping\n" {
		t.Fatalf("c3 expected %q, got %q", "ping\n", resp)
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

	// c3 as receiver
	c3 := dialChat(t, addr.String(), 2*time.Second)
	defer c3.Close()
	r3 := bufio.NewReader(c3)

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

	resp, err := r3.ReadString('\n')
	if err != nil {
		t.Fatalf("client3 failed to read broadcast after client1 quit: %v", err)
	}
	if resp != msg {
		t.Fatalf("client3 expected %q, got %q", msg, resp)
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
	r1 := bufio.NewReader(c1)
	_ = c1.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, err = r1.ReadString('\n')
	if err == nil {
		t.Fatalf("expected c1 to NOT receive blue message, but read succeeded")
	}
}

// NOTE: There is a known send/close race during teardown.
// This will be resolved by the planned single-broadcaster refactor.
func TestChatServer_ActiveClientIsNotDisconnectedByIdleTimeout(t *testing.T) {
	ctx := t.Context()

	srv := NewServer("127.0.0.1:0")
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start chat server: %v", err)
	}

	sender := dialChat(t, addr.String(), 1*time.Second)
	defer sender.Close()

	recv := dialChat(t, addr.String(), 1*time.Second)
	defer recv.Close()
	r := bufio.NewReader(recv)

	for range 3 {
		if _, err := sender.Write([]byte("ping\n")); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		if _, err := r.ReadString('\n'); err != nil {
			t.Fatalf("read failed: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestChatServer_IdleClientDoesNotAffectOthers(t *testing.T) {
	ctx := t.Context()

	srv := NewServer("127.0.0.1:0")
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start chat server: %v", err)
	}

	// A a completely silent client should NOT be disconnected.
	// Only clients that start a line and then stall should time out.
	idle := dialChat(t, addr.String(), 1*time.Second)
	defer idle.Close()

	sender := dialChat(t, addr.String(), 1*time.Second)
	defer sender.Close()

	recv := dialChat(t, addr.String(), 1*time.Second)
	defer recv.Close()
	r := bufio.NewReader(recv)

	// Let time pass longer than the server's current (hardcoded) timeout.
	// The idle client should remain connected, and active clients should still work.
	time.Sleep(300 * time.Millisecond)

	msg := "still-alive\n"
	if _, err := sender.Write([]byte(msg)); err != nil {
		t.Fatalf("sender write failed: %v", err)
	}
	resp, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("receiver read failed: %v", err)
	}
	if resp != msg {
		t.Fatalf("expected %q, got %q", msg, resp)
	}
}

func TestChatServer_PartialLineStallIsDisconnected(t *testing.T) {
	ctx := t.Context()

	srv := NewServer("127.0.0.1:0")
	// You may hardcode the line-completion timeout internally for now (e.g. 200–250ms)
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start chat server: %v", err)
	}

	c := dialChat(t, addr.String(), 1*time.Second)
	defer c.Close()

	// Start a line but do not complete it. This should trigger the slowloris defense.
	if _, err := c.Write([]byte("partial")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// We expect the server to close the connection after the line-completion timeout.
	c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, err = bufio.NewReader(c).ReadByte()
	if err == nil {
		t.Fatalf("expected partial-line client to be disconnected, but read succeeded")
	}
}

func TestChatServer_LongLineTriggersBufferFullFragmentPath(t *testing.T) {
	ctx := t.Context()

	srv := NewServer("127.0.0.1:0")
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start chat server: %v", err)
	}

	// Sender and receiver (sender does not receive its own messages).
	sender := dialChat(t, addr.String(), 2*time.Second)
	defer sender.Close()

	recv := dialChat(t, addr.String(), 2*time.Second)
	defer recv.Close()
	r := bufio.NewReader(recv)

	// bufio.NewReader defaults to a 4096-byte buffer.
	// Make a line longer than that so server-side ReadSlice('\n') hits ErrBufferFull.
	payloadLen := 5000
	payload := bytes.Repeat([]byte("a"), payloadLen)
	msgBytes := append(payload, '\n')

	if len(msgBytes) <= 4096 {
		t.Fatalf("test requires msg > 4096 bytes, got %d", len(msgBytes))
	}

	if _, err := sender.Write(msgBytes); err != nil {
		t.Fatalf("sender write failed: %v", err)
	}

	// Receiver should get the full line, untruncated.
	_ = recv.SetReadDeadline(time.Now().Add(2 * time.Second))
	got, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("receiver read failed: %v", err)
	}

	expected := string(msgBytes)
	if got != expected {
		// Make failures readable if something truncates.
		if len(got) != len(expected) {
			t.Fatalf("expected %d bytes, got %d bytes", len(expected), len(got))
		}
		if !strings.HasSuffix(got, "\n") {
			t.Fatalf("expected newline-terminated message")
		}
		t.Fatalf("expected payload to match exactly")
	}
}
