package control

import (
	"strings"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// Command classification.
//
// Honest scope (book §09): this is NOT a shell parser and Qiyan does not claim
// to "safely restrict arbitrary shell commands by string matching". The
// classifier recognizes a small, curated set of read-only programs and a set
// of known high-impact programs; everything else — including anything with
// shell metacharacters — is classed unrecognized and gated behind explicit
// user approval. The gate is the approval, not a claim of semantic
// understanding.

// CommandClass is the classifier's verdict on one command line.
type CommandClass string

const (
	// CmdSafe: a curated program in a curated form — the standard usage of a
	// read-only tool, or a build/test subcommand whose writes land in the
	// working tree the scope already covers. Safe-to-run, not "provably
	// effect-free".
	CmdSafe CommandClass = "safe"
	// CmdSensitive: a known high-impact program (delete, network, package
	// management, process control, shell wrappers, interpreters).
	CmdSensitive CommandClass = "sensitive"
	// CmdUnrecognized: anything else, including compound or dynamic commands.
	CmdUnrecognized CommandClass = "unrecognized"
)

// shellMetachars are characters that make a command line compound, dynamic or
// redirecting. Their presence alone upgrades any command to unrecognized.
const shellMetachars = ";|&`$><\n\r(){}[]*?~!"

// readOnlyPrograms may run without approval. Every entry's standard usage
// observes; none writes, deletes, installs or reaches the network.
var readOnlyPrograms = map[string]bool{
	"ls": true, "dir": true, "pwd": true, "cd": true,
	"cat": true, "head": true, "tail": true, "wc": true,
	"grep": true, "echo": true, "date": true, "whoami": true,
	"hostname": true, "uname": true, "which": true, "where": true,
	"true": true, "false": true, "test": true, "type": true,
	"stat": true, "file": true, "diff": true, "sort": true, "uniq": true,
}

// subcommandReadOnlyPrograms need a recognized read-only subcommand.
var subcommandReadOnlyPrograms = map[string]map[string]bool{
	"go":   {"version": true, "list": true, "vet": true, "build": true, "test": true, "env": true},
	"git":  {"status": true, "log": true, "diff": true, "show": true, "rev-parse": true, "ls-files": true, "describe": true, "blame": true, "shortlog": true},
	"npm":  {"--version": true, "-v": true},
	"node": {"--version": true, "-v": true},
}

// sensitivePrograms are recognized high-impact programs. Running them is not
// forbidden — it requires the user's explicit approval for the exact command.
var sensitivePrograms = map[string]bool{
	// deletion and filesystem surgery
	"rm": true, "rmdir": true, "rd": true, "del": true, "erase": true,
	"dd": true, "truncate": true, "chmod": true, "chown": true, "chattr": true,
	"icacls": true, "takeown": true, "attrib": true, "ln": true,
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
	"php": true, "sed": true, "awk": true, "xargs": true, "find": true,
	"eval": true, "exec": true, "source": true, "env": true,
	// file moving that can escape scope by argument
	"mv": true, "cp": true, "tee": true,
}

// ClassifyCommand returns the class and resolved program name for a single
// command line. Program is empty when the command cannot be tokenized.
func ClassifyCommand(cmd string) (CommandClass, string) {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return CmdUnrecognized, ""
	}
	if strings.ContainsAny(trimmed, shellMetachars) {
		// Compound, redirected or dynamic: cannot be vouched for.
		return CmdUnrecognized, ""
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return CmdUnrecognized, ""
	}
	prog := strings.ToLower(fields[0])
	prog = strings.Trim(prog, `"'`)
	// Strip any directory part and a Windows extension: /usr/bin/ls, .\ls,
	// C:\Windows\System32\cmd.exe all classify by base name.
	if i := strings.LastIndexAny(prog, `/\`); i >= 0 {
		prog = prog[i+1:]
	}
	prog = strings.TrimSuffix(prog, ".exe")
	prog = strings.TrimSuffix(prog, ".cmd")
	prog = strings.TrimSuffix(prog, ".bat")
	if prog == "" {
		return CmdUnrecognized, ""
	}

	if subs, ok := subcommandReadOnlyPrograms[prog]; ok {
		sub := firstNonFlag(fields[1:])
		if subs[strings.ToLower(sub)] {
			return CmdSafe, prog
		}
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

// firstNonFlag returns the first token not starting with '-' or '/', lowercased
// and quote-trimmed. Git-style global flags ("-C dir") are skipped, but a flag
// that swallows its value ("-C", "-c") before a subcommand yields "", which
// classifies as sensitive — the conservative direction.
func firstNonFlag(fields []string) string {
	for _, f := range fields {
		f = strings.Trim(f, `"'`)
		if f == "" || strings.HasPrefix(f, "-") || strings.HasPrefix(f, "/") {
			continue
		}
		return f
	}
	return ""
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
