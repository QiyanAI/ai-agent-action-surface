// Package lab implements the Agent Safety Lab: static inspection of the
// input kinds Qiyan officially supports (a Qiyan Agent Manifest, a Claude
// Code project configuration, an MCP JSON server configuration, and
// capability declarations that arrived through the Control API), with
// evidence attached to every finding.
//
// Hard rules (book §13): the lab never executes the code of a scanned
// project, never calls an external model, and never connects to an MCP
// server. Everything it reports is read from files or records, and every
// finding names the exact source it came from. Relations that are only
// declared are labelled DECLARED; only a real pre-execution control result
// may ever be labelled ENFORCED.
package lab

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/QiyanAI/ai-agent-action-surface/internal/control"
	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// InputKind names a scan input.
type InputKind string

const (
	InputManifest    InputKind = "manifest"
	InputClaudeJSON  InputKind = "claude-project"
	InputMCPJSON     InputKind = "mcp-json"
	InputDeclaration InputKind = "declaration"
)

// Finding is one evidenced statement about the scanned input.
type Finding struct {
	Subject    string               `json:"subject"`    // agent/tool/server name
	Detail     string               `json:"detail"`     // what was found, as data
	Level      domain.EvidenceLevel `json:"level"`      // DECLARED / STATICALLY_CONNECTED / ...
	Capability domain.Capability    `json:"capability"` // coarse class
	SourceRef  string               `json:"sourceRef"`  // file and location
	Boundary   string               `json:"boundary"`   // what this does NOT cover
}

// Suggestion is a reviewable improvement. Only some are auto-applicable; the
// rest are copy-paste advice. Every suggestion says what it changes, where,
// and what it does not fix.
type Suggestion struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Why         string   `json:"why"`
	Kind        string   `json:"kind"` // "add-deny-rule" | "connect-qiyan" | "review" | "copy-snippet"
	File        string   `json:"file,omitempty"`
	AddDeny     []string `json:"addDenyRules,omitempty"`
	AutoApply   bool     `json:"autoApply"`
	Effect      string   `json:"effect,omitempty"`
	Limitations string   `json:"limitations,omitempty"`
}

// ScanResult is the complete, honest output of one scan.
type ScanResult struct {
	InputKind   InputKind    `json:"inputKind"`
	InputPath   string       `json:"inputPath"`
	Summary     string       `json:"summary"`
	Findings    []Finding    `json:"findings"`
	Suggestions []Suggestion `json:"suggestions"`
	// Unsupported names inputs the user asked for that this build cannot
	// analyse, so absence of findings is never mistaken for safety.
	Unsupported []string `json:"unsupported"`
}

// Scan dispatches on the input kind; both explicit kind and autodetection are
// supported (autodetect by filename/shape, never by executing anything).
func Scan(kind InputKind, path string) (*ScanResult, error) {
	switch kind {
	case InputManifest:
		return ScanManifest(path)
	case InputClaudeJSON:
		return ScanClaudeProject(path)
	case InputMCPJSON:
		return ScanMCPConfig(path)
	case "":
		return scanAuto(path)
	default:
		return nil, domain.NewError(domain.ErrInvalidRequest, "unknown scan input kind %q", kind)
	}
}

func scanAuto(path string) (*ScanResult, error) {
	base := strings.ToLower(filepath.Base(path))
	switch {
	case strings.Contains(base, "manifest"):
		return ScanManifest(path)
	case base == ".mcp.json" || strings.Contains(base, "mcp"):
		return ScanMCPConfig(path)
	default:
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			return ScanClaudeProject(path)
		}
		return nil, domain.NewError(domain.ErrInvalidRequest,
			"cannot tell what kind of input %q is; pass kind: manifest | claude-project | mcp-json", path)
	}
}

// --- Qiyan Agent Manifest ---

type manifestDoc struct {
	ManifestVersion string `json:"manifestVersion"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Runtime         string `json:"runtime"`
	Tools           []struct {
		Name        string   `json:"name"`
		Kind        string   `json:"kind"`
		Description string   `json:"description"`
		Targets     []string `json:"targets"`
	} `json:"tools"`
	Permissions struct {
		Filesystem []string `json:"filesystem"`
		Network    []string `json:"network"`
	} `json:"permissions"`
}

// ScanManifest inspects a Qiyan Agent Manifest (a declaration file).
func ScanManifest(path string) (*ScanResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, domain.NewError(domain.ErrNotFound, "cannot read manifest: %v", err)
	}
	var doc manifestDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, domain.NewError(domain.ErrInvalidRequest, "manifest is not valid JSON: %v", err)
	}
	res := &ScanResult{InputKind: InputManifest, InputPath: path,
		Summary: fmt.Sprintf("manifest %q (version %s, runtime %s) declares %d tool(s)",
			doc.Name, doc.Version, doc.Runtime, len(doc.Tools))}

	hasCredential, hasNetwork, hasExec := false, false, false
	for i, tool := range doc.Tools {
		kind := domain.ActionKind(tool.Kind)
		caps := domain.CapabilitiesForKind(kind)
		cap := domain.CapUnknown
		if len(caps) > 0 {
			cap = caps[0]
		}
		detail := tool.Name + ": " + tool.Description
		if len(tool.Targets) > 0 {
			detail += " (targets: " + strings.Join(tool.Targets, ", ") + ")"
		}
		res.Findings = append(res.Findings, Finding{
			Subject: tool.Name, Detail: detail,
			Level: domain.LevelDeclared, Capability: cap,
			SourceRef: fmt.Sprintf("%s:tools[%d]", path, i),
			Boundary:  "declared by the manifest; nothing here verifies the declaration matches behavior",
		})
		switch cap {
		case domain.CapUseCredentials:
			hasCredential = true
		case domain.CapNetworkAccess, domain.CapExternalState:
			hasNetwork = true
		case domain.CapExecuteProcess, domain.CapDeleteFiles:
			hasExec = true
		}
	}
	if hasCredential && hasNetwork {
		res.Findings = append(res.Findings, Finding{
			Subject: doc.Name, Detail: "declares both credential access and network access",
			Level:      domain.LevelDeclared,
			Capability: domain.CapUseCredentials,
			SourceRef:  path,
			Boundary:   "a combination worth reviewing — this scan cannot know whether any credential is actually sent anywhere",
		})
	}
	if hasExec {
		res.Suggestions = append(res.Suggestions, Suggestion{
			ID: "deny-dangerous-shell", Title: "Block the most destructive shell commands at the runtime",
			Why:  "a tool that can run programs can also delete and exfiltrate",
			Kind: "add-deny-rule", AddDeny: []string{"Bash(rm:*)", "Bash(curl:*)"},
			AutoApply:   false,
			Effect:      "Claude Code itself refuses these commands before Qiyan's gate even runs",
			Limitations: "covers only the Claude Code runtime; does not restrict other programs",
		})
	}
	res.Suggestions = append(res.Suggestions, Suggestion{
		ID: "connect-qiyan", Title: "Run this agent under Qiyan's live control",
		Why:  "declarations describe intent; Qiyan's gate decides at execution time",
		Kind: "connect-qiyan", AutoApply: false,
		Effect: "sensitive actions from this project wait for your approval",
	})
	return res, nil
}

// --- Claude Code project configuration ---

// ScanClaudeProject reads a project's Claude settings and .mcp.json without
// executing anything.
func ScanClaudeProject(projectDir string) (*ScanResult, error) {
	fi, err := os.Stat(projectDir)
	if err != nil || !fi.IsDir() {
		return nil, domain.NewError(domain.ErrInvalidRequest, "%s is not a directory", projectDir)
	}
	res := &ScanResult{InputKind: InputClaudeJSON, InputPath: projectDir}
	foundAny := false

	settingsPaths := []string{
		filepath.Join(projectDir, ".claude", "settings.json"),
		filepath.Join(projectDir, ".claude", "settings.local.json"),
	}
	hasQiyanHook := false
	var denyRules []string
	for _, sp := range settingsPaths {
		raw, err := os.ReadFile(sp)
		if err != nil {
			continue
		}
		foundAny = true
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			res.Findings = append(res.Findings, Finding{
				Subject: filepath.Base(sp), Detail: "file exists but is not valid JSON",
				Level: domain.LevelUnknown, Capability: domain.CapUnknown,
				SourceRef: sp, Boundary: "nothing in this file could be analysed",
			})
			continue
		}
		if perms, ok := doc["permissions"].(map[string]any); ok {
			if deny, ok := perms["deny"].([]any); ok {
				for _, d := range deny {
					if s, ok := d.(string); ok {
						denyRules = append(denyRules, s)
					}
				}
			}
			if allow, ok := perms["allow"].([]any); ok {
				for i, a := range allow {
					if s, ok := a.(string); ok {
						res.Findings = append(res.Findings, Finding{
							Subject: s, Detail: "pre-approved permission rule",
							Level: domain.LevelStaticallyConnected, Capability: ruleCapability(s),
							SourceRef: fmt.Sprintf("%s:permissions.allow[%d]", sp, i),
							Boundary:  "a configured rule; it says nothing about what actually runs",
						})
					}
				}
			}
		}
		if hooks, ok := doc["hooks"].(map[string]any); ok {
			raw := mustJSON(hooks)
			if strings.Contains(raw, "hook pretooluse") {
				hasQiyanHook = true
				res.Findings = append(res.Findings, Finding{
					Subject: "Qiyan PreToolUse hook", Detail: "pre-execution gate installed",
					Level: domain.LevelStaticallyConnected, Capability: domain.CapExecuteProcess,
					SourceRef: sp,
					Boundary:  "presence is static; whether it truly blocks was verified only by the connect probe",
				})
			}
		}
	}

	if mcpPath := filepath.Join(projectDir, ".mcp.json"); fileExists(mcpPath) {
		mcpRes, err := ScanMCPConfig(mcpPath)
		if err == nil {
			foundAny = true
			res.Findings = append(res.Findings, mcpRes.Findings...)
		}
	}
	if !foundAny {
		return nil, domain.NewError(domain.ErrNotFound,
			"no Claude settings or .mcp.json found under %s", projectDir)
	}

	if len(denyRules) == 0 {
		res.Suggestions = append(res.Suggestions, Suggestion{
			ID: "add-baseline-denies", Title: "Add two cheap safety rules to this project",
			Why:  "the project configures no denied commands at all",
			Kind: "add-deny-rule", File: filepath.Join(projectDir, ".claude", "settings.local.json"),
			AddDeny:     []string{"Bash(rm:*)", "Bash(curl:*)"},
			AutoApply:   true,
			Effect:      "Claude Code refuses `rm` and `curl` commands in this project before any other gate",
			Limitations: "these two rules only; they do not restrict file writes or other programs",
		})
	}
	if !hasQiyanHook {
		res.Suggestions = append(res.Suggestions, Suggestion{
			ID: "connect-qiyan", Title: "Put this project under Qiyan's live gate",
			Why:  "no pre-execution control is connected to this project",
			Kind: "connect-qiyan", AutoApply: false,
			Effect: "sensitive actions wait for your approval and every decision is recorded",
		})
	}
	res.Summary = fmt.Sprintf("project %s: %d finding(s), %d suggestion(s)",
		projectDir, len(res.Findings), len(res.Suggestions))
	return res, nil
}

func ruleCapability(rule string) domain.Capability {
	switch {
	case strings.HasPrefix(rule, "Bash("):
		return domain.CapExecuteProcess
	case strings.HasPrefix(rule, "Write("), strings.HasPrefix(rule, "Edit("):
		return domain.CapWriteFiles
	case strings.HasPrefix(rule, "Read("):
		return domain.CapReadFiles
	case strings.HasPrefix(rule, "WebFetch("), strings.HasPrefix(rule, "WebSearch("):
		return domain.CapNetworkAccess
	default:
		return domain.CapUnknown
	}
}

// --- MCP JSON ---

type mcpDoc struct {
	MCPServers map[string]struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
		URL     string            `json:"url"`
		Type    string            `json:"type"`
	} `json:"mcpServers"`
}

// ScanMCPConfig inspects an MCP server configuration file. It reads names,
// commands and environment variable *names*; it never starts a server and
// never reads environment values.
func ScanMCPConfig(path string) (*ScanResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, domain.NewError(domain.ErrNotFound, "cannot read MCP config: %v", err)
	}
	var doc mcpDoc
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.MCPServers) == 0 {
		return nil, domain.NewError(domain.ErrInvalidRequest, "%s does not contain an mcpServers object", path)
	}
	res := &ScanResult{InputKind: InputMCPJSON, InputPath: path}

	names := make([]string, 0, len(doc.MCPServers))
	for name := range doc.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		srv := doc.MCPServers[name]
		if srv.Command != "" {
			prog := control.ExtractProgram(srv.Command)
			res.Findings = append(res.Findings, Finding{
				Subject: name, Detail: "launches command: " + prog + " (full: " + srv.Command + ")",
				Level: domain.LevelStaticallyConnected, Capability: domain.CapExecuteProcess,
				SourceRef: path + ":mcpServers." + name + ".command",
				Boundary:  "the server is configured to launch this program; this scan never runs it or reads its code",
			})
		}
		if srv.URL != "" {
			res.Findings = append(res.Findings, Finding{
				Subject: name, Detail: "connects to URL: " + redactURL(srv.URL),
				Level: domain.LevelStaticallyConnected, Capability: domain.CapNetworkAccess,
				SourceRef: path + ":mcpServers." + name + ".url",
				Boundary:  "configured destination only; no connection was made",
			})
		}
		envNames := make([]string, 0, len(srv.Env))
		for k := range srv.Env {
			envNames = append(envNames, k)
		}
		sort.Strings(envNames)
		for i, k := range envNames {
			res.Findings = append(res.Findings, Finding{
				Subject: name, Detail: "reads environment variable " + k,
				Level: domain.LevelStaticallyConnected, Capability: domain.CapUseCredentials,
				SourceRef: fmt.Sprintf("%s:mcpServers.%s.env[%d]", path, name, i),
				Boundary:  "the variable is passed to the server; its value is never read by this scan",
			})
		}
	}
	res.Suggestions = append(res.Suggestions, Suggestion{
		ID: "review-mcp-servers", Title: "Review each MCP server before trusting it",
		Why:  "MCP servers run as programs with your environment and can hold API keys",
		Kind: "review", AutoApply: false,
		Effect: "you decide which servers deserve your credentials",
	})
	res.Summary = fmt.Sprintf("MCP config with %d server(s): %s", len(doc.MCPServers), strings.Join(names, ", "))
	return res, nil
}

func redactURL(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		return u[:i] + "?…"
	}
	return u
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
