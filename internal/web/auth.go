package web

import (
	"encoding/base64"
	"io"
	"net/http"
	"time"
)

const waCookieName = "wa_session"

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login.html", "Sign In", nil)
}

func (s *Server) handleLoginBegin(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	optionsJSON, sessionData, err := s.svc.LoginBegin(r.Context(), email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     waCookieName,
		Value:    base64.StdEncoding.EncodeToString(sessionData),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   300,
	})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(optionsJSON)
}

func (s *Server) handleLoginFinish(w http.ResponseWriter, r *http.Request) {
	waCookie, err := r.Cookie(waCookieName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing session cookie"})
		return
	}
	sessionData, err := base64.StdEncoding.DecodeString(waCookie.Value)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session cookie"})
		return
	}
	responseJSON, err := io.ReadAll(r.Body)
	if err != nil || len(responseJSON) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	user, err := s.svc.LoginFinish(r.Context(), sessionData, responseJSON)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: waCookieName, MaxAge: -1, Path: "/"})
	sess, err := s.svc.CreateSession(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create session"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})
	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/"})
}

func (s *Server) handleRegisterPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "register.html", "Register", nil)
}

func (s *Server) handleRegisterBegin(w http.ResponseWriter, r *http.Request) {
	displayName := r.URL.Query().Get("display_name")
	email := r.URL.Query().Get("email")
	optionsJSON, sessionData, err := s.svc.RegistrationBegin(r.Context(), displayName, email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     waCookieName,
		Value:    base64.StdEncoding.EncodeToString(sessionData),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   300,
	})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(optionsJSON)
}

func (s *Server) handleRegisterFinish(w http.ResponseWriter, r *http.Request) {
	displayName := r.URL.Query().Get("display_name")
	email := r.URL.Query().Get("email")
	waCookie, err := r.Cookie(waCookieName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing session cookie"})
		return
	}
	sessionData, err := base64.StdEncoding.DecodeString(waCookie.Value)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session cookie"})
		return
	}
	responseJSON, err := io.ReadAll(r.Body)
	if err != nil || len(responseJSON) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	user, err := s.svc.RegistrationFinish(r.Context(), displayName, email, sessionData, responseJSON)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: waCookieName, MaxAge: -1, Path: "/"})
	sess, err := s.svc.CreateSession(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create session"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})
	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		_ = s.svc.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, MaxAge: -1, Path: "/"})
	redirect(w, r, "/login")
}
