package trader

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"nofx/decision"
	"nofx/logger"
	"nofx/market"
	"nofx/pool"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/suite"
)

// ============================================================
// AutoTraderTestSuite - 使用 testify/suite 进行结构化测试
// ============================================================

// AutoTraderTestSuite 是 AutoTrader 的测试套件
// 使用 testify/suite 来组织测试，提供统一的 setup/teardown 和 mock 管理
type AutoTraderTestSuite struct {
	suite.Suite

	// 测试对象
	autoTrader *AutoTrader

	// Mock 依赖
	mockTrader *MockTrader
	mockDB     *MockDatabase
	mockLogger logger.IDecisionLogger

	// gomonkey patches
	patches *gomonkey.Patches

	// 测试配置
	config AutoTraderConfig
}

// SetupSuite 在整个测试套件开始前执行一次
func (s *AutoTraderTestSuite) SetupSuite() {
	// 可以在这里初始化一些全局资源
}

// TearDownSuite 在整个测试套件结束后执行一次
func (s *AutoTraderTestSuite) TearDownSuite() {
	// 清理全局资源
}

// SetupTest 在每个测试用例开始前执行
func (s *AutoTraderTestSuite) SetupTest() {
	// 初始化 patches
	s.patches = gomonkey.NewPatches()

	// 创建 mock 对象
	s.mockTrader = &MockTrader{
		balance: map[string]interface{}{
			"totalWalletBalance":    10000.0,
			"availableBalance":      8000.0,
			"totalUnrealizedProfit": 100.0,
		},
		positions: []map[string]interface{}{},
	}

	s.mockDB = &MockDatabase{}

	// 创建临时决策日志记录器
	s.mockLogger = logger.NewDecisionLogger("/tmp/test_decision_logs")

	// 设置默认配置
	s.config = AutoTraderConfig{
		ID:                   "test_trader",
		Name:                 "Test Trader",
		AIModel:              "deepseek",
		Exchange:             "binance",
		InitialBalance:       10000.0,
		ScanInterval:         3 * time.Minute,
		SystemPromptTemplate: "adaptive",
		BTCETHLeverage:       10,
		AltcoinLeverage:      5,
		IsCrossMargin:        true,
	}

	// 创建 AutoTrader 实例（直接构造，不调用 NewAutoTrader 以避免外部依赖）
	s.autoTrader = &AutoTrader{
		id:                    s.config.ID,
		name:                  s.config.Name,
		aiModel:               s.config.AIModel,
		exchange:              s.config.Exchange,
		config:                s.config,
		trader:                s.mockTrader,
		mcpClient:             nil, // 测试中不需要实际的 MCP Client
		decisionLogger:        s.mockLogger,
		initialBalance:        s.config.InitialBalance,
		systemPromptTemplate:  s.config.SystemPromptTemplate,
		defaultCoins:          []string{"BTC", "ETH"},
		tradingCoins:          []string{},
		lastResetTime:         time.Now(),
		startTime:             time.Now(),
		callCount:             0,
		isRunning:             false,
		positionFirstSeenTime: make(map[string]int64),
		lastPositions:         make(map[string]decision.PositionInfo),
		positionStopLoss:      make(map[string]float64),
		positionTakeProfit:    make(map[string]float64),
		stopMonitorCh:         make(chan struct{}),
		peakPnLCache:          make(map[string]float64),
		lastBalanceSyncTime:   time.Now(),
		database:              s.mockDB,
		userID:                "test_user",
	}
}

// TearDownTest 在每个测试用例结束后执行
func (s *AutoTraderTestSuite) TearDownTest() {
	// 重置 gomonkey patches
	if s.patches != nil {
		s.patches.Reset()
	}
}

// ============================================================
// 层次 1: 工具函数测试
// ============================================================

func (s *AutoTraderTestSuite) TestSortDecisionsByPriority() {
	tests := []struct {
		name  string
		input []decision.Decision
	}{
		{
			name: "混合决策_验证优先级排序",
			input: []decision.Decision{
				{Action: "open_long", Symbol: "BTCUSDT"},
				{Action: "close_short", Symbol: "ETHUSDT"},
				{Action: "hold", Symbol: "BNBUSDT"},
				{Action: "update_stop_loss", Symbol: "SOLUSDT"},
				{Action: "open_short", Symbol: "ADAUSDT"},
				{Action: "partial_close", Symbol: "DOGEUSDT"},
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := sortDecisionsByPriority(tt.input)

			s.Equal(len(tt.input), len(result), "结果长度应该相同")

			// 验证优先级是否递增
			getActionPriority := func(action string) int {
				switch action {
				case "close_long", "close_short", "partial_close":
					return 1
				case "update_stop_loss", "update_take_profit":
					return 2
				case "open_long", "open_short":
					return 3
				case "hold", "wait":
					return 4
				default:
					return 999
				}
			}

			for i := 0; i < len(result)-1; i++ {
				currentPriority := getActionPriority(result[i].Action)
				nextPriority := getActionPriority(result[i+1].Action)
				s.LessOrEqual(currentPriority, nextPriority, "优先级应该递增")
			}
		})
	}
}

func (s *AutoTraderTestSuite) TestNormalizeSymbol() {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"已经是标准格式", "BTCUSDT", "BTCUSDT"},
		{"小写转大写", "btcusdt", "BTCUSDT"},
		{"只有币种名称_添加USDT", "BTC", "BTCUSDT"},
		{"带空格_去除空格", " BTC ", "BTCUSDT"},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := normalizeSymbol(tt.input)
			s.Equal(tt.expected, result)
		})
	}
}

// ============================================================
// 层次 2: Getter/Setter 测试
// ============================================================

func (s *AutoTraderTestSuite) TestGettersAndSetters() {
	s.Run("GetID", func() {
		s.Equal("test_trader", s.autoTrader.GetID())
	})

	s.Run("GetName", func() {
		s.Equal("Test Trader", s.autoTrader.GetName())
	})

	s.Run("SetSystemPromptTemplate", func() {
		s.autoTrader.SetSystemPromptTemplate("aggressive")
		s.Equal("aggressive", s.autoTrader.GetSystemPromptTemplate())
	})

	s.Run("SetCustomPrompt", func() {
		s.autoTrader.SetCustomPrompt("custom prompt")
		s.Equal("custom prompt", s.autoTrader.customPrompt)
	})
}

// ============================================================
// 层次 3: PeakPnL 缓存测试
// ============================================================

func (s *AutoTraderTestSuite) TestPeakPnLCache() {
	s.Run("UpdatePeakPnL_首次记录", func() {
		s.autoTrader.UpdatePeakPnL("BTCUSDT", "long", 10.5)
		cache := s.autoTrader.GetPeakPnLCache()
		s.Equal(10.5, cache["BTCUSDT_long"])
	})

	s.Run("UpdatePeakPnL_更新为更高值", func() {
		s.autoTrader.UpdatePeakPnL("BTCUSDT", "long", 15.0)
		cache := s.autoTrader.GetPeakPnLCache()
		s.Equal(15.0, cache["BTCUSDT_long"])
	})

	s.Run("UpdatePeakPnL_不更新为更低值", func() {
		s.autoTrader.UpdatePeakPnL("BTCUSDT", "long", 12.0)
		cache := s.autoTrader.GetPeakPnLCache()
		s.Equal(15.0, cache["BTCUSDT_long"], "峰值应保持不变")
	})

	s.Run("ClearPeakPnLCache", func() {
		s.autoTrader.ClearPeakPnLCache("BTCUSDT", "long")
		cache := s.autoTrader.GetPeakPnLCache()
		_, exists := cache["BTCUSDT_long"]
		s.False(exists, "应该被清除")
	})
}

// ============================================================
// 层次 4: GetStatus 测试
// ============================================================

func (s *AutoTraderTestSuite) TestGetStatus() {
	s.autoTrader.isRunning = true
	s.autoTrader.callCount = 15

	status := s.autoTrader.GetStatus()

	s.Equal("test_trader", status["trader_id"])
	s.Equal("Test Trader", status["trader_name"])
	s.Equal("deepseek", status["ai_model"])
	s.Equal("binance", status["exchange"])
	s.True(status["is_running"].(bool))
	s.Equal(15, status["call_count"])
	s.Equal(10000.0, status["initial_balance"])
}

// ============================================================
// 层次 5: GetAccountInfo 测试
// ============================================================

func (s *AutoTraderTestSuite) TestGetAccountInfo() {
	accountInfo, err := s.autoTrader.GetAccountInfo()

	s.NoError(err)
	s.NotNil(accountInfo)

	// 验证核心字段和数值
	s.Equal(10100.0, accountInfo["total_equity"]) // 10000 + 100
	s.Equal(8000.0, accountInfo["available_balance"])
	s.Equal(100.0, accountInfo["total_pnl"]) // 10100 - 10000
}

// ============================================================
// 层次 6: GetPositions 测试
// ============================================================

func (s *AutoTraderTestSuite) TestGetPositions() {
	s.Run("空持仓", func() {
		positions, err := s.autoTrader.GetPositions()

		s.NoError(err)
		// positions 可能是 nil 或空数组，两者都是有效的
		if positions != nil {
			s.Equal(0, len(positions))
		}
	})

	s.Run("有持仓", func() {
		// 设置 mock 持仓
		s.mockTrader.positions = []map[string]interface{}{
			{
				"symbol":           "BTCUSDT",
				"side":             "long",
				"entryPrice":       50000.0,
				"markPrice":        51000.0,
				"positionAmt":      0.1,
				"unRealizedProfit": 100.0,
				"liquidationPrice": 45000.0,
				"leverage":         10.0,
			},
		}

		positions, err := s.autoTrader.GetPositions()

		s.NoError(err)
		s.Equal(1, len(positions))

		pos := positions[0]
		s.Equal("BTCUSDT", pos["symbol"])
		s.Equal("long", pos["side"])
		s.Equal(0.1, pos["quantity"])
		s.Equal(50000.0, pos["entry_price"])
	})
}

// ============================================================
// 层次 7: getCandidateCoins 测试
// ============================================================

func (s *AutoTraderTestSuite) TestGetCandidateCoins() {
	s.Run("使用数据库默认币种", func() {
		s.autoTrader.defaultCoins = []string{"BTC", "ETH", "BNB"}
		s.autoTrader.tradingCoins = []string{} // 空的自定义币种

		coins, err := s.autoTrader.getCandidateCoins()

		s.NoError(err)
		s.Equal(3, len(coins))
		s.Equal("BTCUSDT", coins[0].Symbol)
		s.Equal("ETHUSDT", coins[1].Symbol)
		s.Equal("BNBUSDT", coins[2].Symbol)
		s.Contains(coins[0].Sources, "default")
	})

	s.Run("使用自定义币种", func() {
		s.autoTrader.tradingCoins = []string{"SOL", "AVAX"}

		coins, err := s.autoTrader.getCandidateCoins()

		s.NoError(err)
		s.Equal(2, len(coins))
		s.Equal("SOLUSDT", coins[0].Symbol)
		s.Equal("AVAXUSDT", coins[1].Symbol)
		s.Contains(coins[0].Sources, "custom")
	})

	s.Run("使用AI500+OI作为fallback", func() {
		s.autoTrader.defaultCoins = []string{} // 空的默认币种
		s.autoTrader.tradingCoins = []string{} // 空的自定义币种

		// Mock pool.GetMergedCoinPool
		s.patches.ApplyFunc(pool.GetMergedCoinPool, func(ai500Limit int) (*pool.MergedCoinPool, error) {
			return &pool.MergedCoinPool{
				AllSymbols: []string{"BTCUSDT", "ETHUSDT"},
				SymbolSources: map[string][]string{
					"BTCUSDT": {"ai500", "oi_top"},
					"ETHUSDT": {"ai500"},
				},
			}, nil
		})

		coins, err := s.autoTrader.getCandidateCoins()

		s.NoError(err)
		s.Equal(2, len(coins))
	})
}

// ============================================================
// 层次 8: buildTradingContext 测试
// ============================================================

func (s *AutoTraderTestSuite) TestBuildTradingContext() {
	// Mock market.Get
	s.patches.ApplyFunc(market.Get, func(symbol string) (*market.Data, error) {
		return &market.Data{Symbol: symbol, CurrentPrice: 50000.0}, nil
	})

	ctx, err := s.autoTrader.buildTradingContext()

	s.NoError(err)
	s.NotNil(ctx)

	// 验证核心字段
	s.Equal(10100.0, ctx.Account.TotalEquity) // 10000 + 100
	s.Equal(8000.0, ctx.Account.AvailableBalance)
	s.Equal(10, ctx.BTCETHLeverage)
	s.Equal(5, ctx.AltcoinLeverage)
}

// ============================================================
// 层次 9: 交易执行测试
// ============================================================

// TestExecuteOpenPosition 测试开仓操作（多空通用）
func (s *AutoTraderTestSuite) TestExecuteOpenPosition() {
	tests := []struct {
		name          string
		action        string
		expectedOrder int64
		existingSide  string
		availBalance  float64
		expectedErr   string
		executeFn     func(*decision.Decision, *logger.DecisionAction) error
	}{
		{
			name:          "成功开多仓",
			action:        "open_long",
			expectedOrder: 123456,
			availBalance:  8000.0,
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeOpenLongWithRecord(d, a)
			},
		},
		{
			name:          "成功开空仓",
			action:        "open_short",
			expectedOrder: 123457,
			availBalance:  8000.0,
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeOpenShortWithRecord(d, a)
			},
		},
		{
			name:         "多仓_保证金不足",
			action:       "open_long",
			availBalance: 0.0,
			expectedErr:  "保证金不足",
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeOpenLongWithRecord(d, a)
			},
		},
		{
			name:         "空仓_保证金不足",
			action:       "open_short",
			availBalance: 0.0,
			expectedErr:  "保证金不足",
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeOpenShortWithRecord(d, a)
			},
		},
		{
			name:         "多仓_已有同方向持仓",
			action:       "open_long",
			existingSide: "long",
			availBalance: 8000.0,
			expectedErr:  "已有多仓",
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeOpenLongWithRecord(d, a)
			},
		},
		{
			name:         "空仓_已有同方向持仓",
			action:       "open_short",
			existingSide: "short",
			availBalance: 8000.0,
			expectedErr:  "已有空仓",
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeOpenShortWithRecord(d, a)
			},
		},
	}

	for _, tt := range tests {
		time.Sleep(time.Millisecond)
		s.Run(tt.name, func() {
			s.patches.ApplyFunc(market.Get, func(symbol string) (*market.Data, error) {
				return &market.Data{Symbol: symbol, CurrentPrice: 50000.0}, nil
			})

			s.mockTrader.balance["availableBalance"] = tt.availBalance
			if tt.existingSide != "" {
				s.mockTrader.positions = []map[string]interface{}{{"symbol": "BTCUSDT", "side": tt.existingSide}}
			} else {
				s.mockTrader.positions = []map[string]interface{}{}
			}

			decision := &decision.Decision{Action: tt.action, Symbol: "BTCUSDT", PositionSizeUSD: 1000.0, Leverage: 10}
			actionRecord := &logger.DecisionAction{Action: tt.action, Symbol: "BTCUSDT"}

			err := tt.executeFn(decision, actionRecord)

			if tt.expectedErr != "" {
				s.Error(err)
				s.Contains(err.Error(), tt.expectedErr)
			} else {
				s.NoError(err)
				s.Equal(tt.expectedOrder, actionRecord.OrderID)
				s.Greater(actionRecord.Quantity, 0.0)
				s.Equal(50000.0, actionRecord.Price)
			}

			// 恢复默认状态
			s.mockTrader.balance["availableBalance"] = 8000.0
			s.mockTrader.positions = []map[string]interface{}{}
		})
	}
}

// TestExecuteClosePosition 测试平仓操作（多空通用）
func (s *AutoTraderTestSuite) TestExecuteClosePosition() {
	tests := []struct {
		name          string
		action        string
		currentPrice  float64
		expectedOrder int64
		executeFn     func(*decision.Decision, *logger.DecisionAction) error
	}{
		{
			name:          "成功平多仓",
			action:        "close_long",
			currentPrice:  51000.0,
			expectedOrder: 123458,
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeCloseLongWithRecord(d, a)
			},
		},
		{
			name:          "成功平空仓",
			action:        "close_short",
			currentPrice:  49000.0,
			expectedOrder: 123459,
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeCloseShortWithRecord(d, a)
			},
		},
	}

	for _, tt := range tests {
		time.Sleep(time.Millisecond)
		s.Run(tt.name, func() {
			s.patches.ApplyFunc(market.Get, func(symbol string) (*market.Data, error) {
				return &market.Data{Symbol: symbol, CurrentPrice: tt.currentPrice}, nil
			})

			decision := &decision.Decision{Action: tt.action, Symbol: "BTCUSDT"}
			actionRecord := &logger.DecisionAction{Action: tt.action, Symbol: "BTCUSDT"}

			err := tt.executeFn(decision, actionRecord)

			s.NoError(err)
			s.Equal(tt.expectedOrder, actionRecord.OrderID)
			s.Equal(tt.currentPrice, actionRecord.Price)
		})
	}
}

// TestExecuteUpdateStopOrTakeProfit 测试更新止损/止盈（多空通用）
func (s *AutoTraderTestSuite) TestExecuteUpdateStopOrTakeProfit() {
	// 使用指针变量来控制 market.Get 的返回值
	var testPrice *float64
	s.patches.ApplyFunc(market.Get, func(symbol string) (*market.Data, error) {
		price := 50000.0
		if testPrice != nil {
			price = *testPrice
		}
		return &market.Data{Symbol: symbol, CurrentPrice: price}, nil
	})

	tests := []struct {
		name         string
		action       string
		symbol       string
		side         string
		currentPrice float64
		newPrice     float64
		hasPosition  bool
		expectedErr  string
		executeFn    func(*decision.Decision, *logger.DecisionAction) error
	}{
		{
			name:         "成功更新多头止损",
			action:       "update_stop_loss",
			symbol:       "BTCUSDT",
			side:         "long",
			currentPrice: 52000.0,
			newPrice:     51000.0,
			hasPosition:  true,
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeUpdateStopLossWithRecord(d, a)
			},
		},
		{
			name:         "成功更新空头止损",
			action:       "update_stop_loss",
			symbol:       "ETHUSDT",
			side:         "short",
			currentPrice: 2900.0,
			newPrice:     2950.0,
			hasPosition:  true,
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeUpdateStopLossWithRecord(d, a)
			},
		},
		{
			name:         "成功更新多头止盈",
			action:       "update_take_profit",
			symbol:       "BTCUSDT",
			side:         "long",
			currentPrice: 52000.0,
			newPrice:     55000.0,
			hasPosition:  true,
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeUpdateTakeProfitWithRecord(d, a)
			},
		},
		{
			name:         "成功更新空头止盈",
			action:       "update_take_profit",
			symbol:       "ETHUSDT",
			side:         "short",
			currentPrice: 2900.0,
			newPrice:     2800.0,
			hasPosition:  true,
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeUpdateTakeProfitWithRecord(d, a)
			},
		},
		{
			name:         "多头止损价格不合理",
			action:       "update_stop_loss",
			symbol:       "BTCUSDT",
			side:         "long",
			currentPrice: 50000.0,
			newPrice:     51000.0,
			hasPosition:  true,
			expectedErr:  "多单止损必须低于当前价格",
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeUpdateStopLossWithRecord(d, a)
			},
		},
		{
			name:         "多头止盈价格不合理",
			action:       "update_take_profit",
			symbol:       "BTCUSDT",
			side:         "long",
			currentPrice: 50000.0,
			newPrice:     49000.0,
			hasPosition:  true,
			expectedErr:  "多单止盈必须高于当前价格",
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeUpdateTakeProfitWithRecord(d, a)
			},
		},
		{
			name:         "止损_持仓不存在",
			action:       "update_stop_loss",
			symbol:       "BTCUSDT",
			currentPrice: 50000.0,
			newPrice:     49000.0,
			hasPosition:  false,
			expectedErr:  "持仓不存在",
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeUpdateStopLossWithRecord(d, a)
			},
		},
		{
			name:         "止盈_持仓不存在",
			action:       "update_take_profit",
			symbol:       "BTCUSDT",
			currentPrice: 50000.0,
			newPrice:     55000.0,
			hasPosition:  false,
			expectedErr:  "持仓不存在",
			executeFn: func(d *decision.Decision, a *logger.DecisionAction) error {
				return s.autoTrader.executeUpdateTakeProfitWithRecord(d, a)
			},
		},
	}

	for _, tt := range tests {
		time.Sleep(time.Millisecond)
		s.Run(tt.name, func() {
			// 设置当前测试用例的价格
			testPrice = &tt.currentPrice

			if tt.hasPosition {
				s.mockTrader.positions = []map[string]interface{}{
					{"symbol": tt.symbol, "side": tt.side, "positionAmt": 0.1},
				}
			} else {
				s.mockTrader.positions = []map[string]interface{}{}
			}

			decision := &decision.Decision{Action: tt.action, Symbol: tt.symbol}
			if tt.action == "update_stop_loss" {
				decision.NewStopLoss = tt.newPrice
			} else {
				decision.NewTakeProfit = tt.newPrice
			}
			actionRecord := &logger.DecisionAction{Action: tt.action, Symbol: tt.symbol}

			err := tt.executeFn(decision, actionRecord)

			if tt.expectedErr != "" {
				s.Error(err)
				s.Contains(err.Error(), tt.expectedErr)
			} else {
				s.NoError(err)
				s.Equal(tt.currentPrice, actionRecord.Price)
			}

			// 恢复默认状态
			s.mockTrader.positions = []map[string]interface{}{}
		})
	}
}

func (s *AutoTraderTestSuite) TestExecutePartialCloseWithRecord() {
	s.Run("成功部分平仓", func() {
		// 设置持仓
		s.mockTrader.positions = []map[string]interface{}{
			{
				"symbol":      "BTCUSDT",
				"side":        "long",
				"positionAmt": 0.1,
				"entryPrice":  50000.0,
				"markPrice":   52000.0,
			},
		}

		// Mock market.Get
		s.patches.ApplyFunc(market.Get, func(symbol string) (*market.Data, error) {
			return &market.Data{
				Symbol:       symbol,
				CurrentPrice: 52000.0,
			}, nil
		})

		decision := &decision.Decision{
			Action:          "partial_close",
			Symbol:          "BTCUSDT",
			ClosePercentage: 50.0,
		}

		actionRecord := &logger.DecisionAction{
			Action: "partial_close",
			Symbol: "BTCUSDT",
		}

		err := s.autoTrader.executePartialCloseWithRecord(decision, actionRecord)

		s.NoError(err)
		s.Equal(0.05, actionRecord.Quantity) // 50% of 0.1
	})

	s.Run("无效的平仓百分比", func() {
		decision := &decision.Decision{
			Action:          "partial_close",
			Symbol:          "BTCUSDT",
			ClosePercentage: 150.0, // 无效
		}

		actionRecord := &logger.DecisionAction{}

		err := s.autoTrader.executePartialCloseWithRecord(decision, actionRecord)

		s.Error(err)
		s.Contains(err.Error(), "平仓百分比必须在 0-100 之间")
	})
}

// ============================================================
// 层次 10: executeDecisionWithRecord 路由测试
// ============================================================

func (s *AutoTraderTestSuite) TestExecuteDecisionWithRecord() {
	// Mock market.Get
	s.patches.ApplyFunc(market.Get, func(symbol string) (*market.Data, error) {
		return &market.Data{
			Symbol:       symbol,
			CurrentPrice: 50000.0,
		}, nil
	})

	s.Run("路由到open_long", func() {
		decision := &decision.Decision{
			Action:          "open_long",
			Symbol:          "BTCUSDT",
			PositionSizeUSD: 1000.0,
			Leverage:        10,
		}
		actionRecord := &logger.DecisionAction{}

		err := s.autoTrader.executeDecisionWithRecord(decision, actionRecord)
		s.NoError(err)
	})

	s.Run("路由到close_long", func() {
		decision := &decision.Decision{
			Action: "close_long",
			Symbol: "BTCUSDT",
		}
		actionRecord := &logger.DecisionAction{}

		err := s.autoTrader.executeDecisionWithRecord(decision, actionRecord)
		s.NoError(err)
	})

	s.Run("路由到hold_不执行", func() {
		decision := &decision.Decision{
			Action: "hold",
			Symbol: "BTCUSDT",
		}
		actionRecord := &logger.DecisionAction{}

		err := s.autoTrader.executeDecisionWithRecord(decision, actionRecord)
		s.NoError(err)
	})

	s.Run("未知action返回错误", func() {
		decision := &decision.Decision{
			Action: "unknown_action",
			Symbol: "BTCUSDT",
		}
		actionRecord := &logger.DecisionAction{}

		err := s.autoTrader.executeDecisionWithRecord(decision, actionRecord)
		s.Error(err)
		s.Contains(err.Error(), "未知的action")
	})
}

func (s *AutoTraderTestSuite) TestCheckPositionDrawdown() {
	tests := []struct {
		name             string
		setupPositions   func()
		setupPeakPnL     func()
		setupFailures    func()
		cleanupFailures  func()
		expectedCacheKey string
		shouldClearCache bool
		skipCacheCheck   bool
	}{
		{
			name:            "获取持仓失败_不panic",
			setupFailures:   func() { s.mockTrader.shouldFailPositions = true },
			cleanupFailures: func() { s.mockTrader.shouldFailPositions = false },
			skipCacheCheck:  true,
		},
		{
			name:           "无持仓_不panic",
			setupPositions: func() { s.mockTrader.positions = []map[string]interface{}{} },
			skipCacheCheck: true,
		},
		{
			name: "收益不足5%_不触发平仓",
			setupPositions: func() {
				s.mockTrader.positions = []map[string]interface{}{
					{"symbol": "BTCUSDT", "side": "long", "positionAmt": 0.1, "entryPrice": 50000.0, "markPrice": 50150.0, "leverage": 10.0},
				}
			},
			setupPeakPnL:   func() { s.autoTrader.ClearPeakPnLCache("BTCUSDT", "long") },
			skipCacheCheck: true,
		},
		{
			name: "回撤不足40%_不触发平仓",
			setupPositions: func() {
				s.mockTrader.positions = []map[string]interface{}{
					{"symbol": "BTCUSDT", "side": "long", "positionAmt": 0.1, "entryPrice": 50000.0, "markPrice": 50400.0, "leverage": 10.0},
				}
			},
			setupPeakPnL:   func() { s.autoTrader.UpdatePeakPnL("BTCUSDT", "long", 10.0) },
			skipCacheCheck: true,
		},
		{
			name: "多头_触发回撤平仓",
			setupPositions: func() {
				s.mockTrader.positions = []map[string]interface{}{
					{"symbol": "BTCUSDT", "side": "long", "positionAmt": 0.1, "entryPrice": 50000.0, "markPrice": 50300.0, "leverage": 10.0},
				}
			},
			setupPeakPnL:     func() { s.autoTrader.UpdatePeakPnL("BTCUSDT", "long", 10.0) },
			expectedCacheKey: "BTCUSDT_long",
			shouldClearCache: true,
		},
		{
			name: "空头_触发回撤平仓",
			setupPositions: func() {
				s.mockTrader.positions = []map[string]interface{}{
					{"symbol": "ETHUSDT", "side": "short", "positionAmt": -0.5, "entryPrice": 3000.0, "markPrice": 2982.0, "leverage": 10.0},
				}
			},
			setupPeakPnL:     func() { s.autoTrader.UpdatePeakPnL("ETHUSDT", "short", 10.0) },
			expectedCacheKey: "ETHUSDT_short",
			shouldClearCache: true,
		},
		{
			name: "多头_平仓失败_保留缓存",
			setupPositions: func() {
				s.mockTrader.positions = []map[string]interface{}{
					{"symbol": "BTCUSDT", "side": "long", "positionAmt": 0.1, "entryPrice": 50000.0, "markPrice": 50300.0, "leverage": 10.0},
				}
			},
			setupPeakPnL:     func() { s.autoTrader.UpdatePeakPnL("BTCUSDT", "long", 10.0) },
			setupFailures:    func() { s.mockTrader.shouldFailCloseLong = true },
			cleanupFailures:  func() { s.mockTrader.shouldFailCloseLong = false },
			expectedCacheKey: "BTCUSDT_long",
			shouldClearCache: false,
		},
		{
			name: "空头_平仓失败_保留缓存",
			setupPositions: func() {
				s.mockTrader.positions = []map[string]interface{}{
					{"symbol": "ETHUSDT", "side": "short", "positionAmt": -0.5, "entryPrice": 3000.0, "markPrice": 2982.0, "leverage": 10.0},
				}
			},
			setupPeakPnL:     func() { s.autoTrader.UpdatePeakPnL("ETHUSDT", "short", 10.0) },
			setupFailures:    func() { s.mockTrader.shouldFailCloseShort = true },
			cleanupFailures:  func() { s.mockTrader.shouldFailCloseShort = false },
			expectedCacheKey: "ETHUSDT_short",
			shouldClearCache: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			if tt.setupPositions != nil {
				tt.setupPositions()
			}
			if tt.setupPeakPnL != nil {
				tt.setupPeakPnL()
			}
			if tt.setupFailures != nil {
				tt.setupFailures()
			}
			if tt.cleanupFailures != nil {
				defer tt.cleanupFailures()
			}

			s.autoTrader.checkPositionDrawdown()

			if !tt.skipCacheCheck {
				cache := s.autoTrader.GetPeakPnLCache()
				_, exists := cache[tt.expectedCacheKey]
				if tt.shouldClearCache {
					s.False(exists, "峰值缓存应该被清理")
				} else {
					s.True(exists, "峰值缓存不应该被清理")
				}
			}

			// 清理状态
			s.mockTrader.positions = []map[string]interface{}{}
		})
	}
}

// ============================================================
// Mock 实现
// ============================================================

// MockDatabase 模拟数据库
type MockDatabase struct {
	shouldFail bool
}

func (m *MockDatabase) UpdateTraderInitialBalance(userID, traderID string, newBalance float64) error {
	if m.shouldFail {
		return errors.New("database error")
	}
	return nil
}

// MockTrader 增强版（添加错误控制）
type MockTrader struct {
	balance                   map[string]interface{}
	positions                 []map[string]interface{}
	shouldFailBalance         bool
	shouldFailPositions       bool
	shouldFailOpenLong        bool
	shouldFailCloseLong       bool
	shouldFailCloseShort      bool
	cancelStopLossCallCount   int
	setStopLossCallCount      int
	cancelTakeProfitCallCount int
	setTakeProfitCallCount    int
}

func (m *MockTrader) GetBalance() (map[string]interface{}, error) {
	if m.shouldFailBalance {
		return nil, errors.New("failed to get balance")
	}
	if m.balance == nil {
		return map[string]interface{}{
			"totalWalletBalance":    10000.0,
			"availableBalance":      8000.0,
			"totalUnrealizedProfit": 100.0,
		}, nil
	}
	return m.balance, nil
}

func (m *MockTrader) GetPositions() ([]map[string]interface{}, error) {
	if m.shouldFailPositions {
		return nil, errors.New("failed to get positions")
	}
	if m.positions == nil {
		return []map[string]interface{}{}, nil
	}
	return m.positions, nil
}

func (m *MockTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	if m.shouldFailOpenLong {
		return nil, errors.New("failed to open long")
	}
	return map[string]interface{}{
		"orderId": int64(123456),
		"symbol":  symbol,
	}, nil
}

func (m *MockTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return map[string]interface{}{
		"orderId": int64(123457),
		"symbol":  symbol,
	}, nil
}

func (m *MockTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	if m.shouldFailCloseLong {
		return nil, errors.New("failed to close long")
	}
	return map[string]interface{}{
		"orderId": int64(123458),
		"symbol":  symbol,
	}, nil
}

func (m *MockTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	if m.shouldFailCloseShort {
		return nil, errors.New("failed to close short")
	}
	return map[string]interface{}{
		"orderId": int64(123459),
		"symbol":  symbol,
	}, nil
}

func (m *MockTrader) SetLeverage(symbol string, leverage int) error {
	return nil
}

func (m *MockTrader) SetMarginMode(symbol string, isCrossMargin bool) error {
	return nil
}

func (m *MockTrader) GetMarketPrice(symbol string) (float64, error) {
	return 50000.0, nil
}

func (m *MockTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	m.setStopLossCallCount++
	return nil
}

func (m *MockTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	m.setTakeProfitCallCount++
	return nil
}

func (m *MockTrader) CancelStopLossOrders(symbol string) error {
	m.cancelStopLossCallCount++
	return nil
}

func (m *MockTrader) CancelTakeProfitOrders(symbol string) error {
	m.cancelTakeProfitCallCount++
	return nil
}

func (m *MockTrader) CancelAllOrders(symbol string) error {
	return nil
}

func (m *MockTrader) CancelStopOrders(symbol string) error {
	return nil
}

func (m *MockTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	return fmt.Sprintf("%.4f", quantity), nil
}

func (m *MockTrader) GetRecentFills(symbol string, startTime int64, endTime int64) ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}

// ============================================================
// 测试套件入口
// ============================================================

// TestAutoTraderTestSuite 运行 AutoTrader 测试套件
func TestAutoTraderTestSuite(t *testing.T) {
	suite.Run(t, new(AutoTraderTestSuite))
}

// ============================================================
// 独立的单元测试 - calculatePnLPercentage 函数测试
// ============================================================

func TestCalculatePnLPercentage(t *testing.T) {
	tests := []struct {
		name          string
		unrealizedPnl float64
		marginUsed    float64
		expected      float64
	}{
		{
			name:          "正常盈利 - 10倍杠杆",
			unrealizedPnl: 100.0,  // 盈利 100 USDT
			marginUsed:    1000.0, // 保证金 1000 USDT
			expected:      10.0,   // 10% 收益率
		},
		{
			name:          "正常亏损 - 10倍杠杆",
			unrealizedPnl: -50.0,  // 亏损 50 USDT
			marginUsed:    1000.0, // 保证金 1000 USDT
			expected:      -5.0,   // -5% 收益率
		},
		{
			name:          "高杠杆盈利 - 价格上涨1%，20倍杠杆",
			unrealizedPnl: 200.0,  // 盈利 200 USDT
			marginUsed:    1000.0, // 保证金 1000 USDT
			expected:      20.0,   // 20% 收益率
		},
		{
			name:          "保证金为0 - 边界情况",
			unrealizedPnl: 100.0,
			marginUsed:    0.0,
			expected:      0.0, // 应该返回 0 而不是除以零错误
		},
		{
			name:          "负保证金 - 边界情况",
			unrealizedPnl: 100.0,
			marginUsed:    -1000.0,
			expected:      0.0, // 应该返回 0（异常情况）
		},
		{
			name:          "盈亏为0",
			unrealizedPnl: 0.0,
			marginUsed:    1000.0,
			expected:      0.0,
		},
		{
			name:          "小额交易",
			unrealizedPnl: 0.5,
			marginUsed:    10.0,
			expected:      5.0,
		},
		{
			name:          "大额盈利",
			unrealizedPnl: 5000.0,
			marginUsed:    10000.0,
			expected:      50.0,
		},
		{
			name:          "极小保证金",
			unrealizedPnl: 1.0,
			marginUsed:    0.01,
			expected:      10000.0, // 100倍收益率
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePnLPercentage(tt.unrealizedPnl, tt.marginUsed)

			// 使用精度比较，避免浮点数误差
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("calculatePnLPercentage(%v, %v) = %v, want %v",
					tt.unrealizedPnl, tt.marginUsed, result, tt.expected)
			}
		})
	}
}

// TestCalculatePnLPercentage_RealWorldScenarios 真实场景测试
func TestCalculatePnLPercentage_RealWorldScenarios(t *testing.T) {
	t.Run("BTC 10倍杠杆，价格上涨2%", func(t *testing.T) {
		// 开仓：1000 USDT 保证金，10倍杠杆 = 10000 USDT 仓位
		// 价格上涨 2% = 200 USDT 盈利
		// 收益率 = 200 / 1000 = 20%
		result := calculatePnLPercentage(200.0, 1000.0)
		expected := 20.0
		if math.Abs(result-expected) > 0.0001 {
			t.Errorf("BTC场景: got %v, want %v", result, expected)
		}
	})

	t.Run("ETH 5倍杠杆，价格下跌3%", func(t *testing.T) {
		// 开仓：2000 USDT 保证金，5倍杠杆 = 10000 USDT 仓位
		// 价格下跌 3% = -300 USDT 亏损
		// 收益率 = -300 / 2000 = -15%
		result := calculatePnLPercentage(-300.0, 2000.0)
		expected := -15.0
		if math.Abs(result-expected) > 0.0001 {
			t.Errorf("ETH场景: got %v, want %v", result, expected)
		}
	})

	t.Run("SOL 20倍杠杆，价格上涨0.5%", func(t *testing.T) {
		// 开仓：500 USDT 保证金，20倍杠杆 = 10000 USDT 仓位
		// 价格上涨 0.5% = 50 USDT 盈利
		// 收益率 = 50 / 500 = 10%
		result := calculatePnLPercentage(50.0, 500.0)
		expected := 10.0
		if math.Abs(result-expected) > 0.0001 {
			t.Errorf("SOL场景: got %v, want %v", result, expected)
		}
	})
}

// ============================================================
// GetPositions 盈亏百分比计算测试 - Issue #8 修复验证
// ============================================================

// TestGetPositions_UnrealizedPnLPercentageStability 测试未实现盈亏百分比的稳定性
// 验证修复 Issue #8：盈亏百分比应该基于开仓价计算保证金，而不是当前价
func (s *AutoTraderTestSuite) TestGetPositions_UnrealizedPnLPercentageStability() {
	tests := []struct {
		name                    string
		entryPrice              float64
		markPrice               float64
		quantity                float64
		leverage                float64
		unrealizedPnl           float64
		expectedMarginUsed      float64
		expectedPnlPct          float64
		description             string
	}{
		{
			name:               "价格上涨_百分比应稳定_基于开仓价",
			entryPrice:         50000.0,
			markPrice:          51000.0, // 价格上涨了 2%
			quantity:           0.1,
			leverage:           10.0,
			unrealizedPnl:      100.0,
			expectedMarginUsed: 500.0,  // 保证金 = 0.1 * 50000 / 10 = 500 (基于开仓价)
			expectedPnlPct:     20.0,   // 100 / 500 * 100 = 20%
			description:        "当价格上涨时，保证金应该基于开仓价(50000)而不是当前价(51000)",
		},
		{
			name:               "价格下跌_百分比应稳定_基于开仓价",
			entryPrice:         50000.0,
			markPrice:          49000.0, // 价格下跌了 2%
			quantity:           0.1,
			leverage:           10.0,
			unrealizedPnl:      -100.0,
			expectedMarginUsed: 500.0,  // 保证金 = 0.1 * 50000 / 10 = 500 (基于开仓价)
			expectedPnlPct:     -20.0,  // -100 / 500 * 100 = -20%
			description:        "当价格下跌时，保证金应该基于开仓价(50000)而不是当前价(49000)",
		},
		{
			name:               "价格大幅上涨_验证百分比不受当前价影响",
			entryPrice:         50000.0,
			markPrice:          55000.0, // 价格上涨了 10%
			quantity:           0.1,
			leverage:           10.0,
			unrealizedPnl:      500.0,
			expectedMarginUsed: 500.0,   // 保证金 = 0.1 * 50000 / 10 = 500 (不是 0.1 * 55000 / 10 = 550)
			expectedPnlPct:     100.0,   // 500 / 500 * 100 = 100%
			description:        "即使价格大幅上涨，保证金也应该固定在开仓价计算值",
		},
		{
			name:               "高杠杆场景_20倍杠杆",
			entryPrice:         3000.0,
			markPrice:          3100.0,
			quantity:           1.0,
			leverage:           20.0,
			unrealizedPnl:      100.0,
			expectedMarginUsed: 150.0,  // 保证金 = 1.0 * 3000 / 20 = 150
			expectedPnlPct:     66.67,  // 100 / 150 * 100 = 66.67%
			description:        "高杠杆下，保证金计算应该基于开仓价",
		},
		{
			name:               "价格不变_盈亏为0",
			entryPrice:         50000.0,
			markPrice:          50000.0,
			quantity:           0.1,
			leverage:           10.0,
			unrealizedPnl:      0.0,
			expectedMarginUsed: 500.0,
			expectedPnlPct:     0.0,
			description:        "价格不变时，盈亏和百分比都应该为0",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// 设置 mock 持仓数据
			s.mockTrader.positions = []map[string]interface{}{
				{
					"symbol":           "BTCUSDT",
					"side":             "long",
					"entryPrice":       tt.entryPrice,
					"markPrice":        tt.markPrice,
					"positionAmt":      tt.quantity,
					"unRealizedProfit": tt.unrealizedPnl,
					"liquidationPrice": 45000.0,
					"leverage":         tt.leverage,
				},
			}

			// 调用 GetPositions
			positions, err := s.autoTrader.GetPositions()

			s.NoError(err, tt.description)
			s.Require().Equal(1, len(positions), "应该有1个持仓")

			pos := positions[0]

			// 验证保证金计算（关键：应该基于 entryPrice）
			actualMarginUsed := pos["margin_used"].(float64)
			s.InDelta(tt.expectedMarginUsed, actualMarginUsed, 0.01,
				"保证金应该基于开仓价(%v)计算，而不是当前价(%v). %s",
				tt.entryPrice, tt.markPrice, tt.description)

			// 验证盈亏百分比
			actualPnlPct := pos["unrealized_pnl_pct"].(float64)
			s.InDelta(tt.expectedPnlPct, actualPnlPct, 0.01,
				"盈亏百分比应该是 %v / %v * 100 = %v%%. %s",
				tt.unrealizedPnl, tt.expectedMarginUsed, tt.expectedPnlPct, tt.description)

			// 额外验证：盈亏百分比应该等于 unrealizedPnl / marginUsed * 100
			expectedCalculatedPct := (tt.unrealizedPnl / tt.expectedMarginUsed) * 100
			s.InDelta(expectedCalculatedPct, actualPnlPct, 0.01,
				"盈亏百分比计算公式验证失败")
		})
	}
}

// TestGetPositions_MarginCalculationRegression 回归测试：验证保证金计算不使用 markPrice
func (s *AutoTraderTestSuite) TestGetPositions_MarginCalculationRegression() {
	s.Run("Issue#8_回归测试_保证金应使用entryPrice", func() {
		// 模拟 Issue #8 的场景：
		// 持仓价格波动时，盈亏百分比不应该随着价格波动而变化（当未实现盈亏不变时）

		entryPrice := 50000.0
		quantity := 0.1
		leverage := 10.0
		unrealizedPnl := 100.0 // 固定盈亏

		// 测试不同的市场价格
		testPrices := []float64{49000.0, 50000.0, 51000.0, 52000.0, 55000.0}

		var pnlPercentages []float64

		for _, markPrice := range testPrices {
			s.mockTrader.positions = []map[string]interface{}{
				{
					"symbol":           "BTCUSDT",
					"side":             "long",
					"entryPrice":       entryPrice,
					"markPrice":        markPrice,
					"positionAmt":      quantity,
					"unRealizedProfit": unrealizedPnl,
					"liquidationPrice": 45000.0,
					"leverage":         leverage,
				},
			}

			positions, err := s.autoTrader.GetPositions()
			s.NoError(err)
			s.Require().Equal(1, len(positions))

			pnlPct := positions[0]["unrealized_pnl_pct"].(float64)
			pnlPercentages = append(pnlPercentages, pnlPct)
		}

		// 验证：所有的盈亏百分比应该相同（因为未实现盈亏相同，保证金基于开仓价固定）
		expectedPnlPct := 20.0 // 100 / (0.1 * 50000 / 10) * 100 = 20%

		for i, pnlPct := range pnlPercentages {
			s.InDelta(expectedPnlPct, pnlPct, 0.01,
				"当市场价=%v时，盈亏百分比应该稳定在%v%%，但实际是%v%%",
				testPrices[i], expectedPnlPct, pnlPct)
		}

		// 验证所有百分比之间的差异应该接近0
		for i := 1; i < len(pnlPercentages); i++ {
			diff := math.Abs(pnlPercentages[i] - pnlPercentages[0])
			s.Less(diff, 0.01,
				"不同市场价下的盈亏百分比应该相同，但价格从%v到%v时，百分比从%v变为%v",
				testPrices[0], testPrices[i], pnlPercentages[0], pnlPercentages[i])
		}
	})
}

// TestUpdateStopLossShouldUpdateMemory 测试 update_stop_loss 应该更新内存中的止损价格
func (s *AutoTraderTestSuite) TestUpdateStopLossShouldUpdateMemory() {
	s.Run("执行update_stop_loss后应该更新positionStopLoss map", func() {
		// 准备：模拟已有持仓
		symbol := "BTCUSDT"
		posKey := "BTCUSDT_short"

		// 初始化止损价格为旧值
		s.autoTrader.positionStopLoss[posKey] = 95000.0

		// 设置 MockTrader 返回的持仓数据
		s.mockTrader.positions = []map[string]interface{}{
			{
				"symbol":      symbol,
				"side":        "short",
				"positionAmt": -0.1,
			},
		}

		// Mock market.Get
		s.patches.ApplyFunc(market.Get, func(sym string) (*market.Data, error) {
			return &market.Data{
				Symbol:       sym,
				CurrentPrice: 94500.0,
			}, nil
		})

		// 执行 update_stop_loss
		newStopLoss := 94571.0
		decision := &decision.Decision{
			Symbol:      symbol,
			Action:      "update_stop_loss",
			NewStopLoss: newStopLoss,
		}
		actionRecord := &logger.DecisionAction{}

		err := s.autoTrader.executeUpdateStopLossWithRecord(decision, actionRecord)

		// 验证
		s.NoError(err, "update_stop_loss应该成功")

		// 🎯 关键验证：内存中的止损价格应该已更新
		actualStopLoss := s.autoTrader.positionStopLoss[posKey]
		s.Equal(newStopLoss, actualStopLoss,
			"executeUpdateStopLoss后，positionStopLoss[%s]应该更新为%.2f，但实际是%.2f",
			posKey, newStopLoss, actualStopLoss)
	})
}

// TestUpdateTakeProfitShouldUpdateMemory 测试 update_take_profit 应该更新内存中的止盈价格
func (s *AutoTraderTestSuite) TestUpdateTakeProfitShouldUpdateMemory() {
	s.Run("执行update_take_profit后应该更新positionTakeProfit map", func() {
		// 准备：模拟已有持仓
		symbol := "BTCUSDT"
		posKey := "BTCUSDT_short"

		// 初始化止盈价格为旧值
		s.autoTrader.positionTakeProfit[posKey] = 92000.0

		// 设置 MockTrader 返回的持仓数据
		s.mockTrader.positions = []map[string]interface{}{
			{
				"symbol":      symbol,
				"side":        "short",
				"positionAmt": -0.1,
			},
		}

		// Mock market.Get
		s.patches.ApplyFunc(market.Get, func(sym string) (*market.Data, error) {
			return &market.Data{
				Symbol:       sym,
				CurrentPrice: 94500.0,
			}, nil
		})

		// 执行 update_take_profit
		newTakeProfit := 93000.0
		decision := &decision.Decision{
			Symbol:        symbol,
			Action:        "update_take_profit",
			NewTakeProfit: newTakeProfit,
		}
		actionRecord := &logger.DecisionAction{}

		err := s.autoTrader.executeUpdateTakeProfitWithRecord(decision, actionRecord)

		// 验证
		s.NoError(err, "update_take_profit应该成功")

		// 🎯 关键验证：内存中的止盈价格应该已更新
		actualTakeProfit := s.autoTrader.positionTakeProfit[posKey]
		s.Equal(newTakeProfit, actualTakeProfit,
			"executeUpdateTakeProfit后，positionTakeProfit[%s]应该更新为%.2f，但实际是%.2f",
			posKey, newTakeProfit, actualTakeProfit)
	})
}

// TestUpdateStopLossSkipDuplicate 测试重复的止损更新应该被跳过
func (s *AutoTraderTestSuite) TestUpdateStopLossSkipDuplicate() {
	s.Run("新止损价格与当前止损相同时应该跳过操作", func() {
		symbol := "BTCUSDT"
		posKey := "BTCUSDT_short"
		currentStopLoss := 95003.0

		// 初始化：当前止损价格为 95003.0
		s.autoTrader.positionStopLoss[posKey] = currentStopLoss

		// 设置 MockTrader 返回的持仓数据
		s.mockTrader.positions = []map[string]interface{}{
			{
				"symbol":      symbol,
				"side":        "short",
				"positionAmt": -0.1,
			},
		}

		// Mock market.Get
		s.patches.ApplyFunc(market.Get, func(sym string) (*market.Data, error) {
			return &market.Data{
				Symbol:       sym,
				CurrentPrice: 93626.0,
			}, nil
		})

		// 执行 update_stop_loss，设置相同的止损价格 95003.0
		decision := &decision.Decision{
			Symbol:      symbol,
			Action:      "update_stop_loss",
			NewStopLoss: 95003.0, // 与当前止损相同
		}
		actionRecord := &logger.DecisionAction{}

		err := s.autoTrader.executeUpdateStopLossWithRecord(decision, actionRecord)

		// 验证：应该成功返回（没有错误）
		s.NoError(err, "重复的止损更新应该直接返回成功")

		// 验证：CancelStopLossOrders 不应该被调用
		s.Equal(0, s.mockTrader.cancelStopLossCallCount,
			"重复止损更新时不应该调用CancelStopLossOrders")

		// 验证：SetStopLoss 不应该被调用
		s.Equal(0, s.mockTrader.setStopLossCallCount,
			"重复止损更新时不应该调用SetStopLoss")
	})

	s.Run("新止损价格与当前止损差异小于0.01时应该跳过", func() {
		symbol := "ETHUSDT"
		posKey := "ETHUSDT_long"
		currentStopLoss := 3000.00

		// 初始化：当前止损价格为 3000.00
		s.autoTrader.positionStopLoss[posKey] = currentStopLoss

		// 设置 MockTrader 返回的持仓数据
		s.mockTrader.positions = []map[string]interface{}{
			{
				"symbol":      symbol,
				"side":        "long",
				"positionAmt": 1.0,
			},
		}

		// Mock market.Get
		s.patches.ApplyFunc(market.Get, func(sym string) (*market.Data, error) {
			return &market.Data{
				Symbol:       sym,
				CurrentPrice: 3100.0,
			}, nil
		})

		// 执行 update_stop_loss，设置 3000.005（差异 < 0.01）
		decision := &decision.Decision{
			Symbol:      symbol,
			Action:      "update_stop_loss",
			NewStopLoss: 3000.005,
		}
		actionRecord := &logger.DecisionAction{}

		err := s.autoTrader.executeUpdateStopLossWithRecord(decision, actionRecord)

		// 验证：应该跳过操作
		s.NoError(err)
		s.Equal(0, s.mockTrader.setStopLossCallCount, "差异<0.01时不应该调用SetStopLoss")
	})
}

// TestUpdateStopLossRatchet 测试止损单向移动机制 (Hard Limit)
func (s *AutoTraderTestSuite) TestUpdateStopLossRatchet() {
	tests := []struct {
		name           string
		symbol         string
		posKey         string // 内存中的 key
		side           string // MockTrader position side
		amount         float64
		currentPrice   float64
		currentSL      float64
		newSL          float64
		expectCall     bool
		expectMemorySL float64
	}{
		{
			name:           "多单_拒绝回调止损(下移)",
			symbol:         "BTCUSDT",
			posKey:         "BTCUSDT_long",
			side:           "long",
			amount:         1.0,
			currentPrice:   55000.0,
			currentSL:      52000.0,
			newSL:          51000.0, // 变差
			expectCall:     false,
			expectMemorySL: 52000.0,
		},
		{
			name:           "多单_接受优化止损(上移)",
			symbol:         "BTCUSDT",
			posKey:         "BTCUSDT_long",
			side:           "long",
			amount:         1.0,
			currentPrice:   55000.0,
			currentSL:      52000.0,
			newSL:          53000.0, // 变好
			expectCall:     true,
			expectMemorySL: 53000.0,
		},
		{
			name:           "空单_拒绝回调止损(上移)",
			symbol:         "ETHUSDT",
			posKey:         "ETHUSDT_short",
			side:           "short",
			amount:         -10.0,
			currentPrice:   2000.0,
			currentSL:      2500.0,
			newSL:          2600.0, // 变差
			expectCall:     false,
			expectMemorySL: 2500.0,
		},
		{
			name:           "空单_接受优化止损(下移)",
			symbol:         "ETHUSDT",
			posKey:         "ETHUSDT_short",
			side:           "short",
			amount:         -10.0,
			currentPrice:   2000.0,
			currentSL:      2500.0,
			newSL:          2400.0, // 变好
			expectCall:     true,
			expectMemorySL: 2400.0,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// 1. 初始化状态
			s.autoTrader.positionStopLoss[tt.posKey] = tt.currentSL
			s.mockTrader.positions = []map[string]interface{}{
				{"symbol": tt.symbol, "side": tt.side, "positionAmt": tt.amount},
			}
			s.mockTrader.setStopLossCallCount = 0 // 重置计数器

			// 2. Mock 市场价格
			s.patches.ApplyFunc(market.Get, func(sym string) (*market.Data, error) {
				return &market.Data{Symbol: sym, CurrentPrice: tt.currentPrice}, nil
			})

			// 3. 执行操作
			decision := &decision.Decision{Symbol: tt.symbol, Action: "update_stop_loss", NewStopLoss: tt.newSL}
			actionRecord := &logger.DecisionAction{}

			err := s.autoTrader.executeUpdateStopLossWithRecord(decision, actionRecord)

			// 4. 验证结果
			s.NoError(err)

			if tt.expectCall {
				s.Equal(1, s.mockTrader.setStopLossCallCount, "应该调用 SetStopLoss")
			} else {
				s.Equal(0, s.mockTrader.setStopLossCallCount, "不应该调用 SetStopLoss")
			}

			s.Equal(tt.expectMemorySL, s.autoTrader.positionStopLoss[tt.posKey], "内存中的止损价格验证失败")
		})
	}
}

// TestUpdateTakeProfitSkipDuplicate 测试重复的止盈更新应该被跳过
func (s *AutoTraderTestSuite) TestUpdateTakeProfitSkipDuplicate() {
	s.Run("新止盈价格与当前止盈相同时应该跳过操作", func() {
		symbol := "BTCUSDT"
		posKey := "BTCUSDT_long"
		currentTakeProfit := 100000.0

		// 初始化：当前止盈价格为 100000.0
		s.autoTrader.positionTakeProfit[posKey] = currentTakeProfit

		// 设置 MockTrader 返回的持仓数据
		s.mockTrader.positions = []map[string]interface{}{
			{
				"symbol":      symbol,
				"side":        "long",
				"positionAmt": 0.1,
			},
		}

		// Mock market.Get
		s.patches.ApplyFunc(market.Get, func(sym string) (*market.Data, error) {
			return &market.Data{
				Symbol:       sym,
				CurrentPrice: 95000.0,
			}, nil
		})

		// 执行 update_take_profit，设置相同的止盈价格
		decision := &decision.Decision{
			Symbol:        symbol,
			Action:        "update_take_profit",
			NewTakeProfit: 100000.0, // 与当前止盈相同
		}
		actionRecord := &logger.DecisionAction{}

		err := s.autoTrader.executeUpdateTakeProfitWithRecord(decision, actionRecord)

		// 验证：应该成功返回
		s.NoError(err, "重复的止盈更新应该直接返回成功")

		// 验证：不应该调用交易所API
		s.Equal(0, s.mockTrader.cancelTakeProfitCallCount,
			"重复止盈更新时不应该调用CancelTakeProfitOrders")
		s.Equal(0, s.mockTrader.setTakeProfitCallCount,
			"重复止盈更新时不应该调用SetTakeProfit")
	})
}
