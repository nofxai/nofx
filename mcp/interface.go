package mcp

import "net/http"

// AIClient AI客户端接口
type AIClient interface {
	SetAPIKey(apiKey string, customURL string, customModel string, provider string)
	// SetTemperature 设置 AI 温度参数（0.0-1.0），控制输出随机性
	SetTemperature(temperature float64)
	// CallWithMessages 使用 system + user prompt 调用AI API
	CallWithMessages(systemPrompt, userPrompt string) (string, error)

	setAuthHeader(reqHeaders http.Header)
}
