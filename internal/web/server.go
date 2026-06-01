package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/samuel/demy/internal/model"
	"github.com/samuel/demy/internal/service"
)

//go:embed templates/* static/*
var embeddedFS embed.FS

// Server is the web UI HTTP server.
type Server struct {
	svc      *service.Service
	renderFn func(ctx context.Context, p *model.UserPlugin, deviceID string) ([]byte, error)
	baseURL  string
	tmplFS   fs.FS
	static   fs.FS
}

// New creates a new Server and validates that the embedded template FS is accessible.
func New(svc *service.Service, baseURL string, renderFn func(context.Context, *model.UserPlugin, string) ([]byte, error)) (*Server, error) {
	tmplFS, err := fs.Sub(embeddedFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("template sub-fs: %w", err)
	}
	staticFS, err := fs.Sub(embeddedFS, "static")
	if err != nil {
		return nil, fmt.Errorf("static sub-fs: %w", err)
	}
	return &Server{
		svc:      svc,
		renderFn: renderFn,
		baseURL:  baseURL,
		tmplFS:   tmplFS,
		static:   staticFS,
	}, nil
}

// Register wires all routes onto mux.
func (s *Server) Register(mux *http.ServeMux) {
	// Static files.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(s.static)))

	// Auth (unauthenticated).
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("GET /auth/login/begin", s.handleLoginBegin)
	mux.HandleFunc("POST /auth/login/finish", s.handleLoginFinish)
	mux.HandleFunc("GET /register", s.handleRegisterPage)
	mux.HandleFunc("GET /auth/register/begin", s.handleRegisterBegin)
	mux.HandleFunc("POST /auth/register/finish", s.handleRegisterFinish)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)

	// Dashboard.
	mux.HandleFunc("GET /", s.requireAuth(s.handleDashboard))

	// Devices.
	mux.HandleFunc("GET /devices", s.requireAuth(s.handleDeviceList))
	mux.HandleFunc("GET /devices/new", s.requireAuth(s.handleDeviceNew))
	mux.HandleFunc("POST /devices", s.requireAuth(s.handleDeviceCreate))
	mux.HandleFunc("GET /devices/{id}", s.requireAuth(s.handleDeviceDetail))
	mux.HandleFunc("POST /devices/{id}", s.requireAuth(s.handleDeviceUpdate))
	mux.HandleFunc("DELETE /devices/{id}", s.requireAuth(s.handleDeviceDelete))
	mux.HandleFunc("POST /devices/{id}/delete", s.requireAuth(s.handleDeviceDelete))
	mux.HandleFunc("POST /devices/{id}/reset-key", s.requireAuth(s.handleDeviceResetKey))
	mux.HandleFunc("POST /devices/{id}/playlist", s.requireAuth(s.handlePlaylistAdd))
	mux.HandleFunc("DELETE /devices/{id}/playlist/{item_id}", s.requireAuth(s.handlePlaylistRemove))
	mux.HandleFunc("POST /devices/{id}/playlist/{item_id}/remove", s.requireAuth(s.handlePlaylistRemove))
	mux.HandleFunc("GET /devices/{id}/playlist/reorder", s.requireAuth(s.handlePlaylistReorder))

	// User plugins.
	mux.HandleFunc("GET /plugins", s.requireAuth(s.handlePluginList))
	mux.HandleFunc("GET /plugins/new", s.requireAuth(s.handlePluginNew))
	mux.HandleFunc("POST /plugins/import", s.requireAuth(s.handlePluginImport))
	mux.HandleFunc("GET /plugins/{id}", s.requireAuth(s.handlePluginDetail))
	mux.HandleFunc("POST /plugins/{id}", s.requireAuth(s.handlePluginUpdate))
	mux.HandleFunc("GET /plugins/{id}/preview", s.requireAuth(s.handlePluginPreview))
	mux.HandleFunc("DELETE /plugins/{id}", s.requireAuth(s.handlePluginDelete))
	mux.HandleFunc("POST /plugins/{id}/delete", s.requireAuth(s.handlePluginDelete))

	// Library.
	mux.HandleFunc("GET /library", s.requireAuth(s.handleLibraryList))
	mux.HandleFunc("GET /library/{slug}", s.requireAuth(s.handleLibraryDetail))
	mux.HandleFunc("POST /library/{slug}/clone", s.requireAuth(s.handleLibraryClone))

	// Settings.
	mux.HandleFunc("GET /settings", s.requireAuth(s.handleSettings))
	mux.HandleFunc("POST /settings", s.requireAuth(s.handleSettingsUpdate))
	mux.HandleFunc("GET /settings/passkeys", s.requireAuth(s.handlePasskeyList))
	mux.HandleFunc("DELETE /settings/passkeys/{id}", s.requireAuth(s.handlePasskeyDelete))
	mux.HandleFunc("POST /settings/passkeys/{id}/delete", s.requireAuth(s.handlePasskeyDelete))

	// Admin.
	mux.HandleFunc("GET /admin/library", s.requireAdmin(s.handleAdminLibrary))
	mux.HandleFunc("GET /admin/library/new", s.requireAdmin(s.handleAdminLibraryNew))
	mux.HandleFunc("POST /admin/library", s.requireAdmin(s.handleAdminLibraryCreate))
	mux.HandleFunc("GET /admin/library/{id}/edit", s.requireAdmin(s.handleAdminLibraryEdit))
	mux.HandleFunc("POST /admin/library/{id}", s.requireAdmin(s.handleAdminLibraryUpdate))
	mux.HandleFunc("GET /admin/users", s.requireAdmin(s.handleAdminUsers))
	mux.HandleFunc("POST /admin/users/{id}/toggle-admin", s.requireAdmin(s.handleAdminToggleAdmin))
}

// handleDashboard renders the main dashboard page.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	user := userFromCtx(r.Context())
	devices, err := s.svc.ListDevices(r.Context(), user.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load devices")
		return
	}
	plugins, err := s.svc.ListUserPlugins(r.Context(), user.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load plugins")
		return
	}
	type dashData struct {
		Devices []*model.Device
		Plugins []*model.UserPlugin
	}
	s.render(w, r, "dashboard.html", "Dashboard", dashData{Devices: devices, Plugins: plugins})
}

// render parses base.html + the named page template together (avoiding {{define "content"}}
// namespace collisions across pages) and executes the combined template set.
func (s *Server) render(w http.ResponseWriter, r *http.Request, name, title string, data any) {
	pd := pageData{
		User:  userFromCtx(r.Context()),
		Title: title,
		Data:  data,
	}
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseFS(s.tmplFS, "base.html", name)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err = tmpl.ExecuteTemplate(w, name, pd); err != nil {
		_ = err
	}
}

// templateFuncs returns the custom template function map.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"str": func(v any) string { return fmt.Sprintf("%s", v) },
	}
}
