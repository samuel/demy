// Package dynamic provides PluginRenderer implementations for dynamic plugin types
// (json_template, starlark) in the Quoin display server.
package dynamic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"maps"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/samuel/demy"
	"github.com/samuel/demy/internal/model"
	"github.com/samuel/demy/internal/plugin"
	"github.com/samuel/demy/scene/xmlscene"
)

// JSONTemplateRenderer implements plugin.PluginRenderer for plugin_type = "json_template".
//
// The plugin Config JSON may contain:
//   - endpoint_url: URL template (can reference {{.Config.key}})
//   - endpoint_headers: map[string]string of extra HTTP headers
//
// Render:
//  1. Parses endpoint_url as a Go text/template and executes it with data {"Config": configMap}.
//  2. Fetches the URL, checking the resource cache first (TTL from p.Refresh).
//  3. Parses the response as JSON.
//  4. Executes p.Template as an XML template (xmlscene) with the parsed JSON data.
//  5. Renders to PNG via demy.
type JSONTemplateRenderer struct{}

type jsonTemplatePluginConfig struct {
	EndpointURL     string            `json:"endpoint_url"`
	EndpointHeaders map[string]string `json:"endpoint_headers"`
}

// jsonTemplateData is passed to XML scene templates. Access fetched data via .Data,
// current local time via .Now, and the device IANA timezone name via .Timezone.
type jsonTemplateData struct {
	Data     any
	Now      time.Time
	Timezone string
}

// Render implements plugin.PluginRenderer.
func (r *JSONTemplateRenderer) Render(ctx context.Context, cfg demy.Config, p *model.UserPlugin, rc plugin.ResourceCache, loc *time.Location) ([]byte, error) {
	var pcfg jsonTemplatePluginConfig
	var configMap map[string]any
	if p.Config != "" {
		if err := json.Unmarshal([]byte(p.Config), &pcfg); err != nil {
			return nil, fmt.Errorf("json_template: parsing plugin config: %w", err)
		}
		if err := json.Unmarshal([]byte(p.Config), &configMap); err != nil {
			return nil, fmt.Errorf("json_template: parsing config map: %w", err)
		}
	}

	// Resolve the endpoint URL via text/template.
	endpointURL, err := resolveURLTemplate(pcfg.EndpointURL, configMap)
	if err != nil {
		return nil, fmt.Errorf("json_template: resolving endpoint_url: %w", err)
	}

	// Determine TTL for the resource cache.
	ttl := 15 * time.Minute
	if p.Refresh != "" {
		if d, err := time.ParseDuration(p.Refresh); err == nil && d > 0 {
			ttl = d
		}
	}

	// Fetch the endpoint, using the resource cache.
	body, err := fetchEndpoint(ctx, p.UserID, endpointURL, pcfg.EndpointHeaders, rc, ttl)
	if err != nil {
		return nil, fmt.Errorf("json_template: fetching endpoint: %w", err)
	}

	// Parse the response body as JSON.
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("json_template: parsing endpoint JSON: %w", err)
	}

	if loc == nil {
		loc = time.UTC
	}
	tmplData := jsonTemplateData{
		Data:     data,
		Now:      time.Now().In(loc),
		Timezone: loc.String(),
	}

	// Execute the XML template with the parsed data.
	root, styles, err := xmlscene.ExecuteScene([]byte(p.Template), tmplData, cfg)
	if err != nil {
		return nil, fmt.Errorf("json_template: executing scene template: %w", err)
	}

	// Merge styles.
	if len(styles) > 0 {
		merged := make(map[string]demy.Style, len(cfg.Styles)+len(styles))
		maps.Copy(merged, cfg.Styles)
		maps.Copy(merged, styles)
		cfg.Styles = merged
	}

	c, err := demy.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("json_template: creating canvas: %w", err)
	}

	outImg, err := c.Render(root, plugin.EffectiveColorMode(cfg))
	if err != nil {
		return nil, fmt.Errorf("json_template: rendering canvas: %w", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, outImg); err != nil {
		return nil, fmt.Errorf("json_template: encoding PNG: %w", err)
	}
	return buf.Bytes(), nil
}

// resolveURLTemplate executes a Go text/template for the endpoint URL.
func resolveURLTemplate(urlTemplate string, configMap map[string]any) (string, error) {
	if urlTemplate == "" {
		return "", fmt.Errorf("endpoint_url is required")
	}
	tmpl, err := template.New("url").Parse(urlTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing url template: %w", err)
	}
	data := map[string]any{
		"Config": configMap,
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing url template: %w", err)
	}
	return buf.String(), nil
}

// fetchEndpoint fetches the given URL, checking and updating the resource cache.
func fetchEndpoint(ctx context.Context, userID, url string, headers map[string]string, rc plugin.ResourceCache, ttl time.Duration) ([]byte, error) {
	if rc != nil {
		data, _, ok, err := rc.Get(ctx, userID, url)
		if err == nil && ok && len(data) > 0 {
			return data, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if rc != nil {
		ct := resp.Header.Get("Content-Type")
		_ = rc.Set(ctx, userID, url, body, ct, ttl)
	}

	return body, nil
}
