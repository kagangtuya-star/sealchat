package api

import (
	"fmt"
	"testing"
)

func TestNormalizeExportColorMap(t *testing.T) {
	t.Parallel()

	result, err := normalizeExportColorMap(map[string]string{
		"identity:role-a": "ABC",
		"identity:role-b": "#123abc",
		"identity:":       "#112233",
		"identity:role-c": "bad-color",
		"user:role-d":     "#445566",
	})
	if err != nil {
		t.Fatalf("normalizeExportColorMap failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("unexpected normalized size: %d", len(result))
	}
	if got := result["identity:role-a"]; got != "#aabbcc" {
		t.Fatalf("unexpected normalized color for role-a: %q", got)
	}
	if got := result["identity:role-b"]; got != "#123abc" {
		t.Fatalf("unexpected normalized color for role-b: %q", got)
	}
}

func TestNormalizeExportColorMapTooManyEntries(t *testing.T) {
	t.Parallel()

	input := make(map[string]string, exportColorProfileMaxEntries+1)
	for i := 0; i < exportColorProfileMaxEntries+1; i++ {
		input[fmt.Sprintf("identity:role-%d", i)] = "#112233"
	}

	_, err := normalizeExportColorMap(input)
	if err == nil {
		t.Fatal("expected error for too many color entries, got nil")
	}
}

func TestNormalizeHexColor(t *testing.T) {
	t.Parallel()

	if got, ok := normalizeHexColor("#abc"); !ok || got != "#aabbcc" {
		t.Fatalf("expected #aabbcc, got %q, ok=%v", got, ok)
	}
	if got, ok := normalizeHexColor("ABCDEF"); !ok || got != "#abcdef" {
		t.Fatalf("expected #abcdef, got %q, ok=%v", got, ok)
	}
	if got, ok := normalizeHexColor("xyz"); ok || got != "" {
		t.Fatalf("expected invalid color, got %q, ok=%v", got, ok)
	}
}
