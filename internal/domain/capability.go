package domain

import "strings"

// Capability is a coarse, user-meaningful class of what an agent can do.
// Capabilities are the vocabulary of the Safety Lab and the Action Graph:
// "Agent → Tool → Action Capability" (book §13.2). They are derived from
// concrete evidence, never assigned by the agent about itself.
type Capability string

const (
	CapReadFiles      Capability = "read_files"
	CapWriteFiles     Capability = "write_files"
	CapDeleteFiles    Capability = "delete_files"
	CapExecuteProcess Capability = "execute_process"
	CapNetworkAccess  Capability = "network_access"
	CapUseCredentials Capability = "use_credentials"
	CapExternalState  Capability = "external_state_change"
	CapDelegate       Capability = "delegate"
	CapInternalState  Capability = "internal_state"
	CapUnknown        Capability = "unknown"
)

// CapabilitiesForKind maps an action kind to the capabilities it exercises.
func CapabilitiesForKind(k ActionKind) []Capability {
	switch k {
	case ActionFileRead:
		return []Capability{CapReadFiles}
	case ActionFileCreate, ActionFileModify:
		return []Capability{CapWriteFiles}
	case ActionFileDelete:
		return []Capability{CapDeleteFiles}
	case ActionProcessExec:
		return []Capability{CapExecuteProcess}
	case ActionNetwork:
		return []Capability{CapNetworkAccess}
	case ActionCredentialUse:
		return []Capability{CapUseCredentials}
	case ActionExternalState:
		return []Capability{CapExternalState}
	case ActionDelegation:
		return []Capability{CapDelegate}
	case ActionSessionLocal:
		return []Capability{CapInternalState}
	default:
		return []Capability{CapUnknown}
	}
}

// toolCapabilities maps the supported runtime's tool names to the capabilities
// they can exercise. This table is intentionally conservative and explicit:
// a tool not listed here has unknown capability and must be treated as
// unrecognized (deny or approval), never guessed (book §09).
var toolCapabilities = map[string][]Capability{
	"Read":         {CapReadFiles},
	"Glob":         {CapReadFiles},
	"Grep":         {CapReadFiles},
	"Write":        {CapWriteFiles},
	"Edit":         {CapWriteFiles},
	"NotebookEdit": {CapWriteFiles},
	"Bash":         {CapExecuteProcess},
	"BashOutput":   {CapReadFiles},
	"KillShell":    {CapExecuteProcess},
	"WebFetch":     {CapNetworkAccess},
	"WebSearch":    {CapNetworkAccess},
	"Task":         {CapDelegate},
	"Agent":        {CapDelegate},
	// Session bookkeeping: no external effect.
	"TodoWrite":  {CapInternalState},
	"TodoRead":   {CapInternalState},
	"Skill":      {CapInternalState},
	"TaskCreate": {CapInternalState},
	"TaskGet":    {CapInternalState},
	"TaskList":   {CapInternalState},
	"TaskUpdate": {CapInternalState},
	"TaskOutput": {CapInternalState},
	"TaskStop":   {CapInternalState},
}

// CapabilitiesForTool returns the capabilities of a known tool and whether the
// tool is recognized at all. Unknown tools yield CapUnknown with ok=false.
//
// Lookup is case-insensitive: MCP tools arrive as "mcp__server__tool" and are
// deliberately NOT matched here — an MCP tool's capabilities come from its
// server's declaration through the Control API or the Safety Lab, not from a
// name pattern (the docs warn against trusting the mcp__ prefix shape).
func CapabilitiesForTool(tool string) ([]Capability, bool) {
	if caps, ok := toolCapabilities[tool]; ok {
		return append([]Capability(nil), caps...), true
	}
	// Case-insensitive fallback for exact names only.
	for name, caps := range toolCapabilities {
		if strings.EqualFold(name, tool) {
			return append([]Capability(nil), caps...), true
		}
	}
	return []Capability{CapUnknown}, false
}

// KnownToolNames lists the recognized built-in tool names (sorted for stable
// display); used by scope editors and the Safety Lab's coverage notes.
func KnownToolNames() []string {
	names := make([]string, 0, len(toolCapabilities))
	for name := range toolCapabilities {
		names = append(names, name)
	}
	return SortedStringsCopy(names)
}
