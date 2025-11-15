package logger

import (
	"strings"
	"testing"
)

func TestRedactAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int // Expected total length
		hasStart bool // Should preserve start
		hasEnd   bool // Should preserve end
	}{
		{
			name:     "standard API key",
			input:    "sk-1234567890abcdef",
			wantLen:  19, // Length should match input
			hasStart: true,
			hasEnd:   true,
		},
		{
			name:     "long API key",
			input:    "sk-1234567890abcdefghijklmnopqrstuvwxyz",
			wantLen:  39,
			hasStart: true,
			hasEnd:   true,
		},
		{
			name:    "short key (all masked)",
			input:   "short",
			wantLen: 5,
		},
		{
			name:    "empty string",
			input:   "",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactAPIKey(tt.input)
			if len(result) != tt.wantLen {
				t.Errorf("RedactAPIKey(%q) length = %d, want %d (got %q)", tt.input, len(result), tt.wantLen, result)
			}
			if tt.hasStart && len(tt.input) > 8 {
				start := tt.input[:4]
				if !strings.HasPrefix(result, start) {
					t.Errorf("Result should preserve first 4 chars: want prefix %q, got %q", start, result)
				}
			}
			if tt.hasEnd && len(tt.input) > 8 {
				end := tt.input[len(tt.input)-4:]
				if !strings.HasSuffix(result, end) {
					t.Errorf("Result should preserve last 4 chars: want suffix %q, got %q", end, result)
				}
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
		// Use realistic API key lengths (at least 16 chars after prefix to match pattern)
		record := &DecisionRecord{
			SystemPrompt: "System configured with API key: sk-1234567890abcdefghij",
			InputPrompt:  "User prompt with key_abcd1234567890efghij",
			ErrorMessage: "Error: Failed to call sk-xyz123456789abcdefgh",
		}

		// Simulate what LogDecision does (we test the redaction functions it uses)
		record.SystemPrompt = RedactSensitiveInfo(record.SystemPrompt)
		record.InputPrompt = RedactSensitiveInfo(record.InputPrompt)
		record.ErrorMessage = RedactSensitiveInfo(record.ErrorMessage)

		// Should not contain the middle parts of keys
		if strings.Contains(record.SystemPrompt, "567890abcd") {
			t.Error("SystemPrompt should have redacted middle part of API key")
		}
		if strings.Contains(record.InputPrompt, "1234567890") {
			t.Error("InputPrompt should have redacted middle part of key")
		}
		if strings.Contains(record.ErrorMessage, "123456789a") {
			t.Error("ErrorMessage should have redacted middle part of API key")
		}
	})
}
