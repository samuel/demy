package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
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

const xkcdCacheTTL = time.Hour

// XKCDRenderer implements plugin.PluginRenderer for the XKCD comic plugin.
type XKCDRenderer struct{}

// Render fetches the latest XKCD comic and renders it to PNG bytes.
// It uses the resource cache for both the comic metadata and the image.
func (r *XKCDRenderer) Render(ctx context.Context, cfg demy.Config, p *model.UserPlugin, rc plugin.ResourceCache, _ *time.Location) ([]byte, error) {
	comic, err := fetchXKCDComicCached(ctx, p.UserID, rc)
	if err != nil {
		return nil, err
	}

	img, err := fetchImageCached(ctx, p.UserID, comic.imageURL, rc)
	if err != nil {
		return nil, fmt.Errorf("fetching xkcd image: %w", err)
	}

	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		return nil, fmt.Errorf("loading bold font: %w", err)
	}
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}
	cfg.Images = map[string]image.Image{"comic": img}

	var buf bytes.Buffer
	if err := renderXKCD(&buf, comic, cfg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type xkcdComic struct {
	title    string
	altText  string
	comicNum string
	imageURL string
}

// xkcdMeta is the JSON-serializable form of xkcdComic for the resource cache.
type xkcdMeta struct {
	Title    string `json:"title"`
	AltText  string `json:"alt_text"`
	ComicNum string `json:"comic_num"`
	ImageURL string `json:"image_url"`
}

type atomFeed struct {
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Summary atomSummary `xml:"summary"`
}

type atomSummary struct {
	HTML string `xml:",chardata"`
}

var (
	reSrc   = regexp.MustCompile(`src="([^"]*)"`)
	reTitle = regexp.MustCompile(`title="([^"]*)"`)
)

func fetchXKCDComicCached(ctx context.Context, userID string, rc plugin.ResourceCache) (*xkcdComic, error) {
	const metaURL = "xkcd-meta"

	if rc != nil {
		data, _, ok, err := rc.Get(ctx, userID, metaURL)
		if err == nil && ok && len(data) > 0 {
			var m xkcdMeta
			if json.Unmarshal(data, &m) == nil {
				return &xkcdComic{
					title:    m.Title,
					altText:  m.AltText,
					comicNum: m.ComicNum,
					imageURL: m.ImageURL,
				}, nil
			}
		}
	}

	comic, err := fetchXKCDComic(ctx)
	if err != nil {
		return nil, err
	}

	if rc != nil {
		m := xkcdMeta{
			Title:    comic.title,
			AltText:  comic.altText,
			ComicNum: comic.comicNum,
			ImageURL: comic.imageURL,
		}
		if data, err := json.Marshal(m); err == nil {
			_ = rc.Set(ctx, userID, metaURL, data, "application/json", xkcdCacheTTL)
		}
	}

	return comic, nil
}

func fetchXKCDComic(ctx context.Context) (*xkcdComic, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://xkcd.com/atom.xml", nil)
	if err != nil {
		return nil, fmt.Errorf("building xkcd feed request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching xkcd feed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xkcd feed returned status %d", resp.StatusCode)
	}

	var feed atomFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("parsing xkcd feed: %w", err)
	}
	if len(feed.Entries) == 0 {
		return nil, fmt.Errorf("xkcd feed has no entries")
	}

	entry := feed.Entries[0]
	summary := entry.Summary.HTML

	srcMatch := reSrc.FindStringSubmatch(summary)
	if len(srcMatch) < 2 {
		return nil, fmt.Errorf("could not find image URL in xkcd summary")
	}
	imageURL := srcMatch[1]

	var altText string
	if m := reTitle.FindStringSubmatch(summary); len(m) >= 2 {
		altText = m[1]
	}

	comicNum := strings.TrimRight(entry.ID, "/")
	if i := strings.LastIndex(comicNum, "/"); i >= 0 {
		comicNum = comicNum[i+1:]
	}

	return &xkcdComic{
		title:    entry.Title,
		altText:  altText,
		comicNum: comicNum,
		imageURL: imageURL,
	}, nil
}

func fetchImageCached(ctx context.Context, userID, url string, rc plugin.ResourceCache) (image.Image, error) {
	// Try retina URL first.
	retinaURL := strings.Replace(url, ".png", "_2x.png", 1)

	if rc != nil {
		if data, _, ok, err := rc.Get(ctx, userID, retinaURL); err == nil && ok {
			if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
				return img, nil
			}
		}
		if data, _, ok, err := rc.Get(ctx, userID, url); err == nil && ok {
			if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
				return img, nil
			}
		}
	}

	img, rawData, err := fetchImageWithBytes(ctx, retinaURL)
	if err != nil {
		img, rawData, err = fetchImageWithBytes(ctx, url)
		if err != nil {
			return nil, err
		}
		if rc != nil {
			_ = rc.Set(ctx, userID, url, rawData, "image/png", xkcdCacheTTL)
		}
		return img, nil
	}

	if rc != nil {
		_ = rc.Set(ctx, userID, retinaURL, rawData, "image/png", xkcdCacheTTL)
	}
	return img, nil
}

func fetchImageWithBytes(ctx context.Context, url string) (image.Image, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("building image request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("image request returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading image body: %w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("decoding image: %w", err)
	}
	return img, data, nil
}

const xkcdTemplate = `<scene>
<styles>
  <style name="comicnum" font="sans" size="11" color="#555555" />
  <style name="title" font="sans" size="13" weight="bold" />
  <style name="alttext" font="sans" size="10" color="#555555" overflow="ellipsis" max-lines="3" />
</styles>
<vstack padding="8">
  <hstack>
	<text class="comicnum" color="#000000" align="left">XKCD  </text>
  	<weighted weight="1">
    	<text class="title" align="center">{{.Title}}</text>
	</weighted>
	<text class="comicnum" align="right">#{{.ComicNum}}</text>
  </hstack>
  <box padding="4"></box>
  <weighted weight="1">
    <image key="comic" scale="fit" dither="atkinson"></image>
  </weighted>
  <box padding="2"></box>
  <text class="alttext" align="center">{{.AltText}}</text>
</vstack>
</scene>`

type xkcdTemplateData struct {
	Title    string
	AltText  string
	ComicNum string
}

func renderXKCD(w io.Writer, comic *xkcdComic, cfg demy.Config) error {
	tmplData := xkcdTemplateData{
		Title:    xmlEsc(comic.title),
		AltText:  xmlEsc(comic.altText),
		ComicNum: xmlEsc(comic.comicNum),
	}

	root, styles, err := xmlscene.ExecuteScene([]byte(xkcdTemplate), tmplData, cfg)
	if err != nil {
		return fmt.Errorf("executing xkcd template: %w", err)
	}

	cfg.Styles = mergeStyles(cfg.Styles, styles)
	c, err := demy.New(cfg)
	if err != nil {
		return fmt.Errorf("creating canvas: %w", err)
	}

	outImg, err := c.Render(root, plugin.EffectiveColorMode(cfg))
	if err != nil {
		return fmt.Errorf("rendering canvas: %w", err)
	}
	return png.Encode(w, outImg)
}
