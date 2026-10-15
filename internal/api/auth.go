package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// Authentication model.
//
// Three credential kinds reach the API, and each is deliberately narrow:
//
//   - Operator token: proves "the local user is at the keyboard". Minted at
//     server start, written 0600 into the state directory. Grants approvals,
//     mission/task management and agent registration. It cannot authorize an
//     action — decisions go through the engine.
//   - Hook token: identifies the local PreToolUse hook channel. It may only
//     submit hook decisions; it can never approve anything.
//   - Agent tokens: issued once at agent registration; stored only as
//     SHA-256. An agent token identifies exactly one agent and is accepted
//     only on endpoints scoped to that agent's own missions and approvals.
//
// None of these is an authorization to act: the engine still decides every
// action, and approvals still require the operator credential.
type authKind int

const (
	authNone authKind = iota
	authOperator
	authHook
	authAgent
	authUISession
)

// authInfo is the resolved identity of one request.
type authInfo struct {
	Kind  authKind
	Agent domain.Agent // set when Kind == authAgent
	// Session carries the UI session for CSRF validation of mutating
	// cookie-authenticated requests; nil for bearer-authenticated requests.
	Session *uiSession
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

// NewToken mints a 256-bit bearer token (hex). Used for operator, hook and
// agent credentials alike.
func NewToken() string {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic("api: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(raw[:])
}

// uiSession is a browser session for the local console. The cookie alone is
// never enough for state changes: every mutating request must also present
// the CSRF token in X-Qiyan-CSRF (double-submit against the session store),
// and Origin/Host checks run in middleware.
type uiSession struct {
	CSRF      string
	CreatedAt time.Time
}

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]uiSession // session id -> session
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: map[string]uiSession{}}
}

func (ss *sessionStore) create() (string, uiSession) {
	sid := NewToken()[:32]
	sess := uiSession{CSRF: NewToken()[:32], CreatedAt: time.Now()}
	ss.mu.Lock()
	ss.sessions[sid] = sess
	ss.mu.Unlock()
	return sid, sess
}

func (ss *sessionStore) get(sid string) (uiSession, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	s, ok := ss.sessions[sid]
	return s, ok
}

// resolveAuth classifies one request's credentials. It never trusts
// agent-supplied identity fields: a Bearer token resolves to an identity, and
// nothing else does.
func (s *Server) resolveAuth(r *http.Request) authInfo {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		tok := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		switch {
		case tok == "":
			return authInfo{}
		case s.OperatorToken != "" && tok == s.OperatorToken:
			return authInfo{Kind: authOperator}
		case s.HookToken != "" && tok == s.HookToken:
			return authInfo{Kind: authHook}
		default:
			if s.Store != nil {
				if ag, ok, err := s.Store.AgentByTokenHash(hashToken(tok)); err == nil && ok {
					return authInfo{Kind: authAgent, Agent: ag}
				}
			}
			return authInfo{}
		}
	}
	if c, err := r.Cookie(uiCookieName); err == nil {
		if sess, ok := s.sessions.get(c.Value); ok {
			return authInfo{Kind: authUISession, Session: &sess}
		}
	}
	return authInfo{}
}
