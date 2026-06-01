package web

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/samuel/demy/internal/model"
)

func (s *Server) handlePluginList(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	plugins, err := s.svc.ListUserPlugins(r.Context(), user.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load plugins")
		return
	}
	s.render(w, r, "plugins.html", "My Plugins", plugins)
}

func (s *Server) handlePluginNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "plugin_new.html", "Add Plugin", nil)
}

func (s *Server) handlePluginImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	user := userFromCtx(r.Context())
	externalURL := r.FormValue("url")
	plugin, err := s.svc.ImportUserPlugin(r.Context(), user.ID, externalURL)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/plugins/"+plugin.ID)
}

func (s *Server) handlePluginDetail(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id := r.PathValue("id")
	plugin, err := s.svc.GetUserPlugin(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "plugin not found")
		return
	}
	type pluginDetailData struct {
		Plugin      *model.UserPlugin
		LibraryInfo *model.LibraryPlugin
	}
	var libInfo *model.LibraryPlugin
	if plugin.LibraryPluginID != "" {
		libInfo, _ = s.svc.GetLibraryPluginByID(r.Context(), plugin.LibraryPluginID)
	}
	s.render(w, r, "plugin.html", plugin.Name, pluginDetailData{Plugin: plugin, LibraryInfo: libInfo})
}

func (s *Server) handlePluginUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	user := userFromCtx(r.Context())
	id := r.PathValue("id")
	plugin, err := s.svc.GetUserPlugin(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "plugin not found")
		return
	}
	if v := r.FormValue("name"); v != "" {
		plugin.Name = v
	}
	if v := r.FormValue("config"); v != "" {
		plugin.Config = v
	}
	if v := r.FormValue("refresh"); v != "" {
		plugin.Refresh = v
	}
	if v := r.FormValue("template"); v != "" {
		plugin.Template = v
	}
	if v := r.FormValue("script"); v != "" {
		plugin.Script = v
	}
	if err = s.svc.UpdateUserPlugin(r.Context(), plugin); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/plugins/"+id)
}

func (s *Server) handlePluginPreview(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id := r.PathValue("id")
	plugin, err := s.svc.GetUserPlugin(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "plugin not found")
		return
	}
	deviceID := r.URL.Query().Get("device_id")
	pngBytes, err := s.renderFn(r.Context(), plugin, deviceID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "render failed: "+err.Error())
		return
	}
	// If the caller wants raw PNG (e.g. device_id provided or explicit accept), serve it directly.
	// Otherwise wrap in an <img> tag for HTMX consumption.
	if deviceID != "" || r.Header.Get("Accept") == "image/png" {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
		return
	}
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<img src="%s" alt="Preview" style="max-width:100%%;border:1px solid #ccc;">`, dataURI)
}

func (s *Server) handlePluginDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id := r.PathValue("id")
	if err := s.svc.DeleteUserPlugin(r.Context(), user.ID, id); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/plugins")
}

func (s *Server) handleLibraryList(w http.ResponseWriter, r *http.Request) {
	plugins, err := s.svc.ListLibraryPlugins(r.Context(), true)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load library")
		return
	}
	s.render(w, r, "library.html", "Plugin Library", plugins)
}

func (s *Server) handleLibraryDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	plugin, err := s.svc.GetLibraryPlugin(r.Context(), slug)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "plugin not found")
		return
	}
	s.render(w, r, "library_plugin.html", plugin.Name, plugin)
}

func (s *Server) handleLibraryClone(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	slug := r.PathValue("slug")
	lib, err := s.svc.GetLibraryPlugin(r.Context(), slug)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "plugin not found")
		return
	}
	up, err := s.svc.CreateUserPlugin(r.Context(), user.ID, lib.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/plugins/"+up.ID)
}
