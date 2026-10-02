package demy

import (
	"math"
	"testing"

	"github.com/go-text/typesetting/font/opentype"
)

// TestEmboldenGrowsOutward checks that emboldening moves a square's edges
// outward for both contour orientations: clockwise (TrueType) and
// counterclockwise (CFF).
func TestEmboldenGrowsOutward(t *testing.T) {
	square := func(clockwise bool) []opentype.Segment {
		pts := [][2]float32{{0, 0}, {100, 0}, {100, 100}, {0, 100}}
		if clockwise {
			pts = [][2]float32{{0, 0}, {0, 100}, {100, 100}, {100, 0}}
		}
		segs := []opentype.Segment{{Op: opentype.SegmentOpMoveTo, Args: [3]opentype.SegmentPoint{{X: pts[0][0], Y: pts[0][1]}}}}
		for _, p := range append(pts[1:], pts[0]) {
			segs = append(segs, opentype.Segment{Op: opentype.SegmentOpLineTo, Args: [3]opentype.SegmentPoint{{X: p[0], Y: p[1]}}})
		}
		return segs
	}
	for _, clockwise := range []bool{true, false} {
		segs := square(clockwise)
		embolden(segs, 10)
		minX, minY := float32(math.Inf(1)), float32(math.Inf(1))
		maxX, maxY := float32(math.Inf(-1)), float32(math.Inf(-1))
		for _, s := range segs {
			p := s.Args[0]
			minX, minY, maxX, maxY = min(minX, p.X), min(minY, p.Y), max(maxX, p.X), max(maxY, p.Y)
		}
		if minX != -10 || minY != -10 || maxX != 110 || maxY != 110 {
			t.Errorf("clockwise %v: bounds (%v,%v)-(%v,%v), want (-10,-10)-(110,110)", clockwise, minX, minY, maxX, maxY)
		}
	}
}
