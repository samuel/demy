package xmlscene_test

import (
	"image"
	"image/color"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/xmlscene"
)

func TestUnmarshalStrict(t *testing.T) {
	tests := map[string]string{
		"unknown element":   `<vstak/>`,
		"unknown attribute": `<vstack gapp="4"/>`,
		"namespaced attr":   `<vstack xmlns:x="u" x:gap="4"/>`,
		"bad align":         `<vstack align="right"/>`,
		"bad dither":        `<image key="k" dither="floyd"/>`,
		"bad ec level":      `<barcode content="x" ec-level="H"/>`,
		"bad barcode type":  `<barcode content="x" type="code128"/>`,
		"bad number":        `<vstack gap="4 px"/>`,
		"bad bool":          `<box clip="1"/>`,
		"bad max lines":     `<text max-lines="two">a</text>`,
		"bad weight":        `<text weight="semibold">a</text>`,
		"bad color":         `<box background="#zzz"/>`,
		"mm without config": `<frame width="5mm"/>`,
		"text children":     `<text>Hi <b>there</b></text>`,
		"leaf children":     `<spacer><text/></spacer>`,
		"leaf text":         `<divider>x</divider>`,
		"container text":    `<vstack>stray</vstack>`,
		"weighted two":      `<weighted><spacer/><spacer/></weighted>`,
		"unknown track":     `<grid><columns><fixd size="1"/></columns></grid>`,
		"duplicate columns": `<grid><columns><auto/></columns><columns><auto/></columns></grid>`,
		"two roots":         `<vstack/><hstack/>`,
		"trailing text":     `<vstack/>garbage`,
		"empty":             ``,
		"nan width":         `<frame width="NaN"/>`,
		"inf width":         `<frame width="+Inf"/>`,
		"infinity px":       `<frame width="infinitypx"/>`,
		"hex width":         `<frame width="0x1p4"/>`,
		"hex unit":          `<frame width="0x10px"/>`,
		"overflow width":    `<frame width="1e400"/>`,
		"nan size":          `<text size="NaN">a</text>`,
		"inf size pt":       `<text size="Infpt">a</text>`,
		"nan line height":   `<text line-height="NaN">a</text>`,
		"inf track":         `<grid><columns><fixed size="Inf"/></columns></grid>`,
		"hex fr":            `<grid><columns><fr weight="0x1"/></columns></grid>`,
		"underscore":        `<vstack gap="1_0"/>`,
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := xmlscene.Unmarshal([]byte(src), demy.Config{}); err == nil {
				t.Errorf("Unmarshal(%q): expected error", src)
			}
		})
	}
}

func TestUnmarshalSceneStrict(t *testing.T) {
	tests := map[string]string{
		"no root":          `<scene><styles/></scene>`,
		"two roots":        `<scene><vstack/><hstack/></scene>`,
		"two styles":       `<scene><styles/><styles/><vstack/></scene>`,
		"style no name":    `<scene><styles><style font="a"/></styles><vstack/></scene>`,
		"duplicate style":  `<scene><styles><style name="a"/><style name="a"/></styles><vstack/></scene>`,
		"non-style child":  `<scene><styles><text/></styles><vstack/></scene>`,
		"scene attribute":  `<scene version="1"><vstack/></scene>`,
		"style bad weight": `<scene><styles><style name="a" weight="heavy"/></styles><vstack/></scene>`,
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := xmlscene.UnmarshalScene([]byte(src), demy.Config{}); err == nil {
				t.Errorf("UnmarshalScene(%q): expected error", src)
			}
		})
	}
}

func TestUnmarshalAllowsProlog(t *testing.T) {
	src := "<?xml version=\"1.0\"?>\n<!-- comment -->\n<vstack><!-- inner --></vstack>\n<!-- after -->\n"
	if _, err := xmlscene.Unmarshal([]byte(src), demy.Config{}); err != nil {
		t.Fatal(err)
	}
}

func TestTextFontSizeUnits(t *testing.T) {
	cfg := demy.TRMNLOriginal
	tests := map[string]float64{
		"12":                        12,
		"12pt":                      12,
		"25.4mm":                    72,
		formatPx(25.4 * cfg.DPMM()): 72, // 25.4 mm expressed in logical px
	}
	for in, want := range tests {
		got, err := xmlscene.Unmarshal([]byte(`<text size="`+in+`">a</text>`), cfg)
		if err != nil {
			t.Fatalf("size=%q: %v", in, err)
		}
		if s := got.(*demy.TextNode).Style.Size; math.Abs(s-want) > 1e-6 { //nolint:forcetypeassert // test
			t.Errorf("size=%q: got %v, want %v", in, s, want)
		}
	}
	if _, err := xmlscene.Unmarshal([]byte(`<text size="12px">a</text>`), demy.Config{}); err == nil {
		t.Error("px font size without physical dimensions: expected error")
	}
	got, err := xmlscene.Unmarshal([]byte(`<text line-height="1.5">a</text>`), demy.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if lh := got.(*demy.TextNode).Style.LineHeight; lh != 1.5 { //nolint:forcetypeassert // test
		t.Errorf("line-height = %v, want 1.5", lh)
	}
}

func formatPx(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) + "px" }

func TestExecuteEscapesData(t *testing.T) {
	const tmpl = `<vstack><text>{{.Name}}</text><text class="{{.Class}}">x</text></vstack>`
	data := map[string]string{
		"Name":  `</text></vstack><image key="secret"/><vstack><text>`,
		"Class": `h1" size="400`,
	}
	got, err := xmlscene.Execute(t.Context(), []byte(tmpl), data, demy.Config{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	vs := got.(*demy.VStackNode) //nolint:forcetypeassert // test
	if len(vs.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(vs.Children))
	}
	if txt := vs.Children[0].(*demy.TextNode).Text; txt != data["Name"] { //nolint:forcetypeassert // test
		t.Errorf("text = %q, want %q", txt, data["Name"])
	}
	if cls := vs.Children[1].(*demy.TextNode).Class; string(cls) != data["Class"] { //nolint:forcetypeassert // test
		t.Errorf("class = %q, want %q", cls, data["Class"])
	}

	got, err = xmlscene.Execute(t.Context(), []byte(`<text>{{.}}</text>`), "AT&T < 3", demy.Config{})
	if err != nil {
		t.Fatalf("Execute with & and <: %v", err)
	}
	if txt := got.(*demy.TextNode).Text; txt != "AT&T < 3" { //nolint:forcetypeassert // test
		t.Errorf("text = %q", txt)
	}
}

func TestExecuteRaw(t *testing.T) {
	const tmpl = `<vstack>{{.Inner | raw}}{{range .Items}}<text>{{.}}</text>{{end}}{{$x := .Title}}<text>{{$x}}</text></vstack>`
	data := map[string]any{"Inner": `<spacer/>`, "Items": []string{"a&b"}, "Title": "t<"}
	got, err := xmlscene.Execute(t.Context(), []byte(tmpl), data, demy.Config{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	vs := got.(*demy.VStackNode) //nolint:forcetypeassert // test
	if len(vs.Children) != 3 {
		t.Fatalf("children = %d, want 3", len(vs.Children))
	}
	if _, ok := vs.Children[0].(*demy.SpacerNode); !ok {
		t.Errorf("raw child = %T, want *SpacerNode", vs.Children[0])
	}
	if txt := vs.Children[2].(*demy.TextNode).Text; txt != "t<" { //nolint:forcetypeassert // test
		t.Errorf("variable text = %q", txt)
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
			if _, err := xmlscene.Marshal(n); err == nil {
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
	data, err := xmlscene.Marshal(root)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := xmlscene.Unmarshal(data, demy.Config{})
	if err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, data)
	}
	if !reflect.DeepEqual(root, got) {
		t.Errorf("round trip mismatch\nwant: %#v\ngot:  %#v\nXML:\n%s", root, got, data)
	}
}
