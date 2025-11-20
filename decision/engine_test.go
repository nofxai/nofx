package decision

import (
	"nofx/market"
	"strings"
	"testing"
)

// TestBuildPromptSnapshot 测试 prompt 快照生成功能
func TestBuildPromptSnapshot(t *testing.T) {
	t.Run("基础模板应该包含硬约束部分", func(t *testing.T) {
		snapshot := BuildPromptSnapshot(
			10000.0, // accountEquity
			5,       // btcEthLeverage
			5,       // altcoinLeverage
			"",      // customPrompt
			false,   // overrideBase
			"default",
		)

		// 验证包含关键部分
		requiredParts := []string{
			"硬约束",
			"风险回报比",
			"最多持仓",
			"杠杆限制",
		}

		for _, part := range requiredParts {
			if !strings.Contains(snapshot, part) {
				t.Errorf("快照应该包含 %q，但没有找到", part)
			}
		}

		// 验证杠杆配置正确
		if !strings.Contains(snapshot, "最大5x杠杆") {
			t.Error("应该包含正确的杠杆配置")
		}
	})

	t.Run("自定义 prompt 应该被追加到基础 prompt", func(t *testing.T) {
		customContent := "我的自定义交易策略：只做多不做空"

		snapshot := BuildPromptSnapshot(
			10000.0,
			5,
			5,
			customContent,
			false, // 不覆盖
			"default",
		)

		// 应该同时包含基础部分和自定义部分
		if !strings.Contains(snapshot, "硬约束") {
			t.Error("应该包含基础 prompt 的硬约束部分")
		}

		if !strings.Contains(snapshot, customContent) {
			t.Errorf("应该包含自定义内容 %q", customContent)
		}

		if !strings.Contains(snapshot, "个性化交易策略") {
			t.Error("应该包含自定义部分的标题")
		}
	})

	t.Run("覆盖模式应该只返回自定义 prompt", func(t *testing.T) {
		customContent := "完全自定义的交易策略"

		snapshot := BuildPromptSnapshot(
			10000.0,
			5,
			5,
			customContent,
			true, // 覆盖基础 prompt
			"default",
		)

		// 应该只包含自定义内容
		if snapshot != customContent {
			t.Errorf("覆盖模式应该返回 %q, 但得到 %q", customContent, snapshot)
		}

		// 不应该包含基础 prompt 的内容
		if strings.Contains(snapshot, "硬约束") {
			t.Error("覆盖模式不应该包含基础 prompt 内容")
		}
	})

	t.Run("空自定义 prompt 应该只返回基础 prompt", func(t *testing.T) {
		snapshot := BuildPromptSnapshot(
			10000.0,
			5,
			5,
			"", // 空自定义
			false,
			"default",
		)

		// 应该包含基础内容
		if !strings.Contains(snapshot, "硬约束") {
			t.Error("应该包含基础 prompt")
		}

		// 不应该包含自定义标记
		if strings.Contains(snapshot, "个性化交易策略") {
			t.Error("空自定义 prompt 不应该有个性化标记")
		}
	})

	t.Run("不同杠杆配置应该正确显示", func(t *testing.T) {
		snapshot := BuildPromptSnapshot(
			20000.0, // 更高的净值
			10,      // BTC/ETH 10x
			3,       // 山寨币 3x
			"",
			false,
			"default",
		)

		// 验证杠杆配置
		if !strings.Contains(snapshot, "最大3x杠杆") {
			t.Error("应该显示山寨币 3x 杠杆")
		}

		if !strings.Contains(snapshot, "最大10x杠杆") {
			t.Error("应该显示 BTC/ETH 10x 杠杆")
		}

		// 验证仓位上限（基于净值计算）
		// 山寨: 20000 * 1.5 = 30000
		// BTC/ETH: 20000 * 10 = 200000
		if !strings.Contains(snapshot, "30000 U") {
			t.Error("应该显示正确的山寨币仓位上限")
		}

		if !strings.Contains(snapshot, "200000 U") {
			t.Error("应该显示正确的 BTC/ETH 仓位上限")
		}
	})

	// 移除旧测试："模板不存在应该降级到 default"
	// 新行为：模板不存在时系统立即退出（资金安全）
	// 相关测试：TestBuildSystemPrompt_NonExistentTemplate_ShouldCallFatal (prompt_test.go)

	t.Run("快照应该包含输出格式说明", func(t *testing.T) {
		snapshot := BuildPromptSnapshot(
			10000.0,
			5,
			5,
			"",
			false,
			"default",
		)

		// 验证包含输出格式相关内容
		formatParts := []string{
			"输出格式",
			"<reasoning>",
			"<decision>",
		}

		for _, part := range formatParts {
			if !strings.Contains(snapshot, part) {
				t.Errorf("应该包含输出格式说明 %q", part)
			}
		}
	})
}

// TestBuildPromptSnapshotConsistency 测试快照的一致性
// 相同参数应该生成相同的快照
func TestBuildPromptSnapshotConsistency(t *testing.T) {
	params := struct {
		equity         float64
		btcEthLeverage int
		altLeverage    int
		custom         string
		override       bool
		template       string
	}{
		equity:         15000.0,
		btcEthLeverage: 8,
		altLeverage:    4,
		custom:         "我的策略",
		override:       false,
		template:       "default",
	}

	// 生成两次快照
	snapshot1 := BuildPromptSnapshot(
		params.equity,
		params.btcEthLeverage,
		params.altLeverage,
		params.custom,
		params.override,
		params.template,
	)

	snapshot2 := BuildPromptSnapshot(
		params.equity,
		params.btcEthLeverage,
		params.altLeverage,
		params.custom,
		params.override,
		params.template,
	)

	// 应该完全一致
	if snapshot1 != snapshot2 {
		t.Error("相同参数生成的快照应该一致")
	}
}

// TestBuildUserPrompt_ShowsStopLossAndTakeProfit 测试持仓信息中显示止损和止盈
func TestBuildUserPrompt_ShowsStopLossAndTakeProfit(t *testing.T) {
	t.Run("持仓有止损和止盈时应该在prompt中显示", func(t *testing.T) {
		ctx := &Context{
			CurrentTime:    "2025-01-17 12:00:00",
			RuntimeMinutes: 30,
			CallCount:      5,
			Account: AccountInfo{
				TotalEquity:      10000.0,
				AvailableBalance: 5000.0,
				UnrealizedPnL:    100.0,
				TotalPnL:         100.0,
				TotalPnLPct:      1.0,
				MarginUsed:       3000.0,
				MarginUsedPct:    30.0,
				PositionCount:    1,
			},
			Positions: []PositionInfo{
				{
					Symbol:           "BTCUSDT",
					Side:             "short",
					EntryPrice:       95462.0,
					MarkPrice:        94616.0,
					Quantity:         0.1,
					Leverage:         5,
					UnrealizedPnL:    84.6,
					UnrealizedPnLPct: 0.89,
					PeakPnLPct:       1.2,
					LiquidationPrice: 120000.0,
					MarginUsed:       1909.24,
					UpdateTime:       1700000000000,
					StopLoss:         94571.0, // 设置了止损
					TakeProfit:       93000.0, // 设置了止盈
				},
			},
			CandidateCoins: []CandidateCoin{},
			MarketDataMap:  make(map[string]*market.Data),
		}

		prompt := buildUserPrompt(ctx)

		// 验证止损价格在prompt中显示
		if !strings.Contains(prompt, "止损") || !strings.Contains(prompt, "94571") {
			t.Errorf("Prompt应该显示止损价格 94571.00, 实际输出:\n%s", prompt)
		}

		// 验证止盈价格在prompt中显示
		if !strings.Contains(prompt, "止盈") || !strings.Contains(prompt, "93000") {
			t.Errorf("Prompt应该显示止盈价格 93000.00, 实际输出:\n%s", prompt)
		}
	})

	t.Run("持仓没有止损和止盈时不应该显示", func(t *testing.T) {
		ctx := &Context{
			CurrentTime:    "2025-01-17 12:00:00",
			RuntimeMinutes: 30,
			CallCount:      5,
			Account: AccountInfo{
				TotalEquity:      10000.0,
				AvailableBalance: 5000.0,
				MarginUsed:       3000.0,
				MarginUsedPct:    30.0,
				PositionCount:    1,
			},
			Positions: []PositionInfo{
				{
					Symbol:           "BTCUSDT",
					Side:             "long",
					EntryPrice:       95000.0,
					MarkPrice:        95500.0,
					Quantity:         0.1,
					Leverage:         5,
					UnrealizedPnL:    50.0,
					UnrealizedPnLPct: 0.52,
					LiquidationPrice: 80000.0,
					MarginUsed:       1900.0,
					UpdateTime:       1700000000000,
					// 没有设置 StopLoss 和 TakeProfit (默认为0)
				},
			},
			CandidateCoins: []CandidateCoin{},
			MarketDataMap:  make(map[string]*market.Data),
		}

		prompt := buildUserPrompt(ctx)

		// 当没有设置止损/止盈时，prompt不应该显示0值
		// (或者应该显示"未设置"等提示)
		// 这里我们验证至少不会显示误导性的"止损 0.00"
		if strings.Contains(prompt, "止损 0.00") || strings.Contains(prompt, "止盈 0.00") {
			t.Errorf("Prompt不应该显示零值的止损/止盈, 实际输出:\n%s", prompt)
		}
	})
}
