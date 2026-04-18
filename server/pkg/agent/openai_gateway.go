package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type gatewayChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content,omitempty"`
}

type gatewayChatCompletionRequest struct {
	Model              string               `json:"model"`
	Messages           []gatewayChatMessage `json:"messages"`
	Stream             bool                 `json:"stream"`
	SessionID          string               `json:"session_id,omitempty"`
	WorkingDir         string               `json:"working_dir,omitempty"`
	MaxTurns           int                  `json:"max_turns,omitempty"`
	PermissionMode     string               `json:"permission_mode,omitempty"`
	AppendSystemPrompt string               `json:"append_system_prompt,omitempty"`
	Verbose            bool                 `json:"verbose,omitempty"`
}

type gatewayToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type gatewayChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content   string            `json:"content"`
			ToolCalls []gatewayToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	SessionID string `json:"session_id"`
}

func executeViaOpenAIGateway(ctx context.Context, provider string, cfg Config, prompt string, opts ExecOptions) (*Session, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 20 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	msgCh := make(chan Message, 64)
	resCh := make(chan Result, 1)

	go func() {
		defer cancel()
		defer close(msgCh)
		defer close(resCh)

		startTime := time.Now()
		trySend(msgCh, Message{Type: MessageStatus, Status: "running"})

		resp, err := callOpenAIGateway(runCtx, provider, cfg, prompt, opts)
		duration := time.Since(startTime)
		if err != nil {
			status := "failed"
			finalError := err.Error()
			switch runCtx.Err() {
			case context.DeadlineExceeded:
				status = "timeout"
				finalError = fmt.Sprintf("%s gateway timed out after %s", provider, timeout)
			case context.Canceled:
				status = "aborted"
				finalError = "execution cancelled"
			}
			resCh <- Result{
				Status:     status,
				Error:      finalError,
				DurationMs: duration.Milliseconds(),
			}
			return
		}

		for _, toolCall := range resp.ToolCalls {
			var input map[string]any
			if strings.TrimSpace(toolCall.Function.Arguments) != "" {
				_ = json.Unmarshal([]byte(toolCall.Function.Arguments), &input)
			}
			trySend(msgCh, Message{
				Type:   MessageToolUse,
				Tool:   toolCall.Function.Name,
				CallID: toolCall.ID,
				Input:  input,
			})
		}
		if resp.Content != "" {
			trySend(msgCh, Message{Type: MessageText, Content: resp.Content})
		}

		resCh <- Result{
			Status:     "completed",
			Output:     resp.Content,
			DurationMs: duration.Milliseconds(),
			SessionID:  resp.SessionID,
		}
	}()

	return &Session{Messages: msgCh, Result: resCh}, nil
}

type gatewayExecutionResult struct {
	Content   string
	SessionID string
	ToolCalls []gatewayToolCall
}

func callOpenAIGateway(ctx context.Context, provider string, cfg Config, prompt string, opts ExecOptions) (gatewayExecutionResult, error) {
	payload := gatewayChatCompletionRequest{
		Model:    gatewayModel(provider, opts.Model),
		Messages: []gatewayChatMessage{{Role: "user", Content: prompt}},
		Stream:   false,
		Verbose:  true,
	}
	if opts.ResumeSessionID != "" {
		payload.SessionID = opts.ResumeSessionID
	}
	if opts.Cwd != "" {
		payload.WorkingDir = opts.Cwd
	}
	if opts.MaxTurns > 0 {
		payload.MaxTurns = opts.MaxTurns
	}
	if opts.SystemPrompt != "" {
		payload.AppendSystemPrompt = opts.SystemPrompt
	}
	if provider == "claude" {
		payload.PermissionMode = "bypassPermissions"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return gatewayExecutionResult{}, fmt.Errorf("marshal gateway request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gatewayChatCompletionsURL(cfg.GatewayBaseURL), bytes.NewReader(body))
	if err != nil {
		return gatewayExecutionResult{}, fmt.Errorf("build gateway request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+gatewayAPIKey(cfg))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return gatewayExecutionResult{}, fmt.Errorf("call %s gateway: %w", provider, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return gatewayExecutionResult{}, fmt.Errorf("read %s gateway response: %w", provider, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return gatewayExecutionResult{}, fmt.Errorf("%s gateway returned %d: %s", provider, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed gatewayChatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return gatewayExecutionResult{}, fmt.Errorf("decode %s gateway response: %w", provider, err)
	}
	if len(parsed.Choices) == 0 {
		return gatewayExecutionResult{}, fmt.Errorf("%s gateway returned no choices", provider)
	}

	choice := parsed.Choices[0]
	return gatewayExecutionResult{
		Content:   choice.Message.Content,
		SessionID: parsed.SessionID,
		ToolCalls: choice.Message.ToolCalls,
	}, nil
}

func gatewayModel(provider, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return "backend_default"
}

func gatewayAPIKey(cfg Config) string {
	if strings.TrimSpace(cfg.GatewayAPIKey) != "" {
		return strings.TrimSpace(cfg.GatewayAPIKey)
	}
	return "not-needed"
}

func gatewayChatCompletionsURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed + "/chat/completions"
	}

	parsed, err := url.Parse(trimmed)
	if err == nil && strings.TrimSpace(parsed.Path) == "" {
		return trimmed + "/v1/chat/completions"
	}
	return trimmed + "/chat/completions"
}
