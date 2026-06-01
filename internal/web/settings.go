package web

import (
	"net/http"
)

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	s.render(w, r, "settings.html", "Settings", user)
}

func (s *Server) handleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	user := userFromCtx(r.Context())
	if v := r.FormValue("display_name"); v != "" {
		user.DisplayName = v
	}
	if v := r.FormValue("email"); v != "" {
		user.Email = v
	}
	if err := s.svc.UpdateUser(r.Context(), user); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/settings")
}

func (s *Server) handlePasskeyList(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	passkeys, err := s.svc.GetPasskeys(r.Context(), user.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load passkeys")
		return
	}
	s.render(w, r, "passkeys.html", "Passkeys", passkeys)
}

func (s *Server) handlePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id := r.PathValue("id")
	passkeys, err := s.svc.GetPasskeys(r.Context(), user.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load passkeys")
		return
	}
	if len(passkeys) <= 1 {
		writeError(w, r, http.StatusBadRequest, "cannot delete the last passkey")
		return
	}
	if err = s.svc.DeletePasskey(r.Context(), user.ID, id); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/settings/passkeys")
}
