package api

import (
	"net/url"
	"os"
	"os/exec"
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

func TestFSStateDirAliasSpellingRefused(t *testing.T) {
	// Regression (adversarial review finding 1): the refusal must compare the
	// PHYSICALLY RESOLVED state directory, not only the configured spelling —
	// otherwise a request under an alias spelling (symlink/junction ancestor,
	// 8.3 short name, macOS /tmp) can list and create inside the state dir.
	realState := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias-state")
	if err := os.Symlink(realState, alias); err != nil {
		if runtime.GOOS == "windows" {
			// Junction fallback; works without developer mode.
			if out, err2 := exec.Command("cmd", "/c", "mklink", "/J", alias, realState).CombinedOutput(); err2 != nil {
				t.Skipf("cannot create a directory alias here: %v (%s)", err2, out)
			}
		} else {
			t.Skipf("cannot create symlinks here: %v", err)
		}
	}

	f := newAPIFixture(t)
	f.api.StateDir = alias // configured under the alias spelling
	if err := os.MkdirAll(filepath.Join(realState, "secret-dir"), 0o700); err != nil {
		t.Fatal(err)
	}

	// Listing the resolved spelling of the state dir must be refused.
	resp, _ := f.do(t, "GET", "/v1/fs/list?path="+url.QueryEscape(realState), f.opTok, nil, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("resolved state dir listing: %d, want 403", resp.StatusCode)
	}
	// Creating inside the resolved spelling must be refused.
	resp, _ = f.do(t, "POST", "/v1/fs/mkdir", f.opTok, map[string]any{"path": filepath.Join(realState, "created-through-alias")}, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("mkdir through the resolved spelling: %d, want 403", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(realState, "created-through-alias")); err == nil {
		t.Fatal("a folder was physically created inside the state directory")
	}
	// It must not be offered as a child of its parent either.
	parent, body := f.do(t, "GET", "/v1/fs/list?path="+url.QueryEscape(filepath.Dir(realState)), f.opTok, nil, nil)
	if parent.StatusCode == 200 {
		if entries, ok := body["entries"].([]any); ok {
			for _, e := range entries {
				m, _ := e.(map[string]any)
				if m["name"] == filepath.Base(realState) {
					t.Fatal("the state directory appeared in its parent's listing")
				}
			}
		}
	}
}

func TestFSMkdirRefusesUnsafeNames(t *testing.T) {
	f := newAPIFixture(t)
	base := t.TempDir()
	for _, name := range []string{"evil.", "evil ", "NUL", "CON", "com1.txt"} {
		resp, _ := f.do(t, "POST", "/v1/fs/mkdir", f.opTok, map[string]any{"path": filepath.Join(base, name)}, nil)
		if resp.StatusCode != 400 {
			t.Fatalf("mkdir %q: %d, want 400", name, resp.StatusCode)
		}
	}
	if entries, err := os.ReadDir(base); err != nil || len(entries) != 0 {
		t.Fatalf("unsafe names must not create anything: %v %v", entries, err)
	}
}

func TestFSNormalizeKeepsUNC(t *testing.T) {
	// path.Clean collapses "//server/share" to "/server/share"; fsNormalize
	// must restore the UNC form so Win32 resolves the share, not a local
	// same-shaped path on the current drive.
	if got := fsNormalize(`\\server\share\proj`); got != "//server/share/proj" {
		t.Fatalf("UNC normalize = %q", got)
	}
	if !fsAbsolute("//server/share") {
		t.Fatal("UNC form must be accepted as absolute")
	}
	if got := fsNormalize(`D:\`); got != "D:/" {
		t.Fatalf("drive root normalize = %q", got)
	}
}

func TestFSRootSlashRefusedOnWindows(t *testing.T) {
	// The sibling validators (filepath.IsAbs in validateProject) reject "/"
	// on Windows; the picker must agree with them.
	if runtime.GOOS != "windows" {
		t.Skip("the ambiguity is Windows-specific")
	}
	if fsAbsolute("/") {
		t.Fatal(`"/" must not be accepted as an absolute picker path on Windows`)
	}
	if !fsAbsolute("D:/") || !fsAbsolute("//server/share") {
		t.Fatal("drive roots and UNC forms stay absolute")
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
	// The API answers with the physically resolved directory (EvalSymlinks),
	// which also expands 8.3 short names like RUNNER~1 — resolve the
	// expectation the same way instead of comparing raw spellings.
	resolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	if got := body["path"].(string); got != domain.NormalizePath(resolved, "") {
		t.Fatalf("path = %q, want resolved %q", got, domain.NormalizePath(resolved, ""))
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
