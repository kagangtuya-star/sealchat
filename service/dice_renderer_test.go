package service

import (
	"fmt"
	"strings"
	"testing"

	"sealchat/model"
)

func TestRenderDiceContentBasic(t *testing.T) {
	input := "测试 .r1d6 消息"
	result, err := RenderDiceContent(input, "d6", nil)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if result == nil {
		t.Fatalf("nil result")
	}
	if len(result.Rolls) != 1 {
		t.Fatalf("expected 1 roll, got %d", len(result.Rolls))
	}
	if result.Rolls[0].Formula != "1d6" {
		t.Fatalf("unexpected formula: %s", result.Rolls[0].Formula)
	}
	if !strings.Contains(result.Content, "dice-chip") {
		t.Fatalf("dice chip markup missing: %s", result.Content)
	}
}

func TestRenderDiceContentDefaultCompletion(t *testing.T) {
	input := "roll .rd+1"
	result, err := RenderDiceContent(input, "d12", nil)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(result.Rolls) != 1 {
		t.Fatalf("expected 1 roll, got %d", len(result.Rolls))
	}
	if result.Rolls[0].Formula != "d12+1" {
		t.Fatalf("expected formula d12+1, got %s", result.Rolls[0].Formula)
	}
}

func TestRenderDiceContentDefaultDiceDetailFallback(t *testing.T) {
	input := "。rd"
	result, err := RenderDiceContent(input, "d6", nil)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(result.Rolls) != 1 {
		t.Fatalf("expected 1 roll, got %d", len(result.Rolls))
	}
	roll := result.Rolls[0]
	if roll.ResultValueText == "" {
		t.Fatalf("result value missing")
	}
	want := fmt.Sprintf("[%s=%s]", roll.Formula, roll.ResultValueText)
	if roll.ResultDetail != want {
		t.Fatalf("unexpected detail, want %s, got %s", want, roll.ResultDetail)
	}
}

func TestRenderDiceContentBareCommandUsesDefault(t *testing.T) {
	result, err := RenderDiceContent(".r", "d8", nil)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(result.Rolls) != 1 {
		t.Fatalf("expected 1 roll, got %d", len(result.Rolls))
	}
	if result.Rolls[0].Formula != "d8" {
		t.Fatalf("expected default dice formula, got %s", result.Rolls[0].Formula)
	}
}

func TestRenderDiceContentBraceCommandUsesDefault(t *testing.T) {
	result, err := RenderDiceContent("{r}", "d10", nil)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(result.Rolls) != 1 {
		t.Fatalf("expected 1 roll, got %d", len(result.Rolls))
	}
	if result.Rolls[0].Formula != "d10" {
		t.Fatalf("expected default dice formula for brace, got %s", result.Rolls[0].Formula)
	}
}

func TestRenderDiceContentReuse(t *testing.T) {
	existing := []*model.MessageDiceRollModel{
		{
			RollIndex:       0,
			Formula:         "1d6",
			ResultText:      "stub",
			ResultValueText: "5",
			ResultDetail:    "detail",
		},
	}
	input := "依旧 .r1d6"
	result, err := RenderDiceContent(input, "d6", existing)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(result.Rolls) != 1 {
		t.Fatalf("expected 1 roll, got %d", len(result.Rolls))
	}
	if result.Rolls[0].ResultValueText != "5" {
		t.Fatalf("should reuse value, got %s", result.Rolls[0].ResultValueText)
	}
}

func TestRenderDiceContentRichPayload(t *testing.T) {
	input := `{"type":"doc","content":[]}`
	result, err := RenderDiceContent(input, "d6", nil)
	if err != nil {
		t.Fatalf("rich payload render failed: %v", err)
	}
	if result.Content != input {
		t.Fatalf("rich payload should stay unchanged")
	}
	if len(result.Rolls) != 0 {
		t.Fatalf("rich payload should not produce rolls")
	}
}
