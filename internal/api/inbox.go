package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Message struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Archived  bool   `json:"archived"`
	CreatedAt string `json:"created_at"`
}

type SendMessageRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func HandleInboxList(w http.ResponseWriter, r *http.Request) {
	role := chi.URLParam(r, "role")

	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// TODO: query database for messages for this role
	_ = user
	messages := []Message{}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"role":     role,
		"messages": messages,
	})
}

func HandleInboxSend(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.To == "" || req.Body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to and body are required"})
		return
	}

	// TODO: insert message into database
	msg := &Message{
		ID:      "generated-id",
		From:    user.Username,
		To:      req.To,
		Subject: req.Subject,
		Body:    req.Body,
	}

	writeJSON(w, http.StatusCreated, msg)
}

func HandleInboxArchive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// TODO: mark message as archived in database
	_ = user
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived", "id": id})
}
