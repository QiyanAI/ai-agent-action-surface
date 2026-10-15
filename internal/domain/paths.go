package domain

import (
	"path"
	"runtime"
	"strings"
)

// Path normalization and containment.
//
// Honest scope (book §12.4): the checks here are lexical and platform-aware.
// They stop `..` escapes, relative-path tricks, separator confusion and case
// tricks on Windows. They are NOT an operating-system sandbox and cannot stop
// a process that resolves symlinks or races between check and use. Callers
// that can provide physically resolved paths (after evaluating symlinks) must
// do so and say so in their coverage record; callers that cannot must not
// claim the later action is enforced.

// NormalizePath converts p into the canonical form used for digests and policy
// checks: absolute against base when relative, forward slashes, cleaned, and
// with a Windows drive letter upper-cased. It performs no filesystem access —
// base must be the caller's own working directory (for hook input, that is the
// `cwd` field the runtime sent).
func NormalizePath(p, base string) string {
	if p == "" {
		return ""
	}
	q := strings.ReplaceAll(p, `\`, "/")
	// Strip \\?\ and \\?\UNC\ verbatim prefixes if present: they bypass
	// cleaning semantics and would otherwise compare unequal to their plain
	// spellings. \\?\C:\x and C:\x name the same file.
	if strings.HasPrefix(q, "//?/UNC/") {
		q = "//" + q[len("//?/UNC/"):]
	} else if strings.HasPrefix(q, "//?/") {
		q = q[len("//?/"):]
	}
	if !isAbsSlash(q) {
		if base == "" {
			base = "."
		}
		b := strings.ReplaceAll(base, `\`, "/")
		q = strings.TrimSuffix(b, "/") + "/" + q
	}
	q = path.Clean(q)
	// Upper-case the drive letter so c:/x and C:/x are one path.
	if len(q) >= 2 && q[1] == ':' {
		q = strings.ToUpper(q[:1]) + q[1:]
	}
	return q
}

// IsAbsoluteNormalized reports whether p is already in the absolute,
// forward-slash form NormalizePath produces — including that it is fully
// cleaned. A path containing "." or ".." segments is NOT normalized: it would
// pass a lexical prefix comparison against a scope root while the filesystem
// resolves it elsewhere. The control engine treats anything else as
// unrecognized rather than guessing.
func IsAbsoluteNormalized(p string) bool {
	if p == "" {
		return false
	}
	if strings.ContainsRune(p, '\\') {
		return false
	}
	if !isAbsSlash(p) {
		return false
	}
	// path.Clean removes "."/".." segments and duplicate slashes; an input
	// that is already canonical survives unchanged.
	return path.Clean(p) == p
}

// isAbsSlash reports whether a forward-slash path is absolute in the current
// platform's terms: "/x", "C:/x" or "//server/share/x".
func isAbsSlash(q string) bool {
	if strings.HasPrefix(q, "/") {
		return true
	}
	if len(q) >= 3 && q[1] == ':' && q[2] == '/' {
		c := q[0]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return true
		}
	}
	return false
}

// pathCompareKey folds case on Windows (and only there), where the filesystem
// itself is case-insensitive: not folding would let "C:/Proj/Secret" slip past
// a "C:/proj" root while still naming the same file.
func pathCompareKey(p string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

// PathWithin reports whether normalized path p lies inside normalized root.
// Equal paths count as within. The root "/" and "C:/" contain everything on
// their volume. Both inputs must already be normalized.
func PathWithin(rootNorm, pNorm string) bool {
	r := pathCompareKey(strings.TrimSuffix(rootNorm, "/"))
	q := pathCompareKey(pNorm)
	if q == r {
		return true
	}
	// Volume root keeps its trailing slash so the boundary check below needs
	// no special case beyond this prefix.
	if !strings.HasSuffix(r, "/") {
		r += "/"
	}
	return strings.HasPrefix(q, r)
}

// PathWithinRoots reports whether any of the normalized roots contains the
// normalized path.
func PathWithinRoots(rootsNorm []string, pNorm string) bool {
	for _, r := range rootsNorm {
		if r == "" {
			continue
		}
		if PathWithin(r, pNorm) {
			return true
		}
	}
	return false
}
