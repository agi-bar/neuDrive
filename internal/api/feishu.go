package api

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/go-chi/chi/v5"
)

var (
	errFeishuAdapterNotConfigured = errors.New("feishu adapter is not configured")
	errFeishuInvalidSignature     = errors.New("invalid feishu signature")
	errFeishuInvalidToken         = errors.New("invalid feishu verification token")
	errFeishuMissingEncryptKey    = errors.New("missing feishu encrypt key")
)

const feishuOpenAPIBaseURL = "https://open.feishu.cn"

type feishuCallbackPayload struct {
	Type      string              `json:"type"`
	Challenge string              `json:"challenge"`
	Token     string              `json:"token"`
	Encrypt   string              `json:"encrypt"`
	Schema    string              `json:"schema"`
	Header    feishuEventHeader   `json:"header"`
	Event     *feishuMessageEvent `json:"event,omitempty"`
}

type feishuEventHeader struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	CreateTime string `json:"create_time"`
	Token      string `json:"token"`
	AppID      string `json:"app_id"`
	TenantKey  string `json:"tenant_key"`
}

type feishuMessageEvent struct {
	Sender  feishuEventSender  `json:"sender"`
	Message feishuEventMessage `json:"message"`
}

type feishuEventSender struct {
	SenderID  feishuUserID `json:"sender_id"`
	TenantKey string       `json:"tenant_key"`
}

type feishuUserID struct {
	UnionID string `json:"union_id"`
	UserID  string `json:"user_id"`
	OpenID  string `json:"open_id"`
}

type feishuEventMessage struct {
	MessageID   string          `json:"message_id"`
	RootID      string          `json:"root_id"`
	ParentID    string          `json:"parent_id"`
	CreateTime  string          `json:"create_time"`
	UpdateTime  string          `json:"update_time"`
	ChatID      string          `json:"chat_id"`
	ThreadID    string          `json:"thread_id"`
	ChatType    string          `json:"chat_type"`
	MessageType string          `json:"message_type"`
	Content     string          `json:"content"`
	Mentions    json.RawMessage `json:"mentions"`
	UserAgent   string          `json:"user_agent"`
}

func (s *Server) handleFeishuEventCallback(w http.ResponseWriter, r *http.Request) {
	if s.Config == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": errFeishuAdapterNotConfigured.Error()})
		return
	}
	if s.Config.FeishuVerificationToken == "" && s.Config.FeishuEncryptKey == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": errFeishuAdapterNotConfigured.Error()})
		return
	}

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	payload, err := parseFeishuCallbackPayload(
		r,
		rawBody,
		s.Config.FeishuVerificationToken,
		s.Config.FeishuEncryptKey,
	)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, errFeishuAdapterNotConfigured) {
			status = http.StatusServiceUnavailable
		} else if strings.Contains(err.Error(), "invalid") == false {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	if payload.Type == "url_verification" {
		writeJSON(w, http.StatusOK, map[string]string{"challenge": payload.Challenge})
		return
	}

	if payload.Header.EventType != "im.message.receive_v1" || payload.Event == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	if s.UserService == nil || s.InboxService == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "feishu event ingestion is not available"})
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing user slug"})
		return
	}

	user, err := s.UserService.GetBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	msg := buildFeishuInboxMessage(payload)
	if _, err := s.InboxService.Send(r.Context(), user.ID, msg); err != nil {
		respondInternalError(w, err)
		return
	}

	if s.Config.FeishuAppID != "" && s.Config.FeishuAppSecret != "" && payload.Event.Message.ChatID != "" {
		chatID := payload.Event.Message.ChatID
		go func() {
			if err := sendFeishuAck(
				context.Background(),
				http.DefaultClient,
				feishuOpenAPIBaseURL,
				s.Config.FeishuAppID,
				s.Config.FeishuAppSecret,
				chatID,
				"neuDrive 已收到你的消息。",
			); err != nil {
				slog.Warn("feishu ack failed", "chat_id", chatID, "error", err)
			}
		}()
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

func parseFeishuCallbackPayload(
	r *http.Request,
	rawBody []byte,
	verificationToken string,
	encryptKey string,
) (*feishuCallbackPayload, error) {
	if verificationToken == "" && encryptKey == "" {
		return nil, errFeishuAdapterNotConfigured
	}

	var wrapper feishuCallbackPayload
	if err := json.Unmarshal(rawBody, &wrapper); err != nil {
		return nil, fmt.Errorf("invalid feishu callback body: %w", err)
	}

	wasEncrypted := wrapper.Encrypt != ""
	if wrapper.Encrypt != "" {
		if encryptKey == "" {
			return nil, errFeishuMissingEncryptKey
		}
		if !verifyFeishuSignature(r, rawBody, encryptKey) {
			return nil, errFeishuInvalidSignature
		}

		decrypted, err := decryptFeishuPayload(wrapper.Encrypt, encryptKey)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(decrypted), &wrapper); err != nil {
			return nil, fmt.Errorf("invalid decrypted feishu body: %w", err)
		}
	}

	if verificationToken == "" && !wasEncrypted {
		return nil, errFeishuInvalidToken
	}
	if !isValidFeishuToken(wrapper, verificationToken) {
		return nil, errFeishuInvalidToken
	}

	return &wrapper, nil
}

func verifyFeishuSignature(r *http.Request, rawBody []byte, encryptKey string) bool {
	timestamp := r.Header.Get("X-Lark-Request-Timestamp")
	nonce := r.Header.Get("X-Lark-Request-Nonce")
	signature := r.Header.Get("X-Lark-Signature")
	if timestamp == "" || nonce == "" || signature == "" {
		return false
	}

	sum := sha256.Sum256(append([]byte(timestamp+nonce+encryptKey), rawBody...))
	return strings.EqualFold(signature, hex.EncodeToString(sum[:]))
}

func decryptFeishuPayload(encrypt, encryptKey string) (string, error) {
	buf, err := base64.StdEncoding.DecodeString(encrypt)
	if err != nil {
		return "", fmt.Errorf("invalid feishu encrypted payload: %w", err)
	}
	if len(buf) < aes.BlockSize {
		return "", errors.New("feishu encrypted payload is too short")
	}

	keyHash := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(keyHash[:])
	if err != nil {
		return "", fmt.Errorf("failed to initialize feishu decrypt cipher: %w", err)
	}

	iv := buf[:aes.BlockSize]
	cipherText := append([]byte(nil), buf[aes.BlockSize:]...)
	if len(cipherText)%aes.BlockSize != 0 {
		return "", errors.New("feishu encrypted payload is not a multiple of the AES block size")
	}

	cipher.NewCBCDecrypter(block, iv).CryptBlocks(cipherText, cipherText)
	cipherText = trimPKCS7(cipherText)
	cipherText = bytes.TrimSpace(cipherText)
	if len(cipherText) == 0 {
		return "", errors.New("feishu decrypted payload is empty")
	}

	start := bytes.IndexByte(cipherText, '{')
	end := bytes.LastIndexByte(cipherText, '}')
	if start >= 0 && end >= start {
		cipherText = cipherText[start : end+1]
	}

	return string(cipherText), nil
}

func trimPKCS7(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	pad := int(data[len(data)-1])
	if pad <= 0 || pad > aes.BlockSize || len(data) < pad {
		return data
	}
	padding := bytes.Repeat([]byte{byte(pad)}, pad)
	if !bytes.HasSuffix(data, padding) {
		return data
	}
	return data[:len(data)-pad]
}

func isValidFeishuToken(payload feishuCallbackPayload, verificationToken string) bool {
	if verificationToken == "" {
		return true
	}
	if payload.Token != "" {
		return payload.Token == verificationToken
	}
	if payload.Header.Token != "" {
		return payload.Header.Token == verificationToken
	}
	return false
}

func buildFeishuInboxMessage(payload *feishuCallbackPayload) models.InboxMessage {
	body := extractFeishuMessageBody(payload.Event.Message.MessageType, payload.Event.Message.Content)
	sender := payload.Event.Sender.SenderID.OpenID
	if sender == "" {
		sender = payload.Event.Sender.SenderID.UserID
	}
	subject := "Feishu message"
	if sender != "" {
		subject = "Feishu: " + sender
	}

	threadID := payload.Event.Message.ThreadID
	if threadID == "" {
		threadID = payload.Event.Message.ChatID
	}
	if threadID == "" {
		threadID = payload.Event.Message.MessageID
	}

	return models.InboxMessage{
		FromAddress: "feishu",
		ToAddress:   "assistant",
		ThreadID:    threadID,
		Priority:    "normal",
		Domain:      "feishu",
		ActionType:  "message.receive",
		Tags: []string{
			"feishu",
			payload.Event.Message.ChatType,
			payload.Event.Message.MessageType,
		},
		Subject: subject,
		Body:    body,
		StructuredPayload: map[string]interface{}{
			"platform":       "feishu",
			"sender_open_id": payload.Event.Sender.SenderID.OpenID,
			"sender_user_id": payload.Event.Sender.SenderID.UserID,
			"chat_id":        payload.Event.Message.ChatID,
			"message_id":     payload.Event.Message.MessageID,
			"thread_id":      payload.Event.Message.ThreadID,
			"message_type":   payload.Event.Message.MessageType,
			"raw_content":    payload.Event.Message.Content,
			"tenant_key":     payload.Header.TenantKey,
			"event_type":     payload.Header.EventType,
		},
	}
}

func extractFeishuMessageBody(messageType, rawContent string) string {
	if messageType == "text" {
		var content struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(rawContent), &content); err == nil && content.Text != "" {
			return content.Text
		}
	}

	if rawContent == "" {
		return "[" + messageType + "]"
	}
	return fmt.Sprintf("[%s] %s", messageType, rawContent)
}

type feishuTenantAccessTokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
}

type feishuSendMessageResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func sendFeishuAck(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	appID string,
	appSecret string,
	chatID string,
	text string,
) error {
	token, err := fetchFeishuTenantAccessToken(ctx, client, baseURL, appID, appSecret)
	if err != nil {
		return err
	}
	return sendFeishuTextMessage(ctx, client, baseURL, token, chatID, text)
}

func fetchFeishuTenantAccessToken(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	appID string,
	appSecret string,
) (string, error) {
	body, err := json.Marshal(map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/open-apis/auth/v3/tenant_access_token/internal",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result feishuTenantAccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode feishu tenant access token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("feishu token endpoint returned %d: %s", resp.StatusCode, result.Msg)
	}
	if result.Code != 0 || result.TenantAccessToken == "" {
		return "", fmt.Errorf("feishu token endpoint error: code=%d msg=%s", result.Code, result.Msg)
	}
	return result.TenantAccessToken, nil
}

func sendFeishuTextMessage(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	tenantAccessToken string,
	chatID string,
	text string,
) error {
	content, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{
		"receive_id": chatID,
		"msg_type":   "text",
		"content":    string(content),
		"uuid":       fmt.Sprintf("neudrive-feishu-%d", time.Now().UnixNano()),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/open-apis/im/v1/messages",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("receive_id_type", "chat_id")
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Authorization", "Bearer "+tenantAccessToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result feishuSendMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode feishu send message response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishu send message returned %d: %s", resp.StatusCode, result.Msg)
	}
	if result.Code != 0 {
		return fmt.Errorf("feishu send message error: code=%d msg=%s", result.Code, result.Msg)
	}
	return nil
}
