package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/samuel/demy/deviceapi"
	"github.com/samuel/demy/internal/model"
	"github.com/samuel/demy/internal/plugin"
)

type deviceListItem struct {
	*model.Device
	Battery *deviceapi.DeviceHeaders
}

func (s *Server) handleDeviceList(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	devices, err := s.svc.ListDevices(r.Context(), user.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load devices")
		return
	}
	items := make([]deviceListItem, len(devices))
	for i, d := range devices {
		items[i] = deviceListItem{Device: d, Battery: parseBatteryInfo(d.LastHeaders)}
	}
	s.render(w, r, "devices.html", "Devices", items)
}

func (s *Server) handleDeviceNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "device_new.html", "New Device", nil)
}

func (s *Server) handleDeviceCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	user := userFromCtx(r.Context())
	name := r.FormValue("name")
	mac := r.FormValue("mac")
	var refreshRate int
	if v := r.FormValue("refresh_rate"); v != "" {
		rr, err := strconv.Atoi(v)
		if err != nil || rr < 0 {
			writeError(w, r, http.StatusBadRequest, "invalid refresh rate")
			return
		}
		refreshRate = rr
	}
	if refreshRate == 0 {
		refreshRate = 300
	}
	canvasCfgJSON, err := parseCanvasConfigForm(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid canvas config: "+err.Error())
		return
	}
	device, err := s.svc.CreateDevice(r.Context(), user.ID, name, mac, refreshRate)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if canvasCfgJSON != "" {
		device.CanvasConfig = canvasCfgJSON
		if err := s.svc.UpdateDevice(r.Context(), device); err != nil {
			writeError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}
	redirect(w, r, "/devices/"+device.ID)
}

func (s *Server) handleDeviceDetail(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id := r.PathValue("id")
	device, err := s.svc.GetDevice(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "device not found")
		return
	}
	playlist, err := s.svc.ListPlaylistItems(r.Context(), id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load playlist")
		return
	}
	plugins, err := s.svc.ListUserPlugins(r.Context(), user.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to load plugins")
		return
	}
	canvasPreset, cc := parseStoredCanvasConfig(device.CanvasConfig)
	pluginNames := make(map[string]string, len(plugins))
	for _, p := range plugins {
		pluginNames[p.ID] = p.Name
	}
	type deviceDetailData struct {
		Device       *model.Device
		Playlist     []*model.PlaylistItem
		Plugins      []*model.UserPlugin
		PluginNames  map[string]string
		CanvasPreset string
		CanvasConfig *plugin.CanvasConfig
		Timezones    []string
		Battery      *deviceapi.DeviceHeaders
	}
	s.render(w, r, "device.html", device.Name, deviceDetailData{
		Device:       device,
		Playlist:     playlist,
		Plugins:      plugins,
		PluginNames:  pluginNames,
		CanvasPreset: canvasPreset,
		CanvasConfig: cc,
		Timezones:    timezones,
		Battery:      parseBatteryInfo(device.LastHeaders),
	})
}

func (s *Server) handleDeviceUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	user := userFromCtx(r.Context())
	id := r.PathValue("id")
	device, err := s.svc.GetDevice(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "device not found")
		return
	}
	if v := r.FormValue("name"); v != "" {
		device.Name = v
	}
	if v := r.FormValue("timezone"); v != "" {
		if _, err := time.LoadLocation(v); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid timezone")
			return
		}
		device.Timezone = v
	}
	if v := r.FormValue("refresh_rate"); v != "" {
		rr, err := strconv.Atoi(v)
		if err != nil || rr < 0 {
			writeError(w, r, http.StatusBadRequest, "invalid refresh rate")
			return
		}
		device.RefreshRate = rr
	}
	canvasCfgJSON, err := parseCanvasConfigForm(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid canvas config: "+err.Error())
		return
	}
	if canvasCfgJSON != "" {
		device.CanvasConfig = canvasCfgJSON
	}
	if err = s.svc.UpdateDevice(r.Context(), device); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/devices/"+id)
}

func (s *Server) handleDeviceDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id := r.PathValue("id")
	if err := s.svc.DeleteDevice(r.Context(), user.ID, id); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/devices")
}

func (s *Server) handleDeviceResetKey(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id := r.PathValue("id")
	if err := s.svc.RevokeAPIKey(r.Context(), user.ID, id); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	redirect(w, r, "/devices/"+id)
}

func (s *Server) handlePlaylistAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	user := userFromCtx(r.Context())
	deviceID := r.PathValue("id")
	userPluginID := r.FormValue("user_plugin_id")
	dwell := r.FormValue("dwell")
	if err := s.svc.AddPlaylistItem(r.Context(), user.ID, deviceID, userPluginID, dwell); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if s.Notifier != nil {
		s.Notifier.NotifyPlaylistChanged(r.Context(), deviceID)
	}
	redirect(w, r, "/devices/"+deviceID)
}

func (s *Server) handlePlaylistRemove(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	deviceID := r.PathValue("id")
	itemID := r.PathValue("item_id")
	if err := s.svc.RemovePlaylistItem(r.Context(), user.ID, deviceID, itemID); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if s.Notifier != nil {
		s.Notifier.NotifyPlaylistChanged(r.Context(), deviceID)
	}
	redirect(w, r, "/devices/"+deviceID)
}

func (s *Server) handlePlaylistReorder(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	user := userFromCtx(r.Context())
	deviceID := r.PathValue("id")
	orderedIDs := r.Form["id"]
	if err := s.svc.ReorderPlaylist(r.Context(), user.ID, deviceID, orderedIDs); err != nil {
		writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if s.Notifier != nil {
		s.Notifier.NotifyPlaylistChanged(r.Context(), deviceID)
	}
	redirect(w, r, "/devices/"+deviceID)
}

// parseCanvasConfigForm reads canvas config form fields and returns a JSON-encoded CanvasConfig string.
// Returns ("", nil) when no preset is selected.
func parseCanvasConfigForm(r *http.Request) (string, error) {
	preset := r.FormValue("canvas_preset")
	switch preset {
	case "TRMNLOriginal", "TRMNLX":
		cc := plugin.CanvasConfig{Preset: preset}
		b, err := json.Marshal(cc)
		if err != nil {
			return "", fmt.Errorf("marshal canvas config: %w", err)
		}
		return string(b), nil
	case "custom":
		var cc plugin.CanvasConfig
		if v := r.FormValue("canvas_width"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return "", errors.New("invalid canvas width")
			}
			cc.Width = n
		}
		if v := r.FormValue("canvas_height"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return "", errors.New("invalid canvas height")
			}
			cc.Height = n
		}
		if v := r.FormValue("canvas_physical_width_mm"); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || f <= 0 {
				return "", errors.New("invalid physical width")
			}
			cc.PhysicalWidthMM = f
		}
		if v := r.FormValue("canvas_physical_height_mm"); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || f <= 0 {
				return "", errors.New("invalid physical height")
			}
			cc.PhysicalHeightMM = f
		}
		if v := r.FormValue("canvas_color_mode"); v != "" {
			cc.ColorMode = v
		}
		b, err := json.Marshal(cc)
		if err != nil {
			return "", fmt.Errorf("marshal canvas config: %w", err)
		}
		return string(b), nil
	case "", "none":
		return "", nil
	default:
		return "", fmt.Errorf("unknown canvas preset %q", preset)
	}
}

// parseBatteryInfo unmarshals LastHeaders JSON and returns battery-related fields, or nil if
// no battery data is present (headers empty or no voltage/percent fields).
func parseBatteryInfo(lastHeaders string) *deviceapi.DeviceHeaders {
	if lastHeaders == "" {
		return nil
	}
	var h deviceapi.DeviceHeaders
	if err := json.Unmarshal([]byte(lastHeaders), &h); err != nil {
		return nil
	}
	if h.PercentCharged == nil && h.BatteryVoltage == nil {
		return nil
	}
	return &h
}

// parseStoredCanvasConfig parses a JSON canvas config string and returns the effective preset name
// ("TRMNLOriginal", "TRMNLX", or "custom") and the parsed struct.
// Returns ("TRMNLOriginal", nil) when the stored config is empty.
func parseStoredCanvasConfig(raw string) (string, *plugin.CanvasConfig) {
	if raw == "" {
		return "TRMNLOriginal", nil
	}
	var cc plugin.CanvasConfig
	if err := json.Unmarshal([]byte(raw), &cc); err != nil {
		return "TRMNLOriginal", nil
	}
	if cc.Preset != "" {
		return cc.Preset, &cc
	}
	return "custom", &cc
}
