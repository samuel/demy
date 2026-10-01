package jsonscene_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/samuel/demy/scene/jsonscene"
)

// FuzzRoundTrip checks that any scene that decodes encodes back to JSON that
// decodes to the same tree and styles, and that encoding is stable.
func FuzzRoundTrip(f *testing.F) {
	f.Add(`{"root":{"type":"vstack","gap":4,"align":"center","children":[{"type":"text","text":"hi","style":{"size":12,"weight":"bold-italic","color":"#ff000080"}},{"type":"frame","width":10}]}}`)
	f.Add(`{"root":{"type":"grid","columns":[{"kind":"fr","weight":1},{"kind":"fixed","size":20},{"kind":"auto"}],"children":[{"type":"text","text":"a"},{"type":"row","children":[{"type":"text","text":"b"}]}]}}`)
	f.Add(`{"styles":{"h":{"size":3,"max_lines":-1}},"root":{"type":"box","clip":true,"background":"#abc","children":[{"type":"barcode","content":"x","ec_level":"high"},{"type":"image","image_key":"k","dither":"atkinson"}]}}`)
	f.Add(`{"root":{"type":"zstack","align":"bottom-trailing","children":[{"type":"weighted","weight":2,"children":[{"type":"spacer"}]},{"type":"divider","color":"muted","size":2}]}}`)
	f.Fuzz(func(t *testing.T, src string) {
		n, styles, err := jsonscene.UnmarshalScene([]byte(src))
		if err != nil {
			return
		}
		j1, err := jsonscene.MarshalScene(n, styles)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		n2, styles2, err := jsonscene.UnmarshalScene(j1)
		if err != nil {
			t.Fatalf("unmarshal of marshaled scene: %v\n%s", err, j1)
		}
		if !reflect.DeepEqual(n, n2) || !reflect.DeepEqual(styles, styles2) {
			t.Fatalf("round trip changed the scene\nfirst:  %#v %#v\nsecond: %#v %#v\nJSON: %s", n, styles, n2, styles2, j1)
		}
		j2, err := jsonscene.MarshalScene(n2, styles2)
		if err != nil {
			t.Fatalf("second marshal: %v", err)
		}
		if !bytes.Equal(j1, j2) {
			t.Fatalf("marshal not stable\n%s\n%s", j1, j2)
		}
	})
}
