package control

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// Command classification.
//
// Honest scope (book §09): this is NOT a shell parser and Qiyan does not claim
// to "safely restrict arbitrary shell commands by string matching". The
// classifier recognizes a small, curated set of read-only programs and a set
// of known high-impact programs; everything else — including anything with
// shell metacharacters, any path-form program, and any argument form that can
// redirect output somewhere unknown — is gated behind explicit user approval.
// The gate is the approval, not a claim of semantic understanding.
//
// Consequences of that honesty, applied deliberately after adversarial review
// and tightened again for R02-SEC-01:
//
//   - Nothing is auto-allowed whose recognized behavior cannot be *proven*
//     to be side-effect-free. Executing project code (go test), persisting
//     configuration (go env -w/-u), setting the system clock (date -s, or
//     BSD-style positional date) and setting the hostname (hostname NAME)
//     are side effects this classifier cannot rule out, so those forms are
//     NOT on the safe list and require explicit approval.
//   - Programs that can write files through their own flags (sort -o,
//     uniq in out, …) are NOT on the safe list at all.
//   - Subcommand programs (go, git) are classified only after skipping flags
//     that take a value, and any flag that redirects output (-o, --output,
//     -toolexec, -vettool, …) or can run an external helper demotes the
//     command.
//   - Git subcommands that transform blob content (diff, show, log, blame)
//     are not safe: repository configuration (.gitattributes textconv,
//     diff.external, GIT_EXTERNAL_DIFF) can make them execute helper
//     programs. Only content-free readers (status, rev-parse, ls-files,
//     describe, shortlog) are kept.
//   - An unknown flag before a subcommand demotes the command: it might have
//     swallowed the real subcommand token.
//   - Path-form programs (./x, /usr/bin/x, ..\\x) are never vouched for by
//     base name, and on Windows a bare name is vouched only if the working
//     directory does not contain a same-named executable that cmd.exe would
//     find first.

// CommandClass is the classifier's verdict on one command line.
type CommandClass string

const (
	// CmdSafe: a curated program in a curated form whose recognized behavior
	// is proven free of side effects — a print-only tool, or a compiler-class
	// subcommand that inspects or compiles without running project code and
	// whose default output lands in the working tree the scope already
	// covers. Commands that run project code or persist configuration are
	// never CmdSafe (R02-SEC-01).
	CmdSafe CommandClass = "safe"
	// CmdSensitive: a known high-impact program (delete, network, package
	// management, process control, shell wrappers, interpreters), or a
	// subcommand form that could not be verified.
	CmdSensitive CommandClass = "sensitive"
	// CmdUnrecognized: anything else, including compound or dynamic commands,
	// path-form programs, and name-shadowed programs.
	CmdUnrecognized CommandClass = "unrecognized"
)

// shellMetachars are characters that make a command line compound, dynamic or
// redirecting. Their presence alone upgrades any command to unrecognized.
const shellMetachars = ";|&`$><\n\r(){}[]*?~!"

// readOnlyPrograms may run without approval. Every entry's recognized usage
// only reads or prints: none of them offers a flag or argument form that
// writes a file or changes system state.
//
// Deliberately absent although superficially read-only: sort (-o writes),
// uniq (second operand writes), date (-s / positional argument sets the
// system clock on GNU and BSD forms alike), hostname (a positional NAME sets
// the host name) — anything whose read-only claim cannot be proven for every
// argument form. A build tool's default output goes to the working tree,
// which the scope already covers, so go build/list/vet remain "safe" only
// through the screened subcommand path below; go test and go env do not
// (R02-SEC-01).
var readOnlyPrograms = map[string]bool{
	"ls": true, "dir": true, "pwd": true, "cd": true,
	"cat": true, "head": true, "tail": true, "wc": true,
	"grep": true, "echo": true, "whoami": true,
	"uname": true, "which": true, "where": true,
	"true": true, "false": true, "test": true, "type": true,
	"stat": true, "file": true, "diff": true,
}

// subcommandReadOnlyPrograms lists subcommands that may run without approval
// once the surrounding flag structure has been verified.
//
// R02-SEC-01 boundary: a subcommand stays here only if it cannot execute
// project code and cannot persist configuration.
//
//   - go: test runs the project's own test binaries (arbitrary code), and
//     env has persistent write modes (-w/-u); both are demoted to sensitive.
//     version/list/vet/build inspect or compile without running project
//     code; vet's -vettool and the build flags that run external helpers are
//     demoted by the flag screen below.
//   - git: diff/show/log/blame can execute configured helper programs
//     (textconv filters, diff.external, GIT_EXTERNAL_DIFF) because they
//     transform blob content; only content-free readers remain.
var subcommandReadOnlyPrograms = map[string]map[string]bool{
	"go":   {"version": true, "list": true, "vet": true, "build": true},
	"git":  {"status": true, "rev-parse": true, "ls-files": true, "describe": true, "shortlog": true},
	"npm":  {"--version": true, "-v": true},
	"node": {"--version": true, "-v": true},
}

// valueFlags lists, per program, the flags that consume the following token.
// A subcommand scan must skip those values or it will read the value as the
// subcommand (git -C log push is `git push` run from directory `log`).
var valueFlags = map[string]map[string]bool{
	"git": {"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true, "--exec-path": true, "--config-env": true},
	"go":  {"-C": true},
}

// outputFlags lists flag prefixes that either redirect a program's writes to
// a caller-chosen location or make it run an external helper. Any occurrence
// demotes a would-be safe command: the default output location (the working
// tree) is inside the scope, a redirected one is not verified to be — and an
// external helper (go -toolexec/-vettool, go test -exec) is project-adjacent
// code execution this classifier cannot vouch for.
var outputFlags = []string{
	"-o", "--output", "-toolexec", "-exec", "-vettool", "-coverprofile", "-cpuprofile",
	"-memprofile", "-blockprofile", "-mutexprofile", "-trace", "-pgo",
	"-overlay", "-modfile", "-C", "--git-dir", "--work-tree", "--exec-path",
}

// sensitivePrograms are recognized high-impact programs. Running them is not
// forbidden — it requires the user's explicit approval for the exact command.
var sensitivePrograms = map[string]bool{
	// deletion and filesystem surgery
	"rm": true, "rmdir": true, "rd": true, "del": true, "erase": true,
	"dd": true, "truncate": true, "chmod": true, "chown": true, "chattr": true,
	"icacls": true, "takeown": true, "attrib": true, "ln": true,
	// writers by flag that cannot be argument-screened reliably
	"sort": true, "uniq": true, "tee": true, "sed": true, "awk": true,
	// process and system control
	"kill": true, "pkill": true, "killall": true, "taskkill": true,
	"shutdown": true, "reboot": true, "service": true, "systemctl": true,
	"sudo": true, "su": true, "runas": true, "reg": true, "schtasks": true,
	// network
	"curl": true, "wget": true, "ssh": true, "scp": true, "sftp": true,
	"rsync": true, "nc": true, "ncat": true, "netcat": true, "telnet": true,
	"http": true, "iwr": true, "invoke-webrequest": true,
	// package managers and build systems (arbitrary script execution)
	"pip": true, "pip3": true, "npm": true, "npx": true, "pnpm": true,
	"yarn": true, "cargo": true, "make": true, "gradle": true, "mvn": true,
	// containers, clouds, infrastructure
	"docker": true, "kubectl": true, "aws": true, "az": true, "gcloud": true,
	"terraform": true, "ansible": true, "helm": true,
	// interpreters and shells (arbitrary code execution / wrapping)
	"powershell": true, "pwsh": true, "cmd": true, "bash": true, "sh": true,
	"zsh": true, "fish": true, "python": true, "python3": true, "py": true,
	"node": true, "deno": true, "bun": true, "perl": true, "ruby": true,
	"php": true, "xargs": true, "find": true,
	"eval": true, "exec": true, "source": true, "env": true,
	// file moving that can escape scope by argument
	"mv": true, "cp": true,
}

// ClassifyCommand returns the class and resolved program name for a single
// command line, using cwd for the Windows cwd-shadowing check.
func ClassifyCommand(cmd, cwd string) (CommandClass, string) {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return CmdUnrecognized, ""
	}
	if strings.ContainsAny(trimmed, shellMetachars) {
		return CmdUnrecognized, ""
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return CmdUnrecognized, ""
	}
	prog := strings.ToLower(fields[0])
	prog = strings.Trim(prog, `"'`)
	if prog == "" {
		return CmdUnrecognized, ""
	}
	if strings.ContainsAny(prog, `/\`) || strings.HasPrefix(prog, "~") {
		// Path form: never vouched for by base name.
		return CmdUnrecognized, baseName(prog)
	}
	prog = strings.TrimSuffix(prog, ".exe")
	prog = strings.TrimSuffix(prog, ".cmd")
	prog = strings.TrimSuffix(prog, ".bat")
	if prog == "" {
		return CmdUnrecognized, ""
	}

	// Windows cwd-first resolution: a bare name can execute a planted
	// ./name.bat from the working directory before PATH is consulted.
	if runtime.GOOS == "windows" {
		if cwd == "" {
			return CmdUnrecognized, prog
		}
		if shadowed, err := programShadowedInCwd(prog, cwd); err != nil || shadowed {
			return CmdUnrecognized, prog
		}
	}

	args := fields[1:]

	if subs, ok := subcommandReadOnlyPrograms[prog]; ok {
		// A pure flag form listed in the table (npm --version) is safe on its
		// own; the subcommand scan below only sees positional tokens.
		if len(args) == 1 && subs[strings.ToLower(strings.Trim(args[0], `"'`))] {
			return CmdSafe, prog
		}
		sub, rest, ok := findSubcommand(fields[1:], valueFlags[prog])
		if !ok {
			// Unverifiable flag structure before the subcommand.
			return CmdSensitive, prog
		}
		if !subs[strings.ToLower(sub)] {
			return CmdSensitive, prog
		}
		if hasOutputFlag(args) {
			return CmdSensitive, prog
		}
		_ = rest
		return CmdSafe, prog
	}
	if hasOutputFlag(args) {
		// A would-be read-only program carrying an output-redirecting flag.
		return CmdSensitive, prog
	}
	if readOnlyPrograms[prog] {
		return CmdSafe, prog
	}
	if sensitivePrograms[prog] {
		return CmdSensitive, prog
	}
	return CmdUnrecognized, prog
}

// findSubcommand scans tokens after the program name for the first positional
// token, skipping flags. Known value-taking flags consume the next token;
// any other flag that is not a plain boolean demotes the command because it
// might have swallowed the subcommand. ok=false means unverifiable.
func findSubcommand(fields []string, value map[string]bool) (sub string, rest []string, ok bool) {
	for i := 0; i < len(fields); i++ {
		tok := strings.Trim(fields[i], `"'`)
		if tok == "" {
			continue
		}
		if !strings.HasPrefix(tok, "-") {
			return tok, fields[i+1:], true
		}
		name := tok
		if idx := strings.IndexByte(name, '='); idx >= 0 {
			name = name[:idx]
		}
		if value[name] {
			if idx := strings.IndexByte(tok, '='); idx >= 0 {
				continue // value attached with '='
			}
			if strings.HasPrefix(tok, "--") {
				// Long flag without '=': its value is the next token.
				i++
			} else if len(tok) > 2 {
				// Short flag with attached value, e.g. -Cdir.
				continue
			} else {
				i++ // separate value token
			}
			continue
		}
		if strings.HasPrefix(tok, "--") {
			// An unrecognized long flag may or may not take a value; without
			// knowing, the following token cannot be trusted as the
			// subcommand.
			return "", nil, false
		}
		// Unknown short flags (-p, -n, …) are assumed boolean; they cannot
		// hide a value in the curated programs both flags precede nothing.
		// Treat a lone "-" as positional.
		if tok == "-" {
			return tok, fields[i+1:], true
		}
		continue
	}
	return "", nil, false
}

// hasOutputFlag reports whether any argument matches an output-redirecting
// flag. Matching is by prefix so -o=x, -oFILE, --output=x and -exec=x are all
// caught.
func hasOutputFlag(args []string) bool {
	for _, a := range args {
		t := strings.Trim(a, `"'`)
		for _, f := range outputFlags {
			if t == f || strings.HasPrefix(t, f+"=") {
				return true
			}
			if strings.HasPrefix(f, "-") && !strings.HasPrefix(f, "--") && strings.HasPrefix(t, f) && !strings.HasPrefix(t, "--") {
				// Short flags catch attached values too (-oFILE).
				return true
			}
		}
	}
	return false
}

// AnalyzeSafeCommandPaths inspects the arguments of a command that would be
// classified CmdSafe and reports the first path-like argument that resolves
// outside the scope roots, plus whether any path-like argument could not be
// resolved at all.
//
// Program-level classification alone would let a curated read-only program
// escape containment through its arguments ("cat C:/outside/secret" reads a
// file the file.read policy would have denied). The caller resolves this the
// same way file actions are resolved: outside → deny, unverifiable → ask.
func AnalyzeSafeCommandPaths(cmd, cwd string, roots []string) (outside string, unverifiable bool) {
	fields := strings.Fields(strings.TrimSpace(cmd))
	if len(fields) < 2 {
		return "", false
	}
	for _, tok := range fields[1:] {
		t := strings.Trim(tok, `"'`)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "-") {
			// A flag: an attached value after '=' can still be a path.
			if i := strings.IndexByte(t, '='); i >= 0 {
				if isPathLike(t[i+1:]) {
					if res := resolveArg(t[i+1:], cwd); res != "" {
						if !domain.PathWithinRoots(roots, res) {
							return res, false
						}
					} else {
						return "", true
					}
				}
				continue
			}
			// A short flag with an attached path value (-f/outside/x) cannot
			// be split reliably.
			if strings.ContainsAny(t, `/\`) {
				return "", true
			}
			continue
		}
		if !isPathLike(t) {
			// A bare word (a subcommand, a pattern, a version tag): resolved
			// against the cwd it stays inside the working tree.
			if cwd == "" {
				continue
			}
			res := domain.NormalizePath(t, cwd)
			if !domain.PathWithinRoots(roots, res) {
				return res, false
			}
			continue
		}
		res := resolveArg(t, cwd)
		if res == "" {
			return "", true
		}
		if !domain.PathWithinRoots(roots, res) {
			return res, false
		}
	}
	return "", false
}

// isPathLike reports whether a token names a filesystem location.
func isPathLike(t string) bool {
	if t == "." || t == ".." || strings.HasPrefix(t, "~") {
		return true
	}
	if strings.ContainsAny(t, `/\`) {
		return true
	}
	// Drive-letter form, C:x.
	if len(t) >= 2 && t[1] == ':' {
		c := t[0]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return true
		}
	}
	return false
}

// resolveArg resolves a path-like argument against the cwd; "" when it cannot
// be resolved (no cwd for a relative argument).
func resolveArg(arg, cwd string) string {
	if domain.IsAbsoluteNormalized(domain.NormalizePath(arg, "")) {
		return domain.NormalizePath(arg, "")
	}
	if cwd == "" {
		return ""
	}
	return domain.NormalizePath(arg, cwd)
}

// ExtractProgram returns the base program name of a command line for the
// action's digest fields. It deliberately performs no trust classification.
func ExtractProgram(cmd string) string {
	fields := strings.Fields(strings.TrimSpace(cmd))
	if len(fields) == 0 {
		return ""
	}
	p := strings.ToLower(strings.Trim(fields[0], `"'"`))
	return baseName(p)
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	return p
}

// programShadowedInCwd reports whether cwd contains an executable that
// Windows' resolution order would prefer over any PATH entry for this bare
// program name. Any stat failure other than "not exist" counts as shadowing:
// when in doubt, approval.
func programShadowedInCwd(prog, cwd string) (bool, error) {
	for _, ext := range []string{"", ".exe", ".cmd", ".bat", ".com"} {
		_, err := os.Stat(filepath.Join(cwd, prog+ext))
		if err == nil {
			return true, nil
		}
		if !os.IsNotExist(err) {
			return true, err
		}
	}
	return false, nil
}

// CommandReasons produces the structured reasons for a classified command, so
// decisions about exec actions carry the same explanation vocabulary as every
// other action.
func CommandReasons(class CommandClass, program string) []domain.Reason {
	switch class {
	case CmdSafe:
		return nil
	case CmdSensitive:
		return []domain.Reason{domain.NewReason(domain.ReasonSensitiveCommand, "program", program)}
	default:
		return []domain.Reason{domain.NewReason(domain.ReasonUnrecognizedCommand, "program", program)}
	}
}
