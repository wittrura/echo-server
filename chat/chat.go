package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

type Server struct {
	addr string

	ln      net.Listener
	clients []net.Conn

	wg sync.WaitGroup
	mu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func NewServer(addr string) *Server {
	return &Server{
		addr: addr,
		done: make(chan struct{}),
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
				fmt.Println("Connection was closed")
				return
			}
			log.Fatal(err)
			return
		}

		s.mu.Lock()
		s.clients = append(s.clients, conn)
		s.mu.Unlock()

		s.wg.Add(1)
		go func() {
			_ = s.handleConn(conn, &s.wg)
		}()
	}
}

// TODO: document
func (s *Server) handleConn(conn net.Conn, wg *sync.WaitGroup) error {
	if wg != nil {
		defer wg.Done()
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	for {
		bytes, err := reader.ReadBytes(byte('\n'))
		if err != nil {
			if err != io.EOF {
				fmt.Println("failed to read data, err:", err)
				return err
			}
			return nil
		}

		// var response []byte
		// normalized := strings.ToUpper(strings.TrimSpace(string(bytes)))
		response := bytes

		for _, client := range s.clients {
			if client != conn {
				_, err := client.Write(response)
				if err != nil {
					fmt.Println("failed to write data, err:", err)
					return err
				}
			}
		}
	}
}

func (s *Server) shutdownWatcher() {
	<-s.ctx.Done()
	fmt.Println("received ctx.Done")

	_ = s.ln.Close()
	fmt.Println("closed listener")

	s.mu.Lock()
	clients := append([]net.Conn(nil), s.clients...)
	s.mu.Unlock()
	for _, c := range clients {
		_ = c.Close()
	}
	fmt.Println("closed all connections")

	s.wg.Wait()
	fmt.Println("waited for connections to finish")

	close(s.done)
	fmt.Println("closed done channel to notify client")
}

// Done returns a channel that is closed when the server has fully shut down
// (accept loop exited and all connection goroutines are done).
func (s *Server) Done() <-chan struct{} {
	return s.done
}
