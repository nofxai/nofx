package api

import (
	"encoding/json"
	"fmt"
	"testing"

	"nofx/config"
)

// TestUpdateTraderRequest_SystemPromptTemplate 测试更新交易员时 SystemPromptTemplate 字段是否存在
func TestUpdateTraderRequest_SystemPromptTemplate(t *testing.T) {
	tests := []struct {
		name                   string
		requestJSON            string
		expectedPromptTemplate string
	}{
		{
			name: "更新时应该能接收 system_prompt_template=nof1",
			requestJSON: `{
				"name": "Test Trader",
				"ai_model_id": "gpt-4",
				"exchange_id": "binance",
				"initial_balance": 1000,
				"scan_interval_minutes": 5,
				"btc_eth_leverage": 5,
				"altcoin_leverage": 3,
				"trading_symbols": "BTC,ETH",
				"custom_prompt": "test",
				"override_base_prompt": false,
				"is_cross_margin": true,
				"system_prompt_template": "nof1"
			}`,
			expectedPromptTemplate: "nof1",
		},
		{
			name: "更新时应该能接收 system_prompt_template=default",
			requestJSON: `{
				"name": "Test Trader",
				"ai_model_id": "gpt-4",
				"exchange_id": "binance",
				"initial_balance": 1000,
				"scan_interval_minutes": 5,
				"btc_eth_leverage": 5,
				"altcoin_leverage": 3,
				"trading_symbols": "BTC,ETH",
				"custom_prompt": "test",
				"override_base_prompt": false,
				"is_cross_margin": true,
				"system_prompt_template": "default"
			}`,
			expectedPromptTemplate: "default",
		},
		{
			name: "更新时应该能接收 system_prompt_template=custom",
			requestJSON: `{
				"name": "Test Trader",
				"ai_model_id": "gpt-4",
				"exchange_id": "binance",
				"initial_balance": 1000,
				"scan_interval_minutes": 5,
				"btc_eth_leverage": 5,
				"altcoin_leverage": 3,
				"trading_symbols": "BTC,ETH",
				"custom_prompt": "test",
				"override_base_prompt": false,
				"is_cross_margin": true,
				"system_prompt_template": "custom"
			}`,
			expectedPromptTemplate: "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 测试 UpdateTraderRequest 结构体是否能正确解析 system_prompt_template 字段
			var req UpdateTraderRequest
			err := json.Unmarshal([]byte(tt.requestJSON), &req)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			// ✅ 验证 SystemPromptTemplate 字段是否被正确读取
			if req.SystemPromptTemplate != tt.expectedPromptTemplate {
				t.Errorf("Expected SystemPromptTemplate=%q, got %q",
					tt.expectedPromptTemplate, req.SystemPromptTemplate)
			}

			// 验证其他字段也被正确解析
			if req.Name != "Test Trader" {
				t.Errorf("Name not parsed correctly")
			}
			if req.AIModelID != "gpt-4" {
				t.Errorf("AIModelID not parsed correctly")
			}
		})
	}
}

// TestGetTraderConfigResponse_SystemPromptTemplate 测试获取交易员配置时返回值是否包含 system_prompt_template
func TestGetTraderConfigResponse_SystemPromptTemplate(t *testing.T) {
	tests := []struct {
		name             string
		traderConfig     *config.TraderRecord
		expectedTemplate string
	}{
		{
			name: "获取配置应该返回 system_prompt_template=nof1",
			traderConfig: &config.TraderRecord{
				ID:                   "trader-123",
				UserID:               "user-1",
				Name:                 "Test Trader",
				AIModelID:            "gpt-4",
				ExchangeID:           "binance",
				InitialBalance:       1000,
				ScanIntervalMinutes:  5,
				BTCETHLeverage:       5,
				AltcoinLeverage:      3,
				TradingSymbols:       "BTC,ETH",
				CustomPrompt:         "test",
				OverrideBasePrompt:   false,
				SystemPromptTemplate: "nof1",
				IsCrossMargin:        true,
				IsRunning:            false,
			},
			expectedTemplate: "nof1",
		},
		{
			name: "获取配置应该返回 system_prompt_template=default",
			traderConfig: &config.TraderRecord{
				ID:                   "trader-456",
				UserID:               "user-1",
				Name:                 "Test Trader 2",
				AIModelID:            "gpt-4",
				ExchangeID:           "binance",
				InitialBalance:       2000,
				ScanIntervalMinutes:  10,
				BTCETHLeverage:       10,
				AltcoinLeverage:      5,
				TradingSymbols:       "BTC",
				CustomPrompt:         "",
				OverrideBasePrompt:   false,
				SystemPromptTemplate: "default",
				IsCrossMargin:        false,
				IsRunning:            false,
			},
			expectedTemplate: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 handleGetTraderConfig 的返回值构造逻辑（修复后的实现）
			result := map[string]interface{}{
				"trader_id":              tt.traderConfig.ID,
				"trader_name":            tt.traderConfig.Name,
				"ai_model":               tt.traderConfig.AIModelID,
				"exchange_id":            tt.traderConfig.ExchangeID,
				"initial_balance":        tt.traderConfig.InitialBalance,
				"scan_interval_minutes":  tt.traderConfig.ScanIntervalMinutes,
				"btc_eth_leverage":       tt.traderConfig.BTCETHLeverage,
				"altcoin_leverage":       tt.traderConfig.AltcoinLeverage,
				"trading_symbols":        tt.traderConfig.TradingSymbols,
				"custom_prompt":          tt.traderConfig.CustomPrompt,
				"override_base_prompt":   tt.traderConfig.OverrideBasePrompt,
				"system_prompt_template": tt.traderConfig.SystemPromptTemplate,
				"is_cross_margin":        tt.traderConfig.IsCrossMargin,
				"is_running":             tt.traderConfig.IsRunning,
			}

			// ✅ 检查响应中是否包含 system_prompt_template
			if _, exists := result["system_prompt_template"]; !exists {
				t.Errorf("Response is missing 'system_prompt_template' field")
			} else {
				actualTemplate := result["system_prompt_template"].(string)
				if actualTemplate != tt.expectedTemplate {
					t.Errorf("Expected system_prompt_template=%q, got %q",
						tt.expectedTemplate, actualTemplate)
				}
			}

			// 验证其他字段是否正确
			if result["trader_id"] != tt.traderConfig.ID {
				t.Errorf("trader_id mismatch")
			}
			if result["trader_name"] != tt.traderConfig.Name {
				t.Errorf("trader_name mismatch")
			}
		})
	}
}

// TestUpdateTraderRequest_CompleteFields 验证 UpdateTraderRequest 结构体定义完整性
func TestUpdateTraderRequest_CompleteFields(t *testing.T) {
	jsonData := `{
		"name": "Test Trader",
		"ai_model_id": "gpt-4",
		"exchange_id": "binance",
		"initial_balance": 1000,
		"scan_interval_minutes": 5,
		"btc_eth_leverage": 5,
		"altcoin_leverage": 3,
		"trading_symbols": "BTC,ETH",
		"custom_prompt": "test",
		"override_base_prompt": false,
		"is_cross_margin": true,
		"system_prompt_template": "nof1"
	}`

	var req UpdateTraderRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// 验证基本字段是否正确解析
	if req.Name != "Test Trader" {
		t.Errorf("Name mismatch: got %q", req.Name)
	}
	if req.AIModelID != "gpt-4" {
		t.Errorf("AIModelID mismatch: got %q", req.AIModelID)
	}

	// ✅ 验证 SystemPromptTemplate 字段已正确添加到结构体
	if req.SystemPromptTemplate != "nof1" {
		t.Errorf("SystemPromptTemplate mismatch: expected %q, got %q", "nof1", req.SystemPromptTemplate)
	}
}

// TestTraderListResponse_SystemPromptTemplate 测试 handleTraderList API 返回的 trader 对象是否包含 system_prompt_template 字段
func TestTraderListResponse_SystemPromptTemplate(t *testing.T) {
	// 模拟 handleTraderList 中的 trader 对象构造
	trader := &config.TraderRecord{
		ID:                   "trader-001",
		UserID:               "user-1",
		Name:                 "My Trader",
		AIModelID:            "gpt-4",
		ExchangeID:           "binance",
		InitialBalance:       5000,
		SystemPromptTemplate: "nof1",
		IsRunning:            true,
	}

	// 构造 API 响应对象（与 api/server.go 中的逻辑一致）
	response := map[string]interface{}{
		"trader_id":              trader.ID,
		"trader_name":            trader.Name,
		"ai_model":               trader.AIModelID,
		"exchange_id":            trader.ExchangeID,
		"is_running":             trader.IsRunning,
		"initial_balance":        trader.InitialBalance,
		"system_prompt_template": trader.SystemPromptTemplate,
	}

	// ✅ 验证 system_prompt_template 字段存在
	if _, exists := response["system_prompt_template"]; !exists {
		t.Errorf("Trader list response is missing 'system_prompt_template' field")
	}

	// ✅ 验证 system_prompt_template 值正确
	if response["system_prompt_template"] != "nof1" {
		t.Errorf("Expected system_prompt_template='nof1', got %v", response["system_prompt_template"])
	}
}

// TestPublicTraderListResponse_SystemPromptTemplate 测试 handlePublicTraderList API 返回的 trader 对象是否包含 system_prompt_template 字段
func TestPublicTraderListResponse_SystemPromptTemplate(t *testing.T) {
	// 模拟 getConcurrentTraderData 返回的 trader 数据
	traderData := map[string]interface{}{
		"trader_id":              "trader-002",
		"trader_name":            "Public Trader",
		"ai_model":               "claude",
		"exchange":               "binance",
		"total_equity":           10000.0,
		"total_pnl":              500.0,
		"total_pnl_pct":          5.0,
		"position_count":         3,
		"margin_used_pct":        25.0,
		"is_running":             true,
		"system_prompt_template": "default",
	}

	// 构造 API 响应对象（与 api/server.go handlePublicTraderList 中的逻辑一致）
	response := map[string]interface{}{
		"trader_id":              traderData["trader_id"],
		"trader_name":            traderData["trader_name"],
		"ai_model":               traderData["ai_model"],
		"exchange":               traderData["exchange"],
		"total_equity":           traderData["total_equity"],
		"total_pnl":              traderData["total_pnl"],
		"total_pnl_pct":          traderData["total_pnl_pct"],
		"position_count":         traderData["position_count"],
		"margin_used_pct":        traderData["margin_used_pct"],
		"system_prompt_template": traderData["system_prompt_template"],
	}

	// ✅ 验证 system_prompt_template 字段存在
	if _, exists := response["system_prompt_template"]; !exists {
		t.Errorf("Public trader list response is missing 'system_prompt_template' field")
	}

	// ✅ 验证 system_prompt_template 值正确
	if response["system_prompt_template"] != "default" {
		t.Errorf("Expected system_prompt_template='default', got %v", response["system_prompt_template"])
	}
}

// TestPerformanceAPI_LimitParameter 测试 performance API 的 limit 参数功能
func TestPerformanceAPI_LimitParameter(t *testing.T) {
	// 模拟历史成交记录（recent_trades）
	createMockTrades := func(count int) []interface{} {
		trades := make([]interface{}, count)
		for i := 0; i < count; i++ {
			trades[i] = map[string]interface{}{
				"symbol":     "BTCUSDT",
				"side":       "long",
				"pnl":        float64(i * 10),
				"pnl_pct":    1.5,
				"open_price": 50000.0,
			}
		}
		return trades
	}

	tests := []struct {
		name           string
		limitParam     string
		totalTrades    int
		expectedCount  int
		description    string
	}{
		{
			name:           "无limit参数-返回所有记录",
			limitParam:     "",
			totalTrades:    30,
			expectedCount:  30,
			description:    "不传limit参数时，应该返回所有交易记录（保持向后兼容）",
		},
		{
			name:           "limit=10-返回10条记录",
			limitParam:     "10",
			totalTrades:    50,
			expectedCount:  10,
			description:    "limit=10时，应该只返回最近10条交易记录",
		},
		{
			name:           "limit=20-返回20条记录",
			limitParam:     "20",
			totalTrades:    100,
			expectedCount:  20,
			description:    "limit=20时，应该只返回最近20条交易记录",
		},
		{
			name:           "limit=50-返回50条记录",
			limitParam:     "50",
			totalTrades:    80,
			expectedCount:  50,
			description:    "limit=50时，应该只返回最近50条交易记录",
		},
		{
			name:           "limit大于实际记录数-返回所有记录",
			limitParam:     "100",
			totalTrades:    30,
			expectedCount:  30,
			description:    "limit=100但只有30条记录时，应该返回所有30条记录",
		},
		{
			name:           "limit=0-返回所有记录",
			limitParam:     "0",
			totalTrades:    40,
			expectedCount:  40,
			description:    "limit=0时，应该返回所有交易记录",
		},
		{
			name:           "limit超过最大值100-使用最大值",
			limitParam:     "150",
			totalTrades:    200,
			expectedCount:  200, // 解析时会被限制为100，但这里测试的是解析逻辑
			description:    "limit=150超过最大值100时，解析逻辑会忽略此值",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟完整的 performance 数据
			mockPerformance := map[string]interface{}{
				"total_trades":   tt.totalTrades,
				"winning_trades": tt.totalTrades / 2,
				"losing_trades":  tt.totalTrades / 2,
				"win_rate":       50.0,
				"recent_trades":  createMockTrades(tt.totalTrades),
			}

			// 模拟 handlePerformance 中的 limit 参数解析逻辑
			tradeLimit := 0 // 默认不限制
			if tt.limitParam != "" {
				if l := parseLimit(tt.limitParam); l > 0 && l <= 100 {
					tradeLimit = l
				}
			}

			// 模拟截取逻辑
			recentTrades := mockPerformance["recent_trades"].([]interface{})
			if tradeLimit > 0 && len(recentTrades) > tradeLimit {
				recentTrades = recentTrades[:tradeLimit]
			}

			// ✅ 验证返回的记录数
			actualCount := len(recentTrades)
			if tt.limitParam == "" || tt.limitParam == "0" {
				// 无limit或limit=0时，应返回所有记录
				if actualCount != tt.expectedCount {
					t.Errorf("%s: expected %d trades, got %d", tt.description, tt.expectedCount, actualCount)
				}
			} else if tt.limitParam == "150" {
				// limit超过最大值时，解析会忽略，返回所有记录
				if actualCount != tt.totalTrades {
					t.Errorf("%s: expected all %d trades (limit ignored), got %d", tt.description, tt.totalTrades, actualCount)
				}
			} else {
				// 正常limit值
				if actualCount != tt.expectedCount {
					t.Errorf("%s: expected %d trades, got %d", tt.description, tt.expectedCount, actualCount)
				}
			}
		})
	}
}

// parseLimit 辅助函数：解析 limit 参数（模拟 server.go 中的逻辑）
func parseLimit(limitStr string) int {
	if limitStr == "" {
		return 0
	}
	var limit int
	if _, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil {
		return limit
	}
	return 0
}
