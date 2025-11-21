package manager

import (
	"nofx/config"
	"testing"
)

// TestRemoveTrader 测试从内存中移除trader
func TestRemoveTrader(t *testing.T) {
	tm := NewTraderManager()

	// 创建一个模拟的 trader 并添加到 map
	traderID := "test-trader-123"
	tm.traders[traderID] = nil // 使用 nil 作为占位符，实际测试中只需验证删除逻辑

	// 验证 trader 存在
	if _, exists := tm.traders[traderID]; !exists {
		t.Fatal("trader 应该存在于 map 中")
	}

	// 调用 RemoveTrader
	tm.RemoveTrader(traderID)

	// 验证 trader 已被移除
	if _, exists := tm.traders[traderID]; exists {
		t.Error("trader 应该已从 map 中移除")
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

// TestAddTraderFromDB_OpenAIProvider 测试添加使用 OpenAI provider 的交易员时正确设置 API Key
func TestAddTraderFromDB_OpenAIProvider(t *testing.T) {
	tm := NewTraderManager()

	// 准备测试数据
	traderCfg := &config.TraderRecord{
		ID:                  "test-trader-openai",
		UserID:              "test-user",
		Name:                "Test OpenAI Trader",
		AIModelID:           "test-openai-model",
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
		ID:              "test-openai-model",
		UserID:          "test-user",
		Name:            "Test OpenAI",
		Provider:        "openai",
		Enabled:         true,
		APIKey:          "test-api-key-12345",
		CustomAPIURL:    "https://api.openai.com/v1",
		CustomModelName: "gpt-4",
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

	// 调用 addTraderFromDB (内部方法，需要先获取锁)
	err := tm.addTraderFromDB(
		traderCfg,
		aiModelCfg,
		exchangeCfg,
		"",    // coinPoolURL
		"",    // oiTopURL
		10.0,  // maxDailyLoss
		20.0,  // maxDrawdown
		60,    // stopTradingMinutes
		[]string{"BTC", "ETH"}, // defaultCoins
		nil,   // database (可以为 nil，因为我们只测试配置)
		"test-user",
	)

	if err != nil {
		t.Fatalf("添加交易员失败: %v", err)
	}

	// 验证交易员已添加
	at, err := tm.GetTrader("test-trader-openai")
	if err != nil {
		t.Fatalf("获取交易员失败: %v", err)
	}

	if at == nil {
		t.Fatal("交易员不应为 nil")
	}

	// 验证配置是否正确设置
	config := at.GetConfig()

	// 关键验证：OpenAI provider 应该被设置为 AIModel="openai"，并且 CustomAPIKey 应该被设置
	if config.AIModel != "openai" {
		t.Errorf("AIModel 应该是 'openai'，实际是 '%s'", config.AIModel)
	}

	if config.CustomAPIKey == "" {
		t.Error("CustomAPIKey 不应为空，OpenAI provider 的 API Key 应该被正确设置")
	}

	if config.CustomAPIKey != "test-api-key-12345" {
		t.Errorf("CustomAPIKey 应该是 'test-api-key-12345'，实际是 '%s'", config.CustomAPIKey)
	}

	if config.CustomAPIURL != "https://api.openai.com/v1" {
		t.Errorf("CustomAPIURL 应该是 'https://api.openai.com/v1'，实际是 '%s'", config.CustomAPIURL)
	}

	if config.CustomModelName != "gpt-4" {
		t.Errorf("CustomModelName 应该是 'gpt-4'，实际是 '%s'", config.CustomModelName)
	}
}

// TestAddTraderFromDB_AnthropicProvider 测试添加使用 Anthropic provider 的交易员
func TestAddTraderFromDB_AnthropicProvider(t *testing.T) {
	tm := NewTraderManager()

	traderCfg := &config.TraderRecord{
		ID:                  "test-trader-anthropic",
		UserID:              "test-user",
		Name:                "Test Anthropic Trader",
		AIModelID:           "test-anthropic-model",
		ExchangeID:          "binance",
		InitialBalance:      10000,
		ScanIntervalMinutes: 3,
		BTCETHLeverage:      10,
		AltcoinLeverage:     5,
		IsCrossMargin:       true,
	}

	aiModelCfg := &config.AIModelConfig{
		ID:              "test-anthropic-model",
		UserID:          "test-user",
		Name:            "Test Anthropic",
		Provider:        "anthropic",
		Enabled:         true,
		APIKey:          "test-anthropic-key",
		CustomAPIURL:    "https://api.anthropic.com/v1",
		CustomModelName: "claude-3-opus",
	}

	exchangeCfg := &config.ExchangeConfig{
		ID:        "binance",
		UserID:    "test-user",
		Name:      "Binance",
		Type:      "binance",
		Enabled:   true,
		APIKey:    "binance-api-key",
		SecretKey: "binance-secret-key",
	}

	err := tm.addTraderFromDB(
		traderCfg,
		aiModelCfg,
		exchangeCfg,
		"", "", 10.0, 20.0, 60,
		[]string{},
		nil,
		"test-user",
	)

	if err != nil {
		t.Fatalf("添加交易员失败: %v", err)
	}

	at, _ := tm.GetTrader("test-trader-anthropic")
	config := at.GetConfig()

	if config.AIModel != "anthropic" {
		t.Errorf("AIModel 应该是 'anthropic'，实际是 '%s'", config.AIModel)
	}

	if config.CustomAPIKey != "test-anthropic-key" {
		t.Errorf("CustomAPIKey 应该是 'test-anthropic-key'，实际是 '%s'", config.CustomAPIKey)
	}
}
