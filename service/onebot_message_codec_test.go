package service

import (
	"strings"
	"testing"
)

func testOneBotCodecHooks() OneBotMessageCodecHooks {
	return OneBotMessageCodecHooks{
		ResolveUserID: func(numericID int64) (string, error) {
			if numericID == 1001 {
				return "user-1", nil
			}
			return "", ErrOneBotMappingNotFound
		},
		ResolveMessageID: func(numericID int64) (string, error) {
			if numericID == 2001 {
				return "msg-1", nil
			}
			return "", ErrOneBotMappingNotFound
		},
		ResolveUserOneBotID: func(userID string) (int64, error) {
			if userID == "user-1" {
				return 1001, nil
			}
			return 0, ErrOneBotMappingNotFound
		},
		ResolveMessageOneBotID: func(messageID string) (int64, error) {
			if messageID == "msg-1" {
				return 2001, nil
			}
			return 0, ErrOneBotMappingNotFound
		},
		ResolveAttachmentURL: func(token string) (string, error) {
			if token == "att-1" {
				return "https://example.com/api/v1/attachment/att-1", nil
			}
			return "", nil
		},
	}
}

func TestDecodeOneBotMessageString(t *testing.T) {
	decoded, err := DecodeOneBotMessageValue(
		"[CQ:reply,id=2001]你好[CQ:at,qq=1001][CQ:image,file=https://img.example/a.png]",
		false,
		testOneBotCodecHooks(),
	)
	if err != nil {
		t.Fatalf("DecodeOneBotMessageValue failed: %v", err)
	}
	if decoded.QuoteID != "msg-1" {
		t.Fatalf("decoded quote id = %q, want %q", decoded.QuoteID, "msg-1")
	}
	if !strings.Contains(decoded.Content, "你好") {
		t.Fatalf("decoded content should keep text, got %q", decoded.Content)
	}
	if !strings.Contains(decoded.Content, `<at id="user-1"`) {
		t.Fatalf("decoded content should contain mapped at tag, got %q", decoded.Content)
	}
	if !strings.Contains(decoded.Content, `<img src="https://img.example/a.png" />`) {
		t.Fatalf("decoded content should contain img tag, got %q", decoded.Content)
	}
}

func TestDecodeOneBotMessageArray(t *testing.T) {
	decoded, err := DecodeOneBotMessageValue([]any{
		map[string]any{"type": "reply", "data": map[string]any{"id": "2001"}},
		map[string]any{"type": "text", "data": map[string]any{"text": "前缀"}},
		map[string]any{"type": "at", "data": map[string]any{"qq": "1001"}},
		map[string]any{"type": "image", "data": map[string]any{"file": "base64://QUJDRA=="}},
	}, false, testOneBotCodecHooks())
	if err != nil {
		t.Fatalf("DecodeOneBotMessageValue array failed: %v", err)
	}
	if decoded.QuoteID != "msg-1" {
		t.Fatalf("decoded quote id = %q, want %q", decoded.QuoteID, "msg-1")
	}
	if !strings.Contains(decoded.Content, "前缀") {
		t.Fatalf("decoded content should keep text, got %q", decoded.Content)
	}
	if !strings.Contains(decoded.Content, `<at id="user-1"`) {
		t.Fatalf("decoded array content should contain mapped at tag, got %q", decoded.Content)
	}
	if !strings.Contains(decoded.Content, `data:;base64,QUJDRA==`) {
		t.Fatalf("decoded array content should convert base64 image to data URL, got %q", decoded.Content)
	}
}

func TestEncodeOneBotMessage(t *testing.T) {
	encoded, err := EncodeOneBotMessage(
		`前缀<at id="user-1" name="测试用户" /><img src="id:att-1" />后缀`,
		"msg-1",
		testOneBotCodecHooks(),
	)
	if err != nil {
		t.Fatalf("EncodeOneBotMessage failed: %v", err)
	}
	if !strings.HasPrefix(encoded, "[CQ:reply,id=2001]") {
		t.Fatalf("encoded message should start with reply cq, got %q", encoded)
	}
	if !strings.Contains(encoded, "[CQ:at,qq=1001,name=测试用户]") {
		t.Fatalf("encoded message should contain at cq, got %q", encoded)
	}
	wantImage := "[CQ:image,file=https://example.com/api/v1/attachment/att-1,url=https://example.com/api/v1/attachment/att-1]"
	if !strings.Contains(encoded, wantImage) {
		t.Fatalf("encoded message should contain image cq, got %q", encoded)
	}
}
