package starlarkplugin_test

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	sl "go.starlark.net/starlark"
	"golang.org/x/image/font/gofont/goregular"

	"samuelks.com/demy"
	starlarkplugin "samuelks.com/demy/plugin/starlark"
	"samuelks.com/demy/scene/xmlscene"
)

func ExampleEnvironment_LoadBytes() {
	const script = `
styles = {"title": Style(size=18, weight="bold")}

def render(data):
    rows = [HStack(Text(item["name"]), Spacer(), Text(str(item["qty"]))) for item in data["items"]]
    return VStack(Gap(4), Padding(8),
        Text(shout(data["title"]), Class("title")),
        Divider(),
        Children(rows),
        Text(now().format("Mon Jan 2")),
    )
`
	env := &starlarkplugin.Environment{
		Now: func() time.Time { return time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC) },
	}
	env.AddBuiltin("shout", func(_ *sl.Thread, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
		s, _ := sl.AsString(args[0])
		return sl.String(strings.ToUpper(s)), nil
	})

	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": demy.MustLoadFontBytes("sans", goregular.TTF)}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}

	ctx := context.Background()
	p, err := env.LoadBytes(ctx, "groceries.star", []byte(script), cfg, time.UTC)
	if err != nil {
		log.Fatal(err)
	}
	node, err := p.Render(ctx, map[string]any{
		"title": "Groceries",
		"items": []map[string]any{{"name": "Milk", "qty": 2}, {"name": "Eggs", "qty": 12}},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Script styles must be in Config.Styles for Class references to resolve.
	cfg.Styles = p.Styles()
	img, err := demy.MustNew(cfg).Render(context.Background(), node, demy.ColorModeDefault)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(img.Bounds())

	xml, err := xmlscene.Marshal(node)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(xml))
	// Output:
	// (0,0)-(800,480)
	// <vstack gap="4" padding="8">
	// 	<text class="title">GROCERIES</text>
	// 	<divider></divider>
	// 	<hstack>
	// 		<text>Milk</text>
	// 		<spacer weight="1"></spacer>
	// 		<text>2</text>
	// 	</hstack>
	// 	<hstack>
	// 		<text>Eggs</text>
	// 		<spacer weight="1"></spacer>
	// 		<text>12</text>
	// 	</hstack>
	// 	<text>Thu Oct 1</text>
	// </vstack>
}
