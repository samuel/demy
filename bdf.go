package demy

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// bitmapGlyph is one rendered character: a 1-bit mask plus placement metrics.
// All values are in the font's native pixels.
type bitmapGlyph struct {
	width   int // BBX width in pixels (mask columns)
	height  int // BBX height in pixels (mask rows)
	xOffset int // BBX x bearing (left side bearing)
	yOffset int // BBX y offset from baseline to bottom of bitmap (negative for descenders)
	advance int // DWIDTH x-advance
	// bits holds the glyph mask, row-major, MSB-first, one row every
	// ceil(width/8) bytes. Row 0 is the top of the bounding box.
	bits []byte
}

// stride returns the number of bytes per mask row.
func (g bitmapGlyph) stride() int { return (g.width + 7) / 8 }

// pixelSet reports whether the mask pixel at (col, row) is set.
func (g bitmapGlyph) pixelSet(col, row int) bool {
	if col < 0 || row < 0 || col >= g.width || row >= g.height {
		return false
	}
	b := g.bits[row*g.stride()+col/8]
	return b&(0x80>>(col%8)) != 0
}

// bitmapFont is a parsed BDF font at one native pixel size.
type bitmapFont struct {
	pixelSize    int
	ascent       int // baseline to top
	descent      int // baseline to bottom (positive)
	lineHeight   int
	spaceAdvance int  // advance for runes with no glyph and no default glyph
	defaultChar  rune // -1 if absent
	glyphs       map[rune]bitmapGlyph
}

// glyph returns the glyph for r, falling back to the font's default character.
// The boolean is false when neither r nor a default glyph is available, in which
// case the caller should advance by spaceAdvance.
func (f *bitmapFont) glyph(r rune) (bitmapGlyph, bool) {
	if g, ok := f.glyphs[r]; ok {
		return g, true
	}
	if f.defaultChar >= 0 {
		if g, ok := f.glyphs[f.defaultChar]; ok {
			return g, true
		}
	}
	return bitmapGlyph{}, false
}

// maxBDFGlyphSize bounds glyph bounding boxes, offsets, advances, and font
// metrics so malformed fonts can't request huge allocations or overflow
// placement math.
const maxBDFGlyphSize = 4096

// maxBDFBitmapBytes bounds the total size of a font's glyph masks. Masks are
// padded to their bounding box, so without it a small font that declares
// large, empty glyphs would take far more memory than its size.
const maxBDFBitmapBytes = 1 << 24

// bdfMetric reports an error unless v is within ±maxBDFGlyphSize, or within
// [0, maxBDFGlyphSize] when nonNegative is set.
func bdfMetric(name string, v int, nonNegative bool) error {
	lo := -maxBDFGlyphSize
	if nonNegative {
		lo = 0
	}
	if v < lo || v > maxBDFGlyphSize {
		return fmt.Errorf("bdf: %s %d out of range [%d, %d]", name, v, lo, maxBDFGlyphSize)
	}
	return nil
}

// parseBDF parses the subset of the Adobe BDF format that demy needs: the font
// bounding box, ascent/descent and default-char properties, and each glyph's
// encoding, advance, bounding box, and bitmap rows. Unknown keywords are ignored.
func parseBDF(data []byte) (*bitmapFont, error) {
	f := &bitmapFont{defaultChar: -1, glyphs: map[rune]bitmapGlyph{}}

	var (
		haveAscent, haveDescent bool
		bbHeight, bbYOffset     int
		haveBoundingBox         bool
	)

	// Per-glyph parse state.
	var (
		inChar    bool
		inBitmap  bool
		enc       rune
		dwidth    int
		hasDWidth bool
		gw, gh    int
		gxoff     int
		gyoff     int
		rows      []byte
		bytesWide int

		bitmapBytes int // total size of stored glyph masks
	)

	resetGlyph := func() {
		inChar, inBitmap = false, false
		enc, dwidth, hasDWidth = -1, 0, false
		gw, gh, gxoff, gyoff = 0, 0, 0, 0
		rows = nil
		bytesWide = 0
	}
	resetGlyph()

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		if inBitmap {
			if strings.HasPrefix(line, "ENDCHAR") {
				if enc >= 0 {
					bitmapBytes += gh * bytesWide
					if bitmapBytes > maxBDFBitmapBytes {
						return nil, fmt.Errorf("bdf: glyph bitmaps take more than %d bytes", maxBDFBitmapBytes)
					}
					// Pad missing rows so every mask row is addressable.
					rows = append(rows, make([]byte, gh*bytesWide-len(rows))...)
					if !hasDWidth {
						dwidth = gw
					}
					f.glyphs[enc] = bitmapGlyph{
						width:   gw,
						height:  gh,
						xOffset: gxoff,
						yOffset: gyoff,
						advance: dwidth,
						bits:    rows,
					}
				}
				resetGlyph()
				continue
			}
			if len(rows) >= gh*bytesWide {
				continue // ignore rows beyond the bounding box
			}
			row, err := parseHexRow(line, bytesWide)
			if err != nil {
				return nil, fmt.Errorf("bdf: bitmap row %q: %w", line, err)
			}
			rows = append(rows, row...)
			continue
		}

		fields := strings.Fields(line)
		keyword := fields[0]
		switch keyword {
		case "FONTBOUNDINGBOX":
			if len(fields) >= 5 {
				bbHeight = atoiOr(fields[2], 0)
				bbYOffset = atoiOr(fields[4], 0)
				if err := errors.Join(
					bdfMetric("font bounding box height", bbHeight, true),
					bdfMetric("font bounding box y offset", bbYOffset, false),
				); err != nil {
					return nil, err
				}
				haveBoundingBox = true
			}
		case "FONT_ASCENT":
			if len(fields) >= 2 {
				f.ascent = atoiOr(fields[1], 0)
				if err := bdfMetric("FONT_ASCENT", f.ascent, false); err != nil {
					return nil, err
				}
				haveAscent = true
			}
		case "FONT_DESCENT":
			if len(fields) >= 2 {
				f.descent = atoiOr(fields[1], 0)
				if err := bdfMetric("FONT_DESCENT", f.descent, false); err != nil {
					return nil, err
				}
				haveDescent = true
			}
		case "DEFAULT_CHAR":
			if len(fields) >= 2 {
				f.defaultChar = toRune(atoiOr(fields[1], -1))
			}
		case "STARTCHAR":
			resetGlyph()
			inChar = true
		case "ENCODING":
			if inChar {
				enc = parseEncoding(fields)
			}
		case "DWIDTH":
			if inChar && len(fields) >= 2 {
				dwidth = atoiOr(fields[1], 0)
				if err := bdfMetric("DWIDTH", dwidth, true); err != nil {
					return nil, err
				}
				hasDWidth = true
			}
		case "BBX":
			if inChar && len(fields) >= 5 {
				gw = atoiOr(fields[1], 0)
				gh = atoiOr(fields[2], 0)
				gxoff = atoiOr(fields[3], 0)
				gyoff = atoiOr(fields[4], 0)
				if gw < 0 || gh < 0 || gw > maxBDFGlyphSize || gh > maxBDFGlyphSize {
					return nil, fmt.Errorf("bdf: glyph bounding box %dx%d out of range [0, %d]", gw, gh, maxBDFGlyphSize)
				}
				if err := errors.Join(bdfMetric("BBX x offset", gxoff, false), bdfMetric("BBX y offset", gyoff, false)); err != nil {
					return nil, err
				}
				bytesWide = (gw + 7) / 8
			}
		case "BITMAP":
			if inChar {
				inBitmap = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("bdf: read: %w", err)
	}
	if inChar {
		return nil, errors.New("bdf: missing ENDCHAR at end of file")
	}

	if len(f.glyphs) == 0 {
		return nil, errors.New("bdf: no glyphs found")
	}

	// Derive ascent/descent from the bounding box when not given as properties.
	if !haveAscent && haveBoundingBox {
		f.ascent = bbHeight + bbYOffset
	}
	if !haveDescent && haveBoundingBox {
		f.descent = -bbYOffset
	}
	if f.ascent <= 0 && haveBoundingBox {
		f.ascent = bbHeight
	}
	f.lineHeight = f.ascent + f.descent
	if f.lineHeight <= 0 {
		f.lineHeight = bbHeight
	}
	if f.lineHeight <= 0 || f.lineHeight > maxBDFGlyphSize {
		return nil, fmt.Errorf("bdf: line height %d out of range [1, %d]", f.lineHeight, maxBDFGlyphSize)
	}
	f.pixelSize = f.lineHeight

	if g, ok := f.glyphs[' ']; ok && g.advance > 0 {
		f.spaceAdvance = g.advance
	} else {
		f.spaceAdvance = f.pixelSize / 2
		if f.spaceAdvance <= 0 {
			f.spaceAdvance = 1
		}
	}

	return f, nil
}

// parseEncoding reads the codepoint from an ENCODING line. BDF uses a single
// value (e.g. "ENCODING 65"); non-standard glyphs may use "-1" optionally
// followed by an alternate codepoint.
func parseEncoding(fields []string) rune {
	if len(fields) < 2 {
		return -1
	}
	v := atoiOr(fields[1], -1)
	if v < 0 && len(fields) >= 3 {
		v = atoiOr(fields[2], -1)
	}
	return toRune(v)
}

// toRune converts v to a rune, returning -1 when it isn't a valid codepoint.
func toRune(v int) rune {
	if v < 0 || v > unicode.MaxRune {
		return -1
	}
	return rune(v)
}

// parseHexRow decodes one BITMAP line into at most bytesWide bytes.
func parseHexRow(line string, bytesWide int) ([]byte, error) {
	line = strings.TrimSpace(line)
	if len(line)%2 != 0 {
		return nil, errors.New("odd number of hex digits")
	}
	out := make([]byte, 0, bytesWide)
	for i := 0; i+1 < len(line) && len(out) < bytesWide; i += 2 {
		b, err := strconv.ParseUint(line[i:i+2], 16, 8)
		if err != nil {
			return nil, err
		}
		out = append(out, byte(b))
	}
	// Pad short rows so the stride stays consistent.
	for len(out) < bytesWide {
		out = append(out, 0)
	}
	return out, nil
}

// atoiOr parses s as an integer, returning def on failure.
func atoiOr(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
