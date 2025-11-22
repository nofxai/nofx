package backtest

import (
	"fmt"
	"strings"

	"nofx/mcp"
)

func configureMCPClient(cfg BacktestConfig, base mcp.AIClient) (mcp.AIClient, error) {
	// Always create a new client for backtest isolation
	// (cannot copy interface, so always create new)
	client := mcp.New()

	// Configure API key and endpoint based on provider
	provider := strings.ToLower(strings.TrimSpace(cfg.AICfg.Provider))

	// Set default BaseURL for known providers if not specified
	baseURL := strings.TrimSpace(cfg.AICfg.BaseURL)
	// 为 DeepSeek 和 Qwen 设置默认 URL（它们有专用客户端）
	// OpenAI/Gemini/Groq 的默认 URL 由 mcp.SetAPIKey 自动处理
	if baseURL == "" {
		switch provider {
		case "deepseek":
			baseURL = mcp.DefaultDeepSeekBaseURL
		case "qwen":
			baseURL = mcp.DefaultQwenBaseURL
		}
	}

	// Validate required fields based on provider
	switch provider {
	case "", "inherit", "default":
		// Use base client's configuration if available, otherwise require explicit config
		if base != nil && cfg.AICfg.APIKey == "" {
			// Inherit from base (already configured)
			return base, nil
		}
		if cfg.AICfg.APIKey == "" {
			return nil, fmt.Errorf("api key is required")
		}
	case "deepseek", "qwen", "openai", "gemini", "groq":
		if cfg.AICfg.APIKey == "" {
			return nil, fmt.Errorf("%s provider requires api key", provider)
		}
	case "custom":
		if baseURL == "" || cfg.AICfg.APIKey == "" || cfg.AICfg.Model == "" {
			return nil, fmt.Errorf("custom provider requires base_url, api key and model")
		}
	default:
		return nil, fmt.Errorf("unsupported ai provider %s", cfg.AICfg.Provider)
	}

	// Use unified SetAPIKey method for all providers
	client.SetAPIKey(cfg.AICfg.APIKey, baseURL, cfg.AICfg.Model, provider)

	if cfg.AICfg.Temperature > 0 {
		// no direct field, but we keep for completeness
	}

	return client, nil
}
