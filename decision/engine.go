package decision

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"nofx/market"
	"nofx/mcp"
	"nofx/pool"
	"os"
	"regexp"
	"strings"
	"time"
)

// fatalFunc 是可替换的致命错误处理函数（仅用于测试）
// 生产环境永远使用 realFatal，确保系统真正退出
var fatalFunc = realFatal

// realFatal 真正的致命错误处理（调用 os.Exit）
// ⚠️ 资金安全关键：当配置错误时，必须阻止系统启动
func realFatal(format string, v ...interface{}) {
	log.Printf(format, v...)
	os.Exit(1) // 进程立即退出
}

// 预编译正则表达式（性能优化：避免每次调用时重新编译）
var (
	// ✅ 安全的正則：精確匹配 ```json 代碼塊
	// 使用反引號 + 拼接避免轉義問題
	reJSONFence      = regexp.MustCompile(`(?is)` + "```json\\s*(\\[\\s*\\{.*?\\}\\s*\\])\\s*```")
	reJSONArray      = regexp.MustCompile(`(?is)\[\s*\{.*?\}\s*\]`)
	reArrayHead      = regexp.MustCompile(`^\[\s*\{`)
	reArrayOpenSpace = regexp.MustCompile(`^\[\s+\{`)
	reInvisibleRunes = regexp.MustCompile("[\u200B\u200C\u200D\uFEFF]")

	// 新增：XML标签提取（支持思维链中包含任何字符）
	reReasoningTag = regexp.MustCompile(`(?s)<reasoning>(.*?)</reasoning>`)
	reDecisionTag  = regexp.MustCompile(`(?s)<decision>(.*?)</decision>`)
)

// PositionInfo 持仓信息
type PositionInfo struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"` // "long" or "short"
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	PeakPnLPct       float64 `json:"peak_pnl_pct"` // 历史最高收益率（百分比）
	LiquidationPrice float64 `json:"liquidation_price"`
	MarginUsed       float64 `json:"margin_used"`
	UpdateTime       int64   `json:"update_time"`           // 持仓更新时间戳（毫秒）
	StopLoss         float64 `json:"stop_loss,omitempty"`   // 止损价格（用于推断平仓原因）
	TakeProfit       float64 `json:"take_profit,omitempty"` // 止盈价格（用于推断平仓原因）
}

// AccountInfo 账户信息
type AccountInfo struct {
	TotalEquity      float64 `json:"total_equity"`      // 账户净值
	AvailableBalance float64 `json:"available_balance"` // 可用余额
	UnrealizedPnL    float64 `json:"unrealized_pnl"`    // 未实现盈亏
	TotalPnL         float64 `json:"total_pnl"`         // 总盈亏
	TotalPnLPct      float64 `json:"total_pnl_pct"`     // 总盈亏百分比
	MarginUsed       float64 `json:"margin_used"`       // 已用保证金
	MarginUsedPct    float64 `json:"margin_used_pct"`   // 保证金使用率
	PositionCount    int     `json:"position_count"`    // 持仓数量
}

// CandidateCoin 候选币种（来自币种池）
type CandidateCoin struct {
	Symbol  string   `json:"symbol"`
	Sources []string `json:"sources"` // 来源: "ai500" 和/或 "oi_top"
}

// OITopData 持仓量增长Top数据（用于AI决策参考）
type OITopData struct {
	Rank              int     // OI Top排名
	OIDeltaPercent    float64 // 持仓量变化百分比（1小时）
	OIDeltaValue      float64 // 持仓量变化价值
	PriceDeltaPercent float64 // 价格变化百分比
	NetLong           float64 // 净多仓
	NetShort          float64 // 净空仓
}

// Context 交易上下文（传递给AI的完整信息）
type Context struct {
	CurrentTime     string                             `json:"current_time"`
	RuntimeMinutes  int                                `json:"runtime_minutes"`
	CallCount       int                                `json:"call_count"`
	Exchange        string                             `json:"-"` // 交易所名称（binance/hyperliquid）
	Account         AccountInfo                        `json:"account"`
	Positions       []PositionInfo                     `json:"positions"`
	CandidateCoins  []CandidateCoin                    `json:"candidate_coins"`
	PromptVariant   string                             `json:"prompt_variant,omitempty"`
	MarketDataMap   map[string]*market.Data            `json:"-"` // 不序列化，但内部使用
	MultiTFMarket   map[string]map[string]*market.Data `json:"-"`
	OITopDataMap    map[string]*OITopData              `json:"-"` // OI Top数据映射
	Performance     interface{}                        `json:"-"` // 历史表现分析（logger.PerformanceAnalysis）
	BTCETHLeverage  int                                `json:"-"` // BTC/ETH杠杆倍数（从配置读取）
	AltcoinLeverage int                                `json:"-"` // 山寨币杠杆倍数（从配置读取）
	BTCDailyTrend   string                             `json:"-"` // BTC 日线趋势 "bullish"/"bearish"/"neutral"
}

// Decision AI的交易决策
type Decision struct {
	Symbol string `json:"symbol"`
	Action string `json:"action"` // "open_long", "open_short", "close_long", "close_short", "update_stop_loss", "update_take_profit", "partial_close", "hold", "wait"

	// 开仓参数
	Leverage        int     `json:"leverage,omitempty"`
	PositionSizeUSD float64 `json:"position_size_usd,omitempty"`
	StopLoss        float64 `json:"stop_loss,omitempty"`
	TakeProfit      float64 `json:"take_profit,omitempty"`

	// 调整参数（新增）
	NewStopLoss     float64 `json:"new_stop_loss,omitempty"`    // 用于 update_stop_loss
	NewTakeProfit   float64 `json:"new_take_profit,omitempty"`  // 用于 update_take_profit
	ClosePercentage float64 `json:"close_percentage,omitempty"` // 用于 partial_close (0-100)

	// 通用参数
	RiskUSD   float64 `json:"risk_usd,omitempty"` // 最大美元风险
	Reasoning string  `json:"reasoning"`
}

// FullDecision AI的完整决策（包含思维链）
type FullDecision struct {
	SystemPrompt string     `json:"system_prompt"` // 系统提示词（发送给AI的系统prompt）
	UserPrompt   string     `json:"user_prompt"`   // 发送给AI的输入prompt
	CoTTrace     string     `json:"cot_trace"`     // 思维链分析（AI输出）
	Decisions    []Decision `json:"decisions"`     // 具体决策列表
	Timestamp    time.Time  `json:"timestamp"`
	// AIRequestDurationMs 记录 AI API 调用耗时（毫秒）方便排查延迟问题
	AIRequestDurationMs int64  `json:"ai_request_duration_ms,omitempty"`
	PromptHash          string `json:"prompt_hash,omitempty"` // Prompt 模板的 hash（用于区分不同版本）
}

// GetFullDecision 获取AI的完整交易决策（批量分析所有币种和持仓）
func GetFullDecision(ctx *Context, mcpClient mcp.AIClient) (*FullDecision, error) {
	return GetFullDecisionWithCustomPrompt(ctx, mcpClient, "", false, "")
}

// GetFullDecisionWithCustomPrompt 获取AI的完整交易决策（支持自定义prompt和模板选择）
func GetFullDecisionWithCustomPrompt(ctx *Context, mcpClient mcp.AIClient, customPrompt string, overrideBase bool, templateName string) (*FullDecision, error) {
	// 1. 为所有币种获取市场数据
	if err := fetchMarketDataForContext(ctx); err != nil {
		return nil, fmt.Errorf("获取市场数据失败: %w", err)
	}

	// 2. 计算 Prompt Hash（基于模板文件内容，不受动态值影响）
	promptHash := calculatePromptHashFromTemplate(templateName, customPrompt, overrideBase)

	// 3. 构建 System Prompt（固定规则）和 User Prompt（动态数据）
	minPositionSize := getMinPositionSize(ctx.Exchange)
	systemPrompt := buildSystemPromptWithCustom(ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, customPrompt, overrideBase, templateName, minPositionSize)
	userPrompt := buildUserPrompt(ctx)

	// 4. 调用AI API（使用 system + user prompt）
	aiCallStart := time.Now()
	aiResponse, err := mcpClient.CallWithMessages(systemPrompt, userPrompt)
	aiCallDuration := time.Since(aiCallStart)
	if err != nil {
		return nil, fmt.Errorf("调用AI API失败: %w", err)
	}

	// 5. 解析AI响应
	decision, err := parseFullDecisionResponse(aiResponse, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, ctx.Exchange)

	// 无论是否有错误，都要保存 SystemPrompt、UserPrompt 和 PromptHash（用于调试和决策未执行后的问题定位）
	if decision != nil {
		decision.Timestamp = time.Now()
		decision.SystemPrompt = systemPrompt // 保存系统prompt
		decision.UserPrompt = userPrompt     // 保存输入prompt
		decision.PromptHash = promptHash     // 保存 prompt hash
		decision.AIRequestDurationMs = aiCallDuration.Milliseconds()
	}

	if err != nil {
		return decision, fmt.Errorf("解析AI响应失败: %w", err)
	}

	decision.Timestamp = time.Now()
	decision.SystemPrompt = systemPrompt // 保存系统prompt
	decision.UserPrompt = userPrompt     // 保存输入prompt
	decision.PromptHash = promptHash     // 保存 prompt hash
	return decision, nil
}

// fetchMarketDataForContext 为上下文中的所有币种获取市场数据和OI数据
func fetchMarketDataForContext(ctx *Context) error {
	ctx.MarketDataMap = make(map[string]*market.Data)
	ctx.OITopDataMap = make(map[string]*OITopData)

	// 收集所有需要获取数据的币种
	symbolSet := make(map[string]bool)

	// 1. 优先获取持仓币种的数据（这是必须的）
	for _, pos := range ctx.Positions {
		symbolSet[pos.Symbol] = true
	}

	// 2. 候选币种数量根据账户状态动态调整
	maxCandidates := calculateMaxCandidates(ctx)
	for i, coin := range ctx.CandidateCoins {
		if i >= maxCandidates {
			break
		}
		symbolSet[coin.Symbol] = true
	}

	// 并发获取市场数据
	// 持仓币种集合（用于判断是否跳过OI检查）
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}

	for symbol := range symbolSet {
		data, err := market.Get(symbol)
		if err != nil {
			// 单个币种失败不影响整体，只记录错误
			continue
		}

		// ⚠️ 流动性过滤：持仓价值低于阈值的币种不做（多空都不做）
		// 持仓价值 = 持仓量 × 当前价格
		// 但现有持仓必须保留（需要决策是否平仓）
		// 💡 OI 門檻配置：用戶可根據風險偏好調整
		const minOIThresholdMillions = 15.0 // 可調整：15M(保守) / 10M(平衡) / 8M(寬鬆) / 5M(激進)

		isExistingPosition := positionSymbols[symbol]
		if !isExistingPosition && data.OpenInterest != nil && data.CurrentPrice > 0 {
			// 计算持仓价值（USD）= 持仓量 × 当前价格
			oiValue := data.OpenInterest.Latest * data.CurrentPrice
			oiValueInMillions := oiValue / 1_000_000 // 转换为百万美元单位
			if oiValueInMillions < minOIThresholdMillions {
				log.Printf("⚠️  %s 持仓价值过低(%.2fM USD < %.1fM)，跳过此币种 [持仓量:%.0f × 价格:%.4f]",
					symbol, oiValueInMillions, minOIThresholdMillions, data.OpenInterest.Latest, data.CurrentPrice)
				continue
			}
		}

		ctx.MarketDataMap[symbol] = data
	}

	// 提取 BTC 日线趋势（如果存在）
	if btcData, ok := ctx.MarketDataMap["BTCUSDT"]; ok && btcData.DailyContext != nil {
		ctx.BTCDailyTrend = btcData.DailyContext.TrendBias
	}

	// 加载OI Top数据（不影响主流程）
	oiPositions, err := pool.GetOITopPositions()
	if err == nil {
		for _, pos := range oiPositions {
			// 标准化符号匹配
			symbol := pos.Symbol
			ctx.OITopDataMap[symbol] = &OITopData{
				Rank:              pos.Rank,
				OIDeltaPercent:    pos.OIDeltaPercent,
				OIDeltaValue:      pos.OIDeltaValue,
				PriceDeltaPercent: pos.PriceDeltaPercent,
				NetLong:           pos.NetLong,
				NetShort:          pos.NetShort,
			}
		}
	}

	return nil
}

// calculateMaxCandidates 根据账户状态计算需要分析的候选币种数量
func calculateMaxCandidates(ctx *Context) int {
	// ⚠️ 重要：限制候选币种数量，避免 Prompt 过大
	// 根据持仓数量动态调整：持仓越少，可以分析更多候选币
	const (
		maxCandidatesWhenEmpty    = 30 // 无持仓时最多分析30个候选币
		maxCandidatesWhenHolding1 = 25 // 持仓1个时最多分析25个候选币
		maxCandidatesWhenHolding2 = 20 // 持仓2个时最多分析20个候选币
		maxCandidatesWhenHolding3 = 15 // 持仓3个时最多分析15个候选币（避免 Prompt 过大）
	)

	positionCount := len(ctx.Positions)
	var maxCandidates int

	switch positionCount {
	case 0:
		maxCandidates = maxCandidatesWhenEmpty
	case 1:
		maxCandidates = maxCandidatesWhenHolding1
	case 2:
		maxCandidates = maxCandidatesWhenHolding2
	default: // 3+ 持仓
		maxCandidates = maxCandidatesWhenHolding3
	}

	// 返回实际候选币数量和上限中的较小值
	return min(len(ctx.CandidateCoins), maxCandidates)
}

// calculatePromptHashFromTemplate 计算 Prompt Hash（基于模板内容，不受动态值影响）
// 规则：只基于模板文件内容和 customPrompt，不包含 accountEquity 等动态值
func calculatePromptHashFromTemplate(templateName string, customPrompt string, overrideBase bool) string {
	var content strings.Builder

	// 如果覆盖基础 prompt 且有自定义 prompt，只使用自定义 prompt
	if overrideBase && customPrompt != "" {
		content.WriteString(customPrompt)
	} else {
		// 获取模板内容
		if templateName == "" {
			templateName = "default"
		}

		template, err := GetPromptTemplate(templateName)
		if err != nil {
			// 模板不存在，尝试 default
			template, err = GetPromptTemplate("default")
			if err != nil {
				// 连 default 都不存在，使用固定字符串
				content.WriteString("builtin_fallback_prompt")
			} else {
				content.WriteString(template.Content)
			}
		} else {
			content.WriteString(template.Content)
		}

		// 添加自定义 prompt（如果有）
		if customPrompt != "" {
			content.WriteString("\n\n# CUSTOM\n")
			content.WriteString(customPrompt)
		}
	}

	// 计算 MD5 hash
	hash := md5.Sum([]byte(content.String()))
	return hex.EncodeToString(hash[:])
}

// buildSystemPromptWithCustom 构建包含自定义内容的 System Prompt
func buildSystemPromptWithCustom(accountEquity float64, btcEthLeverage, altcoinLeverage int, customPrompt string, overrideBase bool, templateName string, minPositionSize float64) string {
	// 如果覆盖基础prompt且有自定义prompt，只使用自定义prompt
	if overrideBase && customPrompt != "" {
		return customPrompt
	}

	// 获取基础prompt（使用指定的模板）
	basePrompt := buildSystemPrompt(accountEquity, btcEthLeverage, altcoinLeverage, templateName, minPositionSize)

	// 如果没有自定义prompt，直接返回基础prompt
	if customPrompt == "" {
		return basePrompt
	}

	// 添加自定义prompt部分到基础prompt
	var sb strings.Builder
	sb.WriteString(basePrompt)
	sb.WriteString("\n\n")
	sb.WriteString("# 📌 个性化交易策略\n\n")
	sb.WriteString(customPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("注意: 以上个性化策略是对基础规则的补充，不能违背基础风险控制原则。\n")

	return sb.String()
}

// buildSystemPrompt 构建 System Prompt（使用模板+动态部分）
func buildSystemPrompt(accountEquity float64, btcEthLeverage, altcoinLeverage int, templateName string, minPositionSize float64) string {
	var sb strings.Builder

	// 1. 加载提示词模板（核心交易策略部分）
	if templateName == "" {
		templateName = "default" // 默认使用 default 模板
	}

	template, err := GetPromptTemplate(templateName)
	if err != nil {
		// 如果模板不存在，列出可用模板并退出系统
		availableTemplates := GetAllPromptTemplateNames()
		log.Printf("❌ 致命错误：系统提示词模板 '%s' 不存在", templateName)
		log.Printf("📋 当前可用的模板列表: %v", availableTemplates)
		// 使用 fatalFunc（生产环境调用 os.Exit(1)，测试环境可替换）
		fatalFunc("系统无法启动，请检查交易员配置中的 system_prompt_template 字段")
	}

	sb.WriteString(template.Content)
	sb.WriteString("\n\n")

	// 2. 硬约束（风险控制）- 动态生成
	sb.WriteString("# 硬约束（风险控制）\n\n")
	sb.WriteString("1. 风险回报比: 必须 ≥ 1:3（冒1%风险，赚3%+收益）\n")
	sb.WriteString("2. 最多持仓: 3个币种（质量>数量）\n")
	sb.WriteString(fmt.Sprintf("3. ⚠️ **单币仓位上限（严格执行）**: 山寨%.0f U | BTC/ETH %.0f U\n",
		accountEquity*1.5, accountEquity*10))
	sb.WriteString("   - **超出此限制的决策将被系统拒绝**\n")
	sb.WriteString("   - 基于净值的仓位上限，实际开仓还需考虑可用余额\n")
	sb.WriteString(fmt.Sprintf("4. 杠杆限制: **山寨币最大%dx杠杆** | **BTC/ETH最大%dx杠杆** (⚠️ 超限将被拒绝)\n", altcoinLeverage, btcEthLeverage))
	sb.WriteString("5. 保证金: 总使用率 ≤ 90%\n")
	sb.WriteString(fmt.Sprintf("6. 开仓金额: **必须 ≥%.0f USDT**（交易所要求）\n", minPositionSize))
	sb.WriteString("7. ⚠️ **开仓保证金检查**: 开仓前必须确保 `所需保证金 ≤ 可用余额`，所需保证金 = position_size_usd / leverage + 手续费\n\n")

	// 3. 输出格式 - 动态生成
	sb.WriteString("# 输出格式 (严格遵守)\n\n")
	sb.WriteString("**必须使用XML标签 <reasoning> 和 <decision> 标签分隔思维链和决策JSON，避免解析错误**\n\n")
	sb.WriteString("## 格式要求\n\n")
	sb.WriteString("<reasoning>\n")
	sb.WriteString("你的思维链分析...\n")
	sb.WriteString("- 简洁分析你的思考过程 \n")
	sb.WriteString("</reasoning>\n\n")
	sb.WriteString("<decision>\n")
	sb.WriteString("```json\n[\n")
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"position_size_usd\": %.0f, \"stop_loss\": 97000, \"take_profit\": 91000, \"risk_usd\": 300, \"reasoning\": \"下跌趋势+MACD死叉\"},\n", btcEthLeverage, accountEquity*8))
	sb.WriteString("  {\"symbol\": \"SOLUSDT\", \"action\": \"update_stop_loss\", \"new_stop_loss\": 155, \"reasoning\": \"移动止损至保本位\"},\n")
	sb.WriteString("  {\"symbol\": \"ETHUSDT\", \"action\": \"close_long\", \"reasoning\": \"止盈离场\"}\n")
	sb.WriteString("]\n```\n")
	sb.WriteString("</decision>\n\n")
	sb.WriteString(fmt.Sprintf("**⚠️ 重要提醒**: position_size_usd 必须 ≤ 单币仓位上限（山寨%.0f U | BTC/ETH %.0f U），否则订单将被拒绝\n\n",
		accountEquity*1.5, accountEquity*10))
	sb.WriteString("## 字段说明\n\n")
	sb.WriteString("- `action`: open_long | open_short | close_long | close_short | update_stop_loss | update_take_profit | partial_close | hold | wait\n")
	sb.WriteString("- 开仓时必填: leverage, position_size_usd, stop_loss, take_profit, risk_usd, reasoning\n")
	sb.WriteString("- update_stop_loss 时必填: new_stop_loss (注意是 new_stop_loss，不是 stop_loss)\n")
	sb.WriteString("- update_take_profit 时必填: new_take_profit (注意是 new_take_profit，不是 take_profit)\n")
	sb.WriteString("- partial_close 时必填: close_percentage (0-100), new_stop_loss, new_take_profit (⚠️ 部分平仓后原订单会被取消，必须为剩余仓位重新设置止损止盈)\n\n")

	return sb.String()
}

// buildUserPrompt 构建 User Prompt（动态数据）
func buildUserPrompt(ctx *Context) string {
	var sb strings.Builder

	// 系统状态
	sb.WriteString(fmt.Sprintf("时间: %s | 周期: #%d | 运行: %d分钟\n\n",
		ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes))

	// BTC 日线趋势摘要
	if ctx.BTCDailyTrend != "" {
		sb.WriteString(fmt.Sprintf("## 📈 BTC Daily Trend: %s\n\n", ctx.BTCDailyTrend))
	}

	// BTC 市场
	if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
		sb.WriteString(fmt.Sprintf("BTC: %.2f (1h: %+.2f%%, 4h: %+.2f%%) | MACD: %.4f | RSI: %.2f\n\n",
			btcData.CurrentPrice, btcData.PriceChange1h, btcData.PriceChange4h,
			btcData.CurrentMACD, btcData.CurrentRSI7))
	}

	// 账户
	sb.WriteString(fmt.Sprintf("账户: 净值%.2f | **可用余额%.2f USDT** (%.1f%%) | 已用保证金%.2f | 盈亏%+.2f%% | 保证金使用率%.1f%% | 持仓%d个\n\n",
		ctx.Account.TotalEquity,
		ctx.Account.AvailableBalance,
		(ctx.Account.AvailableBalance/ctx.Account.TotalEquity)*100,
		ctx.Account.MarginUsed,
		ctx.Account.TotalPnLPct,
		ctx.Account.MarginUsedPct,
		ctx.Account.PositionCount))

	// 持仓（完整市场数据）
	if len(ctx.Positions) > 0 {
		sb.WriteString("## 当前持仓\n")
		for i, pos := range ctx.Positions {
			// 计算持仓时长
			holdingDuration := ""
			if pos.UpdateTime > 0 {
				durationMs := time.Now().UnixMilli() - pos.UpdateTime
				durationMin := durationMs / (1000 * 60) // 转换为分钟
				if durationMin < 60 {
					holdingDuration = fmt.Sprintf(" | 持仓时长%d分钟", durationMin)
				} else {
					durationHour := durationMin / 60
					durationMinRemainder := durationMin % 60
					holdingDuration = fmt.Sprintf(" | 持仓时长%d小时%d分钟", durationHour, durationMinRemainder)
				}
			}

			// 计算仓位价值（用于 partial_close 检查）
			positionValue := math.Abs(pos.Quantity) * pos.MarkPrice

			// 构建止损/止盈信息
			stopLossTakeProfitInfo := ""
			if pos.StopLoss > 0 && pos.TakeProfit > 0 {
				stopLossTakeProfitInfo = fmt.Sprintf(" | 止损%.4f | 止盈%.4f", pos.StopLoss, pos.TakeProfit)
			} else if pos.StopLoss > 0 {
				stopLossTakeProfitInfo = fmt.Sprintf(" | 止损%.4f", pos.StopLoss)
			} else if pos.TakeProfit > 0 {
				stopLossTakeProfitInfo = fmt.Sprintf(" | 止盈%.4f", pos.TakeProfit)
			}

			sb.WriteString(fmt.Sprintf("%d. %s %s | 入场价%.4f 当前价%.4f | 数量%.4f | 仓位价值%.2f USDT | 盈亏%+.2f%% | 盈亏金额%+.2f USDT | 最高收益率%.2f%% | 杠杆%dx | 保证金%.0f | 强平价%.4f%s%s\n\n",
				i+1, pos.Symbol, strings.ToUpper(pos.Side),
				pos.EntryPrice, pos.MarkPrice, pos.Quantity, positionValue, pos.UnrealizedPnLPct, pos.UnrealizedPnL, pos.PeakPnLPct,
				pos.Leverage, pos.MarginUsed, pos.LiquidationPrice, holdingDuration, stopLossTakeProfitInfo))

			// 使用FormatMarketData输出完整市场数据
			// skipSymbolMention=true 因为 Symbol 已经在上面的 header 中显示了
			if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
				sb.WriteString(market.Format(marketData, true))
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("当前持仓: 无\n\n")
	}

	// 候选币种（完整市场数据）
	// 只显示未持仓的币种，避免重复
	sb.WriteString(fmt.Sprintf("## 候选币种 (%d个)\n\n", len(ctx.MarketDataMap)))
	displayedCount := 0
	for _, coin := range ctx.CandidateCoins {
		marketData, hasData := ctx.MarketDataMap[coin.Symbol]
		if !hasData {
			continue
		}

		// 检查是否已经持仓，如果已持仓则跳过（避免重复）
		isHeldPosition := false
		for _, pos := range ctx.Positions {
			if pos.Symbol == coin.Symbol {
				isHeldPosition = true
				break
			}
		}
		if isHeldPosition {
			continue
		}

		displayedCount++

		sourceTags := ""
		if len(coin.Sources) > 1 {
			sourceTags = " (AI500+OI_Top双重信号)"
		} else if len(coin.Sources) == 1 && coin.Sources[0] == "oi_top" {
			sourceTags = " (OI_Top持仓增长)"
		}

		// 使用FormatMarketData输出完整市场数据
		// skipSymbolMention=false 因为这是候选币种列表，需要显示币种名称
		sb.WriteString(fmt.Sprintf("### %d. %s%s\n\n", displayedCount, coin.Symbol, sourceTags))
		sb.WriteString(market.Format(marketData, false))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// 夏普比率（直接传值，不要复杂格式化）
	if ctx.Performance != nil {
		// 直接从interface{}中提取SharpeRatio
		type PerformanceData struct {
			SharpeRatio float64 `json:"sharpe_ratio"`
		}
		var perfData PerformanceData
		if jsonData, err := json.Marshal(ctx.Performance); err == nil {
			if err := json.Unmarshal(jsonData, &perfData); err == nil {
				sb.WriteString(fmt.Sprintf("## 📊 夏普比率: %.2f\n\n", perfData.SharpeRatio))
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("现在请分析并输出决策（思维链 + JSON）\n")

	return sb.String()
}

// parseFullDecisionResponse 解析AI的完整决策响应
func parseFullDecisionResponse(aiResponse string, accountEquity float64, btcEthLeverage, altcoinLeverage int, exchange string) (*FullDecision, error) {
	// 1. 提取思维链
	cotTrace := extractCoTTrace(aiResponse)

	// 2. 提取JSON决策列表
	decisions, err := extractDecisions(aiResponse)
	if err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: []Decision{},
		}, fmt.Errorf("提取决策失败: %w", err)
	}

	// 3. 验证决策
	if err := validateDecisions(decisions, accountEquity, btcEthLeverage, altcoinLeverage, exchange); err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: decisions,
		}, fmt.Errorf("决策验证失败: %w", err)
	}

	return &FullDecision{
		CoTTrace:  cotTrace,
		Decisions: decisions,
	}, nil
}

// extractCoTTrace 提取思维链分析
func extractCoTTrace(response string) string {
	// 方法1: 优先尝试提取 <reasoning> 标签内容
	if match := reReasoningTag.FindStringSubmatch(response); match != nil && len(match) > 1 {
		log.Printf("✓ 使用 <reasoning> 标签提取思维链")
		return strings.TrimSpace(match[1])
	}

	// 方法2: 如果没有 <reasoning> 标签，但有 <decision> 标签，提取 <decision> 之前的内容
	if decisionIdx := strings.Index(response, "<decision>"); decisionIdx > 0 {
		log.Printf("✓ 提取 <decision> 标签之前的内容作为思维链")
		return strings.TrimSpace(response[:decisionIdx])
	}

	// 方法3: 后备方案 - 查找JSON数组的开始位置
	jsonStart := strings.Index(response, "[")
	if jsonStart > 0 {
		log.Printf("⚠️  使用旧版格式（[ 字符分离）提取思维链")
		return strings.TrimSpace(response[:jsonStart])
	}

	// 如果找不到任何标记，整个响应都是思维链
	return strings.TrimSpace(response)
}

// extractDecisions 提取JSON决策列表
func extractDecisions(response string) ([]Decision, error) {
	// 预清洗：去零宽/BOM
	s := removeInvisibleRunes(response)
	s = strings.TrimSpace(s)

	// 🔧 关键修复 (Critical Fix)：在正则匹配之前就先修复全角字符！
	// 否则正则表达式 \[ 无法匹配全角的 ［
	s = fixMissingQuotes(s)

	// 方法1: 优先尝试从 <decision> 标签中提取
	var jsonPart string
	if match := reDecisionTag.FindStringSubmatch(s); match != nil && len(match) > 1 {
		jsonPart = strings.TrimSpace(match[1])
		log.Printf("✓ 使用 <decision> 标签提取JSON")
	} else {
		// 后备方案：使用整个响应
		jsonPart = s
		log.Printf("⚠️  未找到 <decision> 标签，使用全文搜索JSON")
	}

	// 修复 jsonPart 中的全角字符
	jsonPart = fixMissingQuotes(jsonPart)

	// 1) 优先从 ```json 代码块中提取
	if m := reJSONFence.FindStringSubmatch(jsonPart); m != nil && len(m) > 1 {
		jsonContent := strings.TrimSpace(m[1])
		jsonContent = compactArrayOpen(jsonContent) // 把 "[ {" 规整为 "[{"
		jsonContent = fixMissingQuotes(jsonContent) // 二次修复（防止 regex 提取后还有残留全角）
		if err := validateJSONFormat(jsonContent); err != nil {
			return nil, fmt.Errorf("JSON格式验证失败: %w\nJSON内容: %s\n完整响应:\n%s", err, jsonContent, response)
		}
		var decisions []Decision
		if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
			return nil, fmt.Errorf("JSON解析失败: %w\nJSON内容: %s", err, jsonContent)
		}
		return decisions, nil
	}

	// 2) 退而求其次 (Fallback)：全文寻找首个对象数组
	// 注意：此时 jsonPart 已经过 fixMissingQuotes()，全角字符已转换为半角
	jsonContent := strings.TrimSpace(reJSONArray.FindString(jsonPart))
	if jsonContent == "" {
		// 🔧 安全回退 (Safe Fallback)：当AI只输出思维链没有JSON时，生成保底决策（避免系统崩溃）
		log.Printf("⚠️  [SafeFallback] AI未输出JSON决策，进入安全等待模式 (AI response without JSON, entering safe wait mode)")

		// 提取思维链摘要（最多 240 字符）
		cotSummary := jsonPart
		if len(cotSummary) > 240 {
			cotSummary = cotSummary[:240] + "..."
		}

		// 生成保底决策：所有币种进入 wait 状态
		fallbackDecision := Decision{
			Symbol:    "ALL",
			Action:    "wait",
			Reasoning: fmt.Sprintf("模型未输出结构化JSON决策，进入安全等待；摘要：%s", cotSummary),
		}

		return []Decision{fallbackDecision}, nil
	}

	// 🔧 规整格式（此时全角字符已在前面修复过）
	jsonContent = compactArrayOpen(jsonContent)
	jsonContent = fixMissingQuotes(jsonContent) // 二次修复（防止 regex 提取后还有残留全角）

	// 🔧 验证 JSON 格式（检测常见错误）
	if err := validateJSONFormat(jsonContent); err != nil {
		return nil, fmt.Errorf("JSON格式验证失败: %w\nJSON内容: %s\n完整响应:\n%s", err, jsonContent, response)
	}

	// 解析JSON
	var decisions []Decision
	if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w\nJSON内容: %s", err, jsonContent)
	}

	return decisions, nil
}

// fixMissingQuotes 替换中文引号和全角字符为英文引号和半角字符（避免AI输出全角JSON字符导致解析失败）
func fixMissingQuotes(jsonStr string) string {
	// 替换中文引号
	jsonStr = strings.ReplaceAll(jsonStr, "\u201c", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u201d", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u2018", "'")  // '
	jsonStr = strings.ReplaceAll(jsonStr, "\u2019", "'")  // '

	// ⚠️ 替换全角括号、冒号、逗号（防止AI输出全角JSON字符）
	jsonStr = strings.ReplaceAll(jsonStr, "［", "[") // U+FF3B 全角左方括号
	jsonStr = strings.ReplaceAll(jsonStr, "］", "]") // U+FF3D 全角右方括号
	jsonStr = strings.ReplaceAll(jsonStr, "｛", "{") // U+FF5B 全角左花括号
	jsonStr = strings.ReplaceAll(jsonStr, "｝", "}") // U+FF5D 全角右花括号
	jsonStr = strings.ReplaceAll(jsonStr, "：", ":") // U+FF1A 全角冒号
	jsonStr = strings.ReplaceAll(jsonStr, "，", ",") // U+FF0C 全角逗号

	// ⚠️ 替换CJK标点符号（AI在中文上下文中也可能输出这些）
	jsonStr = strings.ReplaceAll(jsonStr, "【", "[") // CJK左方头括号 U+3010
	jsonStr = strings.ReplaceAll(jsonStr, "】", "]") // CJK右方头括号 U+3011
	jsonStr = strings.ReplaceAll(jsonStr, "〔", "[") // CJK左龟壳括号 U+3014
	jsonStr = strings.ReplaceAll(jsonStr, "〕", "]") // CJK右龟壳括号 U+3015
	jsonStr = strings.ReplaceAll(jsonStr, "、", ",") // CJK顿号 U+3001

	// ⚠️ 替换全角空格为半角空格（JSON中不应该有全角空格）
	jsonStr = strings.ReplaceAll(jsonStr, "　", " ") // U+3000 全角空格

	return jsonStr
}

// validateJSONFormat 验证 JSON 格式，检测常见错误
func validateJSONFormat(jsonStr string) error {
	trimmed := strings.TrimSpace(jsonStr)

	// 允许 [ 和 { 之间存在任意空白（含零宽）
	if !reArrayHead.MatchString(trimmed) {
		// 检查是否是纯数字/范围数组（常见错误）
		if strings.HasPrefix(trimmed, "[") && !strings.Contains(trimmed[:min(20, len(trimmed))], "{") {
			return fmt.Errorf("不是有效的决策数组（必须包含对象 {}），实际内容: %s", trimmed[:min(50, len(trimmed))])
		}
		return fmt.Errorf("JSON 必须以 [{ 开头（允许空白），实际: %s", trimmed[:min(20, len(trimmed))])
	}

	// 检查是否包含范围符号 ~（LLM 常见错误）
	if strings.Contains(jsonStr, "~") {
		return fmt.Errorf("JSON 中不可包含范围符号 ~，所有数字必须是精确的单一值")
	}

	// 检查是否包含千位分隔符（如 98,000）
	// 使用简单的模式匹配：数字+逗号+3位数字
	for i := 0; i < len(jsonStr)-4; i++ {
		if jsonStr[i] >= '0' && jsonStr[i] <= '9' &&
			jsonStr[i+1] == ',' &&
			jsonStr[i+2] >= '0' && jsonStr[i+2] <= '9' &&
			jsonStr[i+3] >= '0' && jsonStr[i+3] <= '9' &&
			jsonStr[i+4] >= '0' && jsonStr[i+4] <= '9' {
			return fmt.Errorf("JSON 数字不可包含千位分隔符逗号，发现: %s", jsonStr[i:min(i+10, len(jsonStr))])
		}
	}

	return nil
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// removeInvisibleRunes 去除零宽字符和 BOM，避免肉眼看不见的前缀破坏校验
func removeInvisibleRunes(s string) string {
	return reInvisibleRunes.ReplaceAllString(s, "")
}

// compactArrayOpen 规整开头的 "[ {" → "[{"
func compactArrayOpen(s string) string {
	return reArrayOpenSpace.ReplaceAllString(strings.TrimSpace(s), "[{")
}

// validateDecisions 验证所有决策（需要账户信息和杠杆配置）
func validateDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, exchange string) error {
	for i, decision := range decisions {
		if err := validateDecision(&decision, accountEquity, btcEthLeverage, altcoinLeverage, exchange); err != nil {
			return fmt.Errorf("决策 #%d 验证失败: %w", i+1, err)
		}
	}
	return nil
}

// findMatchingBracket 查找匹配的右括号
func findMatchingBracket(s string, start int) int {
	if start >= len(s) || s[start] != '[' {
		return -1
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// getMinPositionSize 根据交易所返回最小开仓金额
// - Binance: MIN_NOTIONAL=100 USDT
// - Hyperliquid: 最小 12 USDT
// - Aster: 最小 10 USDT（实际 $5 + 安全边际）
func getMinPositionSize(exchange string) float64 {
	switch strings.ToLower(exchange) {
	case "hyperliquid":
		return 12.0
	case "aster":
		return 10.0
	default: // binance 及其他
		return 100.0
	}
}

// validateDecision 验证单个决策的有效性
func validateDecision(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, exchange string) error {
	// 验证action
	validActions := map[string]bool{
		"open_long":          true,
		"open_short":         true,
		"close_long":         true,
		"close_short":        true,
		"update_stop_loss":   true,
		"update_take_profit": true,
		"partial_close":      true,
		"hold":               true,
		"wait":               true,
	}

	if !validActions[d.Action] {
		return fmt.Errorf("无效的action: %s", d.Action)
	}

	// 开仓操作必须提供完整参数
	if d.Action == "open_long" || d.Action == "open_short" {
		// 根据币种使用配置的杠杆上限
		maxLeverage := altcoinLeverage          // 山寨币使用配置的杠杆
		maxPositionValue := accountEquity * 1.5 // 山寨币最多1.5倍账户净值
		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			maxLeverage = btcEthLeverage          // BTC和ETH使用配置的杠杆
			maxPositionValue = accountEquity * 10 // BTC/ETH最多10倍账户净值
		}

		// 杠杆验证：超限时拒绝决策（与 Prompt 表述一致）
		if d.Leverage <= 0 {
			return fmt.Errorf("杠杆必须大于0: %d", d.Leverage)
		}
		if d.Leverage > maxLeverage {
			return fmt.Errorf("杠杆超限(%dx)，%s 最大允许 %dx", d.Leverage, d.Symbol, maxLeverage)
		}
		if d.PositionSizeUSD <= 0 {
			return fmt.Errorf("仓位大小必须大于0: %.2f", d.PositionSizeUSD)
		}

		// ✅ 验证最小开仓金额（使用 getMinPositionSize 保证与 Prompt 一致）
		minPositionSize := getMinPositionSize(exchange)
		if d.PositionSizeUSD < minPositionSize {
			return fmt.Errorf("开仓金额过小(%.2f USDT)，必须≥%.2f USDT（%s 交易所要求）", d.PositionSizeUSD, minPositionSize, exchange)
		}

		// 验证仓位价值上限（加1%容差以避免浮点数精度问题）
		tolerance := maxPositionValue * 0.01 // 1%容差
		if d.PositionSizeUSD > maxPositionValue+tolerance {
			if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
				return fmt.Errorf("BTC/ETH单币种仓位价值不能超过%.0f USDT（10倍账户净值），实际: %.0f", maxPositionValue, d.PositionSizeUSD)
			} else {
				return fmt.Errorf("山寨币单币种仓位价值不能超过%.0f USDT（1.5倍账户净值），实际: %.0f", maxPositionValue, d.PositionSizeUSD)
			}
		}
		if d.StopLoss <= 0 || d.TakeProfit <= 0 {
			return fmt.Errorf("止损和止盈必须大于0")
		}

		// 验证止损止盈的合理性
		if d.Action == "open_long" {
			if d.StopLoss >= d.TakeProfit {
				return fmt.Errorf("做多时止损价必须小于止盈价")
			}
		} else {
			if d.StopLoss <= d.TakeProfit {
				return fmt.Errorf("做空时止损价必须大于止盈价")
			}
		}

		// 验证风险回报比（必须≥1:3）
		// 计算入场价（假设当前市价）
		var entryPrice float64
		if d.Action == "open_long" {
			// 做多：入场价在止损和止盈之间
			entryPrice = d.StopLoss + (d.TakeProfit-d.StopLoss)*0.2 // 假设在20%位置入场
		} else {
			// 做空：入场价在止损和止盈之间
			entryPrice = d.StopLoss - (d.StopLoss-d.TakeProfit)*0.2 // 假设在20%位置入场
		}

		var riskPercent, rewardPercent, riskRewardRatio float64
		if d.Action == "open_long" {
			riskPercent = (entryPrice - d.StopLoss) / entryPrice * 100
			rewardPercent = (d.TakeProfit - entryPrice) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		} else {
			riskPercent = (d.StopLoss - entryPrice) / entryPrice * 100
			rewardPercent = (entryPrice - d.TakeProfit) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		}

		// 硬约束：风险回报比必须≥3.0
		if riskRewardRatio < 3.0 {
			return fmt.Errorf("风险回报比过低(%.2f:1)，必须≥3.0:1 [风险:%.2f%% 收益:%.2f%%] [止损:%.2f 止盈:%.2f]",
				riskRewardRatio, riskPercent, rewardPercent, d.StopLoss, d.TakeProfit)
		}
	}

	// 动态调整止损验证
	if d.Action == "update_stop_loss" {
		if d.NewStopLoss <= 0 {
			return fmt.Errorf("新止损价格必须大于0: %.2f", d.NewStopLoss)
		}
	}

	// 动态调整止盈验证
	if d.Action == "update_take_profit" {
		if d.NewTakeProfit <= 0 {
			return fmt.Errorf("新止盈价格必须大于0: %.2f", d.NewTakeProfit)
		}
	}

	// 部分平仓验证
	if d.Action == "partial_close" {
		if d.ClosePercentage <= 0 || d.ClosePercentage > 100 {
			return fmt.Errorf("平仓百分比必须在0-100之间: %.1f", d.ClosePercentage)
		}
	}

	return nil
}

// BuildPromptSnapshot 生成完整的 prompt 内容快照（用于回测记录）
// 参数说明：
//   - accountEquity: 账户净值
//   - btcEthLeverage: BTC/ETH 杠杆
//   - altcoinLeverage: 山寨币杠杆
//   - customPrompt: 自定义 prompt
//   - overrideBase: 是否覆盖基础 prompt
//   - templateName: 模板名称
//   - exchange: 交易所名称
func BuildPromptSnapshot(accountEquity float64, btcEthLeverage, altcoinLeverage int, customPrompt string, overrideBase bool, templateName string, exchange string) string {
	minPositionSize := getMinPositionSize(exchange)
	return buildSystemPromptWithCustom(accountEquity, btcEthLeverage, altcoinLeverage, customPrompt, overrideBase, templateName, minPositionSize)
}
