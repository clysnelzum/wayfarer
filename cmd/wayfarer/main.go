package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/clysnelzum/wayfarer/internal/api"
	"github.com/clysnelzum/wayfarer/internal/chrome"
	"github.com/clysnelzum/wayfarer/internal/linkedin"
	"github.com/clysnelzum/wayfarer/internal/linkedin/scripts"
)

type config struct {
	ListenAddr string
	APIToken   string
	ChromeWS   string
	ScriptsDir string
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	fs := flag.NewFlagSet("wayfarer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg config
	fs.StringVar(&cfg.ListenAddr, "listen", "127.0.0.1:8765", "loopback HTTP listen address")
	fs.StringVar(&cfg.ScriptsDir, "scripts-dir", getenv("WAYFARER_SCRIPTS_DIR"), "optional directory of LinkedIn extractor JS overrides")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	host, portStr, err := net.SplitHostPort(cfg.ListenAddr)
	if err != nil {
		return config{}, fmt.Errorf("-listen must use a loopback IP and port: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return config{}, errors.New("-listen must use a loopback IP and port")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return config{}, errors.New("-listen must use a valid numeric port (0-65535)")
	}

	cfg.APIToken = getenv("WAYFARER_API_TOKEN")
	if len(cfg.APIToken) < 32 {
		return config{}, errors.New("set WAYFARER_API_TOKEN to a random token of at least 32 characters")
	}

	cfg.ChromeWS = getenv("WAYFARER_CHROME_WS")
	if strings.TrimSpace(cfg.ChromeWS) == "" {
		return config{}, errors.New("set WAYFARER_CHROME_WS to the Chrome DevTools browser WebSocket URL")
	}

	return cfg, nil
}

func loadScriptsRegistry(scriptsDir string) (*scripts.Registry, error) {
	if scriptsDir == "" {
		return scripts.NewRegistry()
	}
	info, err := os.Stat(scriptsDir)
	if err != nil {
		return nil, fmt.Errorf("scripts-dir %q: %w", scriptsDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scripts-dir %q is not a directory", scriptsDir)
	}
	return scripts.NewRegistry(scripts.WithOverrideFS(os.DirFS(scriptsDir)))
}

func run(ctx context.Context, cfg config, logger *slog.Logger) error {
	return runWithReady(ctx, cfg, logger, nil)
}

func runWithReady(ctx context.Context, cfg config, logger *slog.Logger, ready func(addr string)) (retErr error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	reg, err := loadScriptsRegistry(cfg.ScriptsDir)
	if err != nil {
		return err
	}
	digests := reg.Digests()
	logger.Info("loaded extractor scripts", "count", len(digests), "override_dir", cfg.ScriptsDir)
	logger.Debug("extractor script digests", "digests", digests)

	browser, err := chrome.New(cfg.ChromeWS, chrome.WithAuthGuard(reg.AuthGuard()), chrome.WithLogger(logger))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := browser.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close chrome browser: %w", closeErr))
		}
	}()

	client, err := linkedin.NewClient(browser, linkedin.WithScripts(reg), linkedin.WithLogger(logger))
	if err != nil {
		return err
	}

	handler, err := api.NewHandler(cfg.APIToken, client, api.WithLogger(logger))
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      95 * time.Second, // Allow rendered reads and tab cleanup to finish.
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}
	if ready != nil {
		ready(ln.Addr().String())
	}

	serveDone := make(chan struct{})
	shutdownErrCh := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := srv.Shutdown(shutdownCtx)
			if err != nil {
				err = errors.Join(err, srv.Close())
			}
			shutdownErrCh <- err
		case <-serveDone:
			shutdownErrCh <- nil
		}
	}()

	logger.Info("Wayfarer listening", "addr", ln.Addr().String())
	serveErr := srv.Serve(ln)
	close(serveDone)
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}

	shutdownErr := <-shutdownErrCh
	return errors.Join(serveErr, shutdownErr)
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := parseConfig(os.Args[1:], os.Getenv)
	if err != nil {
		logger.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("wayfarer stopped with error", "err", err)
		os.Exit(1)
	}
}
