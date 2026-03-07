package api

import (
	"encoding/json"
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

func TestNormalizeExportColorProfiles(t *testing.T) {
	t.Parallel()

	result, err := normalizeExportColorProfiles(map[string]exportColorProfileEntry{
		"identity:role-a": {Color: "ABC", Name: "  自定义A  ", OriginalName: " 原始A "},
		"identity:role-b": {Name: "仅改名", OriginalName: "原始B"},
		"identity:role-c": {OriginalName: "原始C"},
		"user:role-d":     {Color: "#445566", Name: "忽略"},
	})
	if err != nil {
		t.Fatalf("normalizeExportColorProfiles failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("unexpected normalized profile size: %d", len(result))
	}
	if got := result["identity:role-a"]; got.Color != "#aabbcc" || got.Name != "自定义A" || got.OriginalName != "原始A" {
		t.Fatalf("unexpected normalized profile for role-a: %#v", got)
	}
	if got := result["identity:role-b"]; got.Color != "" || got.Name != "仅改名" || got.OriginalName != "原始B" {
		t.Fatalf("unexpected normalized profile for role-b: %#v", got)
	}
}

func TestParseExportColorProfileJSONSupportsLegacyColorMap(t *testing.T) {
	t.Parallel()

	profiles := parseExportColorProfileJSON(`{"identity:role-a":"#123abc"}`)
	if got := profiles["identity:role-a"]; got.Color != "#123abc" || got.Name != "" {
		t.Fatalf("unexpected legacy profile parse result: %#v", got)
	}
}

func TestParseExportColorProfileJSONSupportsStructuredProfiles(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(exportColorProfileDocument{
		Version: exportColorProfileFormatVersion,
		Profiles: map[string]exportColorProfileEntry{
			"identity:role-a": {Color: "#123abc", Name: "别名A", OriginalName: "原名A"},
		},
	})
	if err != nil {
		t.Fatalf("marshal structured profile failed: %v", err)
	}
	profiles := parseExportColorProfileJSON(string(raw))
	if got := profiles["identity:role-a"]; got.Color != "#123abc" || got.Name != "别名A" || got.OriginalName != "原名A" {
		t.Fatalf("unexpected structured profile parse result: %#v", got)
	}
}

func TestNormalizeExportProfileMatchName(t *testing.T) {
	t.Parallel()

	got := normalizeExportProfileMatchName("  Alice   The   Brave  ")
	if got != "alice the brave" {
		t.Fatalf("unexpected normalized match name: %q", got)
	}
}
