package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/api"
	"github.com/QiyanAI/ai-agent-action-surface/internal/control"
	"github.com/QiyanAI/ai-agent-action-surface/internal/store"
)

// stateDirDefault resolves where Qiyan keeps its local state: the standard
// per-user config location unless overridden by QIYAN_STATE_DIR.
func stateDirDefault() string {
	if v := os.Getenv("QIYAN_STATE_DIR"); v != "" {
		return v
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ".qiyan"
	}
	return filepath.Join(cfg, "QiyanAI", "qiyan")
}

// serverInfo is what the hook and CLI clients read to find the service.
type serverInfo struct {
	Addr string `json:"addr"`
}

func serverInfoPath(dir string) string { return filepath.Join(dir, "server.json") }

// readServerInfo loads the running service's address; an error means no live
// service has been found, which every caller must treat as "deny".
func readServerInfo(dir string) (serverInfo, error) {
	var info serverInfo
	raw, err := os.ReadFile(serverInfoPath(dir))
	if err != nil {
		return info, fmt.Errorf("no Qiyan service info in %s (start it with `qiyan serve`)", dir)
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return info, fmt.Errorf("service info unreadable: %w", err)
	}
	return info, nil
}

// writeSecret writes a credential file readable only by the local user.
func writeSecret(path, value string) error {
	return os.WriteFile(path, []byte(value+"\n"), 0o600)
}

// readSecret reads a credential file and trims the trailing newline.
func readSecret(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := string(raw)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s, nil
}

// runServe starts the local control service: the single writer of the state
// store, the decision authority's API, and (from R02-06 on) the console.
func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:8760", "listen address (loopback only)")
	dir := fs.String("state-dir", stateDirDefault(), "state directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		fmt.Fprintf(stderr, "%s: invalid --addr %q: %v\n", programName, *addr, err)
		return 2
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		fmt.Fprintf(stderr, "%s: refusing to listen on %q — Qiyan's control service is loopback-only by design\n", programName, host)
		return 2
	}

	st, err := store.Open(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: cannot open state directory: %v\n", programName, err)
		return 1
	}
	defer st.Close()

	operatorToken := api.NewToken()
	hookToken := api.NewToken()
	if err := writeSecret(filepath.Join(*dir, "operator.token"), operatorToken); err != nil {
		fmt.Fprintf(stderr, "%s: cannot write operator token: %v\n", programName, err)
		return 1
	}
	if err := writeSecret(filepath.Join(*dir, "hook.token"), hookToken); err != nil {
		fmt.Fprintf(stderr, "%s: cannot write hook token: %v\n", programName, err)
		return 1
	}

	engine := control.New(st)
	srv := api.NewServer(st, engine, version, operatorToken, hookToken)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(stderr, "%s: cannot listen on %s: %v\n", programName, *addr, err)
		return 1
	}
	actual := ln.Addr().String()
	infoRaw, _ := json.Marshal(serverInfo{Addr: actual})
	if err := os.WriteFile(serverInfoPath(*dir), infoRaw, 0o600); err != nil {
		fmt.Fprintf(stderr, "%s: cannot write service info: %v\n", programName, err)
		return 1
	}

	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	fmt.Fprintf(stdout, "Qiyan %s control service listening on http://%s\n", version, actual)
	fmt.Fprintf(stdout, "state dir:      %s\n", *dir)
	fmt.Fprintf(stdout, "operator token: %s\n", filepath.Join(*dir, "operator.token"))
	fmt.Fprintf(stdout, "hook token:     %s\n", filepath.Join(*dir, "hook.token"))
	if err := st.BlockedReason(); err != nil {
		fmt.Fprintf(stderr, "WARNING: state store is BLOCKED (%v).\n", err)
		fmt.Fprintf(stderr, "Every controlled action will be denied until the state directory is repaired.\n")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()

	select {
	case <-ctx.Done():
		fmt.Fprintln(stdout, "shutting down...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "%s: server error: %v\n", programName, err)
			return 1
		}
	}

	if err := st.Compact(); err != nil {
		fmt.Fprintf(stderr, "%s: warning: final compaction failed: %v\n", programName, err)
	}
	return 0
}
