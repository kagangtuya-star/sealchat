package service

import (
	"encoding/json"
	"testing"
)

func TestBuildStateWidgetDataFromContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantN    int // expected number of widgets
		wantOpts [][]string
	}{
		{"plain text no widget", "hello world", 0, nil},
		{"single widget", "[待办|进行中|已完成]", 1, [][]string{{"待办", "进行中", "已完成"}}},
		{"two options", "[是|否]", 1, [][]string{{"是", "否"}}},
		{"multiple widgets", "状态:[待办|完成] 类型:[bug|feat]", 2, [][]string{{"待办", "完成"}, {"bug", "feat"}}},
		{"single option no pipe - not a widget", "[单选项]", 0, nil},
		{"markdown link - skip", "[选项A|选项B](https://example.com)", 0, nil},
		{"widget + markdown link", "[待办|完成] 和 [链接|文本](url)", 1, [][]string{{"待办", "完成"}}},
		{"with satori at tag", `任务 <at id="123" name="test"/> [待办|完成]`, 1, [][]string{{"待办", "完成"}}},
		{"empty content", "", 0, nil},
		{"emoji options", "[⭕|❌|✓]", 1, [][]string{{"⭕", "❌", "✓"}}},
		{"spaces in options", "[ 待办 | 进行中 | 已完成 ]", 1, [][]string{{"待办", "进行中", "已完成"}}},
		{"tiptap rich text", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"状态 "},{"type":"text","marks":[{"type":"bold"}],"text":"[待办|完成]"}]}]}`, 1, [][]string{{"待办", "完成"}}},
		{"tiptap split by marks", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"[待办"},{"type":"text","marks":[{"type":"bold"}],"text":"|进行中"},{"type":"text","text":"|完成]"}]}]}`, 1, [][]string{{"待办", "进行中", "完成"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildStateWidgetDataFromContent(tt.content)

			if tt.wantN == 0 {
				if result != "" {
					t.Errorf("expected empty, got %q", result)
				}
				return
			}

			var entries []StateWidgetEntry
			if err := json.Unmarshal([]byte(result), &entries); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if len(entries) != tt.wantN {
				t.Fatalf("expected %d entries, got %d", tt.wantN, len(entries))
			}
			for i, entry := range entries {
				if entry.Type != "state" {
					t.Errorf("entry[%d].Type = %q, want \"state\"", i, entry.Type)
				}
				if entry.Index != 0 {
					t.Errorf("entry[%d].Index = %d, want 0", i, entry.Index)
				}
				if tt.wantOpts != nil {
					if len(entry.Options) != len(tt.wantOpts[i]) {
						t.Errorf("entry[%d] options len = %d, want %d", i, len(entry.Options), len(tt.wantOpts[i]))
					} else {
						for j, opt := range entry.Options {
							if opt != tt.wantOpts[i][j] {
								t.Errorf("entry[%d].Options[%d] = %q, want %q", i, j, opt, tt.wantOpts[i][j])
							}
						}
					}
				}
			}
		})
	}
}

func TestRotateWidgetIndex(t *testing.T) {
	// Setup: 2 widgets, first with 3 options, second with 2
	initial := `[{"type":"state","options":["A","B","C"],"index":0},{"type":"state","options":["X","Y"],"index":0}]`

	// Rotate first widget
	r1, err := RotateWidgetIndex(initial, 0)
	if err != nil {
		t.Fatal(err)
	}
	var e1 []StateWidgetEntry
	json.Unmarshal([]byte(r1), &e1)
	if e1[0].Index != 1 {
		t.Errorf("after rotate[0]: index = %d, want 1", e1[0].Index)
	}
	if e1[1].Index != 0 {
		t.Errorf("widget[1] should be unchanged: index = %d, want 0", e1[1].Index)
	}

	// Rotate first widget again
	r2, err := RotateWidgetIndex(r1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var e2 []StateWidgetEntry
	json.Unmarshal([]byte(r2), &e2)
	if e2[0].Index != 2 {
		t.Errorf("after 2nd rotate[0]: index = %d, want 2", e2[0].Index)
	}

	// Rotate wraps around (3 options, index 2 → 0)
	r3, err := RotateWidgetIndex(r2, 0)
	if err != nil {
		t.Fatal(err)
	}
	var e3 []StateWidgetEntry
	json.Unmarshal([]byte(r3), &e3)
	if e3[0].Index != 0 {
		t.Errorf("after 3rd rotate[0]: index = %d, want 0 (wrap)", e3[0].Index)
	}

	// Out of range
	_, err = RotateWidgetIndex(initial, 5)
	if err == nil {
		t.Error("expected error for out-of-range index")
	}
	_, err = RotateWidgetIndex(initial, -1)
	if err == nil {
		t.Error("expected error for negative index")
	}

	// Empty widget data
	_, err = RotateWidgetIndex("", 0)
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestBuildStateWidgetDataIdempotent(t *testing.T) {
	content := "状态: [待办|进行中|已完成] 优先级: [高|低]"
	r1 := BuildStateWidgetDataFromContent(content)
	r2 := BuildStateWidgetDataFromContent(content)
	if r1 != r2 {
		t.Errorf("parse not idempotent:\n  r1=%s\n  r2=%s", r1, r2)
	}
}

func TestBuildStateWidgetDataFromContentWithPrevious(t *testing.T) {
	content := "状态: [待办|进行中|已完成] 优先级: [高|低]"
	prev := `[{"type":"state","options":["待办","进行中","已完成"],"index":2},{"type":"state","options":["高","低"],"index":1}]`
	result := BuildStateWidgetDataFromContentWithPrevious(content, prev)
	var entries []StateWidgetEntry
	if err := json.Unmarshal([]byte(result), &entries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Index != 2 {
		t.Fatalf("expected entry[0].Index=2, got %d", entries[0].Index)
	}
	if entries[1].Index != 1 {
		t.Fatalf("expected entry[1].Index=1, got %d", entries[1].Index)
	}
}

func TestBuildStateWidgetDataFromContentWithPreviousFallback(t *testing.T) {
	content := "状态: [待办|完成]"
	prev := `[{"type":"state","options":["A","B"],"index":1}]`
	result := BuildStateWidgetDataFromContentWithPrevious(content, prev)
	var entries []StateWidgetEntry
	if err := json.Unmarshal([]byte(result), &entries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Index != 0 {
		t.Fatalf("expected fallback index=0, got %d", entries[0].Index)
	}
}
