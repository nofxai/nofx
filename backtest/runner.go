package backtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"nofx/decision"
	"nofx/logger"
	"nofx/market"
	"nofx/mcp"
)

var (
	errBacktestCompleted = errors.New("backtest completed")
	errLiquidated        = errors.New("account liquidated")
)

const (
	metricsWriteInterval = 5 * time.Second
	aiDecisionMaxRetries = 3
)

// Runner 封装单次回测运行的生命周期。
type Runner struct {
	cfg     BacktestConfig
	feed    *DataFeed
	account *BacktestAccount

	decisionLogger logger.IDecisionLogger
	mcpClient      mcp.AIClient

	promptSnapshot string // 启动时的完整prompt内容快照（用于保存到metadata）

	statusMu sync.RWMutex
	status   RunState

	stateMu sync.RWMutex
	state   *BacktestState

	pauseCh  chan struct{}
	resumeCh chan struct{}
	stopCh   chan struct{}
	doneCh   chan struct{}

	err              error
	errMu            sync.RWMutex
	lastError        string
	lastCheckpoint   time.Time
	createdAt        time.Time
	lastMetricsWrite time.Time

	aiCache   *AICache
	cachePath string

	lockInfo *RunLockInfo
	lockStop chan struct{}
}

// NewRunner 构建回测运行器。
func NewRunner(cfg BacktestConfig, mcpClient mcp.AIClient) (*Runner, error) {
	if err := ensureRunDir(cfg.RunID); err != nil {
		return nil, err
	}

	client, err := configureMCPClient(cfg, mcpClient)
	if err != nil {
		return nil, err
	}

	feed, err := NewDataFeed(cfg)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(decisionLogDir(cfg.RunID), 0o755); err != nil {
		return nil, err
	}

	dLog := logger.NewDecisionLogger(decisionLogDir(cfg.RunID))
	account := NewBacktestAccount(cfg.InitialBalance, cfg.FeeBps, cfg.SlippageBps)

	// 生成 prompt 内容快照（启动时的完整prompt，用于记录）
	// 回测默认使用 hyperliquid 的最小开仓金额（12 USDT）
	promptSnapshot := decision.BuildPromptSnapshot(
		cfg.InitialBalance,
		cfg.Leverage.BTCETHLeverage,
		cfg.Leverage.AltcoinLeverage,
		cfg.CustomPrompt,
		cfg.OverrideBasePrompt,
		cfg.PromptTemplate,
		"hyperliquid",
	)

	createdAt := time.Now().UTC()
	state := &BacktestState{
		Positions:      make(map[string]PositionSnapshot),
		Cash:           account.Cash(),
		Equity:         cfg.InitialBalance,
		UnrealizedPnL:  0,
		RealizedPnL:    0,
		MaxEquity:      cfg.InitialBalance,
		MinEquity:      cfg.InitialBalance,
		MaxDrawdownPct: 0,
		LastUpdate:     createdAt,
	}

	var (
		aiCache   *AICache
		cachePath string
	)
	if cfg.CacheAI || cfg.ReplayOnly || cfg.SharedAICachePath != "" {
		cachePath = cfg.SharedAICachePath
		if cachePath == "" {
			cachePath = filepath.Join(runDir(cfg.RunID), "ai_cache.json")
		}
		cache, err := LoadAICache(cachePath)
		if err != nil {
			return nil, fmt.Errorf("load ai cache: %w", err)
		}
		aiCache = cache
	}

	r := &Runner{
		cfg:            cfg,
		feed:           feed,
		account:        account,
		decisionLogger: dLog,
		mcpClient:      client,
		promptSnapshot: promptSnapshot,
		status:         RunStateCreated,
		state:          state,
		pauseCh:        make(chan struct{}, 1),
		resumeCh:       make(chan struct{}, 1),
		stopCh:         make(chan struct{}, 1),
		doneCh:         make(chan struct{}),
		createdAt:      createdAt,
		aiCache:        aiCache,
		cachePath:      cachePath,
	}

	if err := r.initLock(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Runner) initLock() error {
	if r.cfg.RunID == "" {
		return fmt.Errorf("run_id required for lock")
	}
	info, err := acquireRunLock(r.cfg.RunID)
	if err != nil {
		return err
	}
	r.lockInfo = info
	r.lockStop = make(chan struct{})
	go r.lockHeartbeatLoop()
	return nil
}

func (r *Runner) lockHeartbeatLoop() {
	ticker := time.NewTicker(lockHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := updateRunLockHeartbeat(r.lockInfo); err != nil {
				log.Printf("failed to update lock heartbeat for %s: %v", r.cfg.RunID, err)
			}
		case <-r.lockStop:
			return
		}
	}
}

func (r *Runner) releaseLock() {
	if r.lockStop != nil {
		close(r.lockStop)
		r.lockStop = nil
	}
	if err := deleteRunLock(r.cfg.RunID); err != nil {
		log.Printf("failed to release lock for %s: %v", r.cfg.RunID, err)
	}
	r.lockInfo = nil
}

// Start 启动回测循环。
func (r *Runner) Start(ctx context.Context) error {
	r.statusMu.Lock()
	if r.status != RunStateCreated && r.status != RunStatePaused {
		r.statusMu.Unlock()
		return fmt.Errorf("cannot start runner in state %s", r.status)
	}
	r.status = RunStateRunning
	r.statusMu.Unlock()

	go r.loop(ctx)
	return nil
}

// PersistMetadata 将当前快照写入 run.json。
func (r *Runner) PersistMetadata() {
	r.persistMetadata()
}

func (r *Runner) setLastError(err error) {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	if err == nil {
		r.lastError = ""
		return
	}
	r.lastError = err.Error()
}

func (r *Runner) lastErrorString() string {
	r.errMu.RLock()
	defer r.errMu.RUnlock()
	return r.lastError
}

// CurrentMetadata 返回当前内存状态对应的元数据。
func (r *Runner) CurrentMetadata() *RunMetadata {
	state := r.snapshotState()
	meta := r.buildMetadata(state, r.Status())
	meta.CreatedAt = r.createdAt
	meta.UpdatedAt = state.LastUpdate
	return meta
}

func (r *Runner) loop(ctx context.Context) {
	defer close(r.doneCh)

	for {
		select {
		case <-ctx.Done():
			r.handleStop(fmt.Errorf("context canceled: %w", ctx.Err()))
			return
		case <-r.stopCh:
			r.handleStop(nil)
			return
		case <-r.pauseCh:
			r.handlePause()
			<-r.resumeCh
			r.resumeFromPause()
		default:
		}

		err := r.stepOnce()
		if errors.Is(err, errBacktestCompleted) {
			r.handleCompletion()
			return
		}
		if errors.Is(err, errLiquidated) {
			r.handleLiquidation()
			return
		}
		if err != nil {
			r.handleFailure(err)
			return
		}
	}
}

func (r *Runner) stepOnce() error {
	state := r.snapshotState()
	if state.BarIndex >= r.feed.DecisionBarCount() {
		return errBacktestCompleted
	}

	ts := r.feed.DecisionTimestamp(state.BarIndex)

	marketData, multiTF, err := r.feed.BuildMarketData(ts)
	if err != nil {
		return err
	}

	// 构建 Close/High/Low 价格映射（用于OHLC风控检查）
	priceMap := make(map[string]float64, len(marketData))
	highMap := make(map[string]float64, len(marketData))
	lowMap := make(map[string]float64, len(marketData))

	for symbol := range marketData {
		// 获取当前K线的OHLC数据
		currentBar, _ := r.feed.decisionBarSnapshot(symbol, ts)
		if currentBar != nil {
			priceMap[symbol] = currentBar.Close
			highMap[symbol] = currentBar.High
			lowMap[symbol] = currentBar.Low
		} else {
			// 降级方案：使用CurrentPrice
			priceMap[symbol] = marketData[symbol].CurrentPrice
			highMap[symbol] = marketData[symbol].CurrentPrice
			lowMap[symbol] = marketData[symbol].CurrentPrice
		}
	}

	callCount := state.DecisionCycle + 1
	shouldDecide := r.shouldTriggerDecision(state.BarIndex)

	var (
		record          *logger.DecisionRecord
		decisionActions []logger.DecisionAction
		tradeEvents     = make([]TradeEvent, 0)
		execLog         []string
		hadError        bool
	)

	// 🔧 修复 BUG 2&3: 使用 OHLC 数据统一检查止损止盈和爆仓（在 AI 决策之前，风控优先）
	slTpEvents, liqEvents := r.checkRiskEventsWithOHLC(priceMap, highMap, lowMap, ts, callCount)
	tradeEvents = append(tradeEvents, slTpEvents...)
	tradeEvents = append(tradeEvents, liqEvents...)
	for _, evt := range slTpEvents {
		execLog = append(execLog, fmt.Sprintf("🛑 %s", evt.Note))
	}
	if len(liqEvents) > 0 {
		hadError = true
		for _, evt := range liqEvents {
			execLog = append(execLog, fmt.Sprintf("🚨 强平: %s", evt.Note))
		}
	}

	decisionAttempted := shouldDecide

	if shouldDecide {
		ctx, rec, err := r.buildDecisionContext(ts, marketData, multiTF, priceMap, callCount)
		if err != nil {
			rec.Success = false
			rec.ErrorMessage = fmt.Sprintf("构建交易上下文失败: %v", err)
			_ = r.logDecision(rec)
			return err
		}
		record = rec

		var (
			fullDecision *decision.FullDecision
			fromCache    bool
			cacheKey     string
		)
		if r.aiCache != nil {
			if key, err := computeCacheKey(ctx, r.cfg.PromptVariant, ts); err == nil {
				cacheKey = key
				if cached, ok := r.aiCache.Get(cacheKey); ok {
					fullDecision = cached
					fromCache = true
				} else if r.cfg.ReplayOnly {
					decisionErr := fmt.Errorf("replay_only enabled but cache miss at %d", ts)
					record.Success = false
					record.ErrorMessage = fmt.Sprintf("没有找到 ts=%d 的缓存决策", ts)
					_ = r.logDecision(record)
					return decisionErr
				}
			} else {
				log.Printf("failed to compute ai cache key: %v", err)
			}
		}

		if !fromCache {
			fd, err := r.invokeAIWithRetry(ctx)
			if err != nil {
				decisionAttempted = true
				hadError = true
				record.Success = false
				record.ErrorMessage = fmt.Sprintf("AI决策失败: %v", err)
				execLog = append(execLog, fmt.Sprintf("⚠️ AI决策失败: %v", err))
				r.setLastError(err)
			} else {
				fullDecision = fd
				if r.cfg.CacheAI && r.aiCache != nil && cacheKey != "" {
					if err := r.aiCache.Put(cacheKey, r.cfg.PromptVariant, ts, fullDecision); err != nil {
						log.Printf("failed to persist ai cache for %s: %v", r.cfg.RunID, err)
					}
				}
			}
		}

		if fullDecision != nil {
			r.fillDecisionRecord(record, fullDecision)

			sorted := sortDecisionsByPriority(fullDecision.Decisions)

			prevLogs := execLog
			decisionActions = make([]logger.DecisionAction, 0, len(sorted))
			execLog = make([]string, 0, len(sorted)+len(prevLogs))
			if len(prevLogs) > 0 {
				execLog = append(execLog, prevLogs...)
			}

			for _, dec := range sorted {
				actionRecord, trades, logEntry, execErr := r.executeDecision(dec, priceMap, ts, callCount)
				if execErr != nil {
					actionRecord.Success = false
					actionRecord.Error = execErr.Error()
					hadError = true
					execLog = append(execLog, fmt.Sprintf("❌ %s %s: %v", dec.Symbol, dec.Action, execErr))
				} else {
					actionRecord.Success = true
					execLog = append(execLog, fmt.Sprintf("✓ %s %s", dec.Symbol, dec.Action))
				}
				if len(trades) > 0 {
					tradeEvents = append(tradeEvents, trades...)
				}
				if logEntry != "" {
					execLog = append(execLog, logEntry)
				}
				decisionActions = append(decisionActions, actionRecord)
			}
		}
	}

	cycleForLog := state.DecisionCycle
	if decisionAttempted {
		cycleForLog = callCount
	}

	// 🔧 修复 BUG 1&5: AI 决策后再次检查（捕获AI修改的止损止盈或新开仓位）
	slTpEvents2, liqEvents2 := r.checkRiskEventsWithOHLC(priceMap, highMap, lowMap, ts, cycleForLog)
	if len(slTpEvents2) > 0 {
		tradeEvents = append(tradeEvents, slTpEvents2...)
		for _, evt := range slTpEvents2 {
			execLog = append(execLog, fmt.Sprintf("🔄 AI 决策后触发: %s", evt.Note))
		}
	}
	if len(liqEvents2) > 0 {
		hadError = true
		tradeEvents = append(tradeEvents, liqEvents2...)
		for _, evt := range liqEvents2 {
			execLog = append(execLog, fmt.Sprintf("🚨 AI 决策后强平: %s", evt.Note))
		}
	}

	if record != nil {
		record.Decisions = decisionActions
		record.ExecutionLog = execLog
		record.Success = !hadError
		if hadError && len(liqEvents)+len(liqEvents2) > 0 {
			record.ErrorMessage = "发生强制平仓"
		}
	}

	equity, unrealized, _ := r.account.TotalEquity(priceMap)
	marginUsed := r.totalMarginUsed()

	r.updateState(ts, equity, unrealized, marginUsed, priceMap, decisionAttempted)

	snapshot := r.snapshotState()
	drawdownPct := 0.0
	if snapshot.MaxEquity > 0 {
		drawdownPct = ((snapshot.MaxEquity - snapshot.Equity) / snapshot.MaxEquity) * 100
	}

	equityPoint := EquityPoint{
		Timestamp:   ts,
		Equity:      snapshot.Equity,
		Available:   snapshot.Cash,
		PnL:         snapshot.Equity - r.account.InitialBalance(),
		PnLPct:      ((snapshot.Equity - r.account.InitialBalance()) / r.account.InitialBalance()) * 100,
		DrawdownPct: drawdownPct,
		Cycle:       snapshot.DecisionCycle,
	}

	if err := appendEquityPoint(r.cfg.RunID, equityPoint); err != nil {
		return err
	}

	for _, evt := range tradeEvents {
		if err := appendTradeEvent(r.cfg.RunID, evt); err != nil {
			return err
		}
	}

	if record != nil {
		if err := r.logDecision(record); err != nil {
			return err
		}
	}

	if err := saveProgress(r.cfg.RunID, &snapshot, &r.cfg); err != nil {
		return err
	}

	if err := r.maybeCheckpoint(); err != nil {
		return err
	}

	r.persistMetadata()
	r.persistMetrics(false)

	if !hadError && !snapshot.Liquidated {
		r.setLastError(nil)
	}

	if snapshot.Liquidated {
		return errLiquidated
	}

	return nil
}

func (r *Runner) buildDecisionContext(ts int64, marketData map[string]*market.Data, multiTF map[string]map[string]*market.Data, priceMap map[string]float64, callCount int) (*decision.Context, *logger.DecisionRecord, error) {
	equity, unrealized, _ := r.account.TotalEquity(priceMap)
	available := r.account.Cash()
	marginUsed := r.totalMarginUsed()
	marginPct := 0.0
	if equity > 0 {
		marginPct = (marginUsed / equity) * 100
	}

	accountInfo := decision.AccountInfo{
		TotalEquity:      equity,
		AvailableBalance: available,
		TotalPnL:         equity - r.account.InitialBalance(),
		TotalPnLPct:      ((equity - r.account.InitialBalance()) / r.account.InitialBalance()) * 100,
		MarginUsed:       marginUsed,
		MarginUsedPct:    marginPct,
		PositionCount:    len(r.account.Positions()),
	}

	positions := r.convertPositions(priceMap)

	candidateCoins := make([]decision.CandidateCoin, 0, len(r.cfg.Symbols))
	for _, sym := range r.cfg.Symbols {
		candidateCoins = append(candidateCoins, decision.CandidateCoin{Symbol: sym})
	}

	runtime := int((ts - int64(r.cfg.StartTS*1000)) / 60000)
	ctx := &decision.Context{
		CurrentTime:     time.UnixMilli(ts).UTC().Format(time.RFC3339),
		RuntimeMinutes:  runtime,
		CallCount:       callCount,
		Account:         accountInfo,
		Positions:       positions,
		CandidateCoins:  candidateCoins,
		PromptVariant:   r.cfg.PromptVariant,
		MarketDataMap:   marketData,
		MultiTFMarket:   multiTF,
		BTCETHLeverage:  r.cfg.Leverage.BTCETHLeverage,
		AltcoinLeverage: r.cfg.Leverage.AltcoinLeverage,
	}

	record := &logger.DecisionRecord{
		AccountState: logger.AccountSnapshot{
			TotalBalance:          accountInfo.TotalEquity,
			AvailableBalance:      accountInfo.AvailableBalance,
			TotalUnrealizedProfit: unrealized,
			PositionCount:         accountInfo.PositionCount,
			MarginUsedPct:         accountInfo.MarginUsedPct,
		},
		CandidateCoins: make([]string, 0, len(candidateCoins)),
		Positions:      r.snapshotPositions(priceMap),
	}
	for _, coin := range candidateCoins {
		record.CandidateCoins = append(record.CandidateCoins, coin.Symbol)
	}
	record.Timestamp = time.UnixMilli(ts).UTC()

	return ctx, record, nil
}

func (r *Runner) fillDecisionRecord(record *logger.DecisionRecord, full *decision.FullDecision) {
	record.InputPrompt = full.UserPrompt
	record.CoTTrace = full.CoTTrace
	if len(full.Decisions) > 0 {
		if data, err := json.MarshalIndent(full.Decisions, "", "  "); err == nil {
			record.DecisionJSON = string(data)
		}
	}
}

func (r *Runner) invokeAIWithRetry(ctx *decision.Context) (*decision.FullDecision, error) {
	var lastErr error
	for attempt := 0; attempt < aiDecisionMaxRetries; attempt++ {
		fd, err := decision.GetFullDecisionWithCustomPrompt(
			ctx,
			r.mcpClient,
			r.cfg.CustomPrompt,
			r.cfg.OverrideBasePrompt,
			r.cfg.PromptTemplate,
		)
		if err == nil {
			return fd, nil
		}
		lastErr = err
		delay := time.Duration(attempt+1) * 500 * time.Millisecond
		time.Sleep(delay)
	}
	return nil, lastErr
}

func (r *Runner) executeDecision(dec decision.Decision, priceMap map[string]float64, ts int64, cycle int) (logger.DecisionAction, []TradeEvent, string, error) {
	symbol := dec.Symbol
	usedLeverage := r.resolveLeverage(dec.Leverage, symbol)
	actionRecord := logger.DecisionAction{
		Action:    dec.Action,
		Symbol:    symbol,
		Leverage:  usedLeverage,
		Timestamp: time.UnixMilli(ts).UTC(),
	}

	basePrice := priceMap[symbol]
	if basePrice <= 0 {
		return actionRecord, nil, "", fmt.Errorf("price unavailable for %s", symbol)
	}
	fillPrice := r.executionPrice(symbol, basePrice, ts)

	switch dec.Action {
	case "open_long":
		qty := r.determineQuantity(dec, basePrice)
		if qty <= 0 {
			return actionRecord, nil, "", fmt.Errorf("invalid qty")
		}
		pos, fee, execPrice, err := r.account.Open(symbol, "long", qty, usedLeverage, fillPrice, dec.StopLoss, dec.TakeProfit, ts)
		if err != nil {
			return actionRecord, nil, "", err
		}
		actionRecord.Quantity = qty
		actionRecord.Price = execPrice
		actionRecord.Leverage = pos.Leverage
		trade := TradeEvent{
			Timestamp:     ts,
			Symbol:        symbol,
			Action:        dec.Action,
			Side:          "long",
			Quantity:      qty,
			Price:         execPrice,
			Fee:           fee,
			Slippage:      execPrice - basePrice,
			OrderValue:    execPrice * qty,
			RealizedPnL:   0,
			Leverage:      pos.Leverage,
			Cycle:         cycle,
			PositionAfter: pos.Quantity,
		}
		return actionRecord, []TradeEvent{trade}, "", nil

	case "open_short":
		qty := r.determineQuantity(dec, basePrice)
		if qty <= 0 {
			return actionRecord, nil, "", fmt.Errorf("invalid qty")
		}
		pos, fee, execPrice, err := r.account.Open(symbol, "short", qty, usedLeverage, fillPrice, dec.StopLoss, dec.TakeProfit, ts)
		if err != nil {
			return actionRecord, nil, "", err
		}
		actionRecord.Quantity = qty
		actionRecord.Price = execPrice
		actionRecord.Leverage = pos.Leverage
		trade := TradeEvent{
			Timestamp:     ts,
			Symbol:        symbol,
			Action:        dec.Action,
			Side:          "short",
			Quantity:      qty,
			Price:         execPrice,
			Fee:           fee,
			Slippage:      basePrice - execPrice,
			OrderValue:    execPrice * qty,
			RealizedPnL:   0,
			Leverage:      pos.Leverage,
			Cycle:         cycle,
			PositionAfter: pos.Quantity,
		}
		return actionRecord, []TradeEvent{trade}, "", nil

	case "close_long":
		qty := r.determineCloseQuantity(symbol, "long", dec)
		if qty <= 0 {
			return actionRecord, nil, "", fmt.Errorf("invalid close qty")
		}
		posLev := r.account.positionLeverage(symbol, "long")
		realized, fee, execPrice, err := r.account.Close(symbol, "long", qty, fillPrice)
		if err != nil {
			return actionRecord, nil, "", err
		}
		actionRecord.Quantity = qty
		actionRecord.Price = execPrice
		actionRecord.Leverage = posLev
		trade := TradeEvent{
			Timestamp:     ts,
			Symbol:        symbol,
			Action:        dec.Action,
			Side:          "long",
			Quantity:      qty,
			Price:         execPrice,
			Fee:           fee,
			Slippage:      basePrice - execPrice,
			OrderValue:    execPrice * qty,
			RealizedPnL:   realized - fee,
			Leverage:      posLev,
			Cycle:         cycle,
			PositionAfter: r.remainingPosition(symbol, "long"),
		}
		return actionRecord, []TradeEvent{trade}, "", nil

	case "close_short":
		qty := r.determineCloseQuantity(symbol, "short", dec)
		if qty <= 0 {
			return actionRecord, nil, "", fmt.Errorf("invalid close qty")
		}
		posLev := r.account.positionLeverage(symbol, "short")
		realized, fee, execPrice, err := r.account.Close(symbol, "short", qty, fillPrice)
		if err != nil {
			return actionRecord, nil, "", err
		}
		actionRecord.Quantity = qty
		actionRecord.Price = execPrice
		actionRecord.Leverage = posLev
		trade := TradeEvent{
			Timestamp:     ts,
			Symbol:        symbol,
			Action:        dec.Action,
			Side:          "short",
			Quantity:      qty,
			Price:         execPrice,
			Fee:           fee,
			Slippage:      execPrice - basePrice,
			OrderValue:    execPrice * qty,
			RealizedPnL:   realized - fee,
			Leverage:      posLev,
			Cycle:         cycle,
			PositionAfter: r.remainingPosition(symbol, "short"),
		}
		return actionRecord, []TradeEvent{trade}, "", nil

	case "update_stop_loss":
		// 尝试更新多头或空头持仓的止损
		var err error
		var side string
		if err = r.account.UpdateStopLoss(symbol, "long", dec.NewStopLoss); err != nil {
			if err = r.account.UpdateStopLoss(symbol, "short", dec.NewStopLoss); err != nil {
				return actionRecord, nil, "", fmt.Errorf("no position to update stop loss for %s", symbol)
			}
			side = "short"
		} else {
			side = "long"
		}
		msg := fmt.Sprintf("更新 %s %s 止损至 %.4f", symbol, side, dec.NewStopLoss)
		return actionRecord, nil, msg, nil

	case "update_take_profit":
		// 尝试更新多头或空头持仓的止盈
		var err error
		var side string
		if err = r.account.UpdateTakeProfit(symbol, "long", dec.NewTakeProfit); err != nil {
			if err = r.account.UpdateTakeProfit(symbol, "short", dec.NewTakeProfit); err != nil {
				return actionRecord, nil, "", fmt.Errorf("no position to update take profit for %s", symbol)
			}
			side = "short"
		} else {
			side = "long"
		}
		msg := fmt.Sprintf("更新 %s %s 止盈至 %.4f", symbol, side, dec.NewTakeProfit)
		return actionRecord, nil, msg, nil

	case "partial_close":
		// TODO: 实现部分平仓逻辑
		return actionRecord, nil, "部分平仓暂不支持", nil

	case "hold", "wait":
		return actionRecord, nil, fmt.Sprintf("保持仓位: %s", dec.Action), nil
	default:
		return actionRecord, nil, "", fmt.Errorf("unsupported action %s", dec.Action)
	}
}

func (r *Runner) determineQuantity(dec decision.Decision, price float64) float64 {
	snapshot := r.snapshotState()
	equity := snapshot.Equity
	if equity <= 0 {
		equity = r.account.InitialBalance()
	}
	sizeUSD := dec.PositionSizeUSD
	if sizeUSD <= 0 {
		sizeUSD = 0.05 * equity
	}
	qty := sizeUSD / price
	if qty < 0 {
		qty = 0
	}
	return qty
}

func (r *Runner) determineCloseQuantity(symbol, side string, dec decision.Decision) float64 {
	for _, pos := range r.account.Positions() {
		if pos.Symbol == strings.ToUpper(symbol) && pos.Side == side {
			return pos.Quantity
		}
	}
	return 0
}

func (r *Runner) resolveLeverage(requested int, symbol string) int {
	if requested > 0 {
		return requested
	}
	sym := strings.ToUpper(symbol)
	if sym == "BTCUSDT" || sym == "ETHUSDT" {
		if r.cfg.Leverage.BTCETHLeverage > 0 {
			return r.cfg.Leverage.BTCETHLeverage
		}
	} else {
		if r.cfg.Leverage.AltcoinLeverage > 0 {
			return r.cfg.Leverage.AltcoinLeverage
		}
	}
	return 5
}

func (r *Runner) remainingPosition(symbol, side string) float64 {
	for _, pos := range r.account.Positions() {
		if pos.Symbol == strings.ToUpper(symbol) && pos.Side == side {
			return pos.Quantity
		}
	}
	return 0
}

func (r *Runner) snapshotPositions(priceMap map[string]float64) []logger.PositionSnapshot {
	positions := r.account.Positions()
	list := make([]logger.PositionSnapshot, 0, len(positions))
	for _, pos := range positions {
		price := priceMap[pos.Symbol]
		list = append(list, logger.PositionSnapshot{
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			PositionAmt:      pos.Quantity,
			EntryPrice:       pos.EntryPrice,
			MarkPrice:        price,
			UnrealizedProfit: unrealizedPnL(pos, price),
			Leverage:         float64(pos.Leverage),
			LiquidationPrice: pos.LiquidationPrice,
		})
	}
	return list
}

func (r *Runner) convertPositions(priceMap map[string]float64) []decision.PositionInfo {
	positions := r.account.Positions()
	list := make([]decision.PositionInfo, 0, len(positions))
	for _, pos := range positions {
		price := priceMap[pos.Symbol]
		list = append(list, decision.PositionInfo{
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			EntryPrice:       pos.EntryPrice,
			MarkPrice:        price,
			Quantity:         pos.Quantity,
			Leverage:         pos.Leverage,
			UnrealizedPnL:    unrealizedPnL(pos, price),
			UnrealizedPnLPct: 0,
			LiquidationPrice: pos.LiquidationPrice,
			MarginUsed:       pos.Margin,
			UpdateTime:       time.Now().UnixMilli(),
		})
	}
	return list
}

func (r *Runner) executionPrice(symbol string, markPrice float64, ts int64) float64 {
	curr, next := r.feed.decisionBarSnapshot(symbol, ts)
	switch r.cfg.FillPolicy {
	case FillPolicyNextOpen:
		if next != nil && next.Open > 0 {
			return next.Open
		}
	case FillPolicyBarVWAP:
		if curr != nil {
			if vwap := barVWAP(*curr); vwap > 0 {
				return vwap
			}
		}
	case FillPolicyMidPrice:
		if curr != nil && curr.High > 0 && curr.Low > 0 {
			return (curr.High + curr.Low) / 2
		}
	}
	return markPrice
}

func (r *Runner) totalMarginUsed() float64 {
	sum := 0.0
	for _, pos := range r.account.Positions() {
		sum += pos.Margin
	}
	return sum
}

func (r *Runner) updateState(ts int64, equity, unrealized, marginUsed float64, priceMap map[string]float64, advancedDecision bool) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	if r.state.MaxEquity == 0 || equity > r.state.MaxEquity {
		r.state.MaxEquity = equity
	}
	if r.state.MinEquity == 0 || equity < r.state.MinEquity {
		r.state.MinEquity = equity
	}
	if r.state.MaxEquity > 0 {
		drawdown := ((r.state.MaxEquity - equity) / r.state.MaxEquity) * 100
		if drawdown > r.state.MaxDrawdownPct {
			r.state.MaxDrawdownPct = drawdown
		}
	}

	positions := make(map[string]PositionSnapshot)
	for _, pos := range r.account.Positions() {
		key := fmt.Sprintf("%s:%s", pos.Symbol, pos.Side)
		positions[key] = PositionSnapshot{
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			Quantity:         pos.Quantity,
			AvgPrice:         pos.EntryPrice,
			Leverage:         pos.Leverage,
			LiquidationPrice: pos.LiquidationPrice,
			MarginUsed:       pos.Margin,
			OpenTime:         pos.OpenTime,
			StopLoss:         pos.StopLoss,
			TakeProfit:       pos.TakeProfit,
		}
	}

	r.state.BarTimestamp = ts
	r.state.BarIndex++
	if advancedDecision {
		r.state.DecisionCycle++
	}
	r.state.Cash = r.account.Cash()
	r.state.Equity = equity
	r.state.UnrealizedPnL = unrealized
	r.state.RealizedPnL = r.account.RealizedPnL()
	r.state.Positions = positions
	r.state.LastUpdate = time.Now().UTC()
}

func (r *Runner) maybeCheckpoint() error {
	state := r.snapshotState()
	shouldCheckpoint := false

	if r.cfg.CheckpointIntervalBars > 0 && state.BarIndex > 0 && state.BarIndex%r.cfg.CheckpointIntervalBars == 0 {
		shouldCheckpoint = true
	}

	interval := time.Duration(r.cfg.CheckpointIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if time.Since(r.lastCheckpoint) >= interval {
		shouldCheckpoint = true
	}

	if !shouldCheckpoint {
		return nil
	}

	if err := r.saveCheckpoint(state); err != nil {
		return err
	}

	return nil
}

func (r *Runner) snapshotForCheckpoint(state BacktestState) []PositionSnapshot {
	res := make([]PositionSnapshot, 0, len(state.Positions))
	for _, pos := range state.Positions {
		res = append(res, pos)
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].Symbol == res[j].Symbol {
			return res[i].Side < res[j].Side
		}
		return res[i].Symbol < res[j].Symbol
	})
	return res
}

func (r *Runner) checkLiquidation(ts int64, priceMap map[string]float64, cycle int) ([]TradeEvent, string, error) {
	positions := append([]*position(nil), r.account.Positions()...)
	events := make([]TradeEvent, 0)
	var noteBuilder strings.Builder

	for _, pos := range positions {
		price := priceMap[pos.Symbol]
		liqPrice := pos.LiquidationPrice
		trigger := false
		execPrice := price
		if pos.Side == "long" {
			if price <= liqPrice && liqPrice > 0 {
				trigger = true
				execPrice = liqPrice
			}
		} else {
			if price >= liqPrice && liqPrice > 0 {
				trigger = true
				execPrice = liqPrice
			}
		}
		if !trigger {
			continue
		}

		realized, fee, finalPrice, err := r.account.Close(pos.Symbol, pos.Side, pos.Quantity, execPrice)
		if err != nil {
			return nil, "", err
		}

		noteBuilder.WriteString(fmt.Sprintf("%s %s @ %.4f; ", pos.Symbol, pos.Side, finalPrice))

		evt := TradeEvent{
			Timestamp:       ts,
			Symbol:          pos.Symbol,
			Action:          "liquidated",
			Side:            pos.Side,
			Quantity:        pos.Quantity,
			Price:           finalPrice,
			Fee:             fee,
			Slippage:        0,
			OrderValue:      finalPrice * pos.Quantity,
			RealizedPnL:     realized - fee,
			Leverage:        pos.Leverage,
			Cycle:           cycle,
			PositionAfter:   0,
			LiquidationFlag: true,
			Note:            fmt.Sprintf("forced liquidation at %.4f", finalPrice),
		}
		events = append(events, evt)
	}

	if len(events) == 0 {
		return events, "", nil
	}

	note := strings.TrimSuffix(noteBuilder.String(), "; ")

	r.stateMu.Lock()
	r.state.Liquidated = true
	r.state.LiquidationNote = note
	r.stateMu.Unlock()

	return events, note, nil
}

// checkRiskEventsWithOHLC 使用 OHLC 数据统一检查止损止盈和爆仓
// 返回: (止损止盈事件, 爆仓事件)
// 优先级: 爆仓 > 止损 > 止盈
func (r *Runner) checkRiskEventsWithOHLC(
	priceMap, highMap, lowMap map[string]float64,
	ts int64,
	cycle int,
) ([]TradeEvent, []TradeEvent) {
	slTpEvents := make([]TradeEvent, 0)
	liqEvents := make([]TradeEvent, 0)

	// 复制持仓列表以避免迭代时修改
	positions := append([]*position(nil), r.account.Positions()...)

	for _, pos := range positions {
		currentPrice := priceMap[pos.Symbol]
		high := highMap[pos.Symbol]
		low := lowMap[pos.Symbol]

		if currentPrice <= 0 || high <= 0 || low <= 0 {
			continue
		}

		var triggerType string // "stop_loss", "take_profit", "liquidation"
		var triggerPrice float64
		var reason string

		if pos.Side == "long" {
			// 多头：检查最低价（Low）触发止损/爆仓，最高价（High）触发止盈
			// 优先级：爆仓 > 止损 > 止盈

			if low <= pos.LiquidationPrice && pos.LiquidationPrice > 0 {
				// 强平触发（优先级最高）
				triggerType = "liquidation"
				triggerPrice = pos.LiquidationPrice
				reason = fmt.Sprintf("强制平仓: Low %.4f <= 爆仓价 %.4f", low, pos.LiquidationPrice)

			} else if pos.StopLoss > 0 && low <= pos.StopLoss {
				// 止损触发
				triggerType = "stop_loss"
				triggerPrice = pos.StopLoss
				reason = fmt.Sprintf("多头止损触发: Low %.4f <= %.4f", low, pos.StopLoss)

			} else if pos.TakeProfit > 0 && high >= pos.TakeProfit {
				// 止盈触发（检查最高价）
				triggerType = "take_profit"
				triggerPrice = pos.TakeProfit
				reason = fmt.Sprintf("多头止盈触发: High %.4f >= %.4f", high, pos.TakeProfit)
			}

		} else if pos.Side == "short" {
			// 空头：检查最高价（High）触发止损/爆仓，最低价（Low）触发止盈

			if high >= pos.LiquidationPrice && pos.LiquidationPrice > 0 {
				// 强平触发（优先级最高）
				triggerType = "liquidation"
				triggerPrice = pos.LiquidationPrice
				reason = fmt.Sprintf("强制平仓: High %.4f >= 爆仓价 %.4f", high, pos.LiquidationPrice)

			} else if pos.StopLoss > 0 && high >= pos.StopLoss {
				// 止损触发
				triggerType = "stop_loss"
				triggerPrice = pos.StopLoss
				reason = fmt.Sprintf("空头止损触发: High %.4f >= %.4f", high, pos.StopLoss)

			} else if pos.TakeProfit > 0 && low <= pos.TakeProfit {
				// 止盈触发（检查最低价）
				triggerType = "take_profit"
				triggerPrice = pos.TakeProfit
				reason = fmt.Sprintf("空头止盈触发: Low %.4f <= %.4f", low, pos.TakeProfit)
			}
		}

		if triggerType == "" {
			continue
		}

		// 执行平仓，应用滑点
		fillPrice := r.executionPrice(pos.Symbol, triggerPrice, ts)

		// 🔧 修复：所有触发都应该使用更真实的成交价
		// 止损/止盈/爆仓都是市价单，在市场继续向不利方向移动时会以更差的价格成交
		if pos.Side == "long" {
			// 多头平仓：价格下跌触发，使用更低的价格
			// 参考价格：Low（K线内最不利价格）
			worstPrice := low
			if worstPrice < fillPrice {
				fillPrice = worstPrice
				log.Printf("  ⚠️ %s %s 使用更差的成交价: %.4f (原触发价: %.4f, Low: %.4f)",
					pos.Symbol, triggerType, fillPrice, triggerPrice, low)
			}
		} else {
			// 空头平仓：价格上涨触发，使用更高的价格
			// 参考价格：High（K线内最不利价格）
			worstPrice := high
			if worstPrice > fillPrice {
				fillPrice = worstPrice
				log.Printf("  ⚠️ %s %s 使用更差的成交价: %.4f (原触发价: %.4f, High: %.4f)",
					pos.Symbol, triggerType, fillPrice, triggerPrice, high)
			}
		}

		realized, fee, execPrice, err := r.account.Close(
			pos.Symbol,
			pos.Side,
			pos.Quantity,
			fillPrice,
		)

		if err != nil {
			log.Printf("⚠️ 风险事件平仓失败 [%s %s %s]: %v",
				triggerType, pos.Symbol, pos.Side, err)
			continue
		}

		action := fmt.Sprintf("auto_close_%s_%s", pos.Side, triggerType)
		trade := TradeEvent{
			Timestamp:       ts,
			Symbol:          pos.Symbol,
			Action:          action,
			Side:            pos.Side,
			Quantity:        pos.Quantity,
			Price:           execPrice,
			Fee:             fee,
			RealizedPnL:     realized - fee,
			Leverage:        pos.Leverage,
			Cycle:           cycle,
			Note:            reason,
			LiquidationFlag: triggerType == "liquidation",
		}

		if triggerType == "liquidation" {
			liqEvents = append(liqEvents, trade)
			log.Printf("  🚨 %s (实际价格: %.4f, 盈亏: %.2f USDT)",
				reason, execPrice, realized-fee)
			// 标记回测已爆仓
			r.stateMu.Lock()
			r.state.Liquidated = true
			r.state.LiquidationNote = fmt.Sprintf("%s %s @ %.4f", pos.Symbol, pos.Side, execPrice)
			r.stateMu.Unlock()
		} else {
			slTpEvents = append(slTpEvents, trade)
			log.Printf("  🛑 %s (实际价格: %.4f, 盈亏: %.2f USDT)",
				reason, execPrice, realized-fee)
		}
	}

	return slTpEvents, liqEvents
}

func (r *Runner) shouldTriggerDecision(barIndex int) bool {
	if r.cfg.DecisionCadenceNBars <= 1 {
		return true
	}
	if barIndex < 0 {
		return true
	}
	return barIndex%r.cfg.DecisionCadenceNBars == 0
}

func (r *Runner) handleStop(reason error) {
	r.forceCheckpoint()
	if reason != nil {
		r.setLastError(reason)
	} else {
		r.setLastError(nil)
	}
	r.statusMu.Lock()
	r.err = reason
	r.status = RunStateStopped
	r.statusMu.Unlock()
	r.persistMetadata()
	r.persistMetrics(true)
	r.releaseLock()
}

func (r *Runner) handlePause() {
	r.forceCheckpoint()
	r.setLastError(nil)
	r.statusMu.Lock()
	r.status = RunStatePaused
	r.statusMu.Unlock()
	r.persistMetadata()
	r.persistMetrics(true)
}

func (r *Runner) resumeFromPause() {
	r.setLastError(nil)
	r.statusMu.Lock()
	r.status = RunStateRunning
	r.statusMu.Unlock()
	r.persistMetadata()
}

func (r *Runner) handleCompletion() {
	r.setLastError(nil)
	r.statusMu.Lock()
	r.status = RunStateCompleted
	r.statusMu.Unlock()
	r.persistMetadata()
	r.persistMetrics(true)
	r.releaseLock()
}

func (r *Runner) handleFailure(err error) {
	r.forceCheckpoint()
	if err != nil {
		r.setLastError(err)
	}
	r.statusMu.Lock()
	r.err = err
	r.status = RunStateFailed
	r.statusMu.Unlock()
	r.persistMetadata()
	r.persistMetrics(true)
	r.releaseLock()
}

func (r *Runner) handleLiquidation() {
	r.forceCheckpoint()
	r.setLastError(errLiquidated)
	r.statusMu.Lock()
	r.err = errLiquidated
	r.status = RunStateLiquidated
	r.statusMu.Unlock()
	r.persistMetadata()
	r.persistMetrics(true)
	r.releaseLock()
}

func (r *Runner) Pause() {
	select {
	case r.pauseCh <- struct{}{}:
	default:
	}
}

func (r *Runner) Resume() {
	select {
	case r.resumeCh <- struct{}{}:
	default:
	}
}

func (r *Runner) Stop() {
	select {
	case r.stopCh <- struct{}{}:
	default:
	}
}

func (r *Runner) Wait() error {
	<-r.doneCh
	r.statusMu.RLock()
	defer r.statusMu.RUnlock()
	return r.err
}

// Status 返回当前运行状态。
func (r *Runner) Status() RunState {
	r.statusMu.RLock()
	defer r.statusMu.RUnlock()
	return r.status
}

// StatusPayload 构建用于 API 的状态响应。
func (r *Runner) StatusPayload() StatusPayload {
	snapshot := r.snapshotState()
	progress := progressPercent(snapshot, r.cfg)

	payload := StatusPayload{
		RunID:          r.cfg.RunID,
		State:          r.Status(),
		ProgressPct:    progress,
		ProcessedBars:  snapshot.BarIndex,
		CurrentTime:    snapshot.BarTimestamp,
		DecisionCycle:  snapshot.DecisionCycle,
		Equity:         snapshot.Equity,
		UnrealizedPnL:  snapshot.UnrealizedPnL,
		RealizedPnL:    snapshot.RealizedPnL,
		Note:           snapshot.LiquidationNote,
		LastError:      r.lastErrorString(),
		LastUpdatedIso: snapshot.LastUpdate.UTC().Format(time.RFC3339),
	}
	return payload
}

func (r *Runner) snapshotState() BacktestState {
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()

	copyState := *r.state
	copyState.Positions = make(map[string]PositionSnapshot, len(r.state.Positions))
	for k, v := range r.state.Positions {
		copyState.Positions[k] = v
	}
	return copyState
}

func (r *Runner) persistMetadata() {
	state := r.snapshotState()
	meta := r.buildMetadata(state, r.Status())
	meta.CreatedAt = r.createdAt
	if err := SaveRunMetadata(meta); err != nil {
		log.Printf("failed to save run metadata for %s: %v", r.cfg.RunID, err)
	} else {
		if err := updateRunIndex(meta, &r.cfg); err != nil {
			log.Printf("failed to update index for %s: %v", r.cfg.RunID, err)
		}
	}
}

func (r *Runner) logDecision(record *logger.DecisionRecord) error {
	if record == nil {
		return nil
	}
	if err := r.decisionLogger.LogDecision(record); err != nil {
		return err
	}
	persistDecisionRecord(r.cfg.RunID, record)
	return nil
}

func (r *Runner) persistMetrics(force bool) {
	if r.cfg.RunID == "" {
		return
	}

	if !force && !r.lastMetricsWrite.IsZero() {
		if time.Since(r.lastMetricsWrite) < metricsWriteInterval {
			return
		}
	}

	state := r.snapshotState()
	metrics, err := CalculateMetrics(r.cfg.RunID, &r.cfg, &state)
	if err != nil {
		log.Printf("failed to compute metrics for %s: %v", r.cfg.RunID, err)
		return
	}
	if metrics == nil {
		return
	}
	if err := PersistMetrics(r.cfg.RunID, metrics); err != nil {
		log.Printf("failed to persist metrics for %s: %v", r.cfg.RunID, err)
		return
	}
	r.lastMetricsWrite = time.Now()
}

func (r *Runner) buildMetadata(state BacktestState, runState RunState) *RunMetadata {
	if state.Liquidated && runState != RunStateLiquidated {
		runState = RunStateLiquidated
	}

	progress := progressPercent(state, r.cfg)

	summary := RunSummary{
		SymbolCount:           len(r.cfg.Symbols),
		DecisionTF:            r.cfg.DecisionTimeframe,
		ProcessedBars:         state.BarIndex,
		ProgressPct:           progress,
		EquityLast:            state.Equity,
		MaxDrawdownPct:        state.MaxDrawdownPct,
		Liquidated:            state.Liquidated,
		LiquidationNote:       state.LiquidationNote,
		PromptVariant:         r.cfg.PromptVariant,
		PromptTemplate:        r.cfg.PromptTemplate,
		CustomPrompt:          r.cfg.CustomPrompt,
		OverridePrompt:        r.cfg.OverrideBasePrompt,
		PromptContentSnapshot: r.promptSnapshot,
	}

	meta := &RunMetadata{
		RunID:     r.cfg.RunID,
		UserID:    r.cfg.UserID,
		State:     runState,
		LastError: r.lastErrorString(),
		Summary:   summary,
	}

	return meta
}

func progressPercent(state BacktestState, cfg BacktestConfig) float64 {
	duration := cfg.Duration()
	if duration <= 0 {
		return 0
	}
	if state.BarTimestamp == 0 {
		return 0
	}

	start := time.Unix(cfg.StartTS, 0)
	end := time.Unix(cfg.EndTS, 0)
	current := time.UnixMilli(state.BarTimestamp)

	if !current.After(start) {
		return 0
	}
	if current.After(end) {
		return 100
	}

	elapsed := current.Sub(start)
	pct := float64(elapsed) / float64(duration) * 100
	if pct > 100 {
		pct = 100
	}
	if pct < 0 {
		pct = 0
	}
	return pct
}

func (r *Runner) buildCheckpointFromState(state BacktestState) *Checkpoint {
	return &Checkpoint{
		BarIndex:        state.BarIndex,
		BarTimestamp:    state.BarTimestamp,
		Cash:            state.Cash,
		Equity:          state.Equity,
		UnrealizedPnL:   state.UnrealizedPnL,
		RealizedPnL:     state.RealizedPnL,
		Positions:       r.snapshotForCheckpoint(state),
		DecisionCycle:   state.DecisionCycle,
		Liquidated:      state.Liquidated,
		LiquidationNote: state.LiquidationNote,
		MaxEquity:       state.MaxEquity,
		MinEquity:       state.MinEquity,
		MaxDrawdownPct:  state.MaxDrawdownPct,
		AICacheRef:      r.cachePath,
	}
}

func (r *Runner) saveCheckpoint(state BacktestState) error {
	ckpt := r.buildCheckpointFromState(state)
	if ckpt == nil {
		return nil
	}
	if err := SaveCheckpoint(r.cfg.RunID, ckpt); err != nil {
		return err
	}
	r.lastCheckpoint = time.Now()
	return nil
}

func (r *Runner) forceCheckpoint() {
	state := r.snapshotState()
	if err := r.saveCheckpoint(state); err != nil {
		log.Printf("failed to save checkpoint for %s: %v", r.cfg.RunID, err)
	}
}

func (r *Runner) RestoreFromCheckpoint() error {
	ckpt, err := LoadCheckpoint(r.cfg.RunID)
	if err != nil {
		return err
	}
	return r.applyCheckpoint(ckpt)
}

func (r *Runner) applyCheckpoint(ckpt *Checkpoint) error {
	if ckpt == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	r.account.RestoreFromSnapshots(ckpt.Cash, ckpt.RealizedPnL, ckpt.Positions)
	r.decisionLogger.SetCycleNumber(ckpt.DecisionCycle)
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.state.BarIndex = ckpt.BarIndex
	r.state.BarTimestamp = ckpt.BarTimestamp
	r.state.Cash = ckpt.Cash
	r.state.Equity = ckpt.Equity
	r.state.UnrealizedPnL = ckpt.UnrealizedPnL
	r.state.RealizedPnL = ckpt.RealizedPnL
	r.state.DecisionCycle = ckpt.DecisionCycle
	r.state.Liquidated = ckpt.Liquidated
	r.state.LiquidationNote = ckpt.LiquidationNote
	r.state.MaxEquity = ckpt.MaxEquity
	r.state.MinEquity = ckpt.MinEquity
	r.state.MaxDrawdownPct = ckpt.MaxDrawdownPct
	r.state.Positions = snapshotsToMap(ckpt.Positions)
	r.state.LastUpdate = time.Now().UTC()
	r.lastCheckpoint = time.Now()
	return nil
}

func snapshotsToMap(snaps []PositionSnapshot) map[string]PositionSnapshot {
	positions := make(map[string]PositionSnapshot, len(snaps))
	for _, snap := range snaps {
		key := fmt.Sprintf("%s:%s", snap.Symbol, snap.Side)
		positions[key] = snap
	}
	return positions
}

func sortDecisionsByPriority(decisions []decision.Decision) []decision.Decision {
	if len(decisions) <= 1 {
		return decisions
	}

	priority := func(action string) int {
		switch action {
		case "close_long", "close_short":
			return 1
		case "open_long", "open_short":
			return 2
		case "hold", "wait":
			return 3
		default:
			return 99
		}
	}

	result := make([]decision.Decision, len(decisions))
	copy(result, decisions)

	sort.Slice(result, func(i, j int) bool {
		pi := priority(result[i].Action)
		pj := priority(result[j].Action)
		if pi != pj {
			return pi < pj
		}
		return i < j
	})

	return result
}

func barVWAP(k market.Kline) float64 {
	values := []float64{k.Open, k.High, k.Low, k.Close}
	sum := 0.0
	count := 0.0
	for _, v := range values {
		if v > 0 {
			sum += v
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / count
}
