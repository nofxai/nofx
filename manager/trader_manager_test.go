package manager

import (
	"nofx/config"
	"nofx/trader"
	"testing"
	"time"
)

// TestRemoveTrader_Scenarios 测试移除 Trader 的不同场景
func TestRemoveTrader_Scenarios(t *testing.T) {
	tests := []struct {
		name          string
		traderID      string
		setupRunning  bool
		scanInterval  time.Duration
		expectExists  bool
		expectRunning bool
	}{
		{
			name:          "Remove idle trader",
			traderID:      "test-trader-idle",
			setupRunning:  false,
			scanInterval:  1 * time.Minute,
			expectExists:  false,
			expectRunning: false,
		},
		{
			name:          "Remove running trader",
			traderID:      "test-trader-running",
			setupRunning:  true,
			scanInterval:  100 * time.Millisecond,
			expectExists:  false,
			expectRunning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTraderManager()

			// 创建 AutoTrader 实例
			cfg := trader.AutoTraderConfig{
				ID:             tt.traderID,
				Name:           "Test Trader",
				InitialBalance: 1000,
				ScanInterval:   tt.scanInterval,
			}
			at, _ := trader.NewAutoTrader(cfg, nil, "user1")
			tm.traders[tt.traderID] = at

			// 如果需要，启动 Trader
			if tt.setupRunning {
				go at.Run()
				time.Sleep(50 * time.Millisecond) // 等待启动
				if !at.IsRunning() {
					t.Fatal("Trader 应该是运行状态")
				}
			}

			// 执行移除
			tm.RemoveTrader(tt.traderID)

			// 验证是否存在
			if _, exists := tm.traders[tt.traderID]; exists != tt.expectExists {
				t.Errorf("Trader 存在状态错误: got %v, want %v", exists, tt.expectExists)
			}

			// 验证是否运行
			if at.IsRunning() != tt.expectRunning {
				t.Errorf("Trader 运行状态错误: got %v, want %v", at.IsRunning(), tt.expectRunning)
			}
		})
	}
}

// TestRemoveTrader_NonExistent 测试移除不存在的trader不会报错
func TestRemoveTrader_NonExistent(t *testing.T) {
	tm := NewTraderManager()

	// 尝试移除不存在的 trader，不应该 panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("移除不存在的 trader 不应该 panic: %v", r)
		}
	}()

	tm.RemoveTrader("non-existent-trader")
}

// TestRemoveTrader_Concurrent 测试并发移除trader的安全性
func TestRemoveTrader_Concurrent(t *testing.T) {
	tm := NewTraderManager()
	traderID := "test-trader-concurrent"

	// 添加 trader
	tm.traders[traderID] = nil

	// 并发调用 RemoveTrader
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			tm.RemoveTrader(traderID)
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证 trader 已被移除
	if _, exists := tm.traders[traderID]; exists {
		t.Error("trader 应该已从 map 中移除")
	}
}

// TestGetTrader_AfterRemove 测试移除后获取trader返回错误
func TestGetTrader_AfterRemove(t *testing.T) {
	tm := NewTraderManager()
	traderID := "test-trader-get"

	// 添加 trader
	tm.traders[traderID] = nil

	// 移除 trader
	tm.RemoveTrader(traderID)

	// 尝试获取已移除的 trader
	_, err := tm.GetTrader(traderID)
	if err == nil {
		t.Error("获取已移除的 trader 应该返回错误")
	}
}

// TestAddTraderFromDB_Providers 测试不同 AI Provider 的配置加载
func TestAddTraderFromDB_Providers(t *testing.T) {
	tests := []struct {
		name            string
		provider        string
		apiKey          string
		customURL       string
		customModel     string
		expectAIModel   string
		expectCustomKey string
		expectCustomURL string
		expectModelName string
	}{
		{
			name:            "OpenAI Provider",
			provider:        "openai",
			apiKey:          "test-api-key-12345",
			customURL:       "https://api.openai.com/v1",
			customModel:     "gpt-4",
			expectAIModel:   "openai",
			expectCustomKey: "test-api-key-12345",
			expectCustomURL: "https://api.openai.com/v1",
			expectModelName: "gpt-4",
		},
		{
			name:            "Anthropic Provider",
			provider:        "anthropic",
			apiKey:          "test-anthropic-key",
			customURL:       "https://api.anthropic.com/v1",
			customModel:     "claude-3-opus",
			expectAIModel:   "anthropic",
			expectCustomKey: "test-anthropic-key",
			expectCustomURL: "https://api.anthropic.com/v1",
			expectModelName: "claude-3-opus",
		},
		{
			name:            "Gemini Provider",
			provider:        "gemini",
			apiKey:          "test-gemini-key-12345",
			customURL:       "https://generativelanguage.googleapis.com/v1beta/openai",
			customModel:     "gemini-2.0-flash-exp",
			expectAIModel:   "gemini",
			expectCustomKey: "test-gemini-key-12345",
			expectCustomURL: "https://generativelanguage.googleapis.com/v1beta/openai",
			expectModelName: "gemini-2.0-flash-exp",
		},
		{
			name:            "Groq Provider",
			provider:        "groq",
			apiKey:          "test-groq-key-67890",
			customURL:       "https://api.groq.com/openai/v1",
			customModel:     "llama-3.3-70b-versatile",
			expectAIModel:   "groq",
			expectCustomKey: "test-groq-key-67890",
			expectCustomURL: "https://api.groq.com/openai/v1",
			expectModelName: "llama-3.3-70b-versatile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTraderManager()
			traderID := "test-trader-" + tt.provider

			// 使用辅助函数创建测试配置
			traderCfg, aiModelCfg, exchangeCfg := createTestConfigs(tt.provider, tt.apiKey, tt.customURL, tt.customModel)

			// 调用 addTraderFromDB
			err := tm.addTraderFromDB(
				traderCfg,
				aiModelCfg,
				exchangeCfg,
				"", "", 10.0, 20.0, 60,
				[]string{"BTC", "ETH"},
				nil,
				"test-user",
			)

			if err != nil {
				t.Fatalf("添加交易员失败: %v", err)
			}

			// 验证交易员已添加
			at, err := tm.GetTrader(traderID)
			if err != nil {
				t.Fatalf("获取交易员失败: %v", err)
			}
			if at == nil {
				t.Fatal("交易员不应为 nil")
			}

			// 验证配置
			config := at.GetConfig()

			if config.AIModel != tt.expectAIModel {
				t.Errorf("AIModel 应该是 '%s'，实际是 '%s'", tt.expectAIModel, config.AIModel)
			}

			if config.CustomAPIKey != tt.expectCustomKey {
				t.Errorf("CustomAPIKey 应该是 '%s'，实际是 '%s'", tt.expectCustomKey, config.CustomAPIKey)
			}

			if tt.expectCustomURL != "" && config.CustomAPIURL != tt.expectCustomURL {
				t.Errorf("CustomAPIURL 应该是 '%s'，实际是 '%s'", tt.expectCustomURL, config.CustomAPIURL)
			}

			if tt.expectModelName != "" && config.CustomModelName != tt.expectModelName {
				t.Errorf("CustomModelName 应该是 '%s'，实际是 '%s'", tt.expectModelName, config.CustomModelName)
			}
		})
	}
}

// createTestConfigs 辅助函数：创建测试所需的配置对象，减少代码重复
func createTestConfigs(provider, apiKey, customURL, customModel string) (*config.TraderRecord, *config.AIModelConfig, *config.ExchangeConfig) {
	traderID := "test-trader-" + provider
	modelID := "test-model-" + provider

	traderCfg := &config.TraderRecord{
		ID:                  traderID,
		UserID:              "test-user",
		Name:                "Test Trader",
		AIModelID:           modelID,
		ExchangeID:          "binance",
		InitialBalance:      10000,
		ScanIntervalMinutes: 3,
		IsRunning:           false,
		BTCETHLeverage:      10,
		AltcoinLeverage:     5,
		IsCrossMargin:       true,
		TradingSymbols:      "BTC,ETH",
	}

	aiModelCfg := &config.AIModelConfig{
		ID:              modelID,
		UserID:          "test-user",
		Name:            "Test AI",
		Provider:        provider,
		Enabled:         true,
		APIKey:          apiKey,
		CustomAPIURL:    customURL,
		CustomModelName: customModel,
	}

	exchangeCfg := &config.ExchangeConfig{
		ID:        "binance",
		UserID:    "test-user",
		Name:      "Binance",
		Type:      "binance",
		Enabled:   true,
		APIKey:    "binance-api-key",
		SecretKey: "binance-secret-key",
		Testnet:   false,
	}

	return traderCfg, aiModelCfg, exchangeCfg
}
