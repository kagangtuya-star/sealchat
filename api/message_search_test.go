package api

import (
	"strings"
	"testing"
)

func TestShouldForceLikeFallback(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		keyword string
		mode    string
		expect  bool
	}{
		{
			name:    "fuzzy with chinese",
			keyword: "测试",
			mode:    "fuzzy",
			expect:  true,
		},
		{
			name:    "fuzzy with mixed cjk and ascii",
			keyword: "abc测试123",
			mode:    "fuzzy",
			expect:  true,
		},
		{
			name:    "exact with chinese",
			keyword: "测试",
			mode:    "exact",
			expect:  false,
		},
		{
			name:    "fuzzy with ascii only",
			keyword: "hello world",
			mode:    "fuzzy",
			expect:  false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldForceLikeFallback(tc.keyword, tc.mode)
			if got != tc.expect {
				t.Fatalf("shouldForceLikeFallback(%q, %q) = %v, want %v", tc.keyword, tc.mode, got, tc.expect)
			}
		})
	}
}

func TestBuildSnippetNormalizesRichContentToPlainText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		limit    int
		expected string
	}{
		{
			name:     "html with br tag",
			input:    `<p>第一行<br />第二行</p>`,
			limit:    280,
			expected: "第一行 第二行",
		},
		{
			name: "tiptap json",
			input: `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"状态 "},{"type":"text","marks":[{"type":"bold"}],"text":"完成"}]},{"type":"paragraph","content":[{"type":"text","text":"继续"}]}]}`,
			limit: 280,
			expected: "状态 完成 继续",
		},
		{
			name:     "at tag",
			input:    `<at id="all" name="全体成员" />` + "\n" + `开始`,
			limit:    280,
			expected: "@全体成员 开始",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildSnippet(tc.input, tc.limit)
			if got != tc.expected {
				t.Fatalf("buildSnippet() = %q, want %q", got, tc.expected)
			}
			if strings.ContainsAny(got, "<>{}") {
				t.Fatalf("buildSnippet() should return plain text, got %q", got)
			}
		})
	}
}

func TestBuildSnippetTruncatesByRuneLength(t *testing.T) {
	t.Parallel()

	got := buildSnippet("你好世界和平", 4)
	want := "你好世界…"
	if got != want {
		t.Fatalf("buildSnippet() = %q, want %q", got, want)
	}
}
