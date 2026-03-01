package api

import "testing"

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
