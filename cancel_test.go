package demy_test

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/samuel/demy"
)

// countdownCtx is a context that is canceled from the nth call to Err on.
type countdownCtx struct {
	context.Context
	n int
}

func (c *countdownCtx) Err() error {
	if c.n--; c.n < 0 {
		return context.Canceled
	}
	return nil
}

// TestRenderCancelEveryCheck cancels a render at each point where it checks
// its context and checks that it stops with the context's error.
func TestRenderCancelEveryCheck(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 8, 8))
	root := demy.VStack(demy.Gap(2),
		demy.HStack(demy.Text("left"), demy.Spacer(), demy.Weighted(1, demy.Text("right"))),
		demy.Grid(demy.Columns(demy.GridAuto(), demy.GridFr(1)), demy.Rows(demy.GridFr(1)),
			demy.Text("a"), demy.Box(demy.Clip, demy.Text("b"))),
		demy.ZStack(demy.Frame(10, 10, demy.Image(img, demy.FloydSteinberg)), demy.Barcode("x")),
	)
	c := demy.MustNew(textConfig(200, 120))
	var checks int
	for n := 0; ; n++ {
		ctx := &countdownCtx{Context: t.Context(), n: n}
		_, err := c.Render(ctx, root, demy.TwoBit)
		if err == nil {
			checks = n
			break
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled after %d checks: err = %v, want context.Canceled", n, err)
		}
	}
	if checks < 20 {
		t.Errorf("render checked its context %d times, want it checked throughout", checks)
	}
}

// TestRenderDeadline checks that a render past its deadline stops promptly.
func TestRenderDeadline(t *testing.T) {
	var texts []demy.Node
	for range 16000 {
		texts = append(texts, demy.Text("hi"))
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := demy.MustNew(textConfig(800, 480)).Render(ctx, demy.ZStack(demy.Children(texts...)), demy.TwoBit)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("render took %v after a 20ms deadline", d)
	}
	if !strings.Contains(err.Error(), "render stopped") {
		t.Errorf("err = %v, want it to say the render stopped", err)
	}
}
