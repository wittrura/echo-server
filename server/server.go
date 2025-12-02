package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// RunEchoServer starts a TCP echo server listening on addr.
// It returns the net.Listener so callers can close it to stop the server.
func RunEchoServer(addr string) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					fmt.Println("Connection was closed")
					return
				}
				log.Fatal(err)
			}

			go HandleConn(conn, nil, time.Duration(0))
		}
	}()
	return l, nil
}

func handleWithExplicitRead(conn net.Conn) {
	defer conn.Close()

	buffer := make([]byte, 1024)

	for {
		// read data from the connection
		n, err := conn.Read(buffer)

		if err != nil {
			if err == io.EOF {
				fmt.Println("Client disconnected")
			} else {
				fmt.Printf("Error reading from connection: %v\n", err)
			}
			return
		}
		receivedData := buffer[:n]

		// echo a response back
		_, err = conn.Write(receivedData)
		if err != nil {
			fmt.Printf("Error writing to connection: %v\n", err)
			return
		}
	}
}

// HandleConn reads from conn, echoes data back to the peer,
// and returns the total number of bytes echoed.
// EOF is treated as a normal shutdown and should not be returned as an error.
func HandleConn(conn net.Conn, wg *sync.WaitGroup, readTimeout time.Duration) (int64, error) {
	if wg != nil {
		defer wg.Done()
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	var totalBytesEchoed int64

	for {
		var readDeadline time.Time
		if readTimeout != 0 {
			readDeadline = time.Now().Add(readTimeout)
		}
		conn.SetReadDeadline(readDeadline)
		bytes, err := reader.ReadBytes(byte('\n'))
		if err != nil {
			if err != io.EOF {
				fmt.Println("failed to read data, err:", err)
				return totalBytesEchoed, err
			}
			return totalBytesEchoed, nil
		}

		bytesEchoed, err := conn.Write(bytes)
		if err != nil {
			fmt.Println("failed to write data, err:", err)
			return totalBytesEchoed, err
		}
		totalBytesEchoed += int64(bytesEchoed)
	}
}

// RunEchoServerWithContext starts an echo server on addr.
// It serves until ctx is canceled, then stops accepting new connections
// and waits for all active connections to finish.
// It returns the bound address (which may be different if you pass ":0")
// and a channel that's closed when shutdown is complete.
func RunEchoServerWithContext(ctx context.Context, addr string) (net.Addr, <-chan struct{}, error) {
	server := NewEchoServer(addr, 1000)
	address, err := server.Start(ctx)
	return address, server.done, err
}

// RunEchoServerWithLimits starts an echo server with a max active-connection limit.
func RunEchoServerWithLimits(ctx context.Context, addr string, maxConns int) (net.Addr, <-chan struct{}, error) {
	server := NewEchoServer(addr, maxConns)
	address, err := server.Start(ctx)
	return address, server.done, err
}

type EchoServer struct {
	addr     string
	ln       net.Listener
	maxConns int

	wg    sync.WaitGroup
	mu    sync.Mutex
	stats Stats

	ctx         context.Context
	cancel      context.CancelFunc
	readTimeout time.Duration

	done chan struct{}
}

func NewEchoServer(addr string, maxConns int) *EchoServer {
	return &EchoServer{
		addr:     addr,
		maxConns: maxConns,
		done:     make(chan struct{}),
	}
}

func (s *EchoServer) Start(ctx context.Context) (net.Addr, error) {
	s.ctx, s.cancel = context.WithCancel(ctx)

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return nil, err
	}
	s.ln = ln

	go s.acceptLoop()
	go s.shutdownWatcher()

	return ln.Addr(), nil
}

func (s *EchoServer) acceptLoop() {
	var activeConnections int32

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

		if activeConnections < int32(s.maxConns) {
			atomic.AddInt32(&activeConnections, 1)
			s.wg.Add(1)
			go func() {
				defer func() {
					atomic.AddInt32(&activeConnections, -1)
				}()
				bytes, _ := HandleConn(conn, &s.wg, s.readTimeout)
				s.handleConn(conn.RemoteAddr().String(), bytes)
			}()
		} else {
			_, _ = conn.Write([]byte("server busy, try again later\n"))
			conn.Close()
			s.rejectConn()
		}
	}
}

func (s *EchoServer) shutdownWatcher() {
	<-s.ctx.Done()
	fmt.Println("received ctx.Done")

	_ = s.ln.Close()
	fmt.Println("closed listener")

	s.wg.Wait()
	fmt.Println("waited for connections to finish")

	close(s.done)
	fmt.Println("closed done channel to notify client")
}

func (s *EchoServer) SetReadTimeout(d time.Duration) {
	s.readTimeout = d
}

func (s *EchoServer) Done() <-chan struct{} {
	return s.done
}

func (s *EchoServer) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats // return by value (safe)
}

func (s *EchoServer) handleConn(addr string, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.TotalConnections++
	s.stats.TotalBytesEchoed += bytes
	s.stats.Connections = append(s.stats.Connections, ConnStats{
		RemoteAddr:  addr,
		BytesEchoed: bytes,
	})
}

func (s *EchoServer) rejectConn() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.TotalRejected++
}

// ConnStats represents stats for a single connection.
type ConnStats struct {
	RemoteAddr  string
	BytesEchoed int64
}

// Stats represents global server stats.
type Stats struct {
	TotalConnections int64
	TotalBytesEchoed int64
	TotalRejected    int64
	Connections      []ConnStats
}
