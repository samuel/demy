package web

import (
	"net/http"

	"github.com/samuel/demy/internal/model"
)

func (s *Server) handleAdminLibrary(w http.ResponseWriter, r *http.Request) {
	plugins, err := s.svc.ListLibraryPlugins(r.Context(), false)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load library")
		return
	}
	s.render(w, r, "admin_library.html", "Admin: Library", plugins)
}

func (s *Server) handleAdminLibraryNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "admin_library_edit.html", "Admin: New Plugin", nil)
}

func (s *Server) handleAdminLibraryCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	var isActive bool
	if r.FormValue("is_active") == "1" || r.FormValue("is_active") == "true" {
		isActive = true
	}
	p := &model.LibraryPlugin{
		Name:        r.FormValue("name"),
		Slug:        r.FormValue("slug"),
		Description: r.FormValue("description"),
		PluginType:  model.PluginType(r.FormValue("plugin_type")),
		Template:    r.FormValue("template"),
		Script:      r.FormValue("script"),
		Thumbnail:   r.FormValue("thumbnail"),
		IsActive:    isActive,
	}
	if err := s.svc.CreateLibraryPlugin(r.Context(), p); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/admin/library")
}

func (s *Server) handleAdminLibraryEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plugin, err := s.svc.GetLibraryPluginByID(r.Context(), id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "plugin not found")
		return
	}
	s.render(w, r, "admin_library_edit.html", "Admin: Edit Plugin", plugin)
}

func (s *Server) handleAdminLibraryUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	id := r.PathValue("id")
	plugin, err := s.svc.GetLibraryPluginByID(r.Context(), id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "plugin not found")
		return
	}
	if v := r.FormValue("name"); v != "" {
		plugin.Name = v
	}
	if v := r.FormValue("slug"); v != "" {
		plugin.Slug = v
	}
	if v := r.FormValue("description"); v != "" {
		plugin.Description = v
	}
	if v := r.FormValue("plugin_type"); v != "" {
		plugin.PluginType = model.PluginType(v)
	}
	plugin.Template = r.FormValue("template")
	plugin.Script = r.FormValue("script")
	plugin.Thumbnail = r.FormValue("thumbnail")
	plugin.IsActive = r.FormValue("is_active") == "1" || r.FormValue("is_active") == "true"
	if err = s.svc.UpdateLibraryPlugin(r.Context(), plugin); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/admin/library")
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.svc.ListUsers(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load users")
		return
	}
	s.render(w, r, "admin_users.html", "Admin: Users", users)
}

func (s *Server) handleAdminToggleAdmin(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	target, err := s.svc.GetUser(r.Context(), targetID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "user not found")
		return
	}
	if err = s.svc.UpdateUserAdmin(r.Context(), targetID, !target.IsAdmin); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/admin/users")
}
