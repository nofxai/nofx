package decision

import (
	"nofx/market"
	"strings"
	"testing"
)

const (
	testTime        = "2024-01-01 12:00:00"
	expectedBTCInfo = "BTC: 98000.00"
)

// TestBuildUserPromptBTCNotSelectedShouldNotShowBTCInfo
// 验证 fix: 当 BTC 不在持仓且不在候选列表中时，不应在 prompt 中显示 BTC 市场摘要
func TestBuildUserPromptBTCNotSelectedShouldNotShowBTCInfo(t *testing.T) {
	ctx := &Context{
		CurrentTime:    testTime,
		RuntimeMinutes: 60,
		CallCount:      10,
		Account: AccountInfo{
			TotalEquity: 1000.0,
		},
		Positions:      []PositionInfo{},
		CandidateCoins: []CandidateCoin{{Symbol: "ETHUSDT", Sources: []string{"custom"}}},
		// 模拟 MarketDataMap 中包含 BTC 数据（即使未选中，可能是因为 Trend 分析被 fetch 了）
		MarketDataMap: map[string]*market.Data{
			"BTCUSDT": {
				Symbol:        "BTCUSDT",
				CurrentPrice:  98000.0,
				PriceChange1h: 0.5,
				PriceChange4h: 1.2,
				CurrentMACD:   100.0,
				CurrentRSI7:   60.0,
			},
			"ETHUSDT": {
				Symbol:       "ETHUSDT",
				CurrentPrice: 3000.0,
			},
		},
		BTCDailyTrend: "bullish", // Trend 依然应该显示（如果 policy 允许）
	}

	prompt := buildUserPrompt(ctx)

	// 1. 验证 BTC 市场摘要行不应存在
	if strings.Contains(prompt, expectedBTCInfo) {
		t.Errorf("Prompt 不应包含 BTC 市场摘要，因为 BTC 未被选中\nPrompt片段:\n%s", prompt[:500])
	}

	// 2. 验证 BTC Trend 依然存在 (这是全局上下文，通常保留，除非用户也想隐藏这个)
	// 如果之前的代码保留了 Trend，我们这里也 verify 一下
	if !strings.Contains(prompt, "BTC Daily Trend: bullish") {
		t.Errorf("Prompt 应该包含 BTC Daily Trend")
	}

	// 3. 验证 ETH 应该存在
	if !strings.Contains(prompt, "ETHUSDT") {
		t.Errorf("Prompt 应该包含 ETHUSDT")
	}
}

// TestBuildUserPromptBTCSelectedShouldShowBTCInfo
// 验证: 当 BTC 被选中时（持仓或候选），应该显示 BTC 市场摘要
func TestBuildUserPromptBTCSelectedShouldShowBTCInfo(t *testing.T) {
	// Case 1: BTC is a Candidate
	ctx1 := &Context{
		CurrentTime: testTime,
		Account:     AccountInfo{TotalEquity: 1000.0},
		Positions:   []PositionInfo{},
		CandidateCoins: []CandidateCoin{
			{Symbol: "BTCUSDT", Sources: []string{"custom"}},
		},
		MarketDataMap: map[string]*market.Data{
			"BTCUSDT": {
				Symbol:       "BTCUSDT",
				CurrentPrice: 98000.0,
			},
		},
	}

	prompt1 := buildUserPrompt(ctx1)
	if !strings.Contains(prompt1, expectedBTCInfo) {
		t.Errorf("当 BTC 是候选币种时，Prompt 应该包含 BTC 市场摘要")
	}

	// Case 2: BTC is a Position
	ctx2 := &Context{
		CurrentTime: testTime,
		Account:     AccountInfo{TotalEquity: 1000.0},
		Positions: []PositionInfo{
			{Symbol: "BTCUSDT", Quantity: 0.1},
		},
		CandidateCoins: []CandidateCoin{},
		MarketDataMap: map[string]*market.Data{
			"BTCUSDT": {
				Symbol:       "BTCUSDT",
				CurrentPrice: 98000.0,
			},
		},
	}

	prompt2 := buildUserPrompt(ctx2)
	if !strings.Contains(prompt2, expectedBTCInfo) {
		t.Errorf("当 BTC 是持仓时，Prompt 应该包含 BTC 市场摘要")
	}
}