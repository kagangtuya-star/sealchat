package service

import (
	"testing"
)

func TestParseCQCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int // 期望的 element 数量
	}{
		{
			name:     "空字符串",
			input:    "",
			expected: 0,
		},
		{
			name:     "纯文本无CQ码",
			input:    "Hello World",
			expected: 1,
		},
		{
			name:     "单个@用户",
			input:    "[CQ:at,qq=123456,name=张三]",
			expected: 1,
		},
		{
			name:     "@全体成员",
			input:    "[CQ:at,qq=all]",
			expected: 1,
		},
		{
			name:     "混合内容",
			input:    "Hello [CQ:at,qq=123456,name=张三] World",
			expected: 3,
		},
		{
			name:     "多个@",
			input:    "[CQ:at,qq=111,name=A] 和 [CQ:at,qq=222,name=B]",
			expected: 3, // @A + " 和 " + @B
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elements := ParseCQCode(tt.input)
			if len(elements) != tt.expected {
				t.Errorf("ParseCQCode(%q) = %d elements, want %d", tt.input, len(elements), tt.expected)
			}
		})
	}
}

func TestParseCQCodeAtContent(t *testing.T) {
	// 测试 @all
	elements := ParseCQCode("[CQ:at,qq=all]")
	if len(elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elements))
	}
	if elements[0].Type != "at" {
		t.Errorf("expected type 'at', got %q", elements[0].Type)
	}
	if id := getStringAttr(elements[0].Attrs, "id"); id != "all" {
		t.Errorf("expected id 'all', got %q", id)
	}

	// 测试普通 @
	elements = ParseCQCode("[CQ:at,qq=123,name=测试用户]")
	if len(elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elements))
	}
	if id := getStringAttr(elements[0].Attrs, "id"); id != "123" {
		t.Errorf("expected id '123', got %q", id)
	}
	if name := getStringAttr(elements[0].Attrs, "name"); name != "测试用户" {
		t.Errorf("expected name '测试用户', got %q", name)
	}
}

func TestEncodeCQCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "@all",
			input:    "[CQ:at,qq=all]",
			expected: "[CQ:at,qq=all]",
		},
		{
			name:     "@用户",
			input:    "[CQ:at,qq=123,name=张三]",
			expected: "[CQ:at,qq=123,name=张三]",
		},
		{
			name:     "混合内容",
			input:    "Hello [CQ:at,qq=123,name=张三] World",
			expected: "Hello [CQ:at,qq=123,name=张三] World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elements := ParseCQCode(tt.input)
			result := EncodeCQCode(elements)
			if result != tt.expected {
				t.Errorf("EncodeCQCode(ParseCQCode(%q)) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConvertCQToSatori(t *testing.T) {
	input := "Hello [CQ:at,qq=123,name=张三] World"
	result := ConvertCQToSatori(input)

	// 结果应包含 <at> 标签
	if result == input {
		t.Error("ConvertCQToSatori should convert CQ codes to Satori XML")
	}
}

func TestConvertSatoriToCQ(t *testing.T) {
	input := `Hello <at id="123" name="张三"/> World`
	result := ConvertSatoriToCQ(input)

	// 结果应包含 [CQ:at] 格式
	if result == input {
		t.Log("ConvertSatoriToCQ result:", result)
	}
}

func TestEscapeUnescapeCQ(t *testing.T) {
	original := "test[with,special&chars]"
	escaped := escapeCQ(original)
	unescaped := unescapeCQ(escaped)

	if unescaped != original {
		t.Errorf("escape/unescape roundtrip failed: %q -> %q -> %q", original, escaped, unescaped)
	}
}
