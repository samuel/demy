package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/gif"  // comiccaster serves some comics as GIF
	_ "image/jpeg" // …and some as JPEG
	"image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/samuel/demy"
	"github.com/samuel/demy/internal/model"
	"github.com/samuel/demy/internal/plugin"
	"github.com/samuel/demy/scene/xmlscene"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

const comicCacheTTL = time.Hour

// ComicRenderer implements plugin.PluginRenderer for the daily comic plugin.
// It renders the latest strip from a comiccaster.xyz RSS feed, selected by the
// configured feed slug (e.g. "farside-daily").
type ComicRenderer struct{}

// Render fetches the latest comic for the configured slug and renders it to PNG
// bytes. It uses the resource cache for both the feed metadata and the image.
func (r *ComicRenderer) Render(ctx context.Context, cfg demy.Config, p *model.UserPlugin, rc plugin.ResourceCache, _ *time.Location) ([]byte, error) {
	var pcfg comicPluginConfig
	if p.Config != "" {
		if err := json.Unmarshal([]byte(p.Config), &pcfg); err != nil {
			return nil, fmt.Errorf("comic: parsing plugin config: %w", err)
		}
	}
	slug := strings.TrimSpace(pcfg.Slug)
	if slug == "" {
		return nil, fmt.Errorf("comic plugin requires a 'slug'")
	}

	c, err := fetchComicCached(ctx, p.UserID, slug, rc)
	if err != nil {
		return nil, err
	}

	img, err := fetchImageCached(ctx, p.UserID, c.imageURL, rc)
	if err != nil {
		return nil, fmt.Errorf("fetching comic image: %w", err)
	}

	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		return nil, fmt.Errorf("loading bold font: %w", err)
	}
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}
	cfg.Images = map[string]image.Image{"comic": img}

	var buf bytes.Buffer
	if err := renderComic(&buf, c, cfg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type comicPluginConfig struct {
	Slug string `json:"slug"`
}

type comic struct {
	comicTitle  string // top-level feed (channel) title
	itemTitle   string // current day's item title
	description string // optional caption text, "" when absent
	imageURL    string
}

// comicMeta is the JSON-serializable form of comic for the resource cache.
type comicMeta struct {
	ComicTitle  string `json:"comic_title"`
	ItemTitle   string `json:"item_title"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`
}

type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
}

var (
	reComicParagraph = regexp.MustCompile(`(?is)<p\b[^>]*>(.*?)</p>`)
	reComicAnchor    = regexp.MustCompile(`(?is)<a\b`)
	reComicTag       = regexp.MustCompile(`(?s)<[^>]*>`)
	reComicWS        = regexp.MustCompile(`\s+`)
)

func fetchComicCached(ctx context.Context, userID, slug string, rc plugin.ResourceCache) (*comic, error) {
	metaKey := "comic-meta:" + slug

	if rc != nil {
		data, _, ok, err := rc.Get(ctx, userID, metaKey)
		if err == nil && ok && len(data) > 0 {
			var m comicMeta
			if json.Unmarshal(data, &m) == nil {
				return &comic{
					comicTitle:  m.ComicTitle,
					itemTitle:   m.ItemTitle,
					description: m.Description,
					imageURL:    m.ImageURL,
				}, nil
			}
		}
	}

	c, err := fetchComic(ctx, slug)
	if err != nil {
		return nil, err
	}

	if rc != nil {
		m := comicMeta{
			ComicTitle:  c.comicTitle,
			ItemTitle:   c.itemTitle,
			Description: c.description,
			ImageURL:    c.imageURL,
		}
		if data, err := json.Marshal(m); err == nil {
			_ = rc.Set(ctx, userID, metaKey, data, "application/json", comicCacheTTL)
		}
	}

	return c, nil
}

func fetchComic(ctx context.Context, slug string) (*comic, error) {
	feedURL := "https://comiccaster.xyz/rss/" + url.PathEscape(slug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building comic feed request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching comic feed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("comic feed %q returned status %d", slug, resp.StatusCode)
	}

	return parseComicFeed(resp.Body, slug)
}

// parseComicFeed decodes a comiccaster RSS feed and extracts the most recent
// item's title, image URL, and optional caption, plus the feed's channel title.
func parseComicFeed(r io.Reader, slug string) (*comic, error) {
	var feed rssFeed
	if err := xml.NewDecoder(r).Decode(&feed); err != nil {
		return nil, fmt.Errorf("parsing comic feed: %w", err)
	}
	if len(feed.Channel.Items) == 0 {
		return nil, fmt.Errorf("comic feed %q has no entries", slug)
	}

	item := feed.Channel.Items[0]

	srcMatch := reSrc.FindStringSubmatch(item.Description)
	if len(srcMatch) < 2 {
		return nil, fmt.Errorf("could not find image URL in comic feed %q", slug)
	}

	return &comic{
		comicTitle:  strings.TrimSpace(feed.Channel.Title),
		itemTitle:   strings.TrimSpace(item.Title),
		description: extractComicCaption(item.Description),
		imageURL:    srcMatch[1],
	}, nil
}

// extractComicCaption pulls human-readable caption text out of an item's
// description HTML. It collects the text of <p> elements, skipping the
// boilerplate footer paragraphs that link back to the comic ("Visit … | ©").
// Returns "" when the item has no caption.
func extractComicCaption(html string) string {
	var parts []string
	for _, m := range reComicParagraph.FindAllStringSubmatch(html, -1) {
		inner := m[1]
		if reComicAnchor.MatchString(inner) {
			continue
		}
		text := strings.TrimSpace(reComicWS.ReplaceAllString(reComicTag.ReplaceAllString(inner, ""), " "))
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

const comicTemplate = `<scene>
<styles>
  <style name="comic-title" font="sans" size="14" weight="bold" align="center" />
  <style name="item-title" font="sans" size="10" color="#555555" align="center" />
  <style name="caption" font="sans" size="10" color="#555555" align="center" overflow="ellipsis" max-lines="3" />
</styles>
<vstack padding="8">
  <text class="comic-title">{{.ComicTitle}}</text>
  {{if .ItemTitle}}<text class="item-title">{{.ItemTitle}}</text>{{end}}
  <box padding="2"></box>
  <weighted weight="1">
    <image key="comic" scale="fit" dither="atkinson"></image>
  </weighted>
  {{if .Description}}<box padding="2"></box>
  <text class="caption">{{.Description}}</text>{{end}}
</vstack>
</scene>`

type comicTemplateData struct {
	ComicTitle  string
	ItemTitle   string
	Description string
}

func renderComic(w io.Writer, c *comic, cfg demy.Config) error {
	tmplData := comicTemplateData{
		ComicTitle:  xmlEsc(c.comicTitle),
		ItemTitle:   xmlEsc(c.itemTitle),
		Description: xmlEsc(c.description),
	}

	root, styles, err := xmlscene.ExecuteScene([]byte(comicTemplate), tmplData, cfg)
	if err != nil {
		return fmt.Errorf("executing comic template: %w", err)
	}

	cfg.Styles = mergeStyles(cfg.Styles, styles)
	canvas, err := demy.New(cfg)
	if err != nil {
		return fmt.Errorf("creating canvas: %w", err)
	}

	outImg, err := canvas.Render(root, plugin.EffectiveColorMode(cfg))
	if err != nil {
		return fmt.Errorf("rendering canvas: %w", err)
	}
	return png.Encode(w, outImg)
}
