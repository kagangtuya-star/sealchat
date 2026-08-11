package service

import "testing"

func TestSerializeMessageContentToCommandTextPreservesLiteralAmpersandCommand(t *testing.T) {
	input := ".st &手枪伤害=1d6+1"
	got, ok := SerializeMessageContentToCommandText(input)
	if !ok {
		t.Fatalf("expected serializer to accept input")
	}
	if got != input {
		t.Fatalf("expected %q, got %q", input, got)
	}
}

func TestSerializeMessageContentToCommandTextDecodesHtmlEntity(t *testing.T) {
	input := ".st &amp;手枪伤害=1d6+1"
	want := ".st &手枪伤害=1d6+1"
	got, ok := SerializeMessageContentToCommandText(input)
	if !ok {
		t.Fatalf("expected serializer to accept input")
	}
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSerializeMessageContentToCommandText_RestoresDiceChipSource(t *testing.T) {
	input := `<span class="dice-chip" data-dice-roll-index="0" data-dice-source=".ra" data-dice-formula="d100"><span class="dice-chip__icon">🎲</span><span class="dice-chip__formula">d100</span><span class="dice-chip__equals">=</span><span class="dice-chip__result">42</span></span> <code>1d100</code> <strong>侦查</strong>`
	got, ok := SerializeMessageContentToCommandText(input)
	if !ok {
		t.Fatalf("expected serializer to succeed")
	}
	want := ".ra `1d100` **侦查**"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSerializeMessageContentToCommandText_RestoresDiceRollGroupSource(t *testing.T) {
	input := `<span class="dice-roll-group" data-dice-source=".r3#d20"><span class="dice-chip" data-dice-source=".r3#d20" data-dice-roll-index="0"></span><span class="dice-chip" data-dice-source=".r3#d20" data-dice-roll-index="1"></span><span class="dice-chip" data-dice-source=".r3#d20" data-dice-roll-index="2"></span></span> <em>检定</em>`
	got, ok := SerializeMessageContentToCommandText(input)
	if !ok {
		t.Fatalf("expected serializer to succeed")
	}
	want := ".r3#d20 *检定*"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSerializeMessageContentToBotCommandTextPreservesMentions(t *testing.T) {
	input := `<p>。ra 力量<at id="Nva7dGShxwcqKa2a" name="ceshi"/></p>`
	got, ok := SerializeMessageContentToBotCommandText(input)
	if !ok {
		t.Fatal("expected command content to serialize")
	}
	want := `。ra 力量 <at id="Nva7dGShxwcqKa2a" name="ceshi"/>`
	if got != want {
		t.Fatalf("serialized bot command = %q, want %q", got, want)
	}

	legacy, ok := SerializeMessageContentToCommandText(input)
	if !ok {
		t.Fatal("expected legacy command content to serialize")
	}
	if legacy != `。ra 力量@ceshi` {
		t.Fatalf("legacy serialized command = %q, want %q", legacy, `。ra 力量@ceshi`)
	}
}

func TestSerializeTipTapBotCommandTextPreservesMentions(t *testing.T) {
	input := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"。ra 力量"},{"type":"satoriMention","attrs":{"id":"Nva7dGShxwcqKa2a","name":"ceshi"}}]}]}`
	got, ok := SerializeMessageContentToBotCommandText(input)
	if !ok {
		t.Fatal("expected TipTap command content to serialize")
	}
	want := `。ra 力量 <at id="Nva7dGShxwcqKa2a" name="ceshi"/>`
	if got != want {
		t.Fatalf("serialized TipTap bot command = %q, want %q", got, want)
	}
}
