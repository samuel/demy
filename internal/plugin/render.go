// Package plugin defines the PluginRenderer interface, ResourceCache interface,
// RenderEngine, CanvasConfig, and BuildDemyConfig for the Quoin display server.
package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image/color"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/samuel/demy"
	"github.com/samuel/demy/deviceapi"
	"github.com/samuel/demy/internal/model"
)

// PluginRenderer renders a plugin instance to PNG bytes.
type PluginRenderer interface {
	Render(ctx context.Context, cfg demy.Config, p *model.UserPlugin, rc ResourceCache, loc *time.Location) ([]byte, error)
}

// ResourceCache caches remote HTTP resources fetched by plugins.
type ResourceCache interface {
	Get(ctx context.Context, userID, url string) (data []byte, contentType string, ok bool, err error)
	Set(ctx context.Context, userID, url string, data []byte, contentType string, ttl time.Duration) error
}

// renderCacheEntry is one cached render result.
type renderCacheEntry struct {
	data        []byte
	renderedAt  time.Time
	fingerprint string
}

// RenderEngine holds registered renderers and a render cache.
type RenderEngine struct {
	store     model.Store
	renderers map[string]PluginRenderer // keyed by plugin_type string
	builtins  map[string]PluginRenderer // keyed by slug for builtin plugins

	mu    sync.RWMutex
	cache map[string]*renderCacheEntry // keyed by fingerprint
}

// NewRenderEngine returns an initialized RenderEngine.
func NewRenderEngine(store model.Store) *RenderEngine {
	return &RenderEngine{
		store:     store,
		renderers: make(map[string]PluginRenderer),
		builtins:  make(map[string]PluginRenderer),
		cache:     make(map[string]*renderCacheEntry),
	}
}

// RegisterBuiltin registers a renderer for a specific builtin slug.
func (e *RenderEngine) RegisterBuiltin(slug string, r PluginRenderer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.builtins[slug] = r
}

// RegisterType registers a renderer for a plugin type (json_template, starlark, etc.).
func (e *RenderEngine) RegisterType(pluginType string, r PluginRenderer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.renderers[pluginType] = r
}

const defaultRefresh = 15 * time.Minute

// Render renders a user plugin to PNG bytes, using the render cache.
// The cache key is a SHA-256 hash of (userPluginID, pixelWidth x pixelHeight, colorMode, timezone).
// If the cached render is still fresh (within the plugin's refresh duration), return cached bytes.
// On render error: serve stale bytes (if any) and log a warning.
func (e *RenderEngine) Render(ctx context.Context, p *model.UserPlugin, devCfg demy.Config, rc ResourceCache, loc *time.Location) ([]byte, time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	ttl := defaultRefresh
	if p.Refresh != "" {
		if d, err := time.ParseDuration(p.Refresh); err == nil && d > 0 {
			ttl = d
		}
	}

	fp := cacheFingerprint(p.ID, devCfg.PixelWidth, devCfg.PixelHeight, devCfg.ColorMode, loc.String())

	e.mu.RLock()
	entry := e.cache[fp]
	e.mu.RUnlock()

	if entry != nil && time.Since(entry.renderedAt) < ttl {
		return entry.data, entry.renderedAt, nil
	}

	renderer, err := e.resolveRenderer(ctx, p)
	if err != nil {
		if entry != nil {
			slog.Warn("plugin renderer not found, serving stale", "plugin_id", p.ID, "err", err)
			return entry.data, entry.renderedAt, nil
		}
		return nil, time.Time{}, err
	}

	data, renderErr := renderer.Render(ctx, devCfg, p, rc, loc)
	if renderErr != nil {
		if entry != nil {
			slog.Warn("plugin render failed, serving stale", "plugin_id", p.ID, "err", renderErr)
			return entry.data, entry.renderedAt, nil
		}
		return nil, time.Time{}, renderErr
	}

	now := time.Now()
	newEntry := &renderCacheEntry{
		data:        data,
		renderedAt:  now,
		fingerprint: fp,
	}

	e.mu.Lock()
	e.cache[fp] = newEntry
	e.mu.Unlock()

	return data, now, nil
}

func (e *RenderEngine) resolveRenderer(ctx context.Context, p *model.UserPlugin) (PluginRenderer, error) {
	if p.PluginType == model.PluginTypeBuiltin {
		lp, err := e.store.GetLibraryPluginByID(ctx, p.LibraryPluginID)
		if err != nil {
			return nil, fmt.Errorf("lookup library plugin for user plugin %q: %w", p.ID, err)
		}
		e.mu.RLock()
		defer e.mu.RUnlock()
		if r, ok := e.builtins[lp.Slug]; ok {
			return r, nil
		}
		return nil, fmt.Errorf("no builtin renderer registered for slug %q", lp.Slug)
	}

	e.mu.RLock()
	defer e.mu.RUnlock()
	if r, ok := e.renderers[string(p.PluginType)]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("no renderer registered for plugin type %q", p.PluginType)
}

// cacheFingerprint produces a stable cache key string for the given parameters.
func cacheFingerprint(userPluginID string, w, h int, mode demy.ColorMode, tz string) string {
	raw := userPluginID + "|" + strconv.Itoa(w) + "x" + strconv.Itoa(h) + "|" + strconv.Itoa(int(mode)) + "|" + tz
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// RenderedFilename builds a content-addressed filename from the PNG bytes.
// Identical renders produce identical filenames, enabling device-side URL caching.
func RenderedFilename(slug string, data []byte) string {
	h := sha256.Sum256(data)
	return slug + "-" + hex.EncodeToString(h[:12]) + ".png"
}

// --- Canvas config ---

// canvasPresets maps preset names to full demy.Config values.
var canvasPresets = map[string]demy.Config{
	"TRMNLOriginal": demy.TRMNLOriginal,
	"TRMNLX":        demy.TRMNLX,
}

// CanvasConfig maps to demy.Config fields. A preset seeds all values; explicit fields override.
// Fonts and Images are populated by plugins at render time and cannot be set here.
type CanvasConfig struct {
	Preset           string                 `json:"preset"`             // named alias, e.g. "TRMNLOriginal"
	Width            int                    `json:"width"`              // pixels; 0 = use preset
	Height           int                    `json:"height"`             // pixels; 0 = use preset
	PhysicalWidthMM  float64                `json:"physical_width_mm"`  // 0 = use preset
	PhysicalHeightMM float64                `json:"physical_height_mm"` // 0 = use preset
	ColorMode        string                 `json:"color_mode"`         // "1bit", "2bit", "4bit", "grayscale", "rgba"; empty = use preset
	Palette          []string               `json:"palette"`            // list of "#rrggbb" color strings
	Roles            *RolesConfig           `json:"roles,omitempty"`
	DefaultStyle     *StyleConfig           `json:"default_style,omitempty"`
	Styles           map[string]StyleConfig `json:"styles,omitempty"`
}

// RolesConfig defines the semantic color roles for a canvas.
type RolesConfig struct {
	Foreground string `json:"foreground"` // "#rrggbb" or empty
	Background string `json:"background"`
	Accent     string `json:"accent"`
	Muted      string `json:"muted"`
}

// StyleConfig is a JSON-serializable mirror of demy.Style.
type StyleConfig struct {
	Font       string  `json:"font"`
	Size       float64 `json:"size"`
	Weight     string  `json:"weight"`      // "regular", "thin", "extralight", "light", "medium", "semibold", "bold", "extrabold", "black"; OR with "italic"
	Color      string  `json:"color"`       // "#rrggbb" or role name ("foreground", etc.)
	Align      string  `json:"align"`       // "left", "center", "right"
	LineHeight float64 `json:"line_height"` // multiplier; 0 means 1.0
	MaxLines   int     `json:"max_lines"`   // 0 = unlimited
	Overflow   string  `json:"overflow"`    // "wrap", "clip", "ellipsis"
}

// BuildDemyConfig constructs a demy.Config from the device's canvas config and request headers.
// Priority: canvas preset → explicit CanvasConfig fields → device headers → TRMNLOriginal.
// Fonts and Images are left nil; plugins populate them.
func BuildDemyConfig(cc *CanvasConfig, headers deviceapi.DeviceHeaders) (demy.Config, error) {
	var cfg demy.Config

	// Start from the preset, device headers, or TRMNLOriginal as the base.
	if cc != nil && cc.Preset != "" {
		preset, ok := canvasPresets[cc.Preset]
		if !ok {
			return demy.Config{}, fmt.Errorf("unknown canvas preset %q", cc.Preset)
		}
		cfg = preset
	} else if cc == nil && headers.Width != nil && headers.Height != nil {
		cfg = demy.TRMNLOriginal
		cfg.PixelWidth = *headers.Width
		cfg.PixelHeight = *headers.Height
		// Scale physical dimensions proportionally.
		if demy.TRMNLOriginal.PixelWidth > 0 {
			cfg.PhysicalWidthMM = float64(*headers.Width) / float64(demy.TRMNLOriginal.PixelWidth) * demy.TRMNLOriginal.PhysicalWidthMM
		}
		if demy.TRMNLOriginal.PixelHeight > 0 {
			cfg.PhysicalHeightMM = float64(*headers.Height) / float64(demy.TRMNLOriginal.PixelHeight) * demy.TRMNLOriginal.PhysicalHeightMM
		}
	} else {
		cfg = demy.TRMNLOriginal
	}

	if cc == nil {
		return cfg, nil
	}

	// Apply explicit overrides.
	if cc.Width > 0 {
		cfg.PixelWidth = cc.Width
	}
	if cc.Height > 0 {
		cfg.PixelHeight = cc.Height
	}
	if cc.PhysicalWidthMM > 0 {
		cfg.PhysicalWidthMM = cc.PhysicalWidthMM
	}
	if cc.PhysicalHeightMM > 0 {
		cfg.PhysicalHeightMM = cc.PhysicalHeightMM
	}

	if len(cc.Palette) > 0 {
		cfg.Palette = nil
		for _, s := range cc.Palette {
			c, err := parseColor(s)
			if err != nil {
				return demy.Config{}, fmt.Errorf("palette color %q: %w", s, err)
			}
			cfg.Palette = append(cfg.Palette, c)
		}
	}

	if cc.Roles != nil {
		if cc.Roles.Foreground != "" {
			c, err := parseColor(cc.Roles.Foreground)
			if err != nil {
				return demy.Config{}, fmt.Errorf("roles.foreground: %w", err)
			}
			cfg.Roles.Foreground = c
		}
		if cc.Roles.Background != "" {
			c, err := parseColor(cc.Roles.Background)
			if err != nil {
				return demy.Config{}, fmt.Errorf("roles.background: %w", err)
			}
			cfg.Roles.Background = c
		}
		if cc.Roles.Accent != "" {
			c, err := parseColor(cc.Roles.Accent)
			if err != nil {
				return demy.Config{}, fmt.Errorf("roles.accent: %w", err)
			}
			cfg.Roles.Accent = c
		}
		if cc.Roles.Muted != "" {
			c, err := parseColor(cc.Roles.Muted)
			if err != nil {
				return demy.Config{}, fmt.Errorf("roles.muted: %w", err)
			}
			cfg.Roles.Muted = c
		}
	}

	if cc.DefaultStyle != nil {
		s, err := buildStyle(cc.DefaultStyle)
		if err != nil {
			return demy.Config{}, fmt.Errorf("default_style: %w", err)
		}
		cfg.DefaultTextStyle = s
	}

	if cc.ColorMode != "" {
		mode, err := parseColorMode(cc.ColorMode)
		if err != nil {
			return demy.Config{}, fmt.Errorf("color_mode: %w", err)
		}
		cfg.ColorMode = mode
	}

	if len(cc.Styles) > 0 {
		if cfg.Styles == nil {
			cfg.Styles = make(map[string]demy.Style, len(cc.Styles))
		}
		for name, sc := range cc.Styles {
			s, err := buildStyle(&sc)
			if err != nil {
				return demy.Config{}, fmt.Errorf("style %q: %w", name, err)
			}
			cfg.Styles[name] = s
		}
	}

	return cfg, nil
}

func buildStyle(sc *StyleConfig) (demy.Style, error) {
	s := demy.Style{
		Font:       sc.Font,
		Size:       sc.Size,
		LineHeight: sc.LineHeight,
		MaxLines:   sc.MaxLines,
	}
	if sc.Color != "" {
		c, err := parseColor(sc.Color)
		if err != nil {
			return demy.Style{}, fmt.Errorf("color: %w", err)
		}
		s.Color = c
	}
	var err error
	s.Weight, err = parseFontWeight(sc.Weight)
	if err != nil {
		return demy.Style{}, err
	}
	s.Align, err = parseTextAlign(sc.Align)
	if err != nil {
		return demy.Style{}, err
	}
	s.Overflow, err = parseTextOverflow(sc.Overflow)
	if err != nil {
		return demy.Style{}, err
	}
	return s, nil
}

// parseColor parses a "#rrggbb" or "#rrggbbaa" hex color string.
func parseColor(s string) (color.Color, error) {
	s = strings.TrimPrefix(s, "#")
	switch len(s) {
	case 6:
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("invalid color %q: %w", "#"+s, err)
		}
		return color.RGBA{R: b[0], G: b[1], B: b[2], A: 0xff}, nil
	case 8:
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("invalid color %q: %w", "#"+s, err)
		}
		return color.RGBA{R: b[0], G: b[1], B: b[2], A: b[3]}, nil
	default:
		return nil, fmt.Errorf("invalid color %q: must be #rrggbb or #rrggbbaa", "#"+s)
	}
}

func parseFontWeight(s string) (demy.FontStyle, error) {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return demy.FontRegular, nil
	}
	italic := strings.Contains(lower, "italic")
	base := strings.TrimSpace(strings.ReplaceAll(lower, "italic", ""))
	var weight demy.FontStyle
	switch base {
	case "", "regular":
		weight = demy.FontRegular
	case "thin":
		weight = demy.FontThin
	case "extralight":
		weight = demy.FontExtraLight
	case "light":
		weight = demy.FontLight
	case "medium":
		weight = demy.FontMedium
	case "semibold":
		weight = demy.FontSemiBold
	case "bold":
		weight = demy.FontBold
	case "extrabold":
		weight = demy.FontExtraBold
	case "black":
		weight = demy.FontBlack
	default:
		return 0, fmt.Errorf("unknown font weight %q", s)
	}
	if italic {
		weight |= demy.FontItalic
	}
	return weight, nil
}

func parseTextAlign(s string) (demy.TextAlign, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "left":
		return demy.TextAlignLeft, nil
	case "center":
		return demy.TextAlignCenter, nil
	case "right":
		return demy.TextAlignRight, nil
	default:
		return 0, fmt.Errorf("unknown text align %q", s)
	}
}

func parseTextOverflow(s string) (demy.TextOverflow, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "wrap":
		return demy.TextOverflowWrap, nil
	case "clip":
		return demy.TextOverflowClip, nil
	case "ellipsis":
		return demy.TextOverflowEllipsis, nil
	default:
		return 0, fmt.Errorf("unknown text overflow %q", s)
	}
}

// parseColorMode converts a color_mode string to a demy.ColorMode.
// Empty string returns (0, nil) meaning "unset; inherit from preset".
func parseColorMode(s string) (demy.ColorMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return 0, nil
	case "rgba":
		return demy.RGBA, nil
	case "grayscale":
		return demy.Grayscale, nil
	case "1bit":
		return demy.OneBit, nil
	case "2bit":
		return demy.TwoBit, nil
	case "4bit":
		return demy.FourBit, nil
	default:
		return 0, fmt.Errorf("unknown color_mode %q: must be rgba, grayscale, 1bit, 2bit, or 4bit", s)
	}
}

// EffectiveColorMode returns cfg.ColorMode, falling back to RGBA when unset.
func EffectiveColorMode(cfg demy.Config) demy.ColorMode {
	if cfg.ColorMode != demy.Unset {
		return cfg.ColorMode
	}
	return demy.RGBA
}
