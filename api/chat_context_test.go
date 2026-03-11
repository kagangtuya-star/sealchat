package api

import "testing"

func TestNormalizeBotCommandContentWithPrefixes_ConvertsTipTapCommand(t *testing.T) {
	input := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":".ra "},{"type":"text","marks":[{"type":"italic"}],"text":"侦查"},{"type":"text","text":" "},{"type":"text","marks":[{"type":"code"}],"text":"1d100"}]}]}`
	got := normalizeBotCommandContentWithPrefixes(input, []string{".", "。"})
	want := ".ra *侦查* `1d100`"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeBotCommandContentWithPrefixes_SupportsCustomPrefix(t *testing.T) {
	input := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"/bot "},{"type":"text","marks":[{"type":"bold"}],"text":"help"}]}]}`
	got := normalizeBotCommandContentWithPrefixes(input, []string{"/"})
	want := "/bot **help**"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeBotCommandContentWithPrefixes_LeavesNonCommandRichTextUntouched(t *testing.T) {
	input := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"普通消息 "},{"type":"text","marks":[{"type":"italic"}],"text":"不会变"}]}]}`
	got := normalizeBotCommandContentWithPrefixes(input, []string{".", "。"})
	if got != input {
		t.Fatalf("expected original content, got %q", got)
	}
}

func TestNormalizeBotCommandContentWithPrefixes_ConvertsLegacyHTMLCommand(t *testing.T) {
	input := `.st运动<em>*3 特技</em><code>+1</code>`
	got := normalizeBotCommandContentWithPrefixes(input, []string{".", "。"})
	want := `.st运动**3 特技*` + "`+1`"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
