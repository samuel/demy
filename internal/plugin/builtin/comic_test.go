package builtin

import (
	"strings"
	"testing"
)

// farsideFeed mirrors a comiccaster.xyz farside-daily feed: the first item has
// no caption (just the boilerplate footer link), the second carries an italic
// caption paragraph.
const farsideFeed = `<?xml version='1.0' encoding='UTF-8'?>
<rss version="2.0"><channel>
<title>The Far Side - Daily Dose - GoComics</title>
<link>https://www.thefarside.com/</link>
<item>
<title>The Far Side - 2026-06-03 #5</title>
<description>&lt;div style="text-align: center;"&gt;&lt;img src="https://comiccaster.xyz/proxy?url=a" alt="The Far Side comic"/&gt;&lt;/div&gt;&lt;p style="font-size: 0.9em;"&gt;&lt;a href="https://www.thefarside.com/"&gt;Visit The Far Side&lt;/a&gt; | © Gary Larson&lt;/p&gt;</description>
</item>
<item>
<title>The Far Side - 2026-06-03 #4</title>
<description>&lt;div&gt;&lt;img src="https://comiccaster.xyz/proxy?url=b"/&gt;&lt;/div&gt;&lt;p style="font-style: italic;"&gt;Primitive man discovers tools.&lt;/p&gt;&lt;p&gt;&lt;a href="https://www.thefarside.com/"&gt;Visit The Far Side&lt;/a&gt; | © Gary Larson&lt;/p&gt;</description>
</item>
</channel></rss>`

// gocomicsFeed mirrors a GoComics-backed feed where the URL carries escaped
// ampersands that must survive XML decoding.
const gocomicsFeed = `<?xml version='1.0' encoding='UTF-8'?>
<rss version="2.0"><channel>
<title>Garfield - GoComics</title>
<item>
<title>Garfield - 2026-06-03</title>
<description>&lt;div&gt;&lt;p style="font-style: italic;"&gt;Comic strip for 2026-06-03&lt;/p&gt;&lt;img src="https://featureassets.gocomics.com/assets/x?optimizer=image&amp;width=2800" alt="Garfield"&gt;&lt;/div&gt;</description>
</item>
</channel></rss>`

func TestParseComicFeed(t *testing.T) {
	tests := []struct {
		name        string
		feed        string
		wantComic   string
		wantItem    string
		wantImage   string
		wantCaption string
	}{
		{
			name:        "farside no caption",
			feed:        farsideFeed,
			wantComic:   "The Far Side - Daily Dose - GoComics",
			wantItem:    "The Far Side - 2026-06-03 #5",
			wantImage:   "https://comiccaster.xyz/proxy?url=a",
			wantCaption: "",
		},
		{
			name:        "gocomics escaped ampersand",
			feed:        gocomicsFeed,
			wantComic:   "Garfield - GoComics",
			wantItem:    "Garfield - 2026-06-03",
			wantImage:   "https://featureassets.gocomics.com/assets/x?optimizer=image&width=2800",
			wantCaption: "Comic strip for 2026-06-03",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := parseComicFeed(strings.NewReader(tt.feed), "test")
			if err != nil {
				t.Fatalf("parseComicFeed: %v", err)
			}
			if c.comicTitle != tt.wantComic {
				t.Errorf("comicTitle = %q, want %q", c.comicTitle, tt.wantComic)
			}
			if c.itemTitle != tt.wantItem {
				t.Errorf("itemTitle = %q, want %q", c.itemTitle, tt.wantItem)
			}
			if c.imageURL != tt.wantImage {
				t.Errorf("imageURL = %q, want %q", c.imageURL, tt.wantImage)
			}
			if c.description != tt.wantCaption {
				t.Errorf("description = %q, want %q", c.description, tt.wantCaption)
			}
		})
	}
}

func TestExtractComicCaption(t *testing.T) {
	// The italic caption is kept; the footer link paragraph is dropped.
	const html = `<div><img src="x"/></div>` +
		`<p style="font-style: italic;">Primitive man discovers tools.</p>` +
		`<p><a href="https://www.thefarside.com/">Visit The Far Side</a> | © Gary Larson</p>`
	if got := extractComicCaption(html); got != "Primitive man discovers tools." {
		t.Errorf("extractComicCaption = %q, want %q", got, "Primitive man discovers tools.")
	}

	// Only a footer link paragraph: no caption.
	const footerOnly = `<div><img src="x"/></div>` +
		`<p><a href="https://www.thefarside.com/">Visit The Far Side</a> | © Gary Larson</p>`
	if got := extractComicCaption(footerOnly); got != "" {
		t.Errorf("extractComicCaption = %q, want empty", got)
	}
}

func TestParseComicFeedNoItems(t *testing.T) {
	const empty = `<rss version="2.0"><channel><title>Empty</title></channel></rss>`
	if _, err := parseComicFeed(strings.NewReader(empty), "empty"); err == nil {
		t.Fatal("expected error for feed with no items")
	}
}
