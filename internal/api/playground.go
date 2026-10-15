package api

import (
	"net/http"

	"github.com/QiyanAI/ai-agent-action-surface/internal/playground"
)

// Playground endpoint: synthetic scenarios, served as inert content. There is
// deliberately no write path — the playground cannot record anything as a
// real event (its package cannot even reach the state store; see the
// separation test in internal/playground).
func (s *Server) handlePlaygroundScenarios(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"note":      "synthetic content: played locally, never recorded as observed or enforced, no external AI is called",
		"scenarios": playground.All(),
	})
}
