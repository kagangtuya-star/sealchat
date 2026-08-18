package api

import (
	"strings"
	"testing"
)

func TestNormalizeCharacterRemarkContent(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantContent string
		wantClear   bool
		wantErr     string
	}{
		{
			name:        "trim surrounding whitespace",
			input:       "  前排侦察  ",
			wantContent: "前排侦察",
			wantClear:   false,
		},
		{
			name:        "blank content becomes clear",
			input:       "   \n\t  ",
			wantContent: "",
			wantClear:   true,
		},
		{
			name:    "over limit returns error",
			input:   strings.Repeat("角", 81),
			wantErr: "角色备注长度需在80个字符以内",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotContent, gotClear, err := normalizeCharacterRemarkContent(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotContent != tt.wantContent {
				t.Fatalf("expected content %q, got %q", tt.wantContent, gotContent)
			}
			if gotClear != tt.wantClear {
				t.Fatalf("expected clear=%v, got %v", tt.wantClear, gotClear)
			}
		})
	}
}
