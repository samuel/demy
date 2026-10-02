package demy

import (
	"math"
	"slices"
	"strings"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

// textLayout is vector text shaped by a measurement, which draw reuses so
// text is shaped once and drawn with the lines it was measured with. Sizes
// are in logical pixels.
type textLayout struct {
	lines  []textLine
	face   faceStyle
	width  float64 // widest line
	height float64
}

// textLine is one laid-out line: its glyphs in visual order, positioned
// relative to the line's left edge and baseline.
type textLine struct {
	glyphs   []placedGlyph
	width    float64
	baseline float64 // from the top of the text
}

type placedGlyph struct {
	id   font.GID
	x, y float64 // Y down
}

// faceStyle is the face chosen for a requested style, with the faux bold and
// italic that make up the difference.
type faceStyle struct {
	face       *font.Face
	style      FontStyle // requested style, normalized
	fauxBold   float64   // outline offset per side, as a fraction of the em
	fauxItalic float64   // horizontal shear
	size       float64   // em size in logical pixels
}

// face picks the closest face to want, preferring matching weight, then
// matching slant. Ties go to the lower style value so the choice is
// deterministic.
func (ff *FontFamily) face(want FontStyle) faceStyle {
	want = want.normalized()
	if f, ok := ff.faces[want]; ok {
		return faceStyle{face: f, style: want}
	}
	styles := slices.Sorted(func(yield func(FontStyle) bool) {
		for s := range ff.faces {
			if !yield(s) {
				return
			}
		}
	})
	best, bestDiff := styles[0], math.Inf(1)
	for _, s := range styles {
		diff := math.Abs(want.fauxWeight() - s.fauxWeight())
		if want.italic() != s.italic() {
			diff += 0.02
		}
		if diff < bestDiff {
			best, bestDiff = s, diff
		}
	}
	fs := faceStyle{face: ff.faces[best], style: want}
	// Only thicken; a lighter weight is drawn with the closest face.
	if d := want.fauxWeight() - best.fauxWeight(); d >= 0.0005 {
		fs.fauxBold = d
	}
	switch {
	case want.italic() && !best.italic():
		fs.fauxItalic = 0.3
	case !want.italic() && best.italic():
		fs.fauxItalic = -0.3
	}
	return fs
}

// singleFace resolves every rune to one face.
type singleFace struct{ face *font.Face }

func (s singleFace) ResolveFace(rune) *font.Face { return s.face }

// lineMetrics returns the face's ascent, descent, and line gap at size.
func lineMetrics(face *font.Face, size float64) (ascent, descent, gap float64) {
	ext, _ := face.FontHExtents()
	scale := size / float64(face.Upem())
	return float64(ext.Ascender) * scale, -float64(ext.Descender) * scale, float64(ext.LineGap) * scale
}

func toFixed(v float64) fixed.Int26_6   { return fixed.Int26_6(math.Round(v * 64)) }
func fromFixed(v fixed.Int26_6) float64 { return float64(v) / 64 }

// maxWrapWidth stands in for an unbounded width.
const maxWrapWidth = fixed.Int26_6(math.MaxInt32 >> 1)

// layoutText shapes and wraps s in ff to width (unbounded for no wrapping),
// keeping at most maxLines lines (0 for no limit). With ellipsis, a truncated
// last line ends in "…". Lines are separated by "\n", "\r\n", or "\r", and
// each starts a new line, even when empty.
func (ff *FontFamily) layoutText(s string, st Style, size, width float64) textLayout {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	fs := ff.face(st.Weight)
	fs.size = size
	lay := textLayout{face: fs}
	if s == "" {
		return lay
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	paras := strings.Split(s, "\n")

	// The shaper rounds sizes up to whole pixels, so shape at one pixel per
	// font unit and scale the result.
	upem := float64(fs.face.Upem())
	unit := size / upem // logical pixels per font unit
	maxW := maxWrapWidth
	if !isUnbounded(width) {
		maxW = toFixed(min(width/unit, fromFixed(maxWrapWidth)))
	}
	sizeFixed := fixed.I(int(fs.face.Upem()))
	shape := func(runes []rune) []shaping.Output {
		in := shaping.Input{
			Text:      runes,
			RunEnd:    len(runes),
			Direction: di.DirectionLTR,
			Face:      fs.face,
			Size:      sizeFixed,
			Language:  language.DefaultLanguage(),
		}
		var outs []shaping.Output
		for _, run := range ff.seg.Split(in, singleFace{fs.face}) {
			outs = append(outs, ff.shaper.Shape(run))
		}
		return outs
	}
	var cfg shaping.WrapConfig
	if st.MaxLines > 0 && st.Overflow == TextOverflowEllipsis {
		if outs := shape([]rune("…")); len(outs) == 1 {
			cfg.Truncator = outs[0]
		}
	}

	var lines []shaping.Line
	for i, para := range paras {
		remaining := st.MaxLines - len(lines)
		if st.MaxLines > 0 && remaining <= 0 {
			break
		}
		runes := []rune(strings.ReplaceAll(para, "\t", " "))
		if len(runes) == 0 {
			lines = append(lines, nil)
			continue
		}
		c := cfg
		if st.MaxLines > 0 && st.Overflow == TextOverflowEllipsis {
			c.TruncateAfterLines = remaining
			c.TextContinues = i < len(paras)-1
		}
		wrapped, _ := ff.wrapper.WrapParagraphF(c, maxW, runes, shaping.NewSliceIterator(shape(runes)))
		for _, l := range wrapped {
			lines = append(lines, slices.Clone(l))
		}
	}
	if st.MaxLines > 0 && len(lines) > st.MaxLines {
		lines = lines[:st.MaxLines]
	}

	ascent, descent, gap := lineMetrics(fs.face, size)
	stretch := st.LineHeight
	if stretch == 0 {
		stretch = 1
	}
	var y float64
	for i, l := range lines {
		a := ascent
		if i > 0 {
			a *= stretch
		}
		tl := textLine{baseline: y + a}
		slices.SortFunc(l, func(a, b shaping.Output) int { return int(a.VisualIndex - b.VisualIndex) })
		var x float64
		for _, out := range l {
			for _, g := range out.Glyphs {
				tl.glyphs = append(tl.glyphs, placedGlyph{
					id: g.GlyphID,
					x:  x + fromFixed(g.XOffset)*unit,
					y:  -fromFixed(g.YOffset) * unit,
				})
				x += fromFixed(g.Advance) * unit
			}
		}
		tl.width = x
		lay.width = max(lay.width, x)
		lay.lines = append(lay.lines, tl)
		y += a + (descent+gap)*stretch
	}
	if len(lines) > 0 {
		lay.height = lay.lines[len(lay.lines)-1].baseline + descent
	}
	return lay
}

// outlineKey identifies a glyph outline with faux styling applied.
type outlineKey struct {
	face  *font.Face
	style FontStyle
	id    font.GID
}

// outline returns the glyph's outline in font units (Y up) with faux bold and
// italic applied. Outlines are cached per family; the count is bounded by the
// fonts' glyphs. The caller holds ff.mu.
func (ff *FontFamily) outline(fs faceStyle, id font.GID) []opentype.Segment {
	key := outlineKey{fs.face, fs.style, id}
	if segs, ok := ff.outlines[key]; ok {
		return segs
	}
	var segs []opentype.Segment
	if o, ok := fs.face.GlyphData(id).(font.GlyphOutline); ok {
		segs = slices.Clone(o.Segments)
		if fs.fauxBold > 0 {
			embolden(segs, float32(fs.fauxBold*float64(fs.face.Upem())))
		}
		if fs.fauxItalic != 0 {
			k := float32(fs.fauxItalic)
			for i := range segs {
				for j := range segs[i].Args {
					segs[i].Args[j].X += k * segs[i].Args[j].Y
				}
			}
		}
	}
	if ff.outlines == nil {
		ff.outlines = map[outlineKey][]opentype.Segment{}
	}
	ff.outlines[key] = segs
	return segs
}

// drawText fills the laid-out lines of ff, aligned within width, with the
// text's top-left corner at (x, y). Coordinates are in logical pixels.
func (ff *FontFamily) drawText(p *painter, lay *textLayout, st Style, x, y, width, scale float64) {
	fs := lay.face
	f := fs.size * scale / float64(fs.face.Upem())
	for _, l := range lay.lines {
		lx := x
		switch st.Align {
		case TextAlignCenter:
			lx += (width - l.width) / 2
		case TextAlignRight:
			lx += width - l.width
		}
		// Snapping the baseline to whole physical pixels keeps horizontal
		// stems and the baseline sharp.
		base := math.Floor((y+l.baseline)*scale + 0.5)
		var pa path
		ff.mu.Lock()
		for _, g := range l.glyphs {
			gx := (lx + g.x) * scale
			gy := base + g.y*scale
			for _, seg := range ff.outline(fs, g.id) {
				pt := func(i int) (float32, float32) {
					a := seg.Args[i]
					return f32(gx + float64(a.X)*f), f32(gy - float64(a.Y)*f)
				}
				switch seg.Op {
				case opentype.SegmentOpMoveTo:
					pa.moveTo(pt(0))
				case opentype.SegmentOpLineTo:
					pa.lineTo(pt(0))
				case opentype.SegmentOpQuadTo:
					bx, by := pt(0)
					cx, cy := pt(1)
					pa.quadTo(bx, by, cx, cy)
				case opentype.SegmentOpCubeTo:
					bx, by := pt(0)
					cx, cy := pt(1)
					dx, dy := pt(2)
					pa.cubeTo(bx, by, cx, cy, dx, dy)
				}
			}
		}
		ff.mu.Unlock()
		p.fill(&pa, st.Color)
	}
}

// embolden offsets each contour of segs outward by d font units, moving every
// point along the bisector of its neighboring edges, as FreeType's
// FT_Outline_EmboldenXY does.
func embolden(segs []opentype.Segment, d float32) {
	type ref struct{ seg, arg int }
	// Orientation: in Y-up coordinates TrueType outer contours run clockwise
	// (negative area), CFF counterclockwise.
	var area float32
	var contours [][]ref
	var cur []ref
	for i, s := range segs {
		if s.Op == opentype.SegmentOpMoveTo && len(cur) > 0 {
			contours = append(contours, cur)
			cur = nil
		}
		for j := range s.ArgsSlice() {
			cur = append(cur, ref{i, j})
		}
	}
	if len(cur) > 0 {
		contours = append(contours, cur)
	}
	at := func(r ref) opentype.SegmentPoint { return segs[r.seg].Args[r.arg] }
	for _, c := range contours {
		for i := range c {
			a, b := at(c[i]), at(c[(i+1)%len(c)])
			area += a.X*b.Y - b.X*a.Y
		}
	}
	if area == 0 {
		return
	}
	clockwise := area < 0

	for _, c := range contours {
		n := len(c)
		// The contour's closing point repeats its first; skip duplicates so
		// each direction comes from a real edge.
		pts := make([]opentype.SegmentPoint, n)
		for i, r := range c {
			pts[i] = at(r)
		}
		shifts := make([][2]float32, n)
		for i := range n {
			cur := pts[i]
			prev, next := cur, cur
			for k := 1; k < n && prev == cur; k++ {
				prev = pts[(i-k+n)%n]
			}
			for k := 1; k < n && next == cur; k++ {
				next = pts[(i+k)%n]
			}
			inX, inY, ok1 := unit(cur.X-prev.X, cur.Y-prev.Y)
			outX, outY, ok2 := unit(next.X-cur.X, next.Y-cur.Y)
			if !ok1 || !ok2 {
				continue
			}
			cos := inX*outX + inY*outY
			if cos <= -0.9375 { // too sharp a turn to shift reliably
				continue
			}
			cos++
			sx, sy := inY+outY, inX+outX
			if clockwise {
				sx = -sx
			} else {
				sy = -sy
			}
			sin := outX*inY - outY*inX
			if clockwise {
				sin = -sin
			}
			// Limit the shift on very sharp corners.
			if d*sin <= d*cos {
				sx, sy = sx*d/cos, sy*d/cos
			} else {
				sx, sy = sx*d/sin, sy*d/sin
			}
			shifts[i] = [2]float32{sx, sy}
		}
		for i, r := range c {
			segs[r.seg].Args[r.arg].X += shifts[i][0]
			segs[r.seg].Args[r.arg].Y += shifts[i][1]
		}
	}
}

func unit(x, y float32) (float32, float32, bool) {
	l := float32(math.Hypot(float64(x), float64(y)))
	if l == 0 {
		return 0, 0, false
	}
	return x / l, y / l, true
}
