package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"example.com/echo-server/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9000", "tbd")
	maxConns := flag.Int("max-conns", 100, "tbd")
	readTimeout := flag.Duration("read-timeout", 0, "tbd")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	server := server.NewEchoServer(*addr, *maxConns)
	server.SetReadTimeout(*readTimeout)
	address, err := server.Start(ctx)
	if err != nil {
		log.Fatalf("failed to start server, err: %v", err)
	}

	fmt.Printf("listening on %s\n", address.String())

	<-ctx.Done()
	fmt.Println("received cancel request")

	<-server.Done()
	fmt.Println("finished shutdown")
}
