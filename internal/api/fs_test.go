package api

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

func TestFSListDriveRoot(t *testing.T) {
	// "D:/" must be listable as a drive root; path.Clean would shorten it to
	// "D:" which names a per-drive working directory on Windows.
	if runtime.GOOS != "windows" {
		t.Skip("drive roots are a Windows concept")
	}
	f := newAPIFixture(t)
	resp, body := f.do(t, "GET", "/v1/fs/list?path="+url.QueryEscape(`D:\`), f.opTok, nil, nil)
	if resp.StatusCode != 200 {
		t.Skipf("no D: drive on this machine: %d", resp.StatusCode)
	}
	if got := body["path"].(string); got != "D:/" {
		t.Fatalf("drive root listed as %q, want D:/", got)
	}
}

func TestFSListDirsOnlyAndAuth(t *testing.T) {
	f := newAPIFixture(t)
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "proj-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "proj-b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "secret.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	// No credential is refused.
	resp, _ := f.do(t, "GET", "/v1/fs/list", "", nil, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated list: %d, want 401", resp.StatusCode)
	}

	// Without a path, the response offers starting places only.
	resp, body := f.do(t, "GET", "/v1/fs/list", f.opTok, nil, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("places: %d", resp.StatusCode)
	}
	places, _ := body["places"].([]any)
	if len(places) == 0 {
		t.Fatal("expected at least one place")
	}

	// A real listing returns directories only — never files.
	resp, body = f.do(t, "GET", "/v1/fs/list?path="+url.QueryEscape(base), f.opTok, nil, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("list: %d %v", resp.StatusCode, body)
	}
	entries, _ := body["entries"].([]any)
	names := map[string]bool{}
	for _, e := range entries {
		m, _ := e.(map[string]any)
		names[m["name"].(string)] = true
	}
	if !names["proj-a"] || !names["proj-b"] {
		t.Fatalf("folders missing from listing: %v", names)
	}
	if names["secret.txt"] {
		t.Fatal("a file appeared in a directories-only listing")
	}
	if got := body["path"].(string); got != domain.NormalizePath(base, "") {
		t.Fatalf("path = %q, want normalized %q", got, domain.NormalizePath(base, ""))
	}
}

func TestFSListRefusesStateDirAndSymlinks(t *testing.T) {
	f := newAPIFixture(t)

	// The state directory is refused outright.
	resp, _ := f.do(t, "GET", "/v1/fs/list?path="+url.QueryEscape(f.api.StateDir), f.opTok, nil, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("state dir listing: %d, want 403", resp.StatusCode)
	}
	// …and a relative path is rejected.
	resp, _ = f.do(t, "GET", "/v1/fs/list?path=not-absolute", f.opTok, nil, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("relative path: %d, want 400", resp.StatusCode)
	}

	// Symlinked directories are not offered for traversal.
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "linkdir")); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
	if err := os.Mkdir(filepath.Join(base, "realdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	resp, body := f.do(t, "GET", "/v1/fs/list?path="+url.QueryEscape(base), f.opTok, nil, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("list: %d", resp.StatusCode)
	}
	for _, e := range body["entries"].([]any) {
		m, _ := e.(map[string]any)
		if m["name"] == "linkdir" {
			t.Fatal("a symlinked directory was offered for traversal")
		}
	}
}

func TestFSMkdirCreatesExactlyOneFolder(t *testing.T) {
	f := newAPIFixture(t)
	base := t.TempDir()
	target := filepath.Join(base, "qiyan-sandbox")

	resp, body := f.do(t, "POST", "/v1/fs/mkdir", f.opTok, map[string]any{"path": target}, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("mkdir: %d %v", resp.StatusCode, body)
	}
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		t.Fatalf("the folder was not created: %v", err)
	}

	// Creating it again is a conflict, not a silent success.
	resp, _ = f.do(t, "POST", "/v1/fs/mkdir", f.opTok, map[string]any{"path": target}, nil)
	if resp.StatusCode != 409 {
		t.Fatalf("duplicate mkdir: %d, want 409", resp.StatusCode)
	}
	// A missing parent is refused rather than created recursively.
	resp, _ = f.do(t, "POST", "/v1/fs/mkdir", f.opTok, map[string]any{"path": filepath.Join(base, "missing", "deep")}, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("missing parent: %d, want 400", resp.StatusCode)
	}
	// The state directory is out of scope.
	resp, _ = f.do(t, "POST", "/v1/fs/mkdir", f.opTok, map[string]any{"path": filepath.Join(f.api.StateDir, "sub")}, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("mkdir in state dir: %d, want 403", resp.StatusCode)
	}
	// Relative paths are refused.
	resp, _ = f.do(t, "POST", "/v1/fs/mkdir", f.opTok, map[string]any{"path": "relative/dir"}, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("relative mkdir: %d, want 400", resp.StatusCode)
	}
}
