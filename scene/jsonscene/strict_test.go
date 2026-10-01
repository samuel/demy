package jsonscene_test

import (
	"encoding/json/jsontext"
	"image"
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/jsonscene"
)

func TestUnmarshalStrict(t *testing.T) {
	tests := map[string]string{
		"unknown field":     `{"type":"vstack","gapp":4}`,
		"unknown style key": `{"type":"text","style":{"line-height":1.2}}`,
		"bad align":         `{"type":"vstack","align":"right"}`,
		"bad dither":        `{"type":"image","image_key":"k","dither":"floyd"}`,
		"bad ec level":      `{"type":"barcode","content":"x","ec_level":"H"}`,
		"bad barcode type":  `{"type":"barcode","content":"x","barcode_type":"code128"}`,
		"bad track":         `{"type":"grid","columns":[{"kind":"px"}]}`,
		"bad color":         `{"type":"box","background":"#zzz"}`,
		"bad weight":        `{"type":"text","style":{"weight":"semibold"}}`,
		"leaf children":     `{"type":"spacer","children":[{"type":"spacer"}]}`,
		"weighted two":      `{"type":"weighted","children":[{"type":"spacer"},{"type":"spacer"}]}`,
		"trailing data":     `{"type":"vstack"} {"type":"hstack"}`,
		"unknown type":      `{"type":"vstak"}`,
		"duplicate field":   `{"type":"vstack","gap":1,"gap":2}`,
		"field case":        `{"type":"vstack","Gap":4}`,
		"invalid utf-8":     "{\"type\":\"text\",\"text\":\"\xff\"}",
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := jsonscene.Unmarshal([]byte(src)); err == nil {
				t.Errorf("Unmarshal(%s): expected error", src)
			}
		})
	}
	if _, _, err := jsonscene.UnmarshalScene([]byte(`{"root":{"type":"vstack"},"extra":1}`)); err == nil {
		t.Error("UnmarshalScene with unknown field: expected error")
	}
}

func TestMarshalNil(t *testing.T) {
	if _, err := jsonscene.Marshal(nil); err == nil {
		t.Error("Marshal(nil): expected error")
	}
	data, err := jsonscene.Marshal(&demy.VStackNode{Children: []demy.Node{demy.Text("a"), nil, (*demy.TextNode)(nil), demy.Text("b")}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := jsonscene.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if n := len(got.(*demy.VStackNode).Children); n != 2 { //nolint:forcetypeassert // test
		t.Errorf("children = %d, want 2", n)
	}
}

func TestMarshalErrors(t *testing.T) {
	tests := map[string]demy.Node{
		"image without key": demy.Image(image.NewRGBA(image.Rect(0, 0, 1, 1))),
		"unknown role":      &demy.DividerNode{Color: demy.RoleColor{Name: "primary"}},
		"empty role":        &demy.DividerNode{Color: demy.RoleColor{}},
		"bad enum":          &demy.VStackNode{Align: demy.Alignment(42)},
	}
	for name, n := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := jsonscene.Marshal(n); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestFullRoundTrip(t *testing.T) {
	root := &demy.VStackNode{Gap: 2, Padding: 3, Align: demy.AlignCenter, Children: []demy.Node{
		&demy.HStackNode{Align: demy.AlignEnd, Children: []demy.Node{
			&demy.WeightedNode{Weight: 2, Child: &demy.SpacerNode{Weight: 3}},
			&demy.DividerNode{Size: 2, Color: demy.RoleColor{Name: "accent"}},
		}},
		&demy.ZStackNode{Align: demy.BottomTrailing, Children: []demy.Node{
			&demy.BoxNode{Padding: 1, BorderWidth: 2, BorderColor: demy.RoleColor{Name: "muted"},
				Background: color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 0x80}, CornerRadius: 4, Clip: true},
		}},
		&demy.FrameNode{Width: 10, Height: 20, Children: []demy.Node{
			&demy.ImageNode{Key: "k", Scale: demy.ImageStretch, Dither: demy.Atkinson},
		}},
		&demy.BarcodeNode{Content: "x", ECLevel: demy.ECLevelLow},
		&demy.TextNode{Text: "t", Class: "c", Style: demy.Style{
			Font: "f", Size: 9, Weight: demy.FontRegular | demy.FontItalic, Align: demy.TextAlignLeft,
			LineHeight: 1.5, MaxLines: demy.MaxLinesUnlimited, Overflow: demy.TextOverflowWrap,
		}},
		&demy.GridNode{RowGap: 1, ColGap: 2, Padding: 3,
			Columns:  []demy.GridTrack{demy.GridFixed(5), demy.GridAuto(), demy.GridFr(2)},
			Rows:     []demy.GridTrack{demy.GridFr(1)},
			Children: []demy.Node{&demy.GridRowNode{Children: []demy.Node{&demy.SpacerNode{}}}},
		},
	}}
	data, err := jsonscene.Marshal(root)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := jsonscene.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, data)
	}
	if !reflect.DeepEqual(root, got) {
		t.Errorf("round trip mismatch\nwant: %#v\ngot:  %#v\nJSON: %s", root, got, data)
	}
}

func TestMarshalOmitsZero(t *testing.T) {
	data, err := jsonscene.Marshal(&demy.VStackNode{Children: []demy.Node{&demy.SpacerNode{}}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if want := `{"type":"vstack","children":[{"type":"spacer"}]}`; string(data) != want {
		t.Errorf("Marshal = %s, want %s", data, want)
	}
}

func TestMarshalOptions(t *testing.T) {
	styles := map[string]demy.Style{"b": {Size: 2}, "a": {Size: 1}, "c": {Size: 3}}
	data, err := jsonscene.MarshalScene(&demy.SpacerNode{}, styles, jsontext.WithIndent("  "))
	if err != nil {
		t.Fatalf("MarshalScene: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "\n  ") {
		t.Errorf("expected indented output:\n%s", s)
	}
	if a, b, c := strings.Index(s, `"a"`), strings.Index(s, `"b"`), strings.Index(s, `"c"`); a >= b || b >= c {
		t.Errorf("style keys not sorted:\n%s", s)
	}
}
