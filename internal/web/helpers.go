package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/samuel/demy/internal/model"
)

// pageData is the standard data envelope passed to every template.
type pageData struct {
	User   *model.User
	Title  string
	Data   any
	Flash  string
	Errors []string
}

func redirect(w http.ResponseWriter, r *http.Request, url string) {
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}
	http.Error(w, msg, status)
}
