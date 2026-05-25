package scene_test

import (
	"image/color"
	"reflect"
	"testing"

	"github.com/samuel/demy"
	scene "github.com/samuel/demy/scene/jsonscene"
)

func TestMarshalScene(t *testing.T) {
	root := &demy.VStackNode{
		Gap: 8,
		Children: []demy.Node{
			&demy.TextNode{Text: "hello", Class: "h1"},
			&demy.TextNode{Text: "world", Class: "body"},
		},
	}
	styles := map[string]demy.Style{
		"h1":   {Font: "sans", Size: 24, Weight: demy.FontBold},
		"body": {Font: "sans", Size: 12},
	}

	data, err := scene.MarshalScene(root, styles)
	if err != nil {
		t.Fatalf("MarshalScene: %v", err)
	}
	t.Logf("JSON:\n%s", data)

	gotNode, gotStyles, err := scene.UnmarshalScene(data)
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

func TestMarshalSceneNoStyles(t *testing.T) {
	root := &demy.TextNode{Text: "hi"}
	data, err := scene.MarshalScene(root, nil)
	if err != nil {
		t.Fatalf("MarshalScene: %v", err)
	}
	gotNode, gotStyles, err := scene.UnmarshalScene(data)
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

func TestMarshalSceneColorStyle(t *testing.T) {
	styles := map[string]demy.Style{
		"muted-label": {Font: "sans", Size: 10, Color: demy.RoleColor{Name: "muted"}},
		"colored":     {Font: "sans", Size: 12, Color: color.RGBA{R: 0x55, G: 0x55, B: 0x55, A: 0xff}},
	}
	data, err := scene.MarshalScene(&demy.SpacerNode{}, styles)
	if err != nil {
		t.Fatalf("MarshalScene: %v", err)
	}
	_, gotStyles, err := scene.UnmarshalScene(data)
	if err != nil {
		t.Fatalf("UnmarshalScene: %v", err)
	}
	if !reflect.DeepEqual(styles, gotStyles) {
		t.Errorf("styles mismatch\nwant: %#v\ngot:  %#v", styles, gotStyles)
	}
}
