package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// Filesystem browsing for project selection (R03-01). The endpoint exists so
// a stranger can pick an existing project folder without typing absolute
// paths, and can create one fresh sandbox for a first try. Boundaries:
//
//   - listing returns directories only — never file entries, never contents;
//   - symlinked entries are not offered for traversal (listing uses lstat
//     semantics), and a requested path is physically resolved once before it
//     is listed, so the picker always shows the real directory;
//   - the state directory (tokens live there, 0600) is refused outright, as
//     are paths inside it;
//   - creation makes exactly one new directory, 0700, when its parent exists.
//
// Both endpoints require the operator credential or the UI session; the
// creation is a mutation and therefore also carries the session CSRF check.

const fsListLimit = 1000

// fsNormalize canonicalizes a user-supplied path for filesystem use. Drive
// roots keep their trailing slash: domain.NormalizePath would reduce "D:/" to
// "D:", which on Windows names a per-drive working directory, not the root.
func fsNormalize(p string) string {
	q := domain.NormalizePath(p, "")
	if len(q) == 2 && q[1] == ':' {
		q += "/"
	}
	return q
}

// fsAbsolute reports whether the fsNormalize'd path is absolute. Drive roots
// ("D:/") are the one form path.Clean shortens, so they are accepted here on
// top of the domain check.
func fsAbsolute(pNorm string) bool {
	if len(pNorm) == 3 && pNorm[1] == ':' && pNorm[2] == '/' {
		c := pNorm[0]
		return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
	}
	return domain.IsAbsoluteNormalized(pNorm)
}

type fsEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// fsPlace is a starting point offered when no path is chosen yet.
type fsPlace struct {
	Kind string `json:"kind"` // home | desktop | documents | cwd | drive
	Path string `json:"path"`
}

// fsListResponse is the wire shape for GET /v1/fs/list.
type fsListResponse struct {
	Places    []fsPlace `json:"places"`
	Path      string    `json:"path,omitempty"`
	Parent    string    `json:"parent,omitempty"`
	Entries   []fsEntry `json:"entries,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
}

func (s *Server) fsPlaces() []fsPlace {
	var places []fsPlace
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		places = append(places, fsPlace{Kind: "home", Path: domain.NormalizePath(home, "")})
		for _, d := range []struct{ kind, name string }{{"desktop", "Desktop"}, {"documents", "Documents"}} {
			p := filepath.Join(home, d.name)
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				places = append(places, fsPlace{Kind: d.kind, Path: domain.NormalizePath(p, "")})
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		places = append(places, fsPlace{Kind: "cwd", Path: domain.NormalizePath(cwd, "")})
	}
	if runtime.GOOS == "windows" {
		for c := 'A'; c <= 'Z'; c++ {
			root := string(c) + ":\\"
			if fi, err := os.Stat(root); err == nil && fi.IsDir() {
				places = append(places, fsPlace{Kind: "drive", Path: string(c) + ":/"})
			}
		}
	} else {
		places = append(places, fsPlace{Kind: "drive", Path: "/"})
	}
	return places
}

// refusedFSDir reports whether the normalized path is the state directory or
// lies inside it.
func (s *Server) refusedFSDir(pNorm string) bool {
	nstate := domain.NormalizePath(s.StateDir, "")
	if nstate == "" {
		return false
	}
	return domain.PathWithin(nstate, pNorm)
}

// resolveDir physically resolves a requested directory, applying the shared
// refusals. It returns the resolved path (forward slashes) or "" after having
// written the error response.
func (s *Server) resolveDir(w http.ResponseWriter, raw string) string {
	pNorm := fsNormalize(raw)
	if !fsAbsolute(pNorm) {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "path must be an absolute path")
		return ""
	}
	if s.refusedFSDir(pNorm) {
		writeError(w, http.StatusForbidden, domain.ErrOutOfScope, "that folder is Qiyan's own state directory and cannot be browsed")
		return ""
	}
	resolved, err := filepath.EvalSymlinks(filepath.FromSlash(pNorm))
	if err != nil {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "that folder does not exist")
		return ""
	}
	fi, err := os.Stat(resolved)
	if err != nil || !fi.IsDir() {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "that path is not a folder")
		return ""
	}
	rNorm := fsNormalize(resolved)
	if s.refusedFSDir(rNorm) {
		writeError(w, http.StatusForbidden, domain.ErrOutOfScope, "that folder is Qiyan's own state directory and cannot be browsed")
		return ""
	}
	return rNorm
}

// listDirs lists the directory's real subdirectories. Symlinks are skipped —
// the picker never invites a path the validation layer cannot vouch for.
func (s *Server) listDirs(dirNorm string) ([]fsEntry, bool, error) {
	items, err := os.ReadDir(filepath.FromSlash(dirNorm))
	if err != nil {
		return nil, false, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name() < items[j].Name() })
	var entries []fsEntry
	truncated := false
	for _, it := range items {
		if !it.IsDir() {
			continue
		}
		child := domain.NormalizePath(filepath.Join(filepath.FromSlash(dirNorm), it.Name()), "")
		if s.refusedFSDir(child) {
			continue
		}
		if len(entries) >= fsListLimit {
			truncated = true
			break
		}
		entries = append(entries, fsEntry{Name: it.Name(), Path: child})
	}
	return entries, truncated, nil
}

func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	resp := fsListResponse{Places: s.fsPlaces()}
	raw := r.URL.Query().Get("path")
	if raw == "" {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	dirNorm := s.resolveDir(w, raw)
	if dirNorm == "" {
		return
	}
	entries, truncated, err := s.listDirs(dirNorm)
	if err != nil {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "that folder cannot be read")
		return
	}
	resp.Path = dirNorm
	if parent := fsNormalize(filepath.Dir(filepath.FromSlash(dirNorm))); parent != dirNorm {
		resp.Parent = parent
	}
	resp.Entries = entries
	resp.Truncated = truncated
	writeJSON(w, http.StatusOK, resp)
}

type fsMkdirReq struct {
	Path string `json:"path"`
}

func (s *Server) handleFSMkdir(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	var req fsMkdirReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	pNorm := fsNormalize(req.Path)
	if !fsAbsolute(pNorm) {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "path must be an absolute path")
		return
	}
	if s.refusedFSDir(pNorm) {
		writeError(w, http.StatusForbidden, domain.ErrOutOfScope, "Qiyan's own state directory cannot be created or browsed")
		return
	}
	base := filepath.Base(filepath.FromSlash(pNorm))
	if base == "." || base == ".." || base == string(filepath.Separator) {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "path must name a new folder")
		return
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(filepath.FromSlash(pNorm)))
	if err != nil {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "the parent folder does not exist")
		return
	}
	if fi, err := os.Stat(parent); err != nil || !fi.IsDir() {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "the parent path is not a folder")
		return
	}
	target := filepath.Join(parent, base)
	targetNorm := domain.NormalizePath(target, "")
	if s.refusedFSDir(targetNorm) {
		writeError(w, http.StatusForbidden, domain.ErrOutOfScope, "Qiyan's own state directory cannot be created or browsed")
		return
	}
	if _, err := os.Stat(target); err == nil {
		writeError(w, http.StatusConflict, domain.ErrConflict, "a file or folder with that name already exists")
		return
	}
	if err := os.Mkdir(target, 0o700); err != nil {
		fail(w, domain.NewError(domain.ErrInternal, "could not create the folder: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"created": targetNorm})
}
