package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
)

const sessionCookieName = "bdsmail_session"

// SessionStore holds sessions in memory. Sessions are lost on restart,
// which is acceptable for a single-server mail service.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]string // token -> username
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]string),
	}
}

func (ss *SessionStore) Create(username string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)

	ss.mu.Lock()
	ss.sessions[token] = username
	ss.mu.Unlock()

	return token, nil
}

func (ss *SessionStore) Get(token string) (string, bool) {
	ss.mu.RLock()
	username, ok := ss.sessions[token]
	ss.mu.RUnlock()
	return username, ok
}

func (ss *SessionStore) Delete(token string) {
	ss.mu.Lock()
	delete(ss.sessions, token)
	ss.mu.Unlock()
}

func createSession(w http.ResponseWriter, ss *SessionStore, username string) error {
	token, err := ss.Create(username)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func getSessionUser(r *http.Request, ss *SessionStore) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	username, ok := ss.Get(cookie.Value)
	if !ok {
		return ""
	}
	return username
}

func clearSession(w http.ResponseWriter, r *http.Request, ss *SessionStore) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		ss.Delete(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}
