package dynamic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"maps"
	"time"

	"github.com/samuel/demy"
	"github.com/samuel/demy/internal/model"
	"github.com/samuel/demy/internal/plugin"
	starlarkplugin "github.com/samuel/demy/plugin/starlark"
)

// StarlarkRenderer implements plugin.PluginRenderer for plugin_type = "starlark".
//
// The plugin Script field contains the Starlark source. The plugin Config JSON is
// parsed into a map[string]any and passed as the data argument to the script's
// render(data) function.
type StarlarkRenderer struct{}

// Render implements plugin.PluginRenderer.
func (r *StarlarkRenderer) Render(ctx context.Context, cfg demy.Config, p *model.UserPlugin, _ plugin.ResourceCache, loc *time.Location) ([]byte, error) {
	if p.Script == "" {
		return nil, fmt.Errorf("starlark: plugin has no script")
	}

	// Parse the plugin config JSON into a map for the script's data argument.
	var configData map[string]any
	if p.Config != "" {
		if err := json.Unmarshal([]byte(p.Config), &configData); err != nil {
			return nil, fmt.Errorf("starlark: parsing plugin config: %w", err)
		}
	}

	// Load and compile the Starlark script.
	var env starlarkplugin.Environment
	sp, err := env.LoadBytes(p.Name+".star", []byte(p.Script), cfg, loc)
	if err != nil {
		return nil, fmt.Errorf("starlark: loading script: %w", err)
	}

	// Merge any styles declared by the script into the config.
	scriptStyles := sp.Styles()
	if len(scriptStyles) > 0 {
		merged := make(map[string]demy.Style, len(cfg.Styles)+len(scriptStyles))
		maps.Copy(merged, cfg.Styles)
		maps.Copy(merged, scriptStyles)
		cfg.Styles = merged
	}

	// Execute the script's render(data) function.
	root, err := sp.Render(ctx, configData)
	if err != nil {
		return nil, fmt.Errorf("starlark: render: %w", err)
	}

	// Create the canvas and render to an image.
	c, err := demy.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("starlark: creating canvas: %w", err)
	}

	outImg, err := c.Render(root, plugin.EffectiveColorMode(cfg))
	if err != nil {
		return nil, fmt.Errorf("starlark: rendering canvas: %w", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, outImg); err != nil {
		return nil, fmt.Errorf("starlark: encoding PNG: %w", err)
	}
	return buf.Bytes(), nil
}
