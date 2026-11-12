package service

import (
	"testing"
	"time"

	"sealchat/model"
)

func TestChatImportCustomCaptureGroups(t *testing.T) {
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	const positional = `^([^|]*)\|([^|]*)\|(.*)$`
	const qqPattern = `^(.+?)\((?:\d+)\)\s+(\d{4}/\d{2}/\d{2}\s+\d{2}:\d{2}:\d{2})\s*(.*)$`
	const namedQQPattern = `^(?P<role>.+?)\((?:\d+)\)\s+(?P<time>\d{4}/\d{2}/\d{2}\s+\d{2}:\d{2}:\d{2})\s*(?P<content>.*)$`
	for _, tc := range []struct {
		name, pattern, input, role, content, timestamp string
		mergeUnmatched                                 bool
	}{
		{
			name: "legacy time role content", pattern: positional,
			input: "2025-12-17 20:04:45|古德|麻雀虽小，五脏俱全",
			role:  "古德", content: "麻雀虽小，五脏俱全", timestamp: "2025-12-17T20:04:45Z",
		},
		{
			name: "issue 305 role time content", pattern: qqPattern,
			input: "古德(1438123508) 2025/12/17 20:04:45 麻雀虽小，五脏俱全",
			role:  "古德", content: "麻雀虽小，五脏俱全", timestamp: "2025-12-17T20:04:45Z",
		},
		{
			name: "issue 305 multiline", pattern: qqPattern, mergeUnmatched: true,
			input: "古德(1438123508) 2025/12/17 20:04:45\n麻雀虽小，五脏俱全\n后续正文",
			role:  "古德", content: "\n麻雀虽小，五脏俱全\n后续正文", timestamp: "2025-12-17T20:04:45Z",
		},
		{
			name: "issue 305 named multiline", pattern: namedQQPattern, mergeUnmatched: true,
			input: "古德(1438123508) 2025/12/17 20:04:45\n麻雀虽小，五脏俱全",
			role:  "古德", content: "\n麻雀虽小，五脏俱全", timestamp: "2025-12-17T20:04:45Z",
		},
		{
			name: "named content time role with extra group", pattern: `^(ignored)\|(?P<content>[^|]*)\|(?P<time>[^|]*)\|(?P<role>.*)$`,
			input: "ignored|麻雀虽小，五脏俱全|2025/12/17 20:04:45|古德",
			role:  "古德", content: "麻雀虽小，五脏俱全", timestamp: "2025-12-17T20:04:45Z",
		},
		{
			name: "named groups override ambiguous timestamps", pattern: `^(?P<content>[^|]*)\|(?P<role>[^|]*)\|(?P<time>.*)$`,
			input: "20:04:45|古德|2025/12/17 20:04:45",
			role:  "古德", content: "20:04:45", timestamp: "2025-12-17T20:04:45Z",
		},
		{
			name: "named empty time never falls back", pattern: `^(?P<time>[^|]*)\|(?P<content>[^|]*)\|(?P<role>.*)$`,
			input: "|2025/12/17 20:04:45|古德", role: "古德", content: "2025/12/17 20:04:45",
		},
		{
			name: "named empty content never falls back", pattern: `^(?P<content>[^|]*)\|(?P<role>[^|]*)\|(?P<time>.*)$`,
			input: "|古德|2025/12/17 20:04:45", role: "古德", timestamp: "2025-12-17T20:04:45Z",
		},
		{
			name: "named without time", pattern: `^(?P<content>[^|]*)\|(?P<role>.*)$`,
			input: "麻雀虽小，五脏俱全|古德", role: "古德", content: "麻雀虽小，五脏俱全",
		},
		{
			name: "partial named group keeps positional compatibility", pattern: `^(?P<role>[^|]*)\|(.*)$`,
			input: "古德|麻雀虽小，五脏俱全", role: "古德", content: "麻雀虽小，五脏俱全",
		},
		{
			name: "legacy extra non-time group", pattern: positional,
			input: "古德|正文|额外捕获", role: "古德", content: "正文",
		},
		{
			name: "later time-like helper group stays positional", pattern: `^([^|]*)\|([^|]*)\|([^|]*)\|(.*)$`,
			input: "古德|正文|额外捕获|2025/12/17 20:04:45", role: "古德", content: "正文",
		},
		{
			name: "ambiguous timestamps keep legacy time first", pattern: positional,
			input: "2025/12/17 20:04:45|20:04:45|正文",
			role:  "20:04:45", content: "正文", timestamp: "2025-12-17T20:04:45Z",
		},
		{
			name: "second group time wins even if content looks like time", pattern: positional,
			input: "古德|2025/12/17 20:04:45|20:04:45",
			role:  "古德", content: "20:04:45", timestamp: "2025-12-17T20:04:45Z",
		},
		{
			name: "invalid timestamp uses role content fallback", pattern: positional,
			input: "2025-99-99 20:04:45|正文|额外捕获", role: "2025-99-99 20:04:45", content: "正文",
		},
		{
			name: "minute timestamp uses existing parser", pattern: positional,
			input: "古德|20:04|正文", role: "古德", content: "正文", timestamp: "2026-11-05T20:04:00Z",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := newChatLogParserAt(&model.ChatImportConfig{
				RegexPattern: tc.pattern, MergeUnmatched: tc.mergeUnmatched,
			}, now)
			if err != nil {
				t.Fatal(err)
			}
			entries, _, skipped := p.ParseLogContent(tc.input)
			if len(entries) != 1 || skipped != 0 {
				t.Fatalf("entries=%d skipped=%d", len(entries), skipped)
			}
			entry := entries[0]
			wantTime := tc.timestamp
			if wantTime == "" {
				wantTime = now.Format(time.RFC3339Nano)
			}
			if entry.RoleName != tc.role || entry.Content != tc.content || entry.RawLine != tc.input ||
				entry.Timestamp == nil || entry.Timestamp.UTC().Format(time.RFC3339Nano) != wantTime {
				t.Fatalf("got=%+v, want role=%q content=%q timestamp=%s", entry, tc.role, tc.content, wantTime)
			}
		})
	}
}

func TestChatImportNamedEmptyRole(t *testing.T) {
	p, err := NewChatLogParser(&model.ChatImportConfig{RegexPattern: `^(?P<time>[^|]*)\|(?P<role>[^|]*)\|(?P<content>.*)$`})
	if err != nil {
		t.Fatal(err)
	}
	entries, _, skipped := p.ParseLogContent("2025/12/17 20:04:45||正文")
	if len(entries) != 0 || skipped != 1 {
		t.Fatalf("empty named role fell back to positional groups: entries=%+v skipped=%d", entries, skipped)
	}
}

func TestChatImportCustomAutomaticTimeline(t *testing.T) {
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	for _, explicit := range []bool{false, true} {
		config := &model.ChatImportConfig{RegexPattern: `^<([^>]+)>(.*)$`, TimeIncrement: 2000}
		if explicit {
			config.BaseTime = &now
		}
		p, err := newChatLogParserAt(config, now)
		if err != nil {
			t.Fatal(err)
		}
		entries, _, skipped := p.ParseLogContent("<古德>first\n<古德>last")
		if len(entries) != 2 || skipped != 0 {
			t.Fatalf("entries=%d skipped=%d", len(entries), skipped)
		}
		for i, entry := range entries {
			step := i - 1
			if explicit {
				step = i
			}
			want := now.Add(time.Duration(step) * 2 * time.Second)
			if entry.RoleName != "古德" || entry.Content != []string{"first", "last"}[i] ||
				entry.Timestamp == nil || !entry.Timestamp.Equal(want) {
				t.Fatalf("explicit=%v entry=%d got=%+v want=%v", explicit, i, entry, want)
			}
		}
	}
}

func TestChatImportCustomMixedTimeline(t *testing.T) {
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	for _, pattern := range []string{
		`^(?:\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\])?<([^>]+)>(.*)$`,
		`^(?:\[(?P<time>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\])?<(?P<role>[^>]+)>(?P<content>.*)$`,
	} {
		p, err := newChatLogParserAt(&model.ChatImportConfig{RegexPattern: pattern, TimeIncrement: 2000}, now)
		if err != nil {
			t.Fatal(err)
		}
		entries, _, skipped := p.ParseLogContent("<古德>first\n[2025-12-17 20:04:45]<古德>original\n<古德>last")
		if len(entries) != 3 || skipped != 0 {
			t.Fatalf("pattern=%q entries=%d skipped=%d", pattern, len(entries), skipped)
		}
		wantTimes := []time.Time{now.Add(-2 * time.Second), time.Date(2025, 12, 17, 20, 4, 45, 0, time.UTC), now}
		for i, entry := range entries {
			if entry.RoleName != "古德" || entry.Content != []string{"first", "original", "last"}[i] ||
				entry.Timestamp == nil || !entry.Timestamp.Equal(wantTimes[i]) {
				t.Fatalf("pattern=%q entry=%d got=%+v want=%v", pattern, i, entry, wantTimes[i])
			}
		}
	}
}

func TestChatImportParseSlashTimestamp(t *testing.T) {
	p, err := NewChatLogParser(&model.ChatImportConfig{TimeZone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	got := p.parseTimestamp("2025/12/17 20:04:45")
	if got == nil || !got.Equal(time.Date(2025, 12, 17, 12, 4, 45, 0, time.UTC)) {
		t.Fatalf("slash timestamp: got %v", got)
	}
}
