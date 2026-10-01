package jsonscene_test

import (
	"encoding/json/jsontext"
	"fmt"
	"log"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/jsonscene"
)

func ExampleMarshal() {
	root := demy.VStack(demy.Gap(4),
		demy.Text("Hello", demy.Class("h1")),
		demy.Divider(),
		demy.Box(demy.Background(demy.RoleColor{Name: "muted"}), demy.Frame(10, 10)),
	)
	data, err := jsonscene.Marshal(root)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
	// Output:
	// {"type":"vstack","gap":4,"children":[{"type":"text","text":"Hello","class":"h1"},{"type":"divider"},{"type":"box","background":"muted","children":[{"type":"frame","width":10,"height":10}]}]}
}

func ExampleUnmarshal() {
	n, err := jsonscene.Unmarshal([]byte(`{"type":"hstack","align":"center","children":[{"type":"text","text":"hi","style":{"size":12,"weight":"bold"}}]}`))
	if err != nil {
		log.Fatal(err)
	}
	h := n.(*demy.HStackNode)              //nolint:forcetypeassert // example
	text := h.Children[0].(*demy.TextNode) //nolint:forcetypeassert // example
	fmt.Println(h.Align == demy.AlignCenter, text.Text, text.Style.Size, text.Style.Weight == demy.FontBold)

	// Decoding is strict.
	_, err = jsonscene.Unmarshal([]byte(`{"type":"hstack","gapp":4}`))
	fmt.Println(err != nil)
	// Output:
	// true hi 12 true
	// true
}

// MarshalScene embeds style definitions; options such as jsontext.WithIndent
// pass through to encoding/json/v2.
func ExampleMarshalScene() {
	styles := map[string]demy.Style{"h1": {Size: 24, Weight: demy.FontBold}}
	data, err := jsonscene.MarshalScene(demy.Text("Title", demy.Class("h1")), styles, jsontext.WithIndent("  "))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
	// Output:
	// {
	//   "styles": {
	//     "h1": {
	//       "size": 24,
	//       "weight": "bold"
	//     }
	//   },
	//   "root": {
	//     "type": "text",
	//     "text": "Title",
	//     "class": "h1"
	//   }
	// }
}
