package logger

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// 性能分析相关常量
const (
	// AIAnalysisSampleSize AI 性能分析的固定样本量
	// 统计指标（胜率、夏普比率等）基于最近 N 笔交易计算
	AIAnalysisSampleSize = 100

	// InitialScanCycles 首次初始化时扫描的决策周期数量
	// 目标：获取足够的交易填充缓存（至少 AIAnalysisSampleSize 笔）
	// 假设每 3 分钟一个周期，10000 个周期 ≈ 500 小时历史数据
	InitialScanCycles = 10000
)

// DecisionRecord 决策记录
type DecisionRecord struct {
	Timestamp      time.Time          `json:"timestamp"`       // 决策时间
	CycleNumber    int                `json:"cycle_number"`    // 周期编号
	Exchange       string             `json:"exchange"`        // 交易所类型 (binance/hyperliquid/aster)
	SystemPrompt   string             `json:"system_prompt"`   // 系统提示词（发送给AI的系统prompt）
	InputPrompt    string             `json:"input_prompt"`    // 发送给AI的输入prompt
	CoTTrace       string             `json:"cot_trace"`       // AI思维链（输出）
	DecisionJSON   string             `json:"decision_json"`   // 决策JSON
	AccountState   AccountSnapshot    `json:"account_state"`   // 账户状态快照
	Positions      []PositionSnapshot `json:"positions"`       // 持仓快照
	CandidateCoins []string           `json:"candidate_coins"` // 候选币种列表
	Decisions      []DecisionAction   `json:"decisions"`       // 执行的决策
	ExecutionLog   []string           `json:"execution_log"`   // 执行日志
	Success        bool               `json:"success"`         // 是否成功
	ErrorMessage   string             `json:"error_message"`   // 错误信息（如果有）
	// AIRequestDurationMs 记录 AI API 调用耗时（毫秒），方便评估调用性能
	AIRequestDurationMs int64  `json:"ai_request_duration_ms,omitempty"`
	PromptHash          string `json:"prompt_hash,omitempty"` // Prompt模板版本哈希
}

// AccountSnapshot 账户状态快照
type AccountSnapshot struct {
	TotalBalance          float64 `json:"total_balance"`
	AvailableBalance      float64 `json:"available_balance"`
	TotalUnrealizedProfit float64 `json:"total_unrealized_profit"`
	PositionCount         int     `json:"position_count"`
	MarginUsedPct         float64 `json:"margin_used_pct"`
	InitialBalance        float64 `json:"initial_balance"` // 记录当时的初始余额基准
}

// PositionSnapshot 持仓快照
type PositionSnapshot struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"`
	PositionAmt      float64 `json:"position_amt"`
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	UnrealizedProfit float64 `json:"unrealized_profit"`
	Leverage         float64 `json:"leverage"`
	LiquidationPrice float64 `json:"liquidation_price"`
}

// DecisionAction 决策动作
type DecisionAction struct {
	Action    string    `json:"action"`    // open_long, open_short, close_long, close_short, update_stop_loss, update_take_profit, partial_close
	Symbol    string    `json:"symbol"`    // 币种
	Quantity  float64   `json:"quantity"`  // 数量（部分平仓时使用）
	Leverage  int       `json:"leverage"`  // 杠杆（开仓时）
	Price     float64   `json:"price"`     // 执行价格
	OrderID   int64     `json:"order_id"`  // 订单ID
	Timestamp time.Time `json:"timestamp"` // 执行时间
	Success   bool      `json:"success"`   // 是否成功
	Error     string    `json:"error"`     // 错误信息

	// 调整参数（用于前端显示）
	NewStopLoss     float64 `json:"new_stop_loss,omitempty"`     // 新止损价格（update_stop_loss 时使用）
	NewTakeProfit   float64 `json:"new_take_profit,omitempty"`   // 新止盈价格（update_take_profit 时使用）
	ClosePercentage float64 `json:"close_percentage,omitempty"`  // 平仓百分比（partial_close 时使用，0-100）
}

// IDecisionLogger 决策日志记录器接口
type IDecisionLogger interface {
	// LogDecision 记录决策
	LogDecision(record *DecisionRecord) error
	// GetLatestRecords 获取最近N条记录（按时间正序：从旧到新）
	GetLatestRecords(n int) ([]*DecisionRecord, error)
	// GetRecordByDate 获取指定日期的所有记录
	GetRecordByDate(date time.Time) ([]*DecisionRecord, error)
	// CleanOldRecords 清理N天前的旧记录
	CleanOldRecords(days int) error
	// GetStatistics 获取统计信息
	GetStatistics() (*Statistics, error)
	// AnalyzePerformance 分析最近N个周期的交易表现
	AnalyzePerformance(lookbackCycles int) (*PerformanceAnalysis, error)
	// SetCycleNumber 设置周期编号（用于回测恢复检查点）
	SetCycleNumber(cycle int)
	// AddTradeToCache 添加交易到缓存
	AddTradeToCache(trade TradeOutcome)
	// GetRecentTrades 从缓存获取最近N条交易
	GetRecentTrades(limit int) []TradeOutcome
	// GetPerformanceWithCache 使用缓存机制获取历史表现分析（懒加载）
	// tradeLimit: 返回的交易记录数量限制
	// filterByPrompt: 是否按当前 PromptHash 过滤交易（默认 false 显示所有）
	GetPerformanceWithCache(tradeLimit int, filterByPrompt bool) (*PerformanceAnalysis, error)
}

// OpenPosition 记录开仓信息（用于主动维护缓存）
type OpenPosition struct {
	Symbol    string
	Side      string  // long/short
	Quantity  float64
	EntryPrice float64
	Leverage  int
	OpenTime  time.Time
	Exchange  string
}

// EquityPoint 账户净值记录点
type EquityPoint struct {
	Timestamp time.Time
	Equity    float64
}

// DecisionLogger 决策日志记录器
type DecisionLogger struct {
	logDir        string
	cycleNumber   int
	tradesCache   []TradeOutcome       // 交易缓存（最新的在前）
	tradeCacheSet map[string]bool      // 已缓存交易的 Set（去重用）
	equityCache   []EquityPoint        // 净值历史缓存（最新的在前）
	cacheMutex    sync.RWMutex         // 缓存读写锁
	maxCacheSize  int                  // 最大缓存条数
	maxEquitySize int                  // 最大净值缓存条数
	openPositions map[string]*OpenPosition // 当前开仓（用于主动维护）
	positionMutex sync.RWMutex             // 持仓读写锁
}

// NewDecisionLogger 创建决策日志记录器
func NewDecisionLogger(logDir string) IDecisionLogger {
	if logDir == "" {
		logDir = "decision_logs"
	}

	// 确保日志目录存在（使用安全权限：只有所有者可访问）
	if err := os.MkdirAll(logDir, 0700); err != nil {
		fmt.Printf("⚠ 创建日志目录失败: %v\n", err)
	}

	// 强制设置目录权限（即使目录已存在）- 确保安全
	if err := os.Chmod(logDir, 0700); err != nil {
		fmt.Printf("⚠ 设置日志目录权限失败: %v\n", err)
	}

	logger := &DecisionLogger{
		logDir:        logDir,
		cycleNumber:   0,
		tradesCache:   make([]TradeOutcome, 0, 100),
		tradeCacheSet: make(map[string]bool, 100),
		equityCache:   make([]EquityPoint, 0, 200),
		maxCacheSize:  100, // 缓存 100 条交易（与前端 limit 最大值一致）
		maxEquitySize: 200, // 缓存 200 个净值点（足够计算SharpeRatio）
		openPositions: make(map[string]*OpenPosition),
	}

	// 🚀 启动时初始化缓存和持仓 (Fix for Issue #43)
	logger.initializeCacheOnStartup()

	return logger
}

// SetCycleNumber 设置周期编号（用于回测恢复检查点）
func (l *DecisionLogger) SetCycleNumber(cycle int) {
	l.cycleNumber = cycle
}

// LogDecision 记录决策
func (l *DecisionLogger) LogDecision(record *DecisionRecord) error {
	l.cycleNumber++
	record.CycleNumber = l.cycleNumber
	record.Timestamp = time.Now()

	// 生成文件名：decision_YYYYMMDD_HHMMSS_cycleN.json
	filename := fmt.Sprintf("decision_%s_cycle%d.json",
		record.Timestamp.Format("20060102_150405"),
		record.CycleNumber)

	filepath := filepath.Join(l.logDir, filename)

	// 序列化为JSON（带缩进，方便阅读）
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化决策记录失败: %w", err)
	}

	// 写入文件（使用安全权限：只有所有者可读写）
	if err := ioutil.WriteFile(filepath, data, 0600); err != nil {
		return fmt.Errorf("写入决策记录失败: %w", err)
	}

	fmt.Printf("📝 决策记录已保存: %s\n", filename)

	// 🚀 主动维护：检测交易完成并更新缓存
	l.updateCacheFromDecision(record)

	// 🚀 记录equity到缓存（用于SharpeRatio计算）
	l.addEquityToCache(record.Timestamp, record.AccountState.TotalBalance)

	return nil
}

// GetLatestRecords 获取最近N条记录（按时间正序：从旧到新）
func (l *DecisionLogger) GetLatestRecords(n int) ([]*DecisionRecord, error) {
	files, err := ioutil.ReadDir(l.logDir)
	if err != nil {
		return nil, fmt.Errorf("读取日志目录失败: %w", err)
	}

	// 按文件名排序（文件名包含timestamp和cycle,最新的在前）
	// 注意: 使用文件名而非修改时间,因为文件名包含精确的时间戳和cycle编号
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() > files[j].Name()
	})

	// 按修改时间倒序收集（最新的在前）
	var records []*DecisionRecord
	count := 0
	for i := 0; i < len(files) && count < n; i++ {
		file := files[i]
		if file.IsDir() {
			continue
		}

		filepath := filepath.Join(l.logDir, file.Name())
		data, err := ioutil.ReadFile(filepath)
		if err != nil {
			continue
		}

		var record DecisionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		records = append(records, &record)
		count++
	}

	// 反转数组，让时间从旧到新排列（用于图表显示）
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	return records, nil
}

// GetRecordByDate 获取指定日期的所有记录
func (l *DecisionLogger) GetRecordByDate(date time.Time) ([]*DecisionRecord, error) {
	dateStr := date.Format("20060102")
	pattern := filepath.Join(l.logDir, fmt.Sprintf("decision_%s_*.json", dateStr))

	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("查找日志文件失败: %w", err)
	}

	var records []*DecisionRecord
	for _, filepath := range files {
		data, err := ioutil.ReadFile(filepath)
		if err != nil {
			continue
		}

		var record DecisionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		records = append(records, &record)
	}

	return records, nil
}

// CleanOldRecords 清理N天前的旧记录
func (l *DecisionLogger) CleanOldRecords(days int) error {
	cutoffTime := time.Now().AddDate(0, 0, -days)

	files, err := ioutil.ReadDir(l.logDir)
	if err != nil {
		return fmt.Errorf("读取日志目录失败: %w", err)
	}

	removedCount := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if file.ModTime().Before(cutoffTime) {
			filepath := filepath.Join(l.logDir, file.Name())
			if err := os.Remove(filepath); err != nil {
				fmt.Printf("⚠ 删除旧记录失败 %s: %v\n", file.Name(), err)
				continue
			}
			removedCount++
		}
	}

	if removedCount > 0 {
		fmt.Printf("🗑️ 已清理 %d 条旧记录（%d天前）\n", removedCount, days)
	}

	return nil
}

// GetStatistics 获取统计信息
func (l *DecisionLogger) GetStatistics() (*Statistics, error) {
	files, err := ioutil.ReadDir(l.logDir)
	if err != nil {
		return nil, fmt.Errorf("读取日志目录失败: %w", err)
	}

	stats := &Statistics{}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filepath := filepath.Join(l.logDir, file.Name())
		data, err := ioutil.ReadFile(filepath)
		if err != nil {
			continue
		}

		var record DecisionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		stats.TotalCycles++

		for _, action := range record.Decisions {
			if action.Success {
				switch action.Action {
				case "open_long", "open_short":
					stats.TotalOpenPositions++
				case "close_long", "close_short", "auto_close_long", "auto_close_short":
					stats.TotalClosePositions++
					// 🔧 BUG FIX：partial_close 不計入 TotalClosePositions，避免重複計數
					// case "partial_close": // 不計數，因為只有完全平倉才算一次
					// update_stop_loss 和 update_take_profit 不計入統計
				}
			}
		}

		if record.Success {
			stats.SuccessfulCycles++
		} else {
			stats.FailedCycles++
		}
	}

	return stats, nil
}

// Statistics 统计信息
type Statistics struct {
	TotalCycles         int `json:"total_cycles"`
	SuccessfulCycles    int `json:"successful_cycles"`
	FailedCycles        int `json:"failed_cycles"`
	TotalOpenPositions  int `json:"total_open_positions"`
	TotalClosePositions int `json:"total_close_positions"`
}

// TradeOutcome 单笔交易结果
type TradeOutcome struct {
	Symbol        string    `json:"symbol"`         // 币种
	Side          string    `json:"side"`           // long/short
	Quantity      float64   `json:"quantity"`       // 仓位数量
	Leverage      int       `json:"leverage"`       // 杠杆倍数
	OpenPrice     float64   `json:"open_price"`     // 开仓价
	ClosePrice    float64   `json:"close_price"`    // 平仓价
	PositionValue float64   `json:"position_value"` // 仓位价值（quantity × openPrice）
	MarginUsed    float64   `json:"margin_used"`    // 保证金使用（positionValue / leverage）
	PnL           float64   `json:"pn_l"`           // 盈亏（USDT）
	PnLPct        float64   `json:"pn_l_pct"`       // 盈亏百分比（相对保证金）
	Duration      string    `json:"duration"`       // 持仓时长
	OpenTime      time.Time `json:"open_time"`      // 开仓时间
	CloseTime     time.Time `json:"close_time"`     // 平仓时间
	WasStopLoss   bool      `json:"was_stop_loss"`  // 是否止损

	// Prompt 版本标识（用于追溯和分组）
	PromptHash string `json:"prompt_hash,omitempty"` // SystemPrompt 的 MD5 hash
}

// PerformanceAnalysis 交易表现分析
type PerformanceAnalysis struct {
	TotalTrades   int                           `json:"total_trades"`   // 总交易数
	WinningTrades int                           `json:"winning_trades"` // 盈利交易数
	LosingTrades  int                           `json:"losing_trades"`  // 亏损交易数
	WinRate       float64                       `json:"win_rate"`       // 胜率
	AvgWin        float64                       `json:"avg_win"`        // 平均盈利
	AvgLoss       float64                       `json:"avg_loss"`       // 平均亏损
	ProfitFactor  float64                       `json:"profit_factor"`  // 盈亏比
	SharpeRatio   float64                       `json:"sharpe_ratio"`   // 夏普比率（风险调整后收益）
	RecentTrades  []TradeOutcome                `json:"recent_trades"`  // 最近N笔交易
	SymbolStats   map[string]*SymbolPerformance `json:"symbol_stats"`   // 各币种表现
	BestSymbol    string                        `json:"best_symbol"`    // 表现最好的币种
	WorstSymbol   string                        `json:"worst_symbol"`   // 表现最差的币种
}

// SymbolPerformance 币种表现统计
type SymbolPerformance struct {
	Symbol        string  `json:"symbol"`         // 币种
	TotalTrades   int     `json:"total_trades"`   // 交易次数
	WinningTrades int     `json:"winning_trades"` // 盈利次数
	LosingTrades  int     `json:"losing_trades"`  // 亏损次数
	WinRate       float64 `json:"win_rate"`       // 胜率
	TotalPnL      float64 `json:"total_pn_l"`     // 总盈亏
	AvgPnL        float64 `json:"avg_pn_l"`       // 平均盈亏
}

// getTakerFeeRate 获取交易所的Taker费率
// 基于公开信息：
// - Aster: Maker 0.010%, Taker 0.035%
// - Hyperliquid: Maker 0.015%, Taker 0.045%
// - Binance Futures: Maker 0.020%, Taker 0.050% (默认费率)
func getTakerFeeRate(exchange string) float64 {
	switch exchange {
	case "aster":
		return 0.00035 // 0.035%
	case "hyperliquid":
		return 0.00045 // 0.045%
	case "binance":
		return 0.0005 // 0.050%
	default:
		// 对于未知交易所，使用保守估计（Binance费率）
		return 0.0005
	}
}

// AnalyzePerformance 分析最近N个周期的交易表现
func (l *DecisionLogger) AnalyzePerformance(lookbackCycles int) (*PerformanceAnalysis, error) {
	records, err := l.GetLatestRecords(lookbackCycles)
	if err != nil {
		return nil, fmt.Errorf("读取历史记录失败: %w", err)
	}

	if len(records) == 0 {
		return &PerformanceAnalysis{
			RecentTrades: []TradeOutcome{},
			SymbolStats:  make(map[string]*SymbolPerformance),
		}, nil
	}

	analysis := &PerformanceAnalysis{
		RecentTrades: []TradeOutcome{},
		SymbolStats:  make(map[string]*SymbolPerformance),
	}

	// 追踪持仓状态：symbol_side -> {side, openPrice, openTime, quantity, leverage}
	openPositions := make(map[string]map[string]interface{})

	// 为了避免开仓记录在窗口外导致匹配失败，需要先从所有历史记录中找出未平仓的持仓
	// 获取更多历史记录来构建完整的持仓状态（使用更大的窗口）
	allRecords, err := l.GetLatestRecords(lookbackCycles * 3) // 扩大3倍窗口
	if err == nil && len(allRecords) >= len(records) {
		// 先从扩大的窗口中收集所有开仓记录
		for _, record := range allRecords {
			for _, action := range record.Decisions {
				if !action.Success {
					continue
				}

				symbol := action.Symbol
				side := ""
				if action.Action == "open_long" || action.Action == "close_long" || action.Action == "partial_close" || action.Action == "auto_close_long" {
					side = "long"
				} else if action.Action == "open_short" || action.Action == "close_short" || action.Action == "auto_close_short" {
					side = "short"
				}

				// partial_close 需要根據持倉判斷方向
				if action.Action == "partial_close" && side == "" {
					for key, pos := range openPositions {
						if posSymbol, _ := pos["side"].(string); key == symbol+"_"+posSymbol {
							side = posSymbol
							break
						}
					}
				}

				posKey := symbol + "_" + side

				switch action.Action {
				case "open_long", "open_short":
					// 记录开仓
					openPositions[posKey] = map[string]interface{}{
						"side":      side,
						"openPrice": action.Price,
						"openTime":  action.Timestamp,
						"quantity":  action.Quantity,
						"leverage":  action.Leverage,
					}
				case "close_long", "close_short", "auto_close_long", "auto_close_short":
					// 移除已平仓记录
					delete(openPositions, posKey)
					// partial_close 不處理，保留持倉記錄
				}
			}
		}
	}

	// 遍历分析窗口内的记录，生成交易结果
	for _, record := range records {
		for _, action := range record.Decisions {
			if !action.Success {
				continue
			}

			symbol := action.Symbol
			side := ""
			if action.Action == "open_long" || action.Action == "close_long" || action.Action == "partial_close" || action.Action == "auto_close_long" {
				side = "long"
			} else if action.Action == "open_short" || action.Action == "close_short" || action.Action == "auto_close_short" {
				side = "short"
			}

			// partial_close 需要根據持倉判斷方向
			if action.Action == "partial_close" {
				// 從 openPositions 中查找持倉方向
				for key, pos := range openPositions {
					if posSymbol, _ := pos["side"].(string); key == symbol+"_"+posSymbol {
						side = posSymbol
						break
					}
				}
			}

			posKey := symbol + "_" + side // 使用symbol_side作为key，区分多空持仓

			switch action.Action {
			case "open_long", "open_short":
				// 更新开仓记录（可能已经在预填充时记录过了）
				openPositions[posKey] = map[string]interface{}{
					"side":               side,
					"openPrice":          action.Price,
					"openTime":           action.Timestamp,
					"quantity":           action.Quantity,
					"leverage":           action.Leverage,
					"remainingQuantity":  action.Quantity, // 🔧 BUG FIX：追蹤剩餘數量
					"accumulatedPnL":     0.0,             // 🔧 BUG FIX：累積部分平倉盈虧
					"partialCloseCount":  0,               // 🔧 BUG FIX：部分平倉次數
					"partialCloseVolume": 0.0,             // 🔧 BUG FIX：部分平倉總量
				}

			case "close_long", "close_short", "partial_close", "auto_close_long", "auto_close_short":
				// 查找对应的开仓记录（可能来自预填充或当前窗口）
				if openPos, exists := openPositions[posKey]; exists {
					openPrice := openPos["openPrice"].(float64)
					openTime := openPos["openTime"].(time.Time)
					side := openPos["side"].(string)
					quantity := openPos["quantity"].(float64)
					leverage := openPos["leverage"].(int)

					// 🔧 BUG FIX：取得追蹤字段（若不存在則初始化）
					remainingQty, _ := openPos["remainingQuantity"].(float64)
					if remainingQty == 0 {
						remainingQty = quantity // 兼容舊數據（沒有 remainingQuantity 字段）
					}
					accumulatedPnL, _ := openPos["accumulatedPnL"].(float64)
					partialCloseCount, _ := openPos["partialCloseCount"].(int)
					partialCloseVolume, _ := openPos["partialCloseVolume"].(float64)

					// 对于 partial_close，使用实际平仓数量；否则使用剩余仓位数量
					actualQuantity := remainingQty
					if action.Action == "partial_close" {
						actualQuantity = action.Quantity
					}

					// 计算本次平仓的盈亏（USDT）- 包含手续费
					var pnl float64
					if side == "long" {
						pnl = actualQuantity * (action.Price - openPrice)
					} else {
						pnl = actualQuantity * (openPrice - action.Price)
					}

					// ⚠️ 扣除交易手续费（开仓 + 平仓各一次）
					// 获取交易所费率（从record中获取，如果没有则使用默认值）
					feeRate := getTakerFeeRate(record.Exchange)
					openFee := actualQuantity * openPrice * feeRate   // 开仓手续费
					closeFee := actualQuantity * action.Price * feeRate // 平仓手续费
					totalFees := openFee + closeFee
					pnl -= totalFees // 从盈亏中扣除手续费

					// 🔧 BUG FIX：處理 partial_close 聚合邏輯
					if action.Action == "partial_close" {
						// 累積盈虧和數量
						accumulatedPnL += pnl
						remainingQty -= actualQuantity
						partialCloseCount++
						partialCloseVolume += actualQuantity

						// 更新 openPositions（保留持倉記錄，但更新追蹤數據）
						openPos["remainingQuantity"] = remainingQty
						openPos["accumulatedPnL"] = accumulatedPnL
						openPos["partialCloseCount"] = partialCloseCount
						openPos["partialCloseVolume"] = partialCloseVolume

						// 判斷是否已完全平倉
						if remainingQty <= 0.0001 { // 使用小閾值避免浮點誤差
							// ✅ 完全平倉：記錄為一筆完整交易
							positionValue := quantity * openPrice
							marginUsed := positionValue / float64(leverage)
							pnlPct := 0.0
							if marginUsed > 0 {
								pnlPct = (accumulatedPnL / marginUsed) * 100
							}

							outcome := TradeOutcome{
								Symbol:        symbol,
								Side:          side,
								Quantity:      quantity, // 使用原始總量
								Leverage:      leverage,
								OpenPrice:     openPrice,
								ClosePrice:    action.Price, // 最後一次平倉價格
								PositionValue: positionValue,
								MarginUsed:    marginUsed,
								PnL:           accumulatedPnL, // 🔧 使用累積盈虧
								PnLPct:        pnlPct,
								Duration:      action.Timestamp.Sub(openTime).String(),
								OpenTime:      openTime,
								CloseTime:     action.Timestamp,
							}

							analysis.RecentTrades = append(analysis.RecentTrades, outcome)
							analysis.TotalTrades++ // 🔧 只在完全平倉時計數

							// 🚀 添加到内存缓存
							l.AddTradeToCache(outcome)

							// 分类交易
							if accumulatedPnL > 0 {
								analysis.WinningTrades++
								analysis.AvgWin += accumulatedPnL
							} else if accumulatedPnL < 0 {
								analysis.LosingTrades++
								analysis.AvgLoss += accumulatedPnL
							}

							// 更新币种统计
							if _, exists := analysis.SymbolStats[symbol]; !exists {
								analysis.SymbolStats[symbol] = &SymbolPerformance{
									Symbol: symbol,
								}
							}
							stats := analysis.SymbolStats[symbol]
							stats.TotalTrades++
							stats.TotalPnL += accumulatedPnL
							if accumulatedPnL > 0 {
								stats.WinningTrades++
							} else if accumulatedPnL < 0 {
								stats.LosingTrades++
							}

							// 刪除持倉記錄
							delete(openPositions, posKey)
						}
						// ⚠️ 否則不做任何操作（等待後續 partial_close 或 full close）

					} else {
						// 🔧 完全平倉（close_long/close_short/auto_close）
						// 如果之前有部分平倉，需要加上累積的 PnL
						totalPnL := accumulatedPnL + pnl

						positionValue := quantity * openPrice
						marginUsed := positionValue / float64(leverage)
						pnlPct := 0.0
						if marginUsed > 0 {
							pnlPct = (totalPnL / marginUsed) * 100
						}

						outcome := TradeOutcome{
							Symbol:        symbol,
							Side:          side,
							Quantity:      quantity, // 使用原始總量
							Leverage:      leverage,
							OpenPrice:     openPrice,
							ClosePrice:    action.Price,
							PositionValue: positionValue,
							MarginUsed:    marginUsed,
							PnL:           totalPnL, // 🔧 包含之前部分平倉的 PnL
							PnLPct:        pnlPct,
							Duration:      action.Timestamp.Sub(openTime).String(),
							OpenTime:      openTime,
							CloseTime:     action.Timestamp,
						}

						analysis.RecentTrades = append(analysis.RecentTrades, outcome)
						analysis.TotalTrades++

						// 🚀 添加到内存缓存
						l.AddTradeToCache(outcome)

						// 分类交易
						if totalPnL > 0 {
							analysis.WinningTrades++
							analysis.AvgWin += totalPnL
						} else if totalPnL < 0 {
							analysis.LosingTrades++
							analysis.AvgLoss += totalPnL
						}

						// 更新币种统计
						if _, exists := analysis.SymbolStats[symbol]; !exists {
							analysis.SymbolStats[symbol] = &SymbolPerformance{
								Symbol: symbol,
							}
						}
						stats := analysis.SymbolStats[symbol]
						stats.TotalTrades++
						stats.TotalPnL += totalPnL
						if totalPnL > 0 {
							stats.WinningTrades++
						} else if totalPnL < 0 {
							stats.LosingTrades++
						}

						// 刪除持倉記錄
						delete(openPositions, posKey)
					}
				}
			}
		}
	}

	// 计算统计指标
	if analysis.TotalTrades > 0 {
		analysis.WinRate = (float64(analysis.WinningTrades) / float64(analysis.TotalTrades)) * 100

		// 计算总盈利和总亏损
		totalWinAmount := analysis.AvgWin   // 当前是累加的总和
		totalLossAmount := analysis.AvgLoss // 当前是累加的总和（负数）

		if analysis.WinningTrades > 0 {
			analysis.AvgWin /= float64(analysis.WinningTrades)
		}
		if analysis.LosingTrades > 0 {
			analysis.AvgLoss /= float64(analysis.LosingTrades)
		}

		// Profit Factor = 总盈利 / 总亏损（绝对值）
		// 注意：totalLossAmount 是负数，所以取负号得到绝对值
		if totalLossAmount != 0 {
			analysis.ProfitFactor = totalWinAmount / (-totalLossAmount)
		} else if totalWinAmount > 0 {
			// 只有盈利没有亏损的情况，设置为一个很大的值表示完美策略
			analysis.ProfitFactor = 999.0
		}
	}

	// 计算各币种胜率和平均盈亏
	bestPnL := -999999.0
	worstPnL := 999999.0
	for symbol, stats := range analysis.SymbolStats {
		if stats.TotalTrades > 0 {
			stats.WinRate = (float64(stats.WinningTrades) / float64(stats.TotalTrades)) * 100
			stats.AvgPnL = stats.TotalPnL / float64(stats.TotalTrades)

			if stats.TotalPnL > bestPnL {
				bestPnL = stats.TotalPnL
				analysis.BestSymbol = symbol
			}
			if stats.TotalPnL < worstPnL {
				worstPnL = stats.TotalPnL
				analysis.WorstSymbol = symbol
			}
		}
	}

	// 只保留最近的交易（倒序：最新的在前）
	if len(analysis.RecentTrades) > 10 {
		// 反转数组，让最新的在前
		for i, j := 0, len(analysis.RecentTrades)-1; i < j; i, j = i+1, j-1 {
			analysis.RecentTrades[i], analysis.RecentTrades[j] = analysis.RecentTrades[j], analysis.RecentTrades[i]
		}
		analysis.RecentTrades = analysis.RecentTrades[:10]
	} else if len(analysis.RecentTrades) > 0 {
		// 反转数组
		for i, j := 0, len(analysis.RecentTrades)-1; i < j; i, j = i+1, j-1 {
			analysis.RecentTrades[i], analysis.RecentTrades[j] = analysis.RecentTrades[j], analysis.RecentTrades[i]
		}
	}

	// 计算夏普比率（需要至少2个数据点）
	analysis.SharpeRatio = l.calculateSharpeRatio(records)

	return analysis, nil
}

// calculateSharpeRatio 计算夏普比率
// 基于账户净值的变化计算风险调整后收益
func (l *DecisionLogger) calculateSharpeRatio(records []*DecisionRecord) float64 {
	if len(records) < 2 {
		return 0.0
	}

	// 提取每个周期的账户净值
	// 注意：TotalBalance字段实际存储的是TotalEquity（账户总净值）
	// TotalUnrealizedProfit字段实际存储的是TotalPnL（相对初始余额的盈亏）
	var equities []float64
	for _, record := range records {
		// 直接使用TotalBalance，因为它已经是完整的账户净值
		equity := record.AccountState.TotalBalance
		if equity > 0 {
			equities = append(equities, equity)
		}
	}

	if len(equities) < 2 {
		return 0.0
	}

	// 计算周期收益率（period returns）
	var returns []float64
	for i := 1; i < len(equities); i++ {
		if equities[i-1] > 0 {
			periodReturn := (equities[i] - equities[i-1]) / equities[i-1]
			returns = append(returns, periodReturn)
		}
	}

	if len(returns) == 0 {
		return 0.0
	}

	// 计算平均收益率
	sumReturns := 0.0
	for _, r := range returns {
		sumReturns += r
	}
	meanReturn := sumReturns / float64(len(returns))

	// 计算收益率标准差
	sumSquaredDiff := 0.0
	for _, r := range returns {
		diff := r - meanReturn
		sumSquaredDiff += diff * diff
	}
	variance := sumSquaredDiff / float64(len(returns))
	stdDev := math.Sqrt(variance)

	// 避免除以零
	if stdDev == 0 {
		if meanReturn > 0 {
			return 999.0 // 无波动的正收益
		} else if meanReturn < 0 {
			return -999.0 // 无波动的负收益
		}
		return 0.0
	}

	// 计算夏普比率（假设无风险利率为0）
	// 注：直接返回周期级别的夏普比率（非年化），正常范围 -2 到 +2
	sharpeRatio := meanReturn / stdDev
	return sharpeRatio
}

// updateCacheFromDecision 从决策记录中检测交易完成并主动更新缓存
//
// ⚠️ LIMITATION: 暂不支持 partial_close
// - 原因: partial_close 需要累积多次平仓的盈亏，逻辑复杂
// - 临时方案: 依赖 AnalyzePerformance 在完全平仓时聚合 partial_close 记录并添加到缓存
// - 相关 Issue: https://github.com/NoFxAiOS/nofx/issues/1032
func (l *DecisionLogger) updateCacheFromDecision(record *DecisionRecord) {
	if !record.Success || len(record.Decisions) == 0 {
		return
	}

	for _, decision := range record.Decisions {
		if !decision.Success {
			continue
		}

		switch decision.Action {
		case "open_long", "open_short":
			// 记录开仓
			side := "long"
			if decision.Action == "open_short" {
				side = "short"
			}

			l.positionMutex.Lock()
			l.openPositions[decision.Symbol] = &OpenPosition{
				Symbol:     decision.Symbol,
				Side:       side,
				Quantity:   decision.Quantity,
				EntryPrice: decision.Price,
				Leverage:   decision.Leverage,
				OpenTime:   decision.Timestamp,
				Exchange:   record.Exchange,
			}
			l.positionMutex.Unlock()

		case "close_long", "close_short", "auto_close_long", "auto_close_short":
			// 检测平仓，计算交易并添加到缓存
			l.positionMutex.Lock()
			openPos, exists := l.openPositions[decision.Symbol]
			if !exists {
				l.positionMutex.Unlock()
				continue
			}

			// 计算交易结果（包含 PromptHash）
			trade := l.calculateTrade(openPos, decision, record.Exchange, record.PromptHash)

			// 移除已平仓的持仓
			delete(l.openPositions, decision.Symbol)
			l.positionMutex.Unlock()

			// 添加到缓存
			l.AddTradeToCache(trade)
		}
	}
}

// recoverOpenPositions 从历史文件恢复未平仓的持仓
// 在服务启动时调用,确保重启后能正确追踪之前的开仓
func (l *DecisionLogger) recoverOpenPositions() error {
	// 获取最近的决策文件（扫描最近500个周期,足够覆盖大部分场景）
	records, err := l.GetLatestRecords(500)
	if err != nil {
		return fmt.Errorf("获取历史记录失败: %w", err)
	}

	// 追踪每个币种的最后一次操作
	// key: symbol, value: 最后一次操作及其持仓信息
	lastAction := make(map[string]*struct {
		action   string // "open" or "close"
		position *OpenPosition
	})

	// 按时间顺序遍历所有记录
	for _, record := range records {
		if !record.Success || len(record.Decisions) == 0 {
			continue
		}

		for _, decision := range record.Decisions {
			if !decision.Success {
				continue
			}

			switch decision.Action {
			case "open_long", "open_short":
				// 记录开仓
				side := "long"
				if decision.Action == "open_short" {
					side = "short"
				}

				lastAction[decision.Symbol] = &struct {
					action   string
					position *OpenPosition
				}{
					action: "open",
					position: &OpenPosition{
						Symbol:     decision.Symbol,
						Side:       side,
						Quantity:   decision.Quantity,
						EntryPrice: decision.Price,
						Leverage:   decision.Leverage,
						OpenTime:   decision.Timestamp,
						Exchange:   record.Exchange,
					},
				}

			case "close_long", "close_short", "auto_close_long", "auto_close_short":
				// 记录平仓
				lastAction[decision.Symbol] = &struct {
					action   string
					position *OpenPosition
				}{
					action: "close",
				}
			}
		}
	}

	// 恢复所有未平仓的持仓
	recoveredCount := 0
	for symbol, action := range lastAction {
		if action.action == "open" && action.position != nil {
			l.positionMutex.Lock()
			l.openPositions[symbol] = action.position
			l.positionMutex.Unlock()
			recoveredCount++
			fmt.Printf("  ✓ 恢复未平仓持仓: %s %s (入场价: %.4f, 开仓时间: %s)\n",
				symbol, action.position.Side, action.position.EntryPrice, action.position.OpenTime.Format("2006-01-02 15:04:05"))
		}
	}

	if recoveredCount > 0 {
		fmt.Printf("✅ 成功恢复 %d 个未平仓持仓\n", recoveredCount)
	}

	return nil
}

// initializeCacheOnStartup 在服务启动时初始化缓存和持仓
// 解决 Issue #43: 服务重启后缓存丢失的问题
func (l *DecisionLogger) initializeCacheOnStartup() {
	fmt.Println("🔄 开始初始化缓存和持仓...")

	// 1. 扫描历史文件填充 tradesCache
	if _, err := l.AnalyzePerformance(InitialScanCycles); err != nil {
		fmt.Printf("⚠ 初始化缓存失败: %v\n", err)
		// 不 return,继续尝试恢复持仓
	} else {
		cacheSize := len(l.tradesCache)
		if cacheSize > 0 {
			fmt.Printf("✅ 缓存已初始化: %d 笔交易\n", cacheSize)
		}
	}

	// 2. 恢复未平仓的持仓到 l.openPositions
	//    确保后续平仓操作能正确匹配
	if err := l.recoverOpenPositions(); err != nil {
		fmt.Printf("⚠ 恢复持仓失败: %v\n", err)
	}
}

// filterByPromptHash 过滤交易，只保留匹配指定 PromptHash 的交易
func filterByPromptHash(trades []TradeOutcome, promptHash string) []TradeOutcome {
	if promptHash == "" {
		// 如果 hash 为空，返回所有交易（向后兼容）
		return trades
	}

	filtered := make([]TradeOutcome, 0, len(trades))
	for _, trade := range trades {
		if trade.PromptHash == promptHash {
			filtered = append(filtered, trade)
		}
	}
	return filtered
}

// calculateSharpeRatioFromTrades 从交易列表计算夏普比率
// 用于替代 calculateSharpeRatioFromEquity，支持基于过滤后的交易计算
func (l *DecisionLogger) calculateSharpeRatioFromTrades(trades []TradeOutcome) float64 {
	if len(trades) < 2 {
		return 0.0
	}

	// 从交易重建 equity 序列
	// 假设初始资金（这里用一个合理的默认值，实际不影响收益率计算）
	initialEquity := 10000.0
	equities := make([]float64, len(trades)+1)
	equities[0] = initialEquity

	for i, trade := range trades {
		equities[i+1] = equities[i] + trade.PnL
	}

	// 计算周期收益率
	var returns []float64
	for i := 1; i < len(equities); i++ {
		if equities[i-1] > 0 {
			periodReturn := (equities[i] - equities[i-1]) / equities[i-1]
			returns = append(returns, periodReturn)
		}
	}

	if len(returns) == 0 {
		return 0.0
	}

	// 计算平均收益率
	var sum float64
	for _, r := range returns {
		sum += r
	}
	meanReturn := sum / float64(len(returns))

	// 计算收益率标准差
	sumSquaredDiff := 0.0
	for _, r := range returns {
		diff := r - meanReturn
		sumSquaredDiff += diff * diff
	}
	variance := sumSquaredDiff / float64(len(returns))
	stdDev := math.Sqrt(variance)

	// 避免除以零
	if stdDev == 0 {
		if meanReturn > 0 {
			return 999.0 // 无波动的正收益
		} else if meanReturn < 0 {
			return -999.0 // 无波动的负收益
		}
		return 0.0
	}

	// 计算夏普比率（假设无风险利率为0）
	// 注：直接返回周期级别的夏普比率（非年化），正常范围 -2 到 +2
	sharpeRatio := meanReturn / stdDev
	return sharpeRatio
}

// calculateTrade 计算完整交易的盈亏和其他指标
func (l *DecisionLogger) calculateTrade(openPos *OpenPosition, closeDecision DecisionAction, exchange string, promptHash string) TradeOutcome {
	quantity := openPos.Quantity
	entryPrice := openPos.EntryPrice
	exitPrice := closeDecision.Price
	leverage := openPos.Leverage

	// 计算仓位价值和保证金
	positionValue := quantity * entryPrice
	marginUsed := positionValue / float64(leverage)

	// 计算原始盈亏（不含手续费）
	var rawPnL float64
	if openPos.Side == "long" {
		rawPnL = (exitPrice - entryPrice) * quantity
	} else { // short
		rawPnL = (entryPrice - exitPrice) * quantity
	}

	// 计算手续费
	takerFee := getTakerFeeRate(exchange)
	openFee := positionValue * takerFee
	closeFee := (quantity * exitPrice) * takerFee
	totalFee := openFee + closeFee

	// 最终盈亏 = 原始盈亏 - 手续费
	finalPnL := rawPnL - totalFee

	// 盈亏百分比（相对保证金）
	pnlPct := (finalPnL / marginUsed) * 100

	// 持仓时长
	duration := closeDecision.Timestamp.Sub(openPos.OpenTime)

	return TradeOutcome{
		Symbol:        openPos.Symbol,
		Side:          openPos.Side,
		Quantity:      quantity,
		Leverage:      leverage,
		OpenPrice:     entryPrice,
		ClosePrice:    exitPrice,
		PositionValue: positionValue,
		MarginUsed:    marginUsed,
		PnL:           finalPnL,
		PnLPct:        pnlPct,
		Duration:      duration.String(),
		OpenTime:      openPos.OpenTime,
		CloseTime:     closeDecision.Timestamp,
		WasStopLoss:   false, // TODO: 检测是否止损
		PromptHash:    promptHash,
	}
}

// AddTradeToCache 添加交易到内存缓存（带去重）
func (l *DecisionLogger) AddTradeToCache(trade TradeOutcome) {
	l.cacheMutex.Lock()
	defer l.cacheMutex.Unlock()

	// 生成唯一标识：symbol_side_openTime_closeTime
	tradeKey := fmt.Sprintf("%s_%s_%d_%d",
		trade.Symbol,
		trade.Side,
		trade.OpenTime.Unix(),
		trade.CloseTime.Unix(),
	)

	// 检查是否已存在（去重）
	if l.tradeCacheSet[tradeKey] {
		return // 已存在，跳过
	}

	// 插入到头部（最新的在前）
	l.tradesCache = append([]TradeOutcome{trade}, l.tradesCache...)
	l.tradeCacheSet[tradeKey] = true

	// 限制缓存大小，超出部分丢弃
	if len(l.tradesCache) > l.maxCacheSize {
		// 移除最后一条记录（最旧的）
		removedTrade := l.tradesCache[l.maxCacheSize]
		removedKey := fmt.Sprintf("%s_%s_%d_%d",
			removedTrade.Symbol,
			removedTrade.Side,
			removedTrade.OpenTime.Unix(),
			removedTrade.CloseTime.Unix(),
		)
		delete(l.tradeCacheSet, removedKey) // 从 Set 中删除
		l.tradesCache = l.tradesCache[:l.maxCacheSize]
	}
}

// addEquityToCache 添加净值记录到缓存（用于SharpeRatio计算）
func (l *DecisionLogger) addEquityToCache(timestamp time.Time, equity float64) {
	l.cacheMutex.Lock()
	defer l.cacheMutex.Unlock()

	// 插入到头部（最新的在前）
	point := EquityPoint{
		Timestamp: timestamp,
		Equity:    equity,
	}
	l.equityCache = append([]EquityPoint{point}, l.equityCache...)

	// 限制缓存大小
	if len(l.equityCache) > l.maxEquitySize {
		l.equityCache = l.equityCache[:l.maxEquitySize]
	}
}

// GetRecentTrades 从缓存获取最近N条交易（最新的在前）
func (l *DecisionLogger) GetRecentTrades(limit int) []TradeOutcome {
	l.cacheMutex.RLock()
	defer l.cacheMutex.RUnlock()

	// 如果请求数量超过缓存大小，返回所有缓存
	if limit > len(l.tradesCache) {
		limit = len(l.tradesCache)
	}

	// 返回副本，避免外部修改缓存
	result := make([]TradeOutcome, limit)
	copy(result, l.tradesCache[:limit])
	return result
}

// calculateStatisticsFromTrades 基于交易列表计算统计信息
// 🎯 用于从缓存的交易记录中计算性能指标，避免重复扫描历史文件
func (l *DecisionLogger) calculateStatisticsFromTrades(trades []TradeOutcome) *PerformanceAnalysis {
	analysis := &PerformanceAnalysis{
		RecentTrades: trades,
		SymbolStats:  make(map[string]*SymbolPerformance),
	}

	if len(trades) == 0 {
		return analysis
	}

	// 遍历所有交易，累计统计信息
	for _, trade := range trades {
		analysis.TotalTrades++

		if trade.PnL >= 0 {
			analysis.WinningTrades++
			analysis.AvgWin += trade.PnL
		} else {
			analysis.LosingTrades++
			analysis.AvgLoss += trade.PnL
		}

		// 按币种统计
		if _, exists := analysis.SymbolStats[trade.Symbol]; !exists {
			analysis.SymbolStats[trade.Symbol] = &SymbolPerformance{
				Symbol: trade.Symbol,
			}
		}
		stats := analysis.SymbolStats[trade.Symbol]
		stats.TotalTrades++
		stats.TotalPnL += trade.PnL

		if trade.PnL >= 0 {
			stats.WinningTrades++
		} else {
			stats.LosingTrades++
		}
	}

	// 计算平均值和比率
	if analysis.TotalTrades > 0 {
		analysis.WinRate = (float64(analysis.WinningTrades) / float64(analysis.TotalTrades)) * 100

		totalWinAmount := analysis.AvgWin
		totalLossAmount := analysis.AvgLoss

		if analysis.WinningTrades > 0 {
			analysis.AvgWin /= float64(analysis.WinningTrades)
		}
		if analysis.LosingTrades > 0 {
			analysis.AvgLoss /= float64(analysis.LosingTrades)
		}

		// Profit Factor = 总盈利 / 总亏损（绝对值）
		if totalLossAmount != 0 {
			analysis.ProfitFactor = totalWinAmount / (-totalLossAmount)
		} else if totalWinAmount > 0 {
			analysis.ProfitFactor = 999.0
		}
	}

	// 计算各币种胜率和平均盈亏，找出最佳/最差币种
	bestPnL := -999999.0
	worstPnL := 999999.0
	for symbol, stats := range analysis.SymbolStats {
		if stats.TotalTrades > 0 {
			stats.WinRate = (float64(stats.WinningTrades) / float64(stats.TotalTrades)) * 100
			stats.AvgPnL = stats.TotalPnL / float64(stats.TotalTrades)

			if stats.TotalPnL > bestPnL {
				bestPnL = stats.TotalPnL
				analysis.BestSymbol = symbol
			}
			if stats.TotalPnL < worstPnL {
				worstPnL = stats.TotalPnL
				analysis.WorstSymbol = symbol
			}
		}
	}

	return analysis
}

// calculateSharpeRatioFromEquity 从equity缓存计算夏普比率
func (l *DecisionLogger) calculateSharpeRatioFromEquity() float64 {
	l.cacheMutex.RLock()
	defer l.cacheMutex.RUnlock()

	if len(l.equityCache) < 2 {
		return 0.0
	}

	// equity缓存是从新到旧排列,需要反转为从旧到新
	var equities []float64
	for i := len(l.equityCache) - 1; i >= 0; i-- {
		if l.equityCache[i].Equity > 0 {
			equities = append(equities, l.equityCache[i].Equity)
		}
	}

	if len(equities) < 2 {
		return 0.0
	}

	// 计算周期收益率
	var returns []float64
	for i := 1; i < len(equities); i++ {
		if equities[i-1] > 0 {
			periodReturn := (equities[i] - equities[i-1]) / equities[i-1]
			returns = append(returns, periodReturn)
		}
	}

	if len(returns) == 0 {
		return 0.0
	}

	// 计算平均收益率
	var sum float64
	for _, r := range returns {
		sum += r
	}
	avgReturn := sum / float64(len(returns))

	// 计算标准差
	var variance float64
	for _, r := range returns {
		diff := r - avgReturn
		variance += diff * diff
	}
	variance /= float64(len(returns))
	stdDev := variance

	if variance > 0 {
		stdDev = 1.0
		for i := 0; i < 10; i++ {
			stdDev = (stdDev + variance/stdDev) / 2
		}
	}

	// 夏普比率 = (平均收益率 - 无风险收益率) / 标准差
	// 假设无风险收益率为 0
	if stdDev > 0 {
		return avgReturn / stdDev
	}

	return 0.0
}

// GetPerformanceWithCache 获取 AI 性能分析
//
// 设计原则:
// 1. 统计分析：固定基于最近 100 笔交易（AIAnalysisSampleSize）
// 2. 列表显示：tradeLimit 仅控制返回给前端的交易记录数量
// 3. 数据稳定性：统计指标（胜率、夏普比率等）不受 tradeLimit 影响
// 4. PromptHash 过滤：可选，默认显示所有交易（filterByPrompt=false）
//
// 参数:
//   tradeLimit: 返回给前端的交易列表长度（用户显示偏好，如 10/20/50/100）
//   filterByPrompt: 是否按当前 PromptHash 过滤交易（默认 false 显示所有）
//
// 返回:
//   - total_trades: 分析的交易总数（固定基于 AIAnalysisSampleSize 或缓存全部）
//   - recent_trades: 交易列表（长度 = min(tradeLimit, 实际交易数)）
func (l *DecisionLogger) GetPerformanceWithCache(tradeLimit int, filterByPrompt bool) (*PerformanceAnalysis, error) {
	// 获取用于 AI 分析的固定样本（最近 100 笔交易）
	cachedTrades := l.GetRecentTrades(AIAnalysisSampleSize)

	var filteredTrades []TradeOutcome

	// 🎯 根据用户选择决定是否按 PromptHash 过滤
	if filterByPrompt {
		// 🔍 获取当前的 PromptHash（从最新交易推断）
		var currentPromptHash string
		if len(cachedTrades) > 0 {
			currentPromptHash = cachedTrades[0].PromptHash
		}
		// 过滤：只保留匹配当前 PromptHash 的交易
		filteredTrades = filterByPromptHash(cachedTrades, currentPromptHash)
	} else {
		// 不过滤，显示所有交易
		filteredTrades = cachedTrades
	}

	var performance *PerformanceAnalysis
	var err error

	// 如果过滤后没有交易（首次请求或重启后），扫描历史文件初始化缓存
	if len(filteredTrades) == 0 {
		// 首次请求：扫描历史周期填充缓存
		performance, err = l.AnalyzePerformance(InitialScanCycles)
		if err != nil {
			return nil, fmt.Errorf("初始化缓存失败: %w", err)
		}
		// 重新获取分析样本并根据设置过滤
		cachedTrades = l.GetRecentTrades(AIAnalysisSampleSize)
		if filterByPrompt {
			var currentPromptHash string
			if len(cachedTrades) > 0 {
				currentPromptHash = cachedTrades[0].PromptHash
			}
			filteredTrades = filterByPromptHash(cachedTrades, currentPromptHash)
		} else {
			filteredTrades = cachedTrades
		}
	} else {
		// ✅ 缓存已有数据：基于过滤后的交易计算统计信息
		performance = l.calculateStatisticsFromTrades(filteredTrades)

		// ✅ 从过滤后的交易计算SharpeRatio（而非全局equity缓存）
		performance.SharpeRatio = l.calculateSharpeRatioFromTrades(filteredTrades)
	}

	// 使用过滤后的数据，限制为请求的条数
	if len(filteredTrades) > tradeLimit {
		performance.RecentTrades = filteredTrades[:tradeLimit]
	} else {
		performance.RecentTrades = filteredTrades
	}

	return performance, nil
}
