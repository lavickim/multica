package daemon

import (
	"testing"
)

func TestLoadConfigIncludesExplicitGatewayBaseURLs(t *testing.T) {
	t.Setenv("MULTICA_SERVER_URL", "ws://localhost:8080/ws")
	t.Setenv("MULTICA_CLAUDE_PATH", "/bin/sh")
	t.Setenv("MULTICA_CODEX_PATH", "/bin/sh")
	t.Setenv("MULTICA_CLAUDE_GATEWAY_BASE_URL", "http://127.0.0.1:17755/v1")
	t.Setenv("MULTICA_CODEX_GATEWAY_BASE_URL", "http://127.0.0.1:17750/v1/")

	cfg, err := LoadConfig(Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Agents["claude"].GatewayBaseURL != "http://127.0.0.1:17755/v1" {
		t.Fatalf("unexpected claude gateway URL: %q", cfg.Agents["claude"].GatewayBaseURL)
	}
	if cfg.Agents["codex"].GatewayBaseURL != "http://127.0.0.1:17750/v1" {
		t.Fatalf("unexpected codex gateway URL: %q", cfg.Agents["codex"].GatewayBaseURL)
	}
}

func TestLoadConfigDerivesGatewayBaseURLsFromShiftOSPorts(t *testing.T) {
	t.Setenv("MULTICA_SERVER_URL", "ws://localhost:8080/ws")
	t.Setenv("MULTICA_CLAUDE_PATH", "/bin/sh")
	t.Setenv("MULTICA_CODEX_PATH", "/bin/sh")
	t.Setenv("MULTICA_CLAUDE_GATEWAY_BASE_URL", "")
	t.Setenv("MULTICA_CODEX_GATEWAY_BASE_URL", "")
	t.Setenv("SHIFTOS_CLAUDE_GATEWAY_HOST", "127.0.0.1")
	t.Setenv("SHIFTOS_CLAUDE_GATEWAY_PORT", "27755")
	t.Setenv("SHIFTOS_CODEX_GATEWAY_HOST", "127.0.0.1")
	t.Setenv("SHIFTOS_CODEX_GATEWAY_PORT", "27750")

	cfg, err := LoadConfig(Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Agents["claude"].GatewayBaseURL != "http://127.0.0.1:27755/v1" {
		t.Fatalf("unexpected derived claude gateway URL: %q", cfg.Agents["claude"].GatewayBaseURL)
	}
	if cfg.Agents["codex"].GatewayBaseURL != "http://127.0.0.1:27750/v1" {
		t.Fatalf("unexpected derived codex gateway URL: %q", cfg.Agents["codex"].GatewayBaseURL)
	}
}
