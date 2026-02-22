package protocol

import (
	"strings"
	"testing"
)

func TestElementToStringSelfClosingTags(t *testing.T) {
	root := ElementParse("甲<br />乙")
	if root == nil {
		t.Fatal("expected parsed root")
	}
	got := root.ToString()
	if strings.Contains(got, "</br>") {
		t.Fatalf("unexpected closing br tag: %q", got)
	}
	if !strings.Contains(got, "<br />") {
		t.Fatalf("expected self-closing br tag, got: %q", got)
	}
}

func TestNormalizeNestedEntitiesMultiRound(t *testing.T) {
	input := "&amp;amp;quot; 和 &amp;amp;"
	got := normalizeNestedEntities(input)
	if got != "&quot; 和 &amp;" {
		t.Fatalf("normalizeNestedEntities(%q) = %q", input, got)
	}
}
