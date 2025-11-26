package server

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
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

			go HandleConn(conn)
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
func HandleConn(conn net.Conn) (int64, error) {
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
			return bytesEchoed, nil
		}
	}
}
