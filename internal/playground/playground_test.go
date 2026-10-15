package playground

import (
	"go/build"
	"strings"
	"testing"
)

// The separation of synthetic content from real control is a data-layer
// fact. This test pins it: the playground package must not import anything
// that could record events, reach the engine, or touch state — its scenarios
// are inert content played entirely by the browser.
func TestPlaygroundImportsNothingThatCouldRecord(t *testing.T) {
	pkg, err := build.Import("github.com/QiyanAI/ai-agent-action-surface/internal/playground", ".", build.FindOnly)
	if err != nil {
		t.Skipf("module layout unavailable here: %v", err)
	}
	_ = pkg
	// Static import check via the source files themselves.
	for _, f := range mustGoFiles(t) {
		for _, imp := range f {
			if strings.Contains(imp, "ai-agent-action-surface") {
				t.Fatalf("playground must stay inert, but imports %q (synthetic events could then be recorded as real)", imp)
			}
		}
	}
}

func TestScenariosAreCompleteAndBilingual(t *testing.T) {
	all := All()
	if len(all) < 3 {
		t.Fatalf("expected demo + two training scenarios, got %d", len(all))
	}
	for _, sc := range all {
		if sc.TitleEN == "" || sc.TitleZH == "" || sc.IntroEN == "" || sc.IntroZH == "" {
			t.Errorf("scenario %s must be bilingual end to end", sc.ID)
		}
		if len(sc.Steps) == 0 {
			t.Errorf("scenario %s has no steps", sc.ID)
		}
		for i, st := range sc.Steps {
			if st.TextEN == "" || st.TextZH == "" {
				t.Errorf("scenario %s step %d missing a language", sc.ID, i)
			}
			if st.Action != nil && st.Action.Effect != "allowed" && st.Action.Effect != "blocked" && st.Action.Effect != "awaits-decision" {
				t.Errorf("scenario %s step %d has unknown effect %q", sc.ID, i, st.Action.Effect)
			}
		}
	}
	// The demo must contain the three beats UX-02 promises.
	demo := Demo()
	var sawAllowed, sawBlocked, sawDecision bool
	for _, st := range demo.Steps {
		if st.Action != nil {
			switch st.Action.Effect {
			case "allowed":
				sawAllowed = true
			case "blocked":
				sawBlocked = true
			}
		}
		if st.Decision != nil {
			sawDecision = true
		}
	}
	if !sawAllowed || !sawBlocked || !sawDecision {
		t.Fatalf("demo must show a normal action, a blocked one and a decision (allowed=%v blocked=%v decision=%v)", sawAllowed, sawBlocked, sawDecision)
	}
}
