package xmlscene_test

import (
	"image/color"
	"reflect"
	"testing"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/xmlscene"
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

	gotNode, gotStyles, err := xmlscene.UnmarshalScene(data)
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
	gotNode, gotStyles, err := xmlscene.UnmarshalScene(data)
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
	gotNode, gotStyles, err := xmlscene.UnmarshalScene(data)
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
	gotNode, gotStyles, err := xmlscene.ExecuteScene([]byte(tmpl), data)
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
				Align: demy.BottomTrailing,
				Children: []demy.Node{
					&demy.ImageNode{Key: "bg", Scale: demy.ImageFill, Clip: true},
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

	got, err := xmlscene.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !reflect.DeepEqual(want, got) {
		t.Errorf("round-trip mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestNilRoundTrip(t *testing.T) {
	data, err := xmlscene.Marshal(nil)
	if err != nil {
		t.Fatalf("Marshal(nil): %v", err)
	}
	got, err := xmlscene.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := &demy.SpacerNode{Weight: 0}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("nil round-trip: want %#v, got %#v", want, got)
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

	got, err := xmlscene.Execute([]byte(tmpl), data)
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
