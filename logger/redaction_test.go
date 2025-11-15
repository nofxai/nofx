package logger

import (
	"strings"
	"testing"
)

func TestRedactAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard API key",
			input:    "sk-1234567890abcdef",
			expected: "sk-1********cdef",
		},
		{
			name:     "long API key",
			input:    "sk-1234567890abcdefghijklmnopqrstuvwxyz",
			expected: "sk-1**************************wxyz",
		},
		{
			name:     "short key (all masked)",
			input:    "short",
			expected: "*****",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactAPIKey(tt.input)
			if result != tt.expected {
				t.Errorf("RedactAPIKey(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRedactSensitiveInfo(t *testing.T) {
	t.Run("should redact sk- prefixed API keys", func(t *testing.T) {
		text := "Using API key: sk-1234567890abcdefghij"
		result := RedactSensitiveInfo(text)

		if !strings.Contains(result, "sk-1234**********ghij") {
			t.Errorf("Expected API key to be redacted, got: %s", result)
		}
		if strings.Contains(result, "sk-1234567890abcdefghij") {
			t.Error("Original API key should not be present in result")
		}
	})

	t.Run("should redact key_ prefixed keys", func(t *testing.T) {
		text := "API configuration: key_abcdefghijklmnopqrstuvwxyz"
		result := RedactSensitiveInfo(text)

		if !strings.Contains(result, "key_abcd**********wxyz") {
			t.Errorf("Expected key_ to be redacted, got: %s", result)
		}
	})

	t.Run("should redact hex private keys with 0x prefix", func(t *testing.T) {
		text := "Private key: 0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
		result := RedactSensitiveInfo(text)

		if strings.Contains(result, "1234567890abcdef1234567890abcdef1234567890abcdef12") {
			t.Error("Middle part of private key should be redacted")
		}
		if !strings.Contains(result, "0x1234**********cdef") {
			t.Errorf("Expected redacted hex key, got: %s", result)
		}
	})

	t.Run("should redact plain hex private keys (without 0x)", func(t *testing.T) {
		text := "Key: abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
		result := RedactSensitiveInfo(text)

		if strings.Contains(result, "1234567890abcdef1234567890abcdef1234567890abcdef12") {
			t.Error("Middle part of hex key should be redacted")
		}
	})

	t.Run("should not modify text without sensitive info", func(t *testing.T) {
		text := "This is a normal log message with no sensitive data"
		result := RedactSensitiveInfo(text)

		if result != text {
			t.Errorf("Text without sensitive info should not be modified: %s", result)
		}
	})

	t.Run("should handle multiple keys in same text", func(t *testing.T) {
		text := "API: sk-1234567890abcdefghij and Private: 0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
		result := RedactSensitiveInfo(text)

		// Both should be redacted
		if strings.Contains(result, "sk-1234567890abcdefghij") {
			t.Error("First API key should be redacted")
		}
		if strings.Contains(result, "1234567890abcdef1234567890abcdef1234567890") {
			t.Error("Private key should be redacted")
		}
	})
}

func TestRedactionInDecisionLog(t *testing.T) {
	t.Run("should redact API keys in prompts", func(t *testing.T) {
		record := &DecisionRecord{
			SystemPrompt: "System configured with API key: sk-secretkey12345",
			InputPrompt:  "User prompt with key_anotherkey67890",
			ErrorMessage: "Error: Failed to call sk-yetanotherkey",
		}

		// Simulate what LogDecision does (we test the redaction functions it uses)
		record.SystemPrompt = RedactSensitiveInfo(record.SystemPrompt)
		record.InputPrompt = RedactSensitiveInfo(record.InputPrompt)
		record.ErrorMessage = RedactSensitiveInfo(record.ErrorMessage)

		if strings.Contains(record.SystemPrompt, "sk-secretkey12345") {
			t.Error("SystemPrompt should have redacted API key")
		}
		if strings.Contains(record.InputPrompt, "key_anotherkey67890") {
			t.Error("InputPrompt should have redacted key")
		}
		if strings.Contains(record.ErrorMessage, "sk-yetanotherkey") {
			t.Error("ErrorMessage should have redacted API key")
		}
	})
}
