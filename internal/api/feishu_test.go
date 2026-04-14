package api

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/config"
	"github.com/go-chi/chi/v5"
)

func TestParseFeishuCallbackPayloadPlainURLVerification(t *testing.T) {
	body := `{"challenge":"feishu-challenge","token":"test-token","type":"url_verification"}`
	req := httptest.NewRequest("POST", "/api/adapters/feishu/testuser/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	payload, err := parseFeishuCallbackPayload(req, []byte(body), "test-token", "")
	if err != nil {
		t.Fatalf("parseFeishuCallbackPayload returned error: %v", err)
	}

	if payload.Type != "url_verification" {
		t.Fatalf("expected url_verification, got %q", payload.Type)
	}
	if payload.Challenge != "feishu-challenge" {
		t.Fatalf("expected challenge feishu-challenge, got %q", payload.Challenge)
	}
}

func TestParseFeishuCallbackPayloadEncryptedURLVerification(t *testing.T) {
	plaintext := `{"challenge":"encrypted-challenge","token":"test-token","type":"url_verification"}`
	encryptKey := "test-encrypt-key"
	encrypted := encryptFeishuPayloadForTest(t, plaintext, encryptKey)
	body := `{"encrypt":"` + encrypted + `"}`

	req := httptest.NewRequest("POST", "/api/adapters/feishu/testuser/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lark-Request-Timestamp", "1710000000")
	req.Header.Set("X-Lark-Request-Nonce", "nonce-1")
	req.Header.Set("X-Lark-Signature", feishuSignatureForTest("1710000000", "nonce-1", encryptKey, []byte(body)))

	payload, err := parseFeishuCallbackPayload(req, []byte(body), "test-token", encryptKey)
	if err != nil {
		t.Fatalf("parseFeishuCallbackPayload returned error: %v", err)
	}

	if payload.Challenge != "encrypted-challenge" {
		t.Fatalf("expected decrypted challenge, got %q", payload.Challenge)
	}
}

func TestBuildFeishuInboxMessageFromTextEvent(t *testing.T) {
	payload := &feishuCallbackPayload{
		Header: feishuEventHeader{
			EventType: "im.message.receive_v1",
			TenantKey: "tenant-key",
		},
		Event: &feishuMessageEvent{
			Sender: feishuEventSender{
				SenderID: feishuUserID{OpenID: "ou_sender"},
			},
			Message: feishuEventMessage{
				MessageID:   "om_message",
				ChatID:      "oc_chat",
				ThreadID:    "omt_thread",
				ChatType:    "p2p",
				MessageType: "text",
				Content:     `{"text":"hello from feishu"}`,
			},
		},
	}

	msg := buildFeishuInboxMessage(payload)
	if msg.ToAddress != "assistant" {
		t.Fatalf("expected to assistant, got %q", msg.ToAddress)
	}
	if msg.FromAddress != "feishu" {
		t.Fatalf("expected from feishu, got %q", msg.FromAddress)
	}
	if msg.Body != "hello from feishu" {
		t.Fatalf("expected text body, got %q", msg.Body)
	}
	if msg.Domain != "feishu" {
		t.Fatalf("expected feishu domain, got %q", msg.Domain)
	}
	if msg.ThreadID != "omt_thread" {
		t.Fatalf("expected thread id omt_thread, got %q", msg.ThreadID)
	}
}

func TestHandleFeishuEventCallbackURLVerification(t *testing.T) {
	s := &Server{
		Config: &config.Config{
			FeishuVerificationToken: "test-token",
		},
	}

	body := `{"challenge":"challenge-value","token":"test-token","type":"url_verification"}`
	req := httptest.NewRequest("POST", "/api/adapters/feishu/testuser/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", "testuser")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	s.handleFeishuEventCallback(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["challenge"] != "challenge-value" {
		t.Fatalf("expected challenge response, got %v", resp)
	}
}

func TestSendFeishuAckUsesTenantAccessTokenAndChatMessageAPI(t *testing.T) {
	var sawTokenRequest bool
	var sawMessageRequest bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			sawTokenRequest = true
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode token request: %v", err)
			}
			if body["app_id"] != "cli_test_app" || body["app_secret"] != "app_secret_value" {
				t.Fatalf("unexpected token request body: %v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","tenant_access_token":"t_test_access_token"}`)
		case r.URL.Path == "/open-apis/im/v1/messages":
			sawMessageRequest = true
			if r.URL.Query().Get("receive_id_type") != "chat_id" {
				t.Fatalf("unexpected receive_id_type: %q", r.URL.Query().Get("receive_id_type"))
			}
			if got := r.Header.Get("Authorization"); got != "Bearer t_test_access_token" {
				t.Fatalf("unexpected Authorization header: %q", got)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode message request: %v", err)
			}
			if body["receive_id"] != "oc_chat_123" {
				t.Fatalf("unexpected receive_id: %q", body["receive_id"])
			}
			if body["msg_type"] != "text" {
				t.Fatalf("unexpected msg_type: %q", body["msg_type"])
			}
			var content map[string]string
			if err := json.Unmarshal([]byte(body["content"]), &content); err != nil {
				t.Fatalf("failed to decode content payload: %v", err)
			}
			if content["text"] != "neuDrive 已收到你的消息。" {
				t.Fatalf("unexpected message text: %q", content["text"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	err := sendFeishuAck(
		context.Background(),
		ts.Client(),
		ts.URL,
		"cli_test_app",
		"app_secret_value",
		"oc_chat_123",
		"neuDrive 已收到你的消息。",
	)
	if err != nil {
		t.Fatalf("sendFeishuAck returned error: %v", err)
	}
	if !sawTokenRequest {
		t.Fatal("expected token endpoint to be called")
	}
	if !sawMessageRequest {
		t.Fatal("expected send message endpoint to be called")
	}
}

func encryptFeishuPayloadForTest(t *testing.T, plaintext, encryptKey string) string {
	t.Helper()

	keyHash := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(keyHash[:])
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}

	iv := []byte("1234567890abcdef")
	data := pkcs7Pad([]byte(plaintext), aes.BlockSize)
	encrypted := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, data)

	return base64.StdEncoding.EncodeToString(append(iv, encrypted...))
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(data, bytesRepeat(byte(padding), padding)...)
}

func bytesRepeat(b byte, count int) []byte {
	out := make([]byte, count)
	for i := range out {
		out[i] = b
	}
	return out
}

func feishuSignatureForTest(timestamp, nonce, encryptKey string, rawBody []byte) string {
	sum := sha256.Sum256(append([]byte(timestamp+nonce+encryptKey), rawBody...))
	return hex.EncodeToString(sum[:])
}
