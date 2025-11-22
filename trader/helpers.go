package trader

// DefaultSlippage 默认滑点缓冲 (0.5%)，用于开平仓
const DefaultSlippage = 0.005

// DefaultStopLossSlippage 默认止损滑点缓冲 (2%)，用于止损止盈单
const DefaultStopLossSlippage = 0.02

// CalculateAggressiveLimitPrice 计算激进限价单价格 (用于开平仓)
func CalculateAggressiveLimitPrice(side string, currentPrice float64, slippage float64) float64 {
	if slippage <= 0 {
		slippage = DefaultSlippage
	}

	if side == "BUY" {
		return currentPrice * (1 + slippage)
	}
	return currentPrice * (1 - slippage)
}

// CalculateStopLimitPrice 计算止损限价单价格 (用于SL/TP)
func CalculateStopLimitPrice(positionSide string, triggerPrice float64, slippage float64) float64 {
	if slippage <= 0 {
		slippage = DefaultStopLossSlippage
	}

	if positionSide == "LONG" {
		// Long SL -> Sell -> Limit Price < Trigger
		return triggerPrice * (1 - slippage)
	}

	// Short SL -> Buy -> Limit Price > Trigger
	return triggerPrice * (1 + slippage)
}
