package api

import (
	"strings"
	"testing"
)

func TestMissionRunBodyDecodesStrictly(t *testing.T) {
	// Regression (adversarial review finding 13): the run endpoint used to
	// discard decode errors, so a typo like "claudebin" was silently dropped
	// and the run fell back to PATH detection. The contract promises strict
	// decoding everywhere; the body stays optional, but when present it is
	// checked.
	f := newAPIFixture(t)
	f.setup(t, "controlled_auto")
	path := "/v1/missions/" + string(f.mission.ID) + "/run"

	// An unknown field is rejected (note: JSON keys that differ only in case
	// are matched case-insensitively by Go's decoder — that is decoding, not
	// dropping; a typo with different letters must be refused).
	resp, body := f.do(t, "POST", path, f.opTok, map[string]any{"claude_bin": "C:/pin/claude.exe"}, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("unknown field: %d %v, want 400", resp.StatusCode, body)
	}
	msg := ""
	if e, ok := body["error"].(map[string]any); ok {
		msg, _ = e["message"].(string)
	}
	if !strings.Contains(msg, "invalid JSON body") {
		t.Fatalf("unknown field must be reported as an invalid body, got %q", msg)
	}

	// A type mismatch is refused the same way.
	resp, _ = f.do(t, "POST", path, f.opTok, map[string]any{"claudeBin": 123}, nil)
	if resp.StatusCode != 400 {
		t.Fatalf("type mismatch: %d, want 400", resp.StatusCode)
	}

	// An empty body stays acceptable: the answer must be about Claude Code
	// detection (or a started run), never about JSON.
	resp, body = f.do(t, "POST", path, f.opTok, nil, nil)
	if resp.StatusCode == 400 {
		if e, ok := body["error"].(map[string]any); ok {
			if m, _ := e["message"].(string); strings.Contains(m, "invalid JSON") || strings.Contains(m, "body is required") {
				t.Fatalf("an absent body must remain optional, got %q", m)
			}
		}
	}
}
