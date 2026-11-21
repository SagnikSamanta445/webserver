// Command proxy runs the caching HTTP/HTTPS forward proxy.
//
// Usage: proxy <port>
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/SagnikSamanta445/webserver/internal/cache"
	"github.com/SagnikSamanta445/webserver/internal/proxy"
	"github.com/SagnikSamanta445/webserver/internal/stats"
)

const reportInterval = 15 * time.Second

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: ./proxy <port>")
		os.Exit(1)
	}
	port, err := strconv.Atoi(os.Args[1])
	if err != nil || port < 0 || port > 65535 {
		fmt.Fprintf(os.Stderr, "invalid port %q\n", os.Args[1])
		os.Exit(1)
	}

	st := stats.New()
	lru := cache.New(cache.MaxCacheSize)
	lru.OnEvict = func(key string, size int) {
		fmt.Printf("EVICTING: %s (%d bytes)\n", key, size)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}
	fmt.Printf("Proxy listening on port %d\n", port)

	var printMu sync.Mutex
	printStats := func() {
		printMu.Lock()
		defer printMu.Unlock()
		stats.WriteReport(os.Stdout, st.Snapshot(), lru.Snapshot(), time.Now())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stats.RunReporter(ctx, reportInterval, printStats)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		fmt.Println("\nShutting down proxy...")
		printStats()
		os.Exit(0)
	}()

	srv := proxy.New(lru, st)
	srv.Logger = log.New(os.Stderr, "proxy: ", log.LstdFlags)
	if err := srv.Serve(ln); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
