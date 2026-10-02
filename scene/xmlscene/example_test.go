package xmlscene_test

import (
	"context"
	"fmt"
	"log"

	"samuelks.com/demy"
	"samuelks.com/demy/scene/xmlscene"
)

func ExampleMarshal() {
	root := demy.VStack(demy.Gap(4),
		demy.Text("Hello", demy.Style{Size: 12, Weight: demy.FontBold}),
		demy.HStack(demy.Spacer(), demy.Text("right")),
	)
	data, err := xmlscene.Marshal(root)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
	// Output:
	// <vstack gap="4">
	// 	<text size="12" weight="bold">Hello</text>
	// 	<hstack>
	// 		<spacer weight="1"></spacer>
	// 		<text>right</text>
	// 	</hstack>
	// </vstack>
}

// Unmarshal converts physical units with the Config's pixel density.
func ExampleUnmarshal() {
	n, err := xmlscene.Unmarshal([]byte(`<frame width="10mm" height="40"><text size="12">hi</text></frame>`), demy.TRMNLOriginal)
	if err != nil {
		log.Fatal(err)
	}
	f := n.(*demy.FrameNode)               //nolint:forcetypeassert // example
	text := f.Children[0].(*demy.TextNode) //nolint:forcetypeassert // example
	fmt.Printf("%.1f x %.0f, %q\n", f.Width, f.Height, text.Text)
	// Output: 49.1 x 40, "hi"
}

// Execute renders a text/template to a node tree. Template output is
// XML-escaped, so data can't inject elements.
func ExampleExecute() {
	const tmpl = `<vstack gap="4">
  <text>{{.Title}}</text>
  {{range .Items}}<hstack><text>{{.Name}}</text><spacer/><text>{{.Qty}}</text></hstack>
  {{end}}
</vstack>`
	data := map[string]any{
		"Title": "Groceries <today>",
		"Items": []map[string]any{{"Name": "Milk", "Qty": 2}, {"Name": "Eggs", "Qty": 12}},
	}
	n, err := xmlscene.Execute(context.Background(), []byte(tmpl), data, demy.Config{})
	if err != nil {
		log.Fatal(err)
	}
	out, err := xmlscene.Marshal(n)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(out))
	// Output:
	// <vstack gap="4">
	// 	<text>Groceries &lt;today&gt;</text>
	// 	<hstack>
	// 		<text>Milk</text>
	// 		<spacer></spacer>
	// 		<text>2</text>
	// 	</hstack>
	// 	<hstack>
	// 		<text>Eggs</text>
	// 		<spacer></spacer>
	// 		<text>12</text>
	// 	</hstack>
	// </vstack>
}

// ParseTemplate parses a template once so it can be executed many times.
func ExampleTemplate() {
	tmpl := xmlscene.MustParseTemplate([]byte(`<text>{{.}}</text>`))
	for _, s := range []string{"first", "second"} {
		n, err := tmpl.Execute(context.Background(), s, demy.Config{})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(n.(*demy.TextNode).Text)
	}
	// Output:
	// first
	// second
}
