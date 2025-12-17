package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"strings"
	"sync"
)

type Server struct {
	addr string

	ln      net.Listener
	clients map[*Client]bool

	wg sync.WaitGroup
	mu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func NewServer(addr string) *Server {
	return &Server{
		addr:    addr,
		done:    make(chan struct{}),
		clients: make(map[*Client]bool),
	}
}

// Start listens on the configured addr, starts accepting connections,
// and returns the bound address (for :0) or an error.
func (s *Server) Start(ctx context.Context) (net.Addr, error) {
	s.ctx, s.cancel = context.WithCancel(ctx)

	var err error
	s.ln, err = net.Listen("tcp", s.addr)
	if err != nil {
		return nil, err
	}

	go s.acceptLoop()
	go s.shutdownWatcher()

	return s.ln.Addr(), nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				fmt.Println("listener was closed")
				return
			}
			fmt.Println("accept error:", err)
			return
		}

		s.mu.Lock()
		c := newClient(conn)
		s.clients[c] = true
		s.mu.Unlock()

		s.wg.Add(1)
		go func() {
			_ = s.handleClient(c, &s.wg)
		}()
		go func() {
			c.write()
		}()
	}
}

// TODO: document
func (s *Server) handleClient(c *Client, wg *sync.WaitGroup) error {
	defer func() {
		wg.Done()

		c.close()

		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
	}()

	reader := bufio.NewReader(c.conn)

	for {
		msg, err := reader.ReadBytes(byte('\n'))
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if ne, ok := err.(*net.OpError); ok && ne.Err.Error() == "use of closed network connection" {
				return nil
			}

			fmt.Println("failed to read data, err:", err)
			return err
		}

		raw := string(msg)
		if after, ok := strings.CutPrefix(raw, "/nick "); ok {
			name := strings.TrimSpace(after)
			c.name = name
			continue
		}
		if after, ok := strings.CutPrefix(raw, "/join "); ok {
			room := strings.TrimSpace(after)
			c.room = room

			// Ack to allow clients/tests to know the server processed the join.
			ack := fmt.Appendf(nil, "joined %s\n", room)
			select {
			case c.send <- ack:
			default:
			}

			continue
		}
		if strings.TrimSpace(raw) == "/quit" {
			return nil
		}

		clients := s.copyClients()
		var peers []*Client
		for client := range clients {
			if client.room == c.room && client != c {
				peers = append(peers, client)
			}
		}

		if c.name != "" {
			msg = fmt.Appendf(nil, "%s: %s", c.name, raw)
		}

		for _, client := range peers {
			select {
			case client.send <- msg:
			default:
			}
		}
	}
}

func (s *Server) shutdownWatcher() {
	<-s.ctx.Done()
	fmt.Println("received ctx.Done")

	_ = s.ln.Close()
	fmt.Println("closed listener")

	clients := s.copyClients()
	for c := range clients {
		c.conn.Close()
	}
	fmt.Println("closed all connections")

	s.wg.Wait()
	fmt.Println("waited for connections to finish")

	close(s.done)
	fmt.Println("closed done channel to notify client")
}

func (s *Server) copyClients() map[*Client]bool {
	clients := make(map[*Client]bool)
	s.mu.Lock()
	maps.Copy(clients, s.clients)
	s.mu.Unlock()
	return clients
}

// Done returns a channel that is closed when the server has fully shut down
// (accept loop exited and all connection goroutines are done).
func (s *Server) Done() <-chan struct{} {
	return s.done
}

type Client struct {
	conn net.Conn
	send chan []byte

	mu sync.Mutex

	name string
	room string
}

func newClient(conn net.Conn) *Client {
	return &Client{
		conn: conn,
		send: make(chan []byte),
		room: "lobby",
	}
}

func (c *Client) write() {
	for msg := range c.send {
		if _, err := c.conn.Write(msg); err != nil {
			fmt.Println("failed to write data, err:", err)
			return
		}
	}
}

func (c *Client) close() {
	_ = c.conn.Close()
	close(c.send)
}
