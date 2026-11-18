package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/clysnelzum/wayfarer/internal/api"
	"github.com/clysnelzum/wayfarer/internal/chrome"
	"github.com/clysnelzum/wayfarer/internal/linkedin"
)

func main() {
	addr := flag.String("listen", "127.0.0.1:8765", "loopback HTTP listen address")
	flag.Parse()
	host, _, err := net.SplitHostPort(*addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("-listen must use a loopback IP and port")
	}
	token := os.Getenv("WAYFARER_API_TOKEN")
	if len(token) < 32 {
		log.Fatal("set WAYFARER_API_TOKEN to a random token of at least 32 characters")
	}
	browser, err := chrome.New(os.Getenv("WAYFARER_CHROME_WS"))
	if err != nil {
		log.Fatal(err)
	}
	defer browser.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.New(token, linkedin.Client{Reader: browser}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second, // Allow the rendered read and tab cleanup to finish.
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		log.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(deadline)
	}()
	log.Printf("Wayfarer listening on %s", ln.Addr())
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-done
}
