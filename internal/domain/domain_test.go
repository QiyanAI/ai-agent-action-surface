package domain

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewIDsAreUniqueAndPrefixed(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := string(NewAgentID())
		if !strings.HasPrefix(id, "ag_") {
			t.Fatalf("agent ID missing prefix: %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = true
	}
	if string(NewTaskID())[:3] != "ta_" || string(NewMissionID())[:3] != "mi_" {
		t.Fatal("unexpected ID prefixes")
	}
}

func TestCanonicalDigestBindsSemanticsNotSpelling(t *testing.T) {
	base := Action{Kind: ActionFileModify, Path: "C:/proj/a.txt"}

	// Raw evidence text must not change the digest: the same action described
	// differently by a runtime is the same action.
	withRaw := base
	withRaw.Raw = `{"tool":"Write","note":"different words entirely"}`
	if base.CanonicalDigest() != withRaw.CanonicalDigest() {
		t.Fatal("Raw must be excluded from the digest")
	}

	// Changing the target must change the digest (S06's basis).
	moved := base
	moved.Path = "C:/proj/b.txt"
	if base.CanonicalDigest() == moved.CanonicalDigest() {
		t.Fatal("changed target must change the digest")
	}

	// Changing the kind must change the digest.
	read := base
	read.Kind = ActionFileRead
	if base.CanonicalDigest() == read.CanonicalDigest() {
		t.Fatal("changed kind must change the digest")
	}

	// Field-boundary collision: naive concatenation would make these equal.
	a := Action{Kind: ActionProcessExec, Command: "ab", Path: "c"}
	b := Action{Kind: ActionProcessExec, Command: "a", Path: "bc"}
	if a.CanonicalDigest() == b.CanonicalDigest() {
		t.Fatal("length-prefixed encoding must prevent boundary collisions")
	}
}

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		in, base, want string
	}{
		{`C:\Proj\src\..\a.txt`, "", "C:/Proj/a.txt"},
		{`c:/x/y`, "", "C:/x/y"},
		{`a.txt`, "C:/work", "C:/work/a.txt"},
		{`..\up.txt`, "C:/work/sub", "C:/work/up.txt"},
		{`\\?\C:\x\y`, "", "C:/x/y"},
	}
	for _, c := range cases {
		if got := NormalizePath(c.in, c.base); got != c.want {
			t.Errorf("NormalizePath(%q, %q) = %q, want %q", c.in, c.base, got, c.want)
		}
	}
}

func TestPathWithin(t *testing.T) {
	cases := []struct {
		root, p string
		want    bool
	}{
		{"C:/proj", "C:/proj", true},
		{"C:/proj", "C:/proj/sub/a.txt", true},
		{"C:/proj", "C:/proj2/evil.txt", false}, // sibling-prefix trap
		{"C:/", "C:/anything", true},
		{"/", "/etc/passwd", true},
		{"/srv/app", "/srv/application/x", false},
	}
	for _, c := range cases {
		if got := PathWithin(c.root, c.p); got != c.want {
			t.Errorf("PathWithin(%q, %q) = %v, want %v", c.root, c.p, got, c.want)
		}
	}
	if runtime.GOOS == "windows" {
		if !PathWithin("C:/Proj", "C:/proj/Sub/F.txt") {
			// pathCompareKey folds case only on Windows; this test documents
			// that the filesystem's case-insensitivity is honored there.
			t.Error("windows path comparison must fold case")
		}
	}
	if !PathWithinRoots([]string{"C:/a", "C:/b"}, "C:/b/x") {
		t.Error("PathWithinRoots must find the second root")
	}
}

func TestTaskTransitions(t *testing.T) {
	valid := []struct{ from, to TaskState }{
		{TaskCreated, TaskReady},
		{TaskReady, TaskRunning},
		{TaskRunning, TaskWaitingApproval},
		{TaskWaitingApproval, TaskRunning},
		{TaskRunning, TaskCompleted},
		{TaskRunning, TaskUnknown},
		{TaskUnknown, TaskCompleted}, // reconciliation with verified evidence
	}
	for _, v := range valid {
		if !CanTransition(v.from, v.to) {
			t.Errorf("%s → %s should be valid", v.from, v.to)
		}
	}
	invalid := []struct{ from, to TaskState }{
		{TaskCreated, TaskRunning},   // skipping READY
		{TaskCompleted, TaskRunning}, // out of terminal
		{TaskCancelled, TaskRunning},
		{TaskUnknown, TaskRunning}, // lost events never silently resume
	}
	for _, v := range invalid {
		if CanTransition(v.from, v.to) {
			t.Errorf("%s → %s must be rejected", v.from, v.to)
		}
	}
	if !TaskTerminal(TaskCompleted) || !TaskTerminal(TaskCancelled) {
		t.Error("completed and cancelled are terminal")
	}
}

func TestPolicyValidate(t *testing.T) {
	good := Policy{Mode: ModeCompanion, Scope: Scope{
		Roots:        []string{"C:/proj"},
		AllowedKinds: []ActionKind{ActionFileRead, ActionFileModify},
		AllowedTools: []string{"Read", "Edit"},
		MaxSteps:     20,
		MaxRuntime:   time.Hour,
	}}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}

	noBudget := good
	noBudget.Scope.MaxSteps = 0
	if err := noBudget.Validate(); ClassOf(err) != ErrInvalidRequest {
		t.Error("zero step budget must be rejected (no hidden unlimited default)")
	}

	relativeRoot := good
	relativeRoot.Scope.Roots = []string{"proj"}
	if err := relativeRoot.Validate(); ClassOf(err) != ErrInvalidRequest {
		t.Error("relative scope root must be rejected")
	}

	badMode := good
	badMode.Mode = "yolo"
	if err := badMode.Validate(); ClassOf(err) != ErrInvalidRequest {
		t.Error("unknown mode must be rejected")
	}
}

func TestScopeDigestOrderInsensitiveAndSensitive(t *testing.T) {
	s1 := Scope{Roots: []string{"C:/a", "C:/b"}, AllowedKinds: []ActionKind{ActionFileRead, ActionProcessExec}, AllowedTools: []string{"Edit", "Read"}, MaxSteps: 5, MaxRuntime: time.Minute}
	s2 := Scope{Roots: []string{"C:/b", "C:/a"}, AllowedKinds: []ActionKind{ActionProcessExec, ActionFileRead}, AllowedTools: []string{"Read", "Edit"}, MaxSteps: 5, MaxRuntime: time.Minute}
	if s1.ScopeDigest() != s2.ScopeDigest() {
		t.Error("same scope content in different order must digest equally")
	}
	s3 := s2
	s3.AllowedTools = append([]string{"Bash"}, s3.AllowedTools...)
	if s1.ScopeDigest() == s3.ScopeDigest() {
		t.Error("added tool must change the scope digest")
	}
}

func TestAuthorizationCheck(t *testing.T) {
	now := time.Now()
	scope := Scope{Roots: []string{"C:/proj"}, AllowedKinds: []ActionKind{ActionFileModify}, AllowedTools: []string{"Edit"}, MaxSteps: 5, MaxRuntime: time.Hour}
	action := Action{Kind: ActionFileModify, Path: "C:/proj/a.txt"}
	auth := Authorization{
		ID:           NewApprovalID(),
		MissionID:    NewMissionID(),
		TaskID:       NewTaskID(),
		AgentID:      NewAgentID(),
		ActionDigest: action.CanonicalDigest(),
		ScopeDigest:  scope.ScopeDigest(),
		GrantedAt:    now,
		ExpiresAt:    now.Add(10 * time.Minute),
		GrantedBy:    Actor{Source: SourceLocalUser, Label: "tester"},
	}

	if err := auth.Check(now, action, auth.MissionID, auth.TaskID, auth.AgentID, scope); err != nil {
		t.Fatalf("valid authorization rejected: %v", err)
	}

	// Wrong agent.
	if err := auth.Check(now, action, auth.MissionID, auth.TaskID, NewAgentID(), scope); ClassOf(err) != ErrAuthorizationBinding {
		t.Error("wrong agent must fail binding")
	}
	// Swapped target.
	swapped := action
	swapped.Path = "C:/proj/b.txt"
	if err := auth.Check(now, swapped, auth.MissionID, auth.TaskID, auth.AgentID, scope); ClassOf(err) != ErrAuthorizationBinding {
		t.Error("swapped target must fail binding (S06)")
	}
	// Scope widened after the grant.
	widened := scope
	widened.Roots = append(widened.Roots, "C:/elsewhere")
	if err := auth.Check(now, action, auth.MissionID, auth.TaskID, auth.AgentID, widened); ClassOf(err) != ErrAuthorizationBinding {
		t.Error("scope change must invalidate the grant")
	}
	// Expired.
	if err := auth.Check(now.Add(11*time.Minute), action, auth.MissionID, auth.TaskID, auth.AgentID, scope); ClassOf(err) != ErrApprovalExpired {
		t.Error("expired grant must fail")
	}
	// Replayed.
	used := auth
	usedAt := now.Add(time.Minute)
	used.ConsumedAt = &usedAt
	if err := used.Check(now.Add(2*time.Minute), action, used.MissionID, used.TaskID, used.AgentID, scope); ClassOf(err) != ErrAuthorizationReplayed {
		t.Error("consumed grant must fail as replay")
	}
}

func TestOutcomeCombinationAndFailClosed(t *testing.T) {
	if CombineOutcomes(OutcomeAllow, OutcomeUnknown, OutcomeRequireApproval) != OutcomeRequireApproval {
		t.Error("approval outranks unknown and allow")
	}
	if CombineOutcomes(OutcomeAllow, OutcomeDeny, OutcomeRequireApproval) != OutcomeDeny {
		t.Error("deny must govern")
	}
	d := Decision{Outcome: OutcomeUnknown}
	if d.EnforcementOutcome() != OutcomeDeny {
		t.Error("UNKNOWN on an enforced path must deny, never allow")
	}
	if got := FailClosedOutcome(nil); got != OutcomeAllow {
		t.Error("no error means no blocker")
	}
	if got := FailClosedOutcome(NewError(ErrStoreCorrupted, "x")); got != OutcomeDeny {
		t.Error("store corruption must deny")
	}
	if got := FailClosedOutcome(NewError(ErrApprovalRequired, "x")); got != OutcomeRequireApproval {
		t.Error("approval-required maps to REQUIRE_APPROVAL")
	}
	if got := FailClosedOutcome(NewError(ErrInternal, "x")); got != OutcomeDeny {
		t.Error("internal errors must deny")
	}
}

func TestCapabilitiesForTool(t *testing.T) {
	caps, ok := CapabilitiesForTool("Bash")
	if !ok || len(caps) != 1 || caps[0] != CapExecuteProcess {
		t.Fatalf("Bash capabilities wrong: %v %v", caps, ok)
	}
	caps, ok = CapabilitiesForTool("mcp__server__send_email")
	if ok || caps[0] != CapUnknown {
		t.Fatal("unknown/MCP tools must not be guessed")
	}
}

func TestAgentRoleDefaults(t *testing.T) {
	if !RolePlanner.DefaultReadOnly() || !RoleReviewer.DefaultReadOnly() {
		t.Error("planner and reviewer are read-only by default")
	}
	if RoleBuilder.DefaultReadOnly() {
		t.Error("builder is not read-only by default")
	}
}

// Regression: an absolute path containing ".." passes a lexical prefix check
// against a scope root while the filesystem resolves it elsewhere. The
// normalized-form guard must reject it.
func TestIsAbsoluteNormalizedRejectsUncleanedPaths(t *testing.T) {
	bad := []string{
		"C:/proj/../outside/x",
		"C:/proj/./x",
		"C:/proj//x",
		"proj/x",
		`C:\proj\x`,
	}
	for _, p := range bad {
		if IsAbsoluteNormalized(p) {
			t.Errorf("IsAbsoluteNormalized(%q) must be false", p)
		}
	}
	if !IsAbsoluteNormalized("C:/proj/x") {
		t.Error("clean absolute path must pass")
	}
}

// Regression: unescaped concatenation let one crafted root digest the same
// as two other roots. Length prefixes must make those distinct.
func TestScopeDigestNoConcatenationCollisions(t *testing.T) {
	a := Scope{Roots: []string{"C:/p", "C:/q"}, MaxSteps: 1, MaxRuntime: time.Minute}
	b := Scope{Roots: []string{"C:/p;root:C:/q"}, MaxSteps: 1, MaxRuntime: time.Minute}
	if a.ScopeDigest() == b.ScopeDigest() {
		t.Fatal("root lists that merely concatenate alike must not digest alike")
	}
}

// Regression: an approval for "change this file" must not cover different
// content at the same path when the transport provided the content.
func TestContentDigestBindsOperationContent(t *testing.T) {
	base := Action{Kind: ActionFileModify, Tool: "Edit", Path: "C:/proj/a.go"}
	one := base
	one.ContentDigest = ContentDigestOf("old line", "new line A", "")
	two := base
	two.ContentDigest = ContentDigestOf("old line", "new line B", "")
	if one.CanonicalDigest() == two.CanonicalDigest() {
		t.Fatal("different content at the same path must digest differently")
	}
	if one.CanonicalDigest() != func() string { c := one; return c.CanonicalDigest() }() {
		t.Fatal("identical content must digest identically")
	}
	if base.CanonicalDigest() == one.CanonicalDigest() {
		t.Fatal("an empty content digest must differ from a bound one")
	}
}
