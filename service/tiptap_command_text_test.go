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
