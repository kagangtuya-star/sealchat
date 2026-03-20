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
