package service

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestUploadCompressedExportLogFallback(t *testing.T) {
	var failedCalls int32
	var successCalls int32

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.String() {
			case "https://primary.example.com/dice/api/log":
				atomic.AddInt32(&failedCalls, 1)
				return newHTTPResponse(http.StatusServiceUnavailable, "temporary down"), nil
			case "https://backup.example.com/dice/api/log":
				atomic.AddInt32(&successCalls, 1)
				if r.Method != http.MethodPut {
					t.Fatalf("unexpected method: %s", r.Method)
				}
				if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
					t.Fatalf("unexpected authorization header: %s", auth)
				}
				formValues, fileData := parseMultipartRequestForTest(t, r)
				if got := formValues["name"]; got != "示例日志" {
					t.Fatalf("unexpected name: %s", got)
				}
				if got := formValues["uniform_id"]; got != "Sealchat:custom" {
					t.Fatalf("unexpected uniform_id: %s", got)
				}
				if got := formValues["client"]; got != "Others" {
					t.Fatalf("unexpected client: %s", got)
				}
				if got := formValues["version"]; got != "105" {
					t.Fatalf("unexpected version: %s", got)
				}
				if len(fileData) == 0 {
					t.Fatal("expected compressed data")
				}
				return newHTTPResponse(http.StatusOK, `{"url":"https://result.example.com/rendered.docx"}`), nil
			default:
				t.Fatalf("unexpected endpoint: %s", r.URL.String())
				return nil, nil
			}
		}),
	}

	payload := preparedLogUploadPayload{
		Name:      "示例日志",
		UniformID: "Sealchat:custom",
		Client:    "Others",
		Version:   105,
		Data:      []byte("compressed-json"),
	}

	url, usedEndpoint, err := uploadCompressedExportLogWithClient(
		client,
		[]string{"https://primary.example.com/dice/api/log", "https://backup.example.com/dice/api/log"},
		payload,
		"test-token",
	)
	if err != nil {
		t.Fatalf("uploadCompressedExportLog failed: %v", err)
	}
	if url != "https://result.example.com/rendered.docx" {
		t.Fatalf("unexpected upload url: %s", url)
	}
	if usedEndpoint != "https://backup.example.com/dice/api/log" {
		t.Fatalf("unexpected used endpoint: %s", usedEndpoint)
	}
	if atomic.LoadInt32(&failedCalls) != 1 {
		t.Fatalf("expected one failed endpoint attempt, got %d", failedCalls)
	}
	if atomic.LoadInt32(&successCalls) != 1 {
		t.Fatalf("expected one successful endpoint attempt, got %d", successCalls)
	}
}

func TestNormalizeUploadEndpointsDedup(t *testing.T) {
	got := normalizeUploadEndpoints(
		" https://primary.example.com ",
		[]string{
			"",
			"https://backup-a.example.com",
			"https://primary.example.com",
			" https://backup-b.example.com ",
		},
	)
	want := []string{
		"https://primary.example.com",
		"https://backup-a.example.com",
		"https://backup-b.example.com",
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected endpoint count: %#v", got)
	}
	for i := range want {
		if !strings.EqualFold(got[i], want[i]) {
			t.Fatalf("unexpected endpoint at %d: got %s want %s", i, got[i], want[i])
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func newHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func parseMultipartRequestForTest(t *testing.T, r *http.Request) (map[string]string, []byte) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body failed: %v", err)
	}
	mediaType := r.Header.Get("Content-Type")
	reader := multipart.NewReader(bytes.NewReader(body), strings.TrimPrefix(mediaType[strings.Index(mediaType, "boundary="):], "boundary="))
	values := make(map[string]string)
	var fileData []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next multipart part failed: %v", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read multipart part failed: %v", err)
		}
		if part.FileName() != "" {
			fileData = data
			continue
		}
		values[part.FormName()] = string(data)
	}
	return values, fileData
}
