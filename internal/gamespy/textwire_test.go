package gamespy

import (
	"strings"
	"testing"
)

func TestCreateGameSpyMessageOrdered(t *testing.T) {
	got := string(CreateGameSpyMessageOrdered([]KV{
		{Key: "otherslist", Value: ""},
		{Key: "o", Value: "2"},
		{Key: "uniquenick", Value: "nick"},
		{Key: "oldone", Value: ""},
	}))
	want := `\otherslist\\o\2\uniquenick\nick\oldone\\final\`
	if got != want {
		t.Fatalf("CreateGameSpyMessageOrdered mismatch:\ngot  %q\nwant %q", got, want)
	}

	// Duplicate keys preserve order.
	dup := string(CreateGameSpyMessageOrdered([]KV{
		{Key: "o", Value: "1"},
		{Key: "o", Value: "2"},
		{Key: "uniquenick", Value: "a"},
		{Key: "uniquenick", Value: "b"},
	}))
	if dup != `\o\1\o\2\uniquenick\a\uniquenick\b\final\` {
		t.Fatalf("duplicate keys not preserved in order: %q", dup)
	}

	// Empty values and trailing final marker.
	if !strings.HasSuffix(got, `\final\`) {
		t.Fatalf("message does not end with \\final\\: %q", got)
	}
}
