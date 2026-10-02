package xmlscene_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/xmlscene"
)

const listTemplate = `<vstack gap="4">
  <text>{{.Title}}</text>
  {{range .Items}}<hstack><text>{{.Name}}</text><spacer/><text>{{.Qty}}</text></hstack>
  {{end}}
</vstack>`

func listData(n int) map[string]any {
	items := make([]map[string]any, n)
	for i := range items {
		items[i] = map[string]any{"Name": fmt.Sprintf("item %d", i), "Qty": i}
	}
	return map[string]any{"Title": fmt.Sprintf("List <%d>", n), "Items": items}
}

func TestTemplateReuse(t *testing.T) {
	tmpl, err := xmlscene.ParseTemplate([]byte(listTemplate))
	if err != nil {
		t.Fatal(err)
	}
	for n := range 4 {
		got, err := tmpl.Execute(t.Context(), listData(n), demy.Config{})
		if err != nil {
			t.Fatalf("%d: Template.Execute: %v", n, err)
		}
		want, err := xmlscene.Execute(t.Context(), []byte(listTemplate), listData(n), demy.Config{})
		if err != nil {
			t.Fatalf("%d: Execute: %v", n, err)
		}
		gotXML, err := xmlscene.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		wantXML, err := xmlscene.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		if string(gotXML) != string(wantXML) {
			t.Errorf("%d: got\n%s\nwant\n%s", n, gotXML, wantXML)
		}
	}
}

func TestParseTemplateError(t *testing.T) {
	if _, err := xmlscene.ParseTemplate([]byte(`{{.Unclosed`)); err == nil {
		t.Fatal("ParseTemplate succeeded, want error")
	}
}

// TestTemplateConcurrent checks that each execution of a shared Template uses
// its own context.
func TestTemplateConcurrent(t *testing.T) {
	tmpl := xmlscene.MustParseTemplate([]byte(`{{range .}}{{end}}<spacer/>`))
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			ctx := t.Context()
			if i == 0 {
				ctx = canceled
			}
			_, err := tmpl.Execute(ctx, make([]int, 1000), demy.Config{})
			switch {
			case i == 0 && !errors.Is(err, context.Canceled):
				t.Errorf("canceled execution: err = %v, want context.Canceled", err)
			case i != 0 && err != nil:
				t.Errorf("execution %d: %v", i, err)
			}
		})
	}
	wg.Wait()
}

// TestTemplateDeadline checks that a reused Template still stops silent loops
// and recursion at the deadline.
func TestTemplateDeadline(t *testing.T) {
	tests := map[string]struct {
		tmpl string
		data any
	}{
		"silent range": {`{{range .}}{{end}}<spacer/>`, 1000000000000},
		// Each call makes two more, 2^40 in all.
		"recursion": {`{{define "a"}}{{if .}}{{template "a" (slice . 1)}}{{template "a" (slice . 1)}}{{end}}{{end}}{{template "a" .}}<spacer/>`, make([]int, 40)},
	}
	for name, tt := range tests {
		tmpl := xmlscene.MustParseTemplate([]byte(tt.tmpl))
		// A quick, successful execution first.
		var small any = 1
		if _, ok := tt.data.([]int); ok {
			small = make([]int, 2)
		}
		if _, err := tmpl.Execute(t.Context(), small, demy.Config{}); err != nil {
			t.Fatalf("%s: first execution: %v", name, err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		start := time.Now()
		_, err := tmpl.Execute(ctx, tt.data, demy.Config{})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("%s: err = %v, want context.DeadlineExceeded", name, err)
		}
		if d := time.Since(start); d > 500*time.Millisecond {
			t.Errorf("%s: took %v after a 20ms deadline", name, d)
		}
	}
}

func BenchmarkExecute(b *testing.B) {
	data := listData(20)
	for b.Loop() {
		if _, err := xmlscene.Execute(b.Context(), []byte(listTemplate), data, demy.Config{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTemplateExecute(b *testing.B) {
	tmpl := xmlscene.MustParseTemplate([]byte(listTemplate))
	data := listData(20)
	for b.Loop() {
		if _, err := tmpl.Execute(b.Context(), data, demy.Config{}); err != nil {
			b.Fatal(err)
		}
	}
}
