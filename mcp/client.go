package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	ProviderCustom = "custom"
)

var (
	DefaultTimeout = 120 * time.Second

	// DefaultProviderURLs 各 provider 的默认 API URL
	// 新增 provider 时只需在此 map 中添加即可
	DefaultProviderURLs = map[string]string{
		"openai":    "https://api.openai.com/v1",
		"anthropic": "https://api.anthropic.com/v1",
		"gemini":    "https://generativelanguage.googleapis.com/v1beta/openai",
		"grok":      "https://api.x.ai/v1",
	}

	// DefaultProviderModels 各 provider 的默认模型名称
	DefaultProviderModels = map[string]string{
		"openai":    "gpt-5.1",
		"anthropic": "claude-sonnet-4-20250514",
		"gemini":    "gemini-2.5-pro",
		"grok":      "grok-4",
	}
)

// Client AI API配置
type Client struct {
	Provider    string
	APIKey      string
	BaseURL     string
	Model       string
	Timeout     time.Duration
	UseFullURL  bool    // 是否使用完整URL（不添加/chat/completions）
	MaxTokens   int     // AI响应的最大token数
	Temperature float64 // AI 温度参数，控制输出随机性（0.0-1.0），默认 0.1
}

func New() AIClient {
	// 从环境变量读取 MaxTokens，默认 2000
	maxTokens := 2000
	if envMaxTokens := os.Getenv("AI_MAX_TOKENS"); envMaxTokens != "" {
		if parsed, err := strconv.Atoi(envMaxTokens); err == nil && parsed > 0 {
			maxTokens = parsed
			log.Printf("🔧 [MCP] 使用环境变量 AI_MAX_TOKENS: %d", maxTokens)
		} else {
			log.Printf("⚠️  [MCP] 环境变量 AI_MAX_TOKENS 无效 (%s)，使用默认值: %d", envMaxTokens, maxTokens)
		}
	}

	// 默认配置
	return &Client{
		Provider:    ProviderDeepSeek,
		BaseURL:     DefaultDeepSeekBaseURL,
		Model:       DefaultDeepSeekModel,
		Timeout:     DefaultTimeout,
		MaxTokens:   maxTokens,
		Temperature: 0.1, // 交易系统默认低温，保证决策一致性
	}
}

// SetAPIKey 设置 API Key 和配置
// provider: 指定 AI 提供商 (openai, gemini, groq, custom 等)
// 如果 apiURL 为空，会根据 provider 使用默认 URL
func (client *Client) SetAPIKey(apiKey, apiURL, customModel, provider string) {
	client.Provider = provider
	client.APIKey = apiKey

	// 如果 URL 为空，根据 provider 使用默认 URL
	if apiURL == "" {
		if defaultURL, ok := DefaultProviderURLs[provider]; ok {
			apiURL = defaultURL
		}
	}

	// 检查URL是否以#结尾，如果是则使用完整URL（不添加/chat/completions）
	if strings.HasSuffix(apiURL, "#") {
		client.BaseURL = strings.TrimSuffix(apiURL, "#")
		client.UseFullURL = true
	} else {
		client.BaseURL = apiURL
		client.UseFullURL = false
	}

	if customModel != "" {
		client.Model = customModel
	} else if defaultModel, ok := DefaultProviderModels[provider]; ok {
		client.Model = defaultModel
	}
	client.Timeout = 120 * time.Second
}

// CallWithMessages 使用 system + user prompt 调用AI API（推荐）
func (client *Client) CallWithMessages(systemPrompt, userPrompt string) (string, error) {
	if client.APIKey == "" {
		return "", fmt.Errorf("AI API密钥未设置，请先调用 SetAPIKey")
	}

	// 重试配置
	maxRetries := 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			fmt.Printf("⚠️  AI API调用失败，正在重试 (%d/%d)...\n", attempt, maxRetries)
		}

		result, err := client.callOnce(systemPrompt, userPrompt)
		if err == nil {
			if attempt > 1 {
				fmt.Printf("✓ AI API重试成功\n")
			}
			return result, nil
		}

		lastErr = err
		// 如果不是网络错误，不重试
		if !isRetryableError(err) {
			return "", err
		}

		// 重试前等待
		if attempt < maxRetries {
			waitTime := time.Duration(attempt) * 2 * time.Second
			fmt.Printf("⏳ 等待%v后重试...\n", waitTime)
			time.Sleep(waitTime)
		}
	}

	return "", fmt.Errorf("重试%d次后仍然失败: %w", maxRetries, lastErr)
}

func (client *Client) setAuthHeader(reqHeader http.Header) {
	if client.Provider == "anthropic" {
		// Anthropic 使用 x-api-key 认证头
		reqHeader.Set("x-api-key", client.APIKey)
		reqHeader.Set("anthropic-version", "2023-06-01")
	} else {
		// OpenAI 兼容 API 使用 Bearer token
		reqHeader.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
	}
}

// SetTemperature 设置 AI 温度参数（0.0-1.0），控制输出随机性
func (client *Client) SetTemperature(temperature float64) {
	client.Temperature = temperature
}

// callOnce 单次调用AI API（内部使用）
func (client *Client) callOnce(systemPrompt, userPrompt string) (string, error) {
	// 打印当前 AI 配置
	log.Printf("📡 [MCP] AI 请求配置:")
	log.Printf("   Provider: %s", client.Provider)
	log.Printf("   BaseURL: %s", client.BaseURL)
	log.Printf("   Model: %s", client.Model)
	log.Printf("   UseFullURL: %v", client.UseFullURL)
	if len(client.APIKey) > 8 {
		log.Printf("   API Key: %s...%s", client.APIKey[:4], client.APIKey[len(client.APIKey)-4:])
	}

	var requestBody map[string]interface{}
	var url string

	if client.Provider == "anthropic" {
		// Anthropic Claude API 格式
		// - system prompt 作为独立字段
		// - messages 只包含 user 消息
		// - 端点是 /messages
		messages := []map[string]string{
			{"role": "user", "content": userPrompt},
		}

		requestBody = map[string]interface{}{
			"model":       client.Model,
			"messages":    messages,
			"temperature": client.Temperature,
			"max_tokens":  client.MaxTokens,
		}

		// Anthropic 的 system prompt 作为独立字段
		if systemPrompt != "" {
			requestBody["system"] = systemPrompt
		}

		baseURL := strings.TrimSuffix(client.BaseURL, "/")
		url = fmt.Sprintf("%s/messages", baseURL)
	} else {
		// OpenAI 兼容格式（包括 DeepSeek, Qwen, Gemini, Groq 等）
		messages := []map[string]string{}
		if systemPrompt != "" {
			messages = append(messages, map[string]string{
				"role":    "system",
				"content": systemPrompt,
			})
		}
		messages = append(messages, map[string]string{
			"role":    "user",
			"content": userPrompt,
		})

		requestBody = map[string]interface{}{
			"model":       client.Model,
			"messages":    messages,
			"temperature": client.Temperature,
			"max_tokens":  client.MaxTokens,
		}

		if client.UseFullURL {
			url = client.BaseURL
		} else {
			baseURL := strings.TrimSuffix(client.BaseURL, "/")
			url = fmt.Sprintf("%s/chat/completions", baseURL)
		}
	}

	log.Printf("📡 [MCP] 请求参数: max_tokens=%d, temperature=%.1f", client.MaxTokens, client.Temperature)

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	log.Printf("📡 [MCP] 请求 URL: %s", url)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client.setAuthHeader(req.Header)

	// 发送请求
	httpClient := &http.Client{Timeout: client.Timeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API返回错误 (status %d): %s", resp.StatusCode, string(body))
	}

	// 根据 provider 解析不同响应格式
	if client.Provider == "anthropic" {
		// Anthropic 响应格式: content[0].text
		var anthropicResult struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			StopReason string `json:"stop_reason"`
			Usage      struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}

		if err := json.Unmarshal(body, &anthropicResult); err != nil {
			return "", fmt.Errorf("解析Anthropic响应失败: %w", err)
		}

		if len(anthropicResult.Content) == 0 {
			return "", fmt.Errorf("Anthropic API返回空响应")
		}

		// 打印响应详情
		log.Printf("📡 [MCP] Anthropic响应详情: stop_reason=%s, input_tokens=%d, output_tokens=%d",
			anthropicResult.StopReason,
			anthropicResult.Usage.InputTokens,
			anthropicResult.Usage.OutputTokens)

		// 检查是否因为长度限制而截断
		if anthropicResult.StopReason == "max_tokens" {
			log.Printf("⚠️  [MCP] 警告: AI响应因max_tokens限制被截断！当前max_tokens=%d, 实际使用output_tokens=%d",
				client.MaxTokens, anthropicResult.Usage.OutputTokens)
		}

		return anthropicResult.Content[0].Text, nil
	}

	// OpenAI 兼容格式响应解析
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("API返回空响应")
	}

	// 打印响应详情
	log.Printf("📡 [MCP] 响应详情: finish_reason=%s, prompt_tokens=%d, completion_tokens=%d, total_tokens=%d",
		result.Choices[0].FinishReason,
		result.Usage.PromptTokens,
		result.Usage.CompletionTokens,
		result.Usage.TotalTokens)

	// 检查是否因为长度限制而截断
	if result.Choices[0].FinishReason == "length" {
		log.Printf("⚠️  [MCP] 警告: AI响应因max_tokens限制被截断！当前max_tokens=%d, 实际使用completion_tokens=%d",
			client.MaxTokens, result.Usage.CompletionTokens)
	}

	return result.Choices[0].Message.Content, nil
}

// isRetryableError 判断错误是否可重试
func isRetryableError(err error) bool {
	errStr := err.Error()
	// 网络错误、超时、EOF等可以重试
	retryableErrors := []string{
		"EOF",
		"timeout",
		"connection reset",
		"connection refused",
		"temporary failure",
		"no such host",
		"stream error",   // HTTP/2 stream 错误
		"INTERNAL_ERROR", // 服务端内部错误
	}
	for _, retryable := range retryableErrors {
		if strings.Contains(errStr, retryable) {
			return true
		}
	}
	return false
}
