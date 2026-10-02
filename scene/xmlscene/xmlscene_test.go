package xmlscene_test

import (
	"image/color"
	"reflect"
	"testing"

	"samuelks.com/demy"
	"samuelks.com/demy/scene/xmlscene"
)

func TestMarshalSceneRoundTrip(t *testing.T) {
	root := &demy.VStackNode{
		Gap: 8,
		Children: []demy.Node{
			&demy.TextNode{Text: "Title", Class: "h1"},
			&demy.TextNode{Text: "Body text", Class: "body"},
		},
	}
	styles := map[string]demy.Style{
		"body": {Font: "sans", Size: 12},
		"h1":   {Font: "sans", Size: 24, Weight: demy.FontBold},
	}

	data, err := xmlscene.MarshalScene(root, styles)
	if err != nil {
		t.Fatalf("MarshalScene: %v", err)
	}
	t.Logf("XML:\n%s", data)

	gotNode, gotStyles, err := xmlscene.UnmarshalScene(data, demy.Config{})
	if err != nil {
		t.Fatalf("UnmarshalScene: %v", err)
	}
	if !reflect.DeepEqual(root, gotNode) {
		t.Errorf("node mismatch\nwant: %#v\ngot:  %#v", root, gotNode)
	}
	if !reflect.DeepEqual(styles, gotStyles) {
		t.Errorf("styles mismatch\nwant: %#v\ngot:  %#v", styles, gotStyles)
	}
}

func TestUnmarshalSceneFallback(t *testing.T) {
	// A bare node element (no <scene> wrapper) should still work.
	data, err := xmlscene.Marshal(&demy.TextNode{Text: "bare"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	gotNode, gotStyles, err := xmlscene.UnmarshalScene(data, demy.Config{})
	if err != nil {
		t.Fatalf("UnmarshalScene fallback: %v", err)
	}
	want := &demy.TextNode{Text: "bare"}
	if !reflect.DeepEqual(want, gotNode) {
		t.Errorf("node mismatch: want %#v, got %#v", want, gotNode)
	}
	if len(gotStyles) != 0 {
		t.Errorf("expected no styles, got %v", gotStyles)
	}
}

func TestMarshalSceneNoStyles(t *testing.T) {
	root := &demy.TextNode{Text: "hi"}
	data, err := xmlscene.MarshalScene(root, nil)
	if err != nil {
		t.Fatalf("MarshalScene: %v", err)
	}
	t.Logf("XML:\n%s", data)
	gotNode, gotStyles, err := xmlscene.UnmarshalScene(data, demy.Config{})
	if err != nil {
		t.Fatalf("UnmarshalScene: %v", err)
	}
	if !reflect.DeepEqual(root, gotNode) {
		t.Errorf("node mismatch")
	}
	if len(gotStyles) != 0 {
		t.Errorf("expected no styles, got %v", gotStyles)
	}
}

func TestMarshalGridRoundTrip(t *testing.T) {
	root := &demy.GridNode{
		Columns: []demy.GridTrack{demy.GridFixed(100), demy.GridAuto(), demy.GridFr(1)},
		Rows:    []demy.GridTrack{demy.GridAuto(), demy.GridFixed(40)},
		RowGap:  4,
		ColGap:  8,
		Padding: 12,
		Children: []demy.Node{
			&demy.TextNode{Text: "a"},
			&demy.TextNode{Text: "b"},
			&demy.TextNode{Text: "c"},
		},
	}
	data, err := xmlscene.Marshal(root)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	t.Logf("XML:\n%s", data)
	got, err := xmlscene.Unmarshal(data, demy.Config{})
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(root, got) {
		t.Errorf("node mismatch\nwant: %#v\ngot:  %#v", root, got)
	}
}

func TestMarshalGridNoRows(t *testing.T) {
	root := &demy.GridNode{
		Columns:  []demy.GridTrack{demy.GridFr(1), demy.GridFr(2)},
		Children: []demy.Node{&demy.TextNode{Text: "x"}, &demy.TextNode{Text: "y"}},
	}
	data, err := xmlscene.Marshal(root)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := xmlscene.Unmarshal(data, demy.Config{})
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(root, got) {
		t.Errorf("node mismatch\nwant: %#v\ngot:  %#v", root, got)
	}
}

func TestMarshalGridRowRoundTrip(t *testing.T) {
	root := &demy.GridNode{
		Columns: []demy.GridTrack{demy.GridAuto(), demy.GridFr(1)},
		RowGap:  6,
		ColGap:  8,
		Children: []demy.Node{
			&demy.GridRowNode{Children: []demy.Node{
				&demy.TextNode{Text: "Name"},
				&demy.TextNode{Text: "Value"},
			}},
			&demy.GridRowNode{Children: []demy.Node{
				&demy.TextNode{Text: "Temp"},
				&demy.TextNode{Text: "72 °F"},
			}},
		},
	}
	data, err := xmlscene.Marshal(root)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	t.Logf("XML:\n%s", data)
	got, err := xmlscene.Unmarshal(data, demy.Config{})
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(root, got) {
		t.Errorf("node mismatch\nwant: %#v\ngot:  %#v", root, got)
	}
}

func TestExecuteScene(t *testing.T) {
	const tmpl = `<scene>
<styles>
<style name="title" font="sans" size="18" weight="bold"/>
<style name="sub" font="sans" size="12" color="#555555"/>
</styles>
<vstack gap="4">
<text class="title">{{.Title}}</text>
<text class="sub">{{.Sub}}</text>
</vstack>
</scene>`

	data := struct{ Title, Sub string }{"Hello", "World"}
	gotNode, gotStyles, err := xmlscene.ExecuteScene(t.Context(), []byte(tmpl), data, demy.Config{})
	if err != nil {
		t.Fatalf("ExecuteScene: %v", err)
	}

	vs, ok := gotNode.(*demy.VStackNode)
	if !ok {
		t.Fatalf("expected *VStackNode, got %T", gotNode)
	}
	if vs.Gap != 4 {
		t.Errorf("gap: want 4, got %v", vs.Gap)
	}
	if len(vs.Children) != 2 {
		t.Fatalf("children: want 2, got %d", len(vs.Children))
	}
	if tn, ok := vs.Children[0].(*demy.TextNode); !ok || tn.Class != "title" || tn.Text != "Hello" {
		t.Errorf("children[0]: %#v", vs.Children[0])
	}
	if tn, ok := vs.Children[1].(*demy.TextNode); !ok || tn.Class != "sub" || tn.Text != "World" {
		t.Errorf("children[1]: %#v", vs.Children[1])
	}

	wantStyles := map[string]demy.Style{
		"title": {Font: "sans", Size: 18, Weight: demy.FontBold},
		"sub":   {Font: "sans", Size: 12, Color: color.RGBA{R: 0x55, G: 0x55, B: 0x55, A: 0xff}},
	}
	if !reflect.DeepEqual(wantStyles, gotStyles) {
		t.Errorf("styles mismatch\nwant: %#v\ngot:  %#v", wantStyles, gotStyles)
	}
}

func TestRoundTrip(t *testing.T) {
	want := &demy.VStackNode{
		Gap:     4,
		Padding: 8,
		Align:   demy.AlignCenter,
		Children: []demy.Node{
			&demy.HStackNode{
				Gap:     2,
				Padding: 4,
				Align:   demy.AlignEnd,
				Children: []demy.Node{
					&demy.TextNode{
						Text: "Hello & <World>",
						Style: demy.Style{
							Font:       "sans",
							Size:       12,
							Weight:     demy.FontBold | demy.FontItalic,
							Color:      demy.RoleColor{Name: "foreground"},
							Align:      demy.TextAlignCenter,
							LineHeight: 1.5,
							MaxLines:   3,
							Overflow:   demy.TextOverflowEllipsis,
						},
					},
					&demy.SpacerNode{Weight: 1},
					&demy.TextNode{Text: "World", Style: demy.Style{Color: color.RGBA{R: 0xff, G: 0x80, B: 0x00, A: 0xff}}},
				},
			},
			&demy.ZStackNode{
				Align: demy.ZAlignBottomTrailing,
				Children: []demy.Node{
					&demy.ImageNode{Key: "bg", Scale: demy.ImageFill},
					&demy.TextNode{Text: "Overlay"},
				},
			},
			&demy.BoxNode{
				Padding:      6,
				BorderWidth:  1,
				BorderColor:  demy.RoleColor{Name: "muted"},
				Background:   color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff},
				CornerRadius: 4,
				Clip:         true,
				Children:     []demy.Node{&demy.DividerNode{}},
			},
			&demy.FrameNode{
				Width:    100,
				Height:   50,
				Children: []demy.Node{&demy.ImageNode{Key: "icon", Scale: demy.ImageStretch}},
			},
		},
	}

	data, err := xmlscene.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	t.Logf("XML:\n%s", data)

	got, err := xmlscene.Unmarshal(data, demy.Config{})
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !reflect.DeepEqual(want, got) {
		t.Errorf("round-trip mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestMarshalNil(t *testing.T) {
	if _, err := xmlscene.Marshal(nil); err == nil {
		t.Error("Marshal(nil): expected error")
	}
	// Nil children are skipped when rendering, so they aren't written.
	data, err := xmlscene.Marshal(&demy.VStackNode{Children: []demy.Node{demy.Text("a"), nil, (*demy.TextNode)(nil), demy.Text("b")}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := xmlscene.Unmarshal(data, demy.Config{})
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if n := len(got.(*demy.VStackNode).Children); n != 2 { //nolint:forcetypeassert // test
		t.Errorf("children = %d, want 2", n)
	}
}

func TestExecute(t *testing.T) {
	const tmpl = `<vstack gap="4">
<text>{{.Title}}</text>
{{range .Items}}<text size="12">{{.}}</text>
{{end}}</vstack>`

	data := struct {
		Title string
		Items []string
	}{
		Title: "Hello",
		Items: []string{"one", "two"},
	}

	got, err := xmlscene.Execute(t.Context(), []byte(tmpl), data, demy.Config{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	vs, ok := got.(*demy.VStackNode)
	if !ok {
		t.Fatalf("expected *VStackNode, got %T", got)
	}
	if vs.Gap != 4 {
		t.Errorf("gap: want 4, got %v", vs.Gap)
	}
	if len(vs.Children) != 3 {
		t.Fatalf("children: want 3, got %d", len(vs.Children))
	}
	texts := []string{"Hello", "one", "two"}
	for i, text := range texts {
		tn, ok := vs.Children[i].(*demy.TextNode)
		if !ok {
			t.Errorf("children[%d]: want *TextNode, got %T", i, vs.Children[i])
			continue
		}
		if tn.Text != text {
			t.Errorf("children[%d].Text: want %q, got %q", i, text, tn.Text)
		}
	}
}

func TestPhysicalUnits(t *testing.T) {
	// TRMNL Original: 800×480px, 163×98mm → DPMM ≈ 4.908
	cfg := demy.TRMNLOriginal
	dpmm := cfg.DPMM()

	const src = `<vstack gap="4mm" padding="2pt">
<frame width="50mm" height="45mm">
<text size="12pt">hello</text>
</frame>
</vstack>`

	got, err := xmlscene.Unmarshal([]byte(src), cfg)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	vs, ok := got.(*demy.VStackNode)
	if !ok {
		t.Fatalf("expected *VStackNode, got %T", got)
	}

	wantGap := 4 * dpmm
	if vs.Gap != wantGap {
		t.Errorf("gap: want %v, got %v", wantGap, vs.Gap)
	}
	wantPadding := 2 * dpmm * 25.4 / 72
	if vs.Padding != wantPadding {
		t.Errorf("padding: want %v, got %v", wantPadding, vs.Padding)
	}

	fr, ok := vs.Children[0].(*demy.FrameNode)
	if !ok {
		t.Fatalf("expected *FrameNode, got %T", vs.Children[0])
	}
	wantW := 50 * dpmm
	wantH := 45 * dpmm
	if fr.Width != wantW {
		t.Errorf("frame width: want %v, got %v", wantW, fr.Width)
	}
	if fr.Height != wantH {
		t.Errorf("frame height: want %v, got %v", wantH, fr.Height)
	}

	txt, ok := fr.Children[0].(*demy.TextNode)
	if !ok {
		t.Fatalf("expected *TextNode, got %T", fr.Children[0])
	}
	// Font sizes are in points, so 12pt stays 12.
	const wantSize = 12
	if txt.Style.Size != wantSize {
		t.Errorf("text size: want %v, got %v", wantSize, txt.Style.Size)
	}
}

func TestPhysicalUnitsBarePixels(t *testing.T) {
	// Zero-value Config (no physical dims): bare numbers still work as pixels.
	const src = `<frame width="100" height="50px"></frame>`
	got, err := xmlscene.Unmarshal([]byte(src), demy.Config{})
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	fr, ok := got.(*demy.FrameNode)
	if !ok {
		t.Fatalf("expected *FrameNode, got %T", got)
	}
	if fr.Width != 100 {
		t.Errorf("width: want 100, got %v", fr.Width)
	}
	if fr.Height != 50 {
		t.Errorf("height: want 50, got %v", fr.Height)
	}
}
