package decision

import (
	"testing"
)

// TestLeverageValidation 测试杠杆验证（超限时拒绝决策）
func TestLeverageValidation(t *testing.T) {
	tests := []struct {
		name            string
		decision        Decision
		accountEquity   float64
		btcEthLeverage  int
		altcoinLeverage int
		wantError       bool
		errorMsg        string
	}{
		{
			name: "山寨币杠杆超限_应该报错",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        20, // 超过上限
				PositionSizeUSD: 100,
				StopLoss:        50,
				TakeProfit:      200,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5, // 上限 5x
			wantError:       true,
			errorMsg:        "杠杆超限",
		},
		{
			name: "BTC杠杆超限_应该报错",
			decision: Decision{
				Symbol:          "BTCUSDT",
				Action:          "open_long",
				Leverage:        20, // 超过上限
				PositionSizeUSD: 1000,
				StopLoss:        90000,
				TakeProfit:      110000,
			},
			accountEquity:   100,
			btcEthLeverage:  10, // 上限 10x
			altcoinLeverage: 5,
			wantError:       true,
			errorMsg:        "杠杆超限",
		},
		{
			name: "杠杆在上限内_应该通过",
			decision: Decision{
				Symbol:          "ETHUSDT",
				Action:          "open_short",
				Leverage:        5, // 未超限
				PositionSizeUSD: 500,
				StopLoss:        4000,
				TakeProfit:      3000,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			wantError:       false,
		},
		{
			name: "杠杆为0_应该报错",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        0, // 无效
				PositionSizeUSD: 100,
				StopLoss:        50,
				TakeProfit:      200,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			wantError:       true,
			errorMsg:        "杠杆必须大于0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDecision(&tt.decision, tt.accountEquity, tt.btcEthLeverage, tt.altcoinLeverage, "hyperliquid")

			// 检查错误状态
			if (err != nil) != tt.wantError {
				t.Errorf("validateDecision() error = %v, wantError %v", err, tt.wantError)
				return
			}

			// 如果期望报错，检查错误消息
			if tt.wantError && tt.errorMsg != "" {
				if err == nil || !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got: %v", tt.errorMsg, err)
				}
			}
		})
	}
}

// TestUpdateStopLossValidation 测试 update_stop_loss 动作的字段验证
func TestUpdateStopLossValidation(t *testing.T) {
	tests := []struct {
		name      string
		decision  Decision
		wantError bool
		errorMsg  string
	}{
		{
			name: "正确使用new_stop_loss字段",
			decision: Decision{
				Symbol:      "SOLUSDT",
				Action:      "update_stop_loss",
				NewStopLoss: 155.5,
				Reasoning:   "移动止损至保本位",
			},
			wantError: false,
		},
		{
			name: "new_stop_loss为0应该报错",
			decision: Decision{
				Symbol:      "SOLUSDT",
				Action:      "update_stop_loss",
				NewStopLoss: 0,
				Reasoning:   "测试错误情况",
			},
			wantError: true,
			errorMsg:  "新止损价格必须大于0",
		},
		{
			name: "new_stop_loss为负数应该报错",
			decision: Decision{
				Symbol:      "SOLUSDT",
				Action:      "update_stop_loss",
				NewStopLoss: -100,
				Reasoning:   "测试错误情况",
			},
			wantError: true,
			errorMsg:  "新止损价格必须大于0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDecision(&tt.decision, 1000.0, 10, 5, "hyperliquid")

			if (err != nil) != tt.wantError {
				t.Errorf("validateDecision() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if tt.wantError && err != nil {
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("错误信息不匹配: got %q, want to contain %q", err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

// TestUpdateTakeProfitValidation 测试 update_take_profit 动作的字段验证
func TestUpdateTakeProfitValidation(t *testing.T) {
	tests := []struct {
		name      string
		decision  Decision
		wantError bool
		errorMsg  string
	}{
		{
			name: "正确使用new_take_profit字段",
			decision: Decision{
				Symbol:        "BTCUSDT",
				Action:        "update_take_profit",
				NewTakeProfit: 98000,
				Reasoning:     "调整止盈至关键阻力位",
			},
			wantError: false,
		},
		{
			name: "new_take_profit为0应该报错",
			decision: Decision{
				Symbol:        "BTCUSDT",
				Action:        "update_take_profit",
				NewTakeProfit: 0,
				Reasoning:     "测试错误情况",
			},
			wantError: true,
			errorMsg:  "新止盈价格必须大于0",
		},
		{
			name: "new_take_profit为负数应该报错",
			decision: Decision{
				Symbol:        "BTCUSDT",
				Action:        "update_take_profit",
				NewTakeProfit: -1000,
				Reasoning:     "测试错误情况",
			},
			wantError: true,
			errorMsg:  "新止盈价格必须大于0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDecision(&tt.decision, 1000.0, 10, 5, "hyperliquid")

			if (err != nil) != tt.wantError {
				t.Errorf("validateDecision() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if tt.wantError && err != nil {
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("错误信息不匹配: got %q, want to contain %q", err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

// TestPartialCloseValidation 测试 partial_close 动作的字段验证
func TestPartialCloseValidation(t *testing.T) {
	tests := []struct {
		name      string
		decision  Decision
		wantError bool
		errorMsg  string
	}{
		{
			name: "正确使用close_percentage字段",
			decision: Decision{
				Symbol:          "ETHUSDT",
				Action:          "partial_close",
				ClosePercentage: 50.0,
				Reasoning:       "锁定一半利润",
			},
			wantError: false,
		},
		{
			name: "close_percentage为0应该报错",
			decision: Decision{
				Symbol:          "ETHUSDT",
				Action:          "partial_close",
				ClosePercentage: 0,
				Reasoning:       "测试错误情况",
			},
			wantError: true,
			errorMsg:  "平仓百分比必须在0-100之间",
		},
		{
			name: "close_percentage超过100应该报错",
			decision: Decision{
				Symbol:          "ETHUSDT",
				Action:          "partial_close",
				ClosePercentage: 150,
				Reasoning:       "测试错误情况",
			},
			wantError: true,
			errorMsg:  "平仓百分比必须在0-100之间",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDecision(&tt.decision, 1000.0, 10, 5, "hyperliquid")

			if (err != nil) != tt.wantError {
				t.Errorf("validateDecision() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if tt.wantError && err != nil {
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("错误信息不匹配: got %q, want to contain %q", err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

// TestMinimumPositionSize 测试最小开仓金额验证（不同交易所不同限制）
func TestMinimumPositionSize(t *testing.T) {
	tests := []struct {
		name            string
		decision        Decision
		accountEquity   float64
		btcEthLeverage  int
		altcoinLeverage int
		exchange        string
		wantError       bool
		errorMsg        string
	}{
		// Hyperliquid 测试（最小 12 USDT）
		{
			name: "Hyperliquid_BTC开仓12USDT_应该通过",
			decision: Decision{
				Symbol:          "BTCUSDT",
				Action:          "open_long",
				Leverage:        10,
				PositionSizeUSD: 12.0,
				StopLoss:        90000,
				TakeProfit:      110000,
			},
			accountEquity:   1000,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			exchange:        "hyperliquid",
			wantError:       false,
		},
		{
			name: "Hyperliquid_ETH开仓12USDT_应该通过",
			decision: Decision{
				Symbol:          "ETHUSDT",
				Action:          "open_short",
				Leverage:        5,
				PositionSizeUSD: 12.0,
				StopLoss:        4000,
				TakeProfit:      3000,
			},
			accountEquity:   1000,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			exchange:        "hyperliquid",
			wantError:       false,
		},
		{
			name: "Hyperliquid_山寨币开仓12USDT_应该通过",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        5,
				PositionSizeUSD: 12.0,
				StopLoss:        50,
				TakeProfit:      200,
			},
			accountEquity:   1000,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			exchange:        "hyperliquid",
			wantError:       false,
		},
		{
			name: "Hyperliquid_开仓11USDT_应该报错",
			decision: Decision{
				Symbol:          "BTCUSDT",
				Action:          "open_long",
				Leverage:        10,
				PositionSizeUSD: 11.0,
				StopLoss:        90000,
				TakeProfit:      110000,
			},
			accountEquity:   1000,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			exchange:        "hyperliquid",
			wantError:       true,
			errorMsg:        "开仓金额过小",
		},
		// Binance 测试（最小 100 USDT）
		{
			name: "Binance_BTC开仓100USDT_应该通过",
			decision: Decision{
				Symbol:          "BTCUSDT",
				Action:          "open_long",
				Leverage:        10,
				PositionSizeUSD: 100.0,
				StopLoss:        90000,
				TakeProfit:      110000,
			},
			accountEquity:   1000,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			exchange:        "binance",
			wantError:       false,
		},
		{
			name: "Binance_ETH开仓100USDT_应该通过",
			decision: Decision{
				Symbol:          "ETHUSDT",
				Action:          "open_short",
				Leverage:        5,
				PositionSizeUSD: 100.0,
				StopLoss:        4000,
				TakeProfit:      3000,
			},
			accountEquity:   1000,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			exchange:        "binance",
			wantError:       false,
		},
		{
			name: "Binance_开仓99USDT_应该报错",
			decision: Decision{
				Symbol:          "BTCUSDT",
				Action:          "open_long",
				Leverage:        10,
				PositionSizeUSD: 99.0,
				StopLoss:        90000,
				TakeProfit:      110000,
			},
			accountEquity:   1000,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			exchange:        "binance",
			wantError:       true,
			errorMsg:        "开仓金额过小",
		},
		{
			name: "Binance_开仓12USDT_应该报错",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        5,
				PositionSizeUSD: 12.0,
				StopLoss:        50,
				TakeProfit:      200,
			},
			accountEquity:   1000,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			exchange:        "binance",
			wantError:       true,
			errorMsg:        "开仓金额过小",
		},
		// Aster 测试（最小 10 USDT，实际 $5 + 安全边际）
		{
			name: "Aster_BTC开仓10USDT_应该通过",
			decision: Decision{
				Symbol:          "BTCUSDT",
				Action:          "open_long",
				Leverage:        10,
				PositionSizeUSD: 10.0,
				StopLoss:        90000,
				TakeProfit:      110000,
			},
			accountEquity:   1000,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			exchange:        "aster",
			wantError:       false,
		},
		{
			name: "Aster_山寨币开仓10USDT_应该通过",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        5,
				PositionSizeUSD: 10.0,
				StopLoss:        50,
				TakeProfit:      200,
			},
			accountEquity:   1000,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			exchange:        "aster",
			wantError:       false,
		},
		{
			name: "Aster_开仓9USDT_应该报错",
			decision: Decision{
				Symbol:          "BTCUSDT",
				Action:          "open_long",
				Leverage:        10,
				PositionSizeUSD: 9.0,
				StopLoss:        90000,
				TakeProfit:      110000,
			},
			accountEquity:   1000,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			exchange:        "aster",
			wantError:       true,
			errorMsg:        "开仓金额过小",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDecision(&tt.decision, tt.accountEquity, tt.btcEthLeverage, tt.altcoinLeverage, tt.exchange)

			if (err != nil) != tt.wantError {
				t.Errorf("validateDecision() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if tt.wantError && err != nil {
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("错误信息不匹配: got %q, want to contain %q", err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

// contains 检查字符串是否包含子串（辅助函数）
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
