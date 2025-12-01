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

			go HandleConn(conn, nil)
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
func HandleConn(conn net.Conn, wg *sync.WaitGroup) (int64, error) {
	if wg != nil {
		defer wg.Done()
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	var bytesEchoed int64
	for {
		bytes, err := reader.ReadBytes(byte('\n'))
		if err != nil {
			if err != io.EOF {
				fmt.Println("failed to read data, err:", err)
				return bytesEchoed, err
			}
			return bytesEchoed, nil
		}

		n, err := conn.Write(bytes)
		bytesEchoed += int64(n)
		if err != nil {
			fmt.Println("failed to write data, err:", err)
			return bytesEchoed, err
		}
	}
}

// RunEchoServerWithContext starts an echo server on addr.
// It serves until ctx is canceled, then stops accepting new connections
// and waits for all active connections to finish.
// It returns the bound address (which may be different if you pass ":0")
// and a channel that's closed when shutdown is complete.
func RunEchoServerWithContext(ctx context.Context, addr string) (net.Addr, <-chan struct{}, error) {
	return RunEchoServerWithLimits(ctx, addr, 1000)
}

// RunEchoServerWithLimits starts an echo server with a max active-connection limit.
func RunEchoServerWithLimits(ctx context.Context, addr string, maxConns int) (net.Addr, <-chan struct{}, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	done := make(chan struct{})

	var wg sync.WaitGroup
	go func() {
		<-ctx.Done()
		fmt.Println("received ctx.Done")

		_ = l.Close()
		fmt.Println("closed listener")

		wg.Wait()
		fmt.Println("waited for connections to finish")

		close(done)
		fmt.Println("closed done channel to notify client")
	}()

	go func(maxConns int) {
		var activeConnections int32

		for {
			conn, err := l.Accept()

			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					fmt.Println("Connection was closed")
					return
				}
				log.Fatal(err)
				return
			}

			if activeConnections < int32(maxConns) {
				atomic.AddInt32(&activeConnections, 1)
				wg.Add(1)
				go func() {
					defer func() {
						atomic.AddInt32(&activeConnections, -1)
					}()
					HandleConn(conn, &wg)
				}()
			} else {
				_, _ = conn.Write([]byte("server busy, try again later\n"))
				conn.Close()
			}
		}
	}(maxConns)

	return l.Addr(), done, nil
}
