package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClaudeGatewayExecuteUsesShiftOSCompatibleRequest(t *testing.T) {
	t.Parallel()

	type requestPayload struct {
		Model              string `json:"model"`
		Stream             bool   `json:"stream"`
		SessionID          string `json:"session_id"`
		WorkingDir         string `json:"working_dir"`
		MaxTurns           int    `json:"max_turns"`
		PermissionMode     string `json:"permission_mode"`
		AppendSystemPrompt string `json:"append_system_prompt"`
		Messages           []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	var gotAuth string
	var got requestPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "claude-opus-4-7",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "gateway hello",
					},
					"finish_reason": "stop",
				},
			},
			"session_id": "sess-claude-1",
		})
	}))
	defer server.Close()

	backend, err := New("claude", Config{
		GatewayBaseURL: server.URL + "/v1",
		GatewayAPIKey:  "not-needed",
		Logger:         slog.Default(),
	})
	if err != nil {
		t.Fatalf("New(claude): %v", err)
	}

	session, err := backend.Execute(context.Background(), "solve the task", ExecOptions{
		Cwd:             "/tmp/work",
		Model:           "claude-opus-4-7",
		SystemPrompt:    "follow the repo rules",
		MaxTurns:        7,
		Timeout:         5 * time.Second,
		ResumeSessionID: "sess-prev",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var messages []Message
	for msg := range session.Messages {
		messages = append(messages, msg)
	}
	result := <-session.Result

	if gotAuth != "Bearer not-needed" {
		t.Fatalf("expected Bearer auth, got %q", gotAuth)
	}
	if got.Stream {
		t.Fatal("expected stream=false request to preserve session_id in gateway response")
	}
	if got.Model != "claude-opus-4-7" {
		t.Fatalf("expected model override, got %q", got.Model)
	}
	if got.SessionID != "sess-prev" {
		t.Fatalf("expected resume session id, got %q", got.SessionID)
	}
	if got.WorkingDir != "/tmp/work" {
		t.Fatalf("expected working dir /tmp/work, got %q", got.WorkingDir)
	}
	if got.MaxTurns != 7 {
		t.Fatalf("expected max_turns=7, got %d", got.MaxTurns)
	}
	if got.PermissionMode != "bypassPermissions" {
		t.Fatalf("expected bypassPermissions, got %q", got.PermissionMode)
	}
	if got.AppendSystemPrompt != "follow the repo rules" {
		t.Fatalf("expected append_system_prompt, got %q", got.AppendSystemPrompt)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" || got.Messages[0].Content != "solve the task" {
		t.Fatalf("unexpected messages payload: %+v", got.Messages)
	}
	if result.Status != "completed" {
		t.Fatalf("expected completed result, got %+v", result)
	}
	if result.Output != "gateway hello" {
		t.Fatalf("expected gateway output, got %q", result.Output)
	}
	if result.SessionID != "sess-claude-1" {
		t.Fatalf("expected gateway session_id, got %q", result.SessionID)
	}
	if len(messages) == 0 || messages[len(messages)-1].Type != MessageText || messages[len(messages)-1].Content != "gateway hello" {
		t.Fatalf("expected final text message, got %+v", messages)
	}
}

func TestCodexGatewayExecuteOmitsUnsupportedPermissionMode(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-5.4-mini",
			"choices":[{"index":0,"message":{"role":"assistant","content":"codex gateway ok"},"finish_reason":"stop"}],
			"session_id":"thread-codex-1"
		}`))
	}))
	defer server.Close()

	backend, err := New("codex", Config{
		GatewayBaseURL: strings.TrimRight(server.URL, "/"),
		Logger:         slog.Default(),
	})
	if err != nil {
		t.Fatalf("New(codex): %v", err)
	}

	session, err := backend.Execute(context.Background(), "review the repo", ExecOptions{
		Cwd:             "/tmp/codex",
		Model:           "gpt-5.4-mini",
		SystemPrompt:    "be concise",
		MaxTurns:        3,
		Timeout:         5 * time.Second,
		ResumeSessionID: "thread-prev",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	for range session.Messages {
	}
	result := <-session.Result

	if got["permission_mode"] != nil {
		t.Fatalf("codex gateway should omit permission_mode, got %v", got["permission_mode"])
	}
	if got["working_dir"] != "/tmp/codex" {
		t.Fatalf("expected working_dir, got %v", got["working_dir"])
	}
	if got["append_system_prompt"] != "be concise" {
		t.Fatalf("expected append_system_prompt, got %v", got["append_system_prompt"])
	}
	if got["session_id"] != "thread-prev" {
		t.Fatalf("expected resume session id, got %v", got["session_id"])
	}
	if result.SessionID != "thread-codex-1" {
		t.Fatalf("expected session_id from gateway, got %q", result.SessionID)
	}
}
