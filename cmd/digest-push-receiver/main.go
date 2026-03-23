package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type ackResponse struct {
	OK             bool   `json:"ok"`
	Path           string `json:"path"`
	Method         string `json:"method"`
	ReceivedAt     string `json:"receivedAt"`
	SignatureValid bool   `json:"signatureValid"`
	TimestampValid bool   `json:"timestampValid"`
	Message        string `json:"message,omitempty"`
	BodyLength     int    `json:"bodyLength"`
}

func main() {
	addr := getenv("DIGEST_PUSH_RECEIVER_ADDR", ":18081")
	path := normalizePath(getenv("DIGEST_PUSH_RECEIVER_PATH", "/digest"))
	secret := strings.TrimSpace(os.Getenv("DIGEST_PUSH_SECRET"))
	maxSkewSeconds := getenvInt("DIGEST_PUSH_MAX_SKEW_SECONDS", 300)

	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		handleDigest(w, r, secret, maxSkewSeconds)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":            "digest-push-receiver",
			"digestPath":      path,
			"signatureEnable": secret != "",
			"maxSkewSeconds":  maxSkewSeconds,
			"now":             time.Now().Format(time.RFC3339),
		})
	})

	log.Printf("digest-push-receiver listening on %s%s", addr, path)
	if secret == "" {
		log.Printf("digest-push-receiver signature check disabled (DIGEST_PUSH_SECRET is empty)")
		log.Printf("digest-push-receiver timestamp check disabled when header is absent")
	} else {
		log.Printf("digest-push-receiver signature check enabled, max skew = %ds", maxSkewSeconds)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-stopCh
		log.Printf("digest-push-receiver shutting down, signal=%s", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("digest-push-receiver shutdown failed: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func handleDigest(w http.ResponseWriter, r *http.Request, secret string, maxSkewSeconds int) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, ackResponse{
			OK:         false,
			Path:       r.URL.Path,
			Method:     r.Method,
			ReceivedAt: time.Now().Format(time.RFC3339),
			Message:    "unsupported method",
		})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ackResponse{
			OK:         false,
			Path:       r.URL.Path,
			Method:     r.Method,
			ReceivedAt: time.Now().Format(time.RFC3339),
			Message:    "read body failed",
		})
		return
	}

	timestamp := strings.TrimSpace(r.Header.Get("X-SealChat-Timestamp"))
	signature := strings.TrimSpace(r.Header.Get("X-SealChat-Signature"))

	timestampValid, timestampMsg := validateTimestamp(timestamp, maxSkewSeconds, secret != "")
	signatureValid, signatureMsg := validateSignature(secret, timestamp, signature, body)

	log.Printf("digest request %s %s remote=%s bodyBytes=%d", r.Method, r.URL.Path, r.RemoteAddr, len(body))
	logHeaders(r.Header)
	log.Printf("digest raw body: %s", strings.TrimSpace(string(body)))

	var pretty any
	if err := json.Unmarshal(body, &pretty); err == nil {
		prettyBody, _ := json.MarshalIndent(pretty, "", "  ")
		log.Printf("digest pretty body:\n%s", prettyBody)
	} else {
		log.Printf("digest body is not valid JSON: %v", err)
	}

	message := "accepted"
	status := http.StatusOK
	if !timestampValid {
		status = http.StatusUnauthorized
		message = timestampMsg
	} else if !signatureValid {
		status = http.StatusUnauthorized
		message = signatureMsg
	}

	writeJSON(w, status, ackResponse{
		OK:             status >= 200 && status < 300,
		Path:           r.URL.Path,
		Method:         r.Method,
		ReceivedAt:     time.Now().Format(time.RFC3339),
		SignatureValid: signatureValid,
		TimestampValid: timestampValid,
		Message:        message,
		BodyLength:     len(body),
	})
}

func validateTimestamp(raw string, maxSkewSeconds int, required bool) (bool, string) {
	if strings.TrimSpace(raw) == "" {
		if !required {
			return true, ""
		}
		return false, "missing X-SealChat-Timestamp"
	}
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return false, "invalid X-SealChat-Timestamp"
	}
	if maxSkewSeconds <= 0 {
		return true, ""
	}
	skew := time.Since(time.Unix(sec, 0))
	if skew < 0 {
		skew = -skew
	}
	if skew > time.Duration(maxSkewSeconds)*time.Second {
		return false, fmt.Sprintf("timestamp expired or skew too large: %s", skew)
	}
	return true, ""
}

func validateSignature(secret, timestamp, signature string, body []byte) (bool, string) {
	if strings.TrimSpace(secret) == "" {
		return true, ""
	}
	if strings.TrimSpace(signature) == "" {
		return false, "missing X-SealChat-Signature"
	}
	expected := sign(secret, timestamp, body)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return false, "signature mismatch"
	}
	return true, ""
}

func sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func logHeaders(header http.Header) {
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		log.Printf("header %s: %s", key, strings.Join(header.Values(key), ", "))
	}
}

func writeJSON(w http.ResponseWriter, status int, payload ackResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/digest"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
