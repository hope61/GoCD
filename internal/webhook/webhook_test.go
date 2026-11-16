package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

const secret = "s3cret"

func sign(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func send(method, event, signature string, body []byte) (int, int) {
	triggered := 0
	h := Handler(secret, func() { triggered++ })

	req := httptest.NewRequest(method, "/", bytes.NewReader(body))
	if event != "" {
		req.Header.Set("X-GitHub-Event", event)
	}
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, triggered
}

func TestHandler(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)

	tests := []struct {
		name      string
		method    string
		event     string
		signature string
		body      []byte
		code      int
		triggered int
	}{
		{"push with valid signature", http.MethodPost, "push", sign(secret, body), body, http.StatusNoContent, 1},
		{"ping is accepted but does nothing", http.MethodPost, "ping", sign(secret, body), body, http.StatusNoContent, 0},
		{"other events do nothing", http.MethodPost, "issues", sign(secret, body), body, http.StatusNoContent, 0},
		{"wrong secret", http.MethodPost, "push", sign("other", body), body, http.StatusUnauthorized, 0},
		{"missing signature", http.MethodPost, "push", "", body, http.StatusUnauthorized, 0},
		{"tampered body", http.MethodPost, "push", sign(secret, body), []byte(`{"ref":"refs/heads/evil"}`), http.StatusUnauthorized, 0},
		{"sha1 style header", http.MethodPost, "push", "sha1=" + sign(secret, body)[7:], body, http.StatusUnauthorized, 0},
		{"GET not allowed", http.MethodGet, "push", sign(secret, body), body, http.StatusMethodNotAllowed, 0},
		{"PUT not allowed", http.MethodPut, "push", sign(secret, body), body, http.StatusMethodNotAllowed, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, triggered := send(tt.method, tt.event, tt.signature, tt.body)
			if code != tt.code || triggered != tt.triggered {
				t.Fatalf("got code %d triggered %d, want code %d triggered %d", code, triggered, tt.code, tt.triggered)
			}
		})
	}
}

func TestHandlerRejectsHugeBody(t *testing.T) {
	body := make([]byte, 25<<20+1)
	code, triggered := send(http.MethodPost, "push", sign(secret, body), body)
	if code != http.StatusBadRequest || triggered != 0 {
		t.Fatalf("got code %d triggered %d, want 400 and no trigger", code, triggered)
	}
}

func TestHandlerAcceptsBodyAtLimit(t *testing.T) {
	body := make([]byte, 25<<20)
	code, triggered := send(http.MethodPost, "push", sign(secret, body), body)
	if code != http.StatusNoContent || triggered != 1 {
		t.Fatalf("got code %d triggered %d, want 204 and one trigger", code, triggered)
	}
}

func TestValidSignature(t *testing.T) {
	body := []byte("hello")
	if !validSignature(secret, body, sign(secret, body)) {
		t.Fatal("correct signature rejected")
	}
	if validSignature(secret, body, sign(secret, body)+"0") {
		t.Fatal("signature with extra character accepted")
	}
	if validSignature("", body, sign(secret, body)) {
		t.Fatal("signature made with another secret accepted")
	}
}
