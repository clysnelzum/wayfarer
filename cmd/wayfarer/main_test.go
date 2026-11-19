package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	testToken    = "0123456789abcdef0123456789abcdef" // 32 chars
	testChromeWS = "ws://127.0.0.1:9222/devtools/browser/test-id"
)

func mapGetenv(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

func TestParseConfig(t *testing.T) {
	baseEnv := map[string]string{
		"WAYFARER_API_TOKEN": testToken,
		"WAYFARER_CHROME_WS": testChromeWS,
	}

	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		want    config
		wantErr bool
	}{
		{
			name: "default listen",
			args: nil,
			env:  baseEnv,
			want: config{
				ListenAddr: "127.0.0.1:8765",
				APIToken:   testToken,
				ChromeWS:   testChromeWS,
			},
		},
		{
			name: "custom loopback ipv4 ephemeral port",
			args: []string{"-listen", "127.0.0.1:0"},
			env:  baseEnv,
			want: config{
				ListenAddr: "127.0.0.1:0",
				APIToken:   testToken,
				ChromeWS:   testChromeWS,
			},
		},
		{
			name: "custom loopback ipv6",
			args: []string{"-listen", "[::1]:9999"},
			env:  baseEnv,
			want: config{
				ListenAddr: "[::1]:9999",
				APIToken:   testToken,
				ChromeWS:   testChromeWS,
			},
		},
		{
			name: "scripts dir env fallback",
			args: nil,
			env: map[string]string{
				"WAYFARER_API_TOKEN":   testToken,
				"WAYFARER_CHROME_WS":   testChromeWS,
				"WAYFARER_SCRIPTS_DIR": "/env/scripts",
			},
			want: config{
				ListenAddr: "127.0.0.1:8765",
				APIToken:   testToken,
				ChromeWS:   testChromeWS,
				ScriptsDir: "/env/scripts",
			},
		},
		{
			name: "scripts dir flag overrides env",
			args: []string{"-scripts-dir", "/flag/scripts"},
			env: map[string]string{
				"WAYFARER_API_TOKEN":   testToken,
				"WAYFARER_CHROME_WS":   testChromeWS,
				"WAYFARER_SCRIPTS_DIR": "/env/scripts",
			},
			want: config{
				ListenAddr: "127.0.0.1:8765",
				APIToken:   testToken,
				ChromeWS:   testChromeWS,
				ScriptsDir: "/flag/scripts",
			},
		},
		{
			name:    "non-loopback 0.0.0.0 rejected",
			args:    []string{"-listen", "0.0.0.0:8765"},
			env:     baseEnv,
			wantErr: true,
		},
		{
			name:    "non-loopback lan ip rejected",
			args:    []string{"-listen", "192.168.1.10:8765"},
			env:     baseEnv,
			wantErr: true,
		},
		{
			name:    "hostname rejected",
			args:    []string{"-listen", "localhost:8765"},
			env:     baseEnv,
			wantErr: true,
		},
		{
			name:    "missing port rejected",
			args:    []string{"-listen", "127.0.0.1"},
			env:     baseEnv,
			wantErr: true,
		},
		{
			name:    "empty port rejected",
			args:    []string{"-listen", "127.0.0.1:"},
			env:     baseEnv,
			wantErr: true,
		},
		{
			name:    "non-numeric port rejected",
			args:    []string{"-listen", "127.0.0.1:http"},
			env:     baseEnv,
			wantErr: true,
		},
		{
			name:    "negative port rejected",
			args:    []string{"-listen", "127.0.0.1:-1"},
			env:     baseEnv,
			wantErr: true,
		},
		{
			name:    "port above 65535 rejected",
			args:    []string{"-listen", "127.0.0.1:65536"},
			env:     baseEnv,
			wantErr: true,
		},
		{
			name: "short token rejected",
			args: nil,
			env: map[string]string{
				"WAYFARER_API_TOKEN": "short-token-31-chars-0123456789",
				"WAYFARER_CHROME_WS": testChromeWS,
			},
			wantErr: true,
		},
		{
			name: "missing token rejected",
			args: nil,
			env: map[string]string{
				"WAYFARER_CHROME_WS": testChromeWS,
			},
			wantErr: true,
		},
		{
			name: "missing chrome ws rejected",
			args: nil,
			env: map[string]string{
				"WAYFARER_API_TOKEN": testToken,
			},
			wantErr: true,
		},
		{
			name: "whitespace chrome ws rejected",
			args: nil,
			env: map[string]string{
				"WAYFARER_API_TOKEN": testToken,
				"WAYFARER_CHROME_WS": "   ",
			},
			wantErr: true,
		},
		{
			name:    "unknown flag rejected",
			args:    []string{"-unknown-flag"},
			env:     baseEnv,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseConfig(tc.args, mapGetenv(tc.env))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseConfig(%v) succeeded with %+v, want error", tc.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseConfig(%v) error = %v, want nil", tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("parseConfig(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestRunCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := config{
		ListenAddr: "127.0.0.1:0",
		APIToken:   testToken,
		ChromeWS:   testChromeWS,
	}
	if err := run(ctx, cfg, nil); err != nil {
		t.Fatalf("run with canceled context returned error: %v", err)
	}
}

func TestRunHealthzAndGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config{
		ListenAddr: "127.0.0.1:0",
		APIToken:   testToken,
		ChromeWS:   testChromeWS,
	}

	addrCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- runWithReady(ctx, cfg, nil, func(addr string) {
			addrCh <- addr
		})
	}()

	var addr string
	select {
	case addr = <-addrCh:
	case err := <-errCh:
		t.Fatalf("runWithReady exited early with error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server to listen")
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr+"/healthz", nil)
	if err != nil {
		t.Fatalf("build healthz request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /healthz response: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("GET /healthz payload = %v, want status=ok", payload)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runWithReady shutdown returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for graceful shutdown")
	}
}

func TestRunInvalidScriptsDir(t *testing.T) {
	tmp := t.TempDir()

	filePath := filepath.Join(tmp, "not-a-dir.js")
	if err := os.WriteFile(filePath, []byte("() => ({})"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	emptyOverrideDir := filepath.Join(tmp, "empty-override")
	if err := os.Mkdir(emptyOverrideDir, 0o755); err != nil {
		t.Fatalf("mkdir empty-override: %v", err)
	}
	if err := os.WriteFile(filepath.Join(emptyOverrideDir, "feed.js"), []byte("   \n"), 0o600); err != nil {
		t.Fatalf("write empty feed.js: %v", err)
	}

	cases := []struct {
		name       string
		scriptsDir string
	}{
		{
			name:       "missing directory",
			scriptsDir: filepath.Join(tmp, "does-not-exist"),
		},
		{
			name:       "regular file instead of directory",
			scriptsDir: filePath,
		},
		{
			name:       "empty override script file",
			scriptsDir: emptyOverrideDir,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cfg := config{
				ListenAddr: "127.0.0.1:0",
				APIToken:   testToken,
				ChromeWS:   testChromeWS,
				ScriptsDir: tc.scriptsDir,
			}
			if err := run(ctx, cfg, nil); err == nil {
				t.Fatalf("run with scriptsDir %q succeeded, want error", tc.scriptsDir)
			}
		})
	}
}

func TestRunValidScriptsDirOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "feed.js"), []byte("() => ({ items: [] })"), 0o600); err != nil {
		t.Fatalf("write override feed.js: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := config{
		ListenAddr: "127.0.0.1:0",
		APIToken:   testToken,
		ChromeWS:   testChromeWS,
		ScriptsDir: dir,
	}
	if err := run(ctx, cfg, nil); err != nil {
		t.Fatalf("run with valid override dir returned error: %v", err)
	}
}
