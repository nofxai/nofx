package trader

import (
	"testing"
)

func TestCalculateStopLimitPrice(t *testing.T) {
	tests := []struct {
		name         string
		positionSide string
		triggerPrice float64
		slippage     float64
		expected     float64
	}{
		{
			name:         "多单止损_默认滑点",
			positionSide: "LONG",
			triggerPrice: 100.0,
			slippage:     0, // 使用默认值
			expected:     98.0, // 100 * (1 - 0.02)
		},
		{
			name:         "空单止损_默认滑点",
			positionSide: "SHORT",
			triggerPrice: 100.0,
			slippage:     0,
			expected:     102.0, // 100 * (1 + 0.02)
		},
		{
			name:         "多单止盈_默认滑点",
			positionSide: "LONG",
			triggerPrice: 120.0,
			slippage:     0,
			expected:     117.6, // 120 * (1 - 0.02)
		},
		{
			name:         "空单止盈_默认滑点",
			positionSide: "SHORT",
			triggerPrice: 80.0,
			slippage:     0,
			expected:     81.6, // 80 * (1 + 0.02)
		},
		{
			name:         "多单止损_自定义滑点1%",
			positionSide: "LONG",
			triggerPrice: 100.0,
			slippage:     0.01,
			expected:     99.0,
		},
		{
			name:         "空单止损_自定义滑点5%",
			positionSide: "SHORT",
			triggerPrice: 100.0,
			slippage:     0.05,
			expected:     105.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateStopLimitPrice(tt.positionSide, tt.triggerPrice, tt.slippage)
			// 浮点数比较，允许微小误差
			if got < tt.expected-0.000001 || got > tt.expected+0.000001 {
				t.Errorf("CalculateStopLimitPrice() = %v, want %v", got, tt.expected)
			}
		})
	}
}
