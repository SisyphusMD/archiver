package envelope

import (
	"bytes"
	"testing"
)

func pages(t *testing.T, p *Page) int {
	t.Helper()
	b, err := p.PDF()
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Count(b, []byte("/Type /Page /Parent"))
}

// filled is a page of n one-line paragraphs.
func filled(n int) *Page {
	p := &Page{}
	for range n {
		p.add("p", "A line of the envelope.")
	}
	return p
}

// sheets is how many pages filled(n) takes at spacing k.
func sheets(n int, k float64) int {
	r := &renderer{k: k}
	r.newPage()
	for range n {
		r.element("p", "A line of the envelope.")
	}
	return len(r.pages)
}

// A page a few lines too long tightens its spacing to stay one sheet (the envelope of three
// storages ran one line over); one far too long keeps its spacing and runs on.
func TestEnvelopeFitsOneSheetWhenItCan(t *testing.T) {
	n := 1
	for sheets(n, 1) == 1 {
		n++
	}
	if got := pages(t, filled(n+2)); got != 1 {
		t.Fatalf("%d paragraphs, two lines past a full page: %d pages, want 1", n+2, got)
	}
	if got, want := pages(t, filled(2*n)), sheets(2*n, 1); got != want || got < 2 {
		t.Fatalf("a page twice too long: %d pages, want %d at full spacing", got, want)
	}
}
