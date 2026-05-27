package main

import (
	"encoding/hex"
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"github.com/samuel/demy"
	"github.com/samuel/demy/deviceapi"
	"gopkg.in/yaml.v3"
)

// canvasPresets maps preset names to full demy.Config values.
var canvasPresets = map[string]demy.Config{
	"TRMNLOriginal": demy.TRMNLOriginal,
	"TRMNLX":        demy.TRMNLX,
}

// Config is the top-level config file structure.
type Config struct {
	Accounts []AccountConfig `yaml:"accounts"`
}

// AccountConfig groups screens and devices under a logical account.
type AccountConfig struct {
	Name          string         `yaml:"name"`
	Timezone      string         `yaml:"timezone"` // IANA timezone name; empty means time.Local
	HomeAssistant HAConfig       `yaml:"home_assistant"`
	Screens       []ScreenConfig `yaml:"screens"`
	Devices       []DeviceConfig `yaml:"devices"`
}

// HAConfig holds Home Assistant connection settings shared by all screens in an account.
type HAConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

// ScreenConfig defines one display screen: a plugin plus its config.
type ScreenConfig struct {
	Name    string         `yaml:"name"`
	Plugin  string         `yaml:"plugin"`  // "weather", "calendar", or "xkcd"
	Refresh Duration       `yaml:"refresh"` // render cache TTL; default 15m
	Config  map[string]any `yaml:"config"`  // plugin-specific settings
}

// DeviceConfig describes one physical device and its playlist.
type DeviceConfig struct {
	ID          string        `yaml:"id"`           // MAC address matching DeviceHeaders.ID
	Name        string        `yaml:"name"`         // human-readable label
	RefreshRate int           `yaml:"refresh_rate"` // TRMNL polling interval in seconds
	Canvas      *CanvasConfig `yaml:"canvas"`       // optional; falls back to device headers then TRMNLOriginal
	Playlist    []string      `yaml:"playlist"`     // ordered list of screen names
}

// CanvasConfig maps to demy.Config fields. A preset seeds all values; explicit fields override.
// Fonts and Images are populated by plugins at render time and cannot be set here.
type CanvasConfig struct {
	Preset           string                 `yaml:"preset"`             // named alias, e.g. "TRMNLOriginal"
	Width            int                    `yaml:"width"`              // pixels; 0 = use preset
	Height           int                    `yaml:"height"`             // pixels; 0 = use preset
	PhysicalWidthMM  float64                `yaml:"physical_width_mm"`  // 0 = use preset
	PhysicalHeightMM float64                `yaml:"physical_height_mm"` // 0 = use preset
	ColorMode        string                 `yaml:"color_mode"`         // "1bit", "2bit", "4bit", "grayscale", "rgba"; empty = use preset
	Palette          []string               `yaml:"palette"`            // list of "#rrggbb" color strings
	Roles            *RolesConfig           `yaml:"roles"`
	DefaultStyle     *StyleConfig           `yaml:"default_style"`
	Styles           map[string]StyleConfig `yaml:"styles"`
}

// RolesConfig defines the semantic color roles for a canvas.
type RolesConfig struct {
	Foreground string `yaml:"foreground"` // "#rrggbb" or empty
	Background string `yaml:"background"`
	Accent     string `yaml:"accent"`
	Muted      string `yaml:"muted"`
}

// StyleConfig is a YAML-serializable mirror of demy.Style.
type StyleConfig struct {
	Font       string  `yaml:"font"`
	Size       float64 `yaml:"size"`
	Weight     string  `yaml:"weight"`      // "regular", "thin", "extralight", "light", "medium", "semibold", "bold", "extrabold", "black"; OR with "italic"
	Color      string  `yaml:"color"`       // "#rrggbb" or role name ("foreground", etc.)
	Align      string  `yaml:"align"`       // "left", "center", "right"
	LineHeight float64 `yaml:"line_height"` // multiplier; 0 means 1.0
	MaxLines   int     `yaml:"max_lines"`   // 0 = unlimited
	Overflow   string  `yaml:"overflow"`    // "wrap", "clip", "ellipsis"
}

// Duration is a YAML-deserializable time.Duration (e.g. "15m", "1h").
type Duration struct{ D time.Duration }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	d.D = dur
	return nil
}

var knownPlugins = map[string]bool{
	"weather":   true,
	"calendar":  true,
	"xkcd":      true,
	"grayscale": true,
}

// loadConfig reads, expands env vars, parses, and validates the YAML config at path.
func loadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	expanded := os.Expand(string(raw), os.Getenv)
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (cfg *Config) validate() error {
	var errs []string
	accountNames := make(map[string]bool)
	allDeviceIDs := make(map[string]string) // deviceID → accountName

	for _, acc := range cfg.Accounts {
		if acc.Name == "" {
			errs = append(errs, "account with empty name")
		}
		if accountNames[acc.Name] {
			errs = append(errs, fmt.Sprintf("duplicate account name %q", acc.Name))
		}
		accountNames[acc.Name] = true

		if acc.Timezone != "" {
			if _, err := time.LoadLocation(acc.Timezone); err != nil {
				errs = append(errs, fmt.Sprintf("account %q: invalid timezone %q: %v", acc.Name, acc.Timezone, err))
			}
		}

		screenNames := make(map[string]bool)
		for _, sc := range acc.Screens {
			if sc.Name == "" {
				errs = append(errs, fmt.Sprintf("account %q: screen with empty name", acc.Name))
			}
			if screenNames[sc.Name] {
				errs = append(errs, fmt.Sprintf("account %q: duplicate screen name %q", acc.Name, sc.Name))
			}
			screenNames[sc.Name] = true
			if !knownPlugins[sc.Plugin] {
				errs = append(errs, fmt.Sprintf("account %q screen %q: unknown plugin %q", acc.Name, sc.Name, sc.Plugin))
			}
		}

		for _, dev := range acc.Devices {
			if dev.ID == "" {
				errs = append(errs, fmt.Sprintf("account %q: device with empty id", acc.Name))
			}
			if other, seen := allDeviceIDs[dev.ID]; seen {
				errs = append(errs, fmt.Sprintf("account %q: device %q already defined in account %q", acc.Name, dev.ID, other))
			}
			allDeviceIDs[dev.ID] = acc.Name

			if len(dev.Playlist) == 0 {
				errs = append(errs, fmt.Sprintf("account %q device %q: empty playlist", acc.Name, dev.ID))
			}
			for _, screenName := range dev.Playlist {
				if !screenNames[screenName] {
					errs = append(errs, fmt.Sprintf("account %q device %q: playlist references unknown screen %q", acc.Name, dev.ID, screenName))
				}
			}

			if dev.Canvas != nil && dev.Canvas.Preset != "" {
				if _, ok := canvasPresets[dev.Canvas.Preset]; !ok {
					errs = append(errs, fmt.Sprintf("account %q device %q: unknown canvas preset %q", acc.Name, dev.ID, dev.Canvas.Preset))
				}
			}
			if dev.Canvas != nil && dev.Canvas.ColorMode != "" {
				if _, err := parseColorMode(dev.Canvas.ColorMode); err != nil {
					errs = append(errs, fmt.Sprintf("account %q device %q: %v", acc.Name, dev.ID, err))
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// buildDemyConfig constructs a demy.Config from the device's canvas config and request headers.
// Priority: canvas preset → explicit CanvasConfig fields → device headers → TRMNLOriginal.
// Fonts and Images are left nil; plugins populate them.
func buildDemyConfig(cc *CanvasConfig, headers deviceapi.DeviceHeaders) (demy.Config, error) {
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

// parseColorMode converts a YAML color_mode string to a demy.ColorMode.
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

// effectiveColorMode returns cfg.ColorMode, falling back to TwoBit when unset.
func effectiveColorMode(cfg demy.Config) demy.ColorMode {
	if cfg.ColorMode != demy.Unset {
		return cfg.ColorMode
	}
	return demy.RGBA
}
