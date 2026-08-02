package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"tfsa-tracker/models"
)

type contextKey string

const (
	UserContextKey contextKey = "userID"
	RoleContextKey contextKey = "userRole"
	CookieName                = "tfsa_session"
)

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]sessionData
}

type sessionData struct {
	userID    int64
	role      models.UserRole
	expiresAt time.Time
}

func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]sessionData),
	}
	go sm.startGC()
	return sm
}

func (sm *SessionManager) CreateSession(w http.ResponseWriter, user *models.User) string {
	sessionID := generateToken()
	expires := time.Now().Add(24 * 7 * time.Hour)

	sm.mu.Lock()
	sm.sessions[sessionID] = sessionData{
		userID:    user.ID,
		role:      user.Role,
		expiresAt: expires,
	}
	sm.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sessionID,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return sessionID
}

func (sm *SessionManager) DestroySession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(CookieName)
	if err == nil {
		sm.mu.Lock()
		delete(sm.sessions, cookie.Value)
		sm.mu.Unlock()
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

func (sm *SessionManager) GetSessionData(r *http.Request) (int64, models.UserRole, bool) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return 0, "", false
	}

	sm.mu.RLock()
	data, exists := sm.sessions[cookie.Value]
	sm.mu.RUnlock()

	if !exists || time.Now().After(data.expiresAt) {
		return 0, "", false
	}

	return data.userID, data.role, true
}

func (sm *SessionManager) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, role, ok := sm.GetSessionData(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, userID)
		ctx = context.WithValue(ctx, RoleContextKey, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (sm *SessionManager) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return sm.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		role, _ := r.Context().Value(RoleContextKey).(models.UserRole)
		if role != models.RoleAdmin {
			http.Error(w, "Forbidden: Admin privilege required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func GetUserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(UserContextKey).(int64)
	return id, ok
}

func (sm *SessionManager) startGC() {
	ticker := time.NewTicker(1 * time.Hour)
	for range ticker.C {
		sm.mu.Lock()
		now := time.Now()
		for id, data := range sm.sessions {
			if now.After(data.expiresAt) {
				delete(sm.sessions, id)
			}
		}
		sm.mu.Unlock()
	}
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
