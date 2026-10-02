package xmlscene_test

import (
	"bytes"
	"reflect"
	"testing"

	"samuelks.com/demy"
	"samuelks.com/demy/scene/jsonscene"
	"samuelks.com/demy/scene/xmlscene"
)

func addRoundTripSeeds(f *testing.F) {
	f.Add(`<vstack gap="4" align="center"><text size="12" weight="bold-italic" color="#ff000080">hi</text><frame width="10"/></vstack>`)
	f.Add(`<grid><columns><fr weight="1"/><fixed size="20"/><auto/></columns><text>a</text><row><text>b</text></row></grid>`)
	f.Add(`<scene><styles><style name="h" size="3" max-lines="-1"/></styles><box clip="true" background="#abc"><barcode content="x" ec-level="high"/><image key="k" dither="atkinson"/></box></scene>`)
	f.Add(`<zstack align="bottom-trailing"><weighted weight="2"><spacer/></weighted><divider color="muted" size="2"/></zstack>`)
	f.Add(`<hstack><text class="h" line-height="1.5" overflow="ellipsis" max-lines="2"> spaced  text </text></hstack>`)
}

// FuzzRoundTrip checks that any scene that decodes encodes back to XML that
// decodes to the same tree and styles, and that encoding is stable.
func FuzzRoundTrip(f *testing.F) {
	addRoundTripSeeds(f)
	f.Fuzz(func(t *testing.T, src string) {
		n, styles, err := xmlscene.UnmarshalScene([]byte(src), demy.Config{})
		if err != nil {
			return
		}
		x1, err := xmlscene.MarshalScene(n, styles)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		n2, styles2, err := xmlscene.UnmarshalScene(x1, demy.Config{})
		if err != nil {
			t.Fatalf("unmarshal of marshaled scene: %v\n%s", err, x1)
		}
		if !reflect.DeepEqual(n, n2) || !reflect.DeepEqual(styles, styles2) {
			t.Fatalf("round trip changed the scene\nfirst:  %#v %#v\nsecond: %#v %#v\nXML: %s", n, styles, n2, styles2, x1)
		}
		x2, err := xmlscene.MarshalScene(n2, styles2)
		if err != nil {
			t.Fatalf("second marshal: %v", err)
		}
		if !bytes.Equal(x1, x2) {
			t.Fatalf("marshal not stable\n%s\n%s", x1, x2)
		}
	})
}

// FuzzRoundTripJSON checks that any XML scene converts to JSON and back to the
// same tree, styles, and XML.
func FuzzRoundTripJSON(f *testing.F) {
	addRoundTripSeeds(f)
	f.Fuzz(func(t *testing.T, src string) {
		n, styles, err := xmlscene.UnmarshalScene([]byte(src), demy.Config{})
		if err != nil {
			return
		}
		x1, err := xmlscene.MarshalScene(n, styles)
		if err != nil {
			t.Fatalf("XML marshal: %v", err)
		}
		j, err := jsonscene.MarshalScene(n, styles)
		if err != nil {
			t.Fatalf("JSON marshal: %v", err)
		}
		n2, styles2, err := jsonscene.UnmarshalScene(j)
		if err != nil {
			t.Fatalf("JSON unmarshal: %v\n%s", err, j)
		}
		if !reflect.DeepEqual(n, n2) || !reflect.DeepEqual(styles, styles2) {
			t.Fatalf("JSON round trip changed the scene\nXML:  %#v %#v\nJSON: %#v %#v\n%s", n, styles, n2, styles2, j)
		}
		x2, err := xmlscene.MarshalScene(n2, styles2)
		if err != nil {
			t.Fatalf("XML marshal after JSON: %v", err)
		}
		if !bytes.Equal(x1, x2) {
			t.Fatalf("XML differs after JSON round trip\n%s\n%s", x1, x2)
		}
	})
}
