package server_test

import (
	"bufio"
	"fmt"
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
