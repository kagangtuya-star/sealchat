package model

import (
	"fmt"
	"sort"
	"testing"

	"sealchat/utils"
)

func initMessageWhisperRecipientTestDB(t *testing.T) {
	t.Helper()
	cfg := &utils.AppConfig{
		DSN: fmt.Sprintf("file:model-message-whisper-recipient-%s?mode=memory&cache=shared", utils.NewID()),
		SQLite: utils.SQLiteConfig{
			EnableWAL:       false,
			TxLockImmediate: false,
			ReadConnections: 1,
			OptimizeOnInit:  false,
		},
	}
	DBInit(cfg)
}

func sortedStrings(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}

func TestReplaceWhisperRecipientsReplacesExistingRecipients(t *testing.T) {
	initMessageWhisperRecipientTestDB(t)

	messageID := "msg-" + utils.NewIDWithLength(8)
	if err := CreateWhisperRecipients(messageID, []string{"u1", "u2"}); err != nil {
		t.Fatalf("seed recipients failed: %v", err)
	}

	if err := ReplaceWhisperRecipients(messageID, []string{"u2", "u3", "u3", ""}); err != nil {
		t.Fatalf("replace recipients failed: %v", err)
	}

	got := sortedStrings(GetWhisperRecipientIDs(messageID))
	want := []string{"u2", "u3"}
	if len(got) != len(want) {
		t.Fatalf("recipient count = %d, want %d; got=%v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("recipient[%d] = %q, want %q (all=%v)", index, got[index], want[index], got)
		}
	}
}

func TestReplaceWhisperRecipientsClearsRecipientsWhenEmpty(t *testing.T) {
	initMessageWhisperRecipientTestDB(t)

	messageID := "msg-" + utils.NewIDWithLength(8)
	if err := CreateWhisperRecipients(messageID, []string{"u1", "u2"}); err != nil {
		t.Fatalf("seed recipients failed: %v", err)
	}

	if err := ReplaceWhisperRecipients(messageID, nil); err != nil {
		t.Fatalf("clear recipients failed: %v", err)
	}

	got := GetWhisperRecipientIDs(messageID)
	if len(got) != 0 {
		t.Fatalf("recipient count = %d, want 0; got=%v", len(got), got)
	}
}
