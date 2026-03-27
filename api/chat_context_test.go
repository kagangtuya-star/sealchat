package api

import (
	"testing"

	"sealchat/protocol"
)

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

func TestNormalizeBotCommandContentWithPrefixes_ConvertsDiceChipHTMLCommand(t *testing.T) {
	input := `<span class="dice-chip" data-dice-roll-index="0" data-dice-source=".ra"><span class="dice-chip__formula">d100</span><span class="dice-chip__equals">=</span><span class="dice-chip__result">42</span></span> <code>1d100</code> <strong>侦查</strong>`
	got := normalizeBotCommandContentWithPrefixes(input, []string{".", "。"})
	want := ".ra `1d100` **侦查**"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeEventForBot_EscapesPlainTextAmpersandCommand(t *testing.T) {
	event := &protocol.Event{
		Type: protocol.EventMessageCreated,
		Message: &protocol.Message{
			Content: ".st &手枪伤害=1d6+1",
		},
	}

	got := normalizeEventForBot(event)
	if got == event {
		t.Fatalf("expected cloned event when content changes")
	}
	if got.Message == nil {
		t.Fatalf("expected message to be preserved")
	}
	want := ".st &amp;手枪伤害=1d6+1"
	if got.Message.Content != want {
		t.Fatalf("expected %q, got %q", want, got.Message.Content)
	}
}
