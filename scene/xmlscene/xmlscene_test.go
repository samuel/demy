package xmlscene_test

import (
	"image/color"
	"reflect"
	"testing"

	"github.com/samuel/pate"
	"github.com/samuel/pate/scene/xmlscene"
)

func TestRoundTrip(t *testing.T) {
	want := &pate.VStackNode{
		Gap:     4,
		Padding: 8,
		Align:   pate.AlignCenter,
		Children: []pate.Node{
			&pate.HStackNode{
				Gap:     2,
				Padding: 4,
				Align:   pate.AlignEnd,
				Children: []pate.Node{
					&pate.TextNode{
						Text: "Hello & <World>",
						Style: pate.Style{
							Font:       "sans",
							Size:       12,
							Weight:     pate.FontBold | pate.FontItalic,
							Color:      pate.RoleColor{Name: "foreground"},
							Align:      pate.TextAlignCenter,
							LineHeight: 1.5,
							MaxLines:   3,
							Overflow:   pate.TextOverflowEllipsis,
						},
					},
					&pate.SpacerNode{Weight: 1},
					&pate.TextNode{Text: "World", Style: pate.Style{Color: color.RGBA{R: 0xff, G: 0x80, B: 0x00, A: 0xff}}},
				},
			},
			&pate.ZStackNode{
				Align: pate.BottomTrailing,
				Children: []pate.Node{
					&pate.ImageNode{Key: "bg", Scale: pate.ImageFill, Clip: true},
					&pate.TextNode{Text: "Overlay"},
				},
			},
			&pate.BoxNode{
				Padding:      6,
				BorderWidth:  1,
				BorderColor:  pate.RoleColor{Name: "muted"},
				Background:   color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff},
				CornerRadius: 4,
				Clip:         true,
				Children:     []pate.Node{&pate.DividerNode{}},
			},
			&pate.FrameNode{
				Width:    100,
				Height:   50,
				Children: []pate.Node{&pate.ImageNode{Key: "icon", Scale: pate.ImageStretch}},
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
	want := &pate.SpacerNode{Weight: 0}
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

	vs, ok := got.(*pate.VStackNode)
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
		tn, ok := vs.Children[i].(*pate.TextNode)
		if !ok {
			t.Errorf("children[%d]: want *TextNode, got %T", i, vs.Children[i])
			continue
		}
		if tn.Text != text {
			t.Errorf("children[%d].Text: want %q, got %q", i, text, tn.Text)
		}
	}
}
