package trader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/stretchr/testify/assert"
)

// 测试用 API 路径常量
const (
	binanceOrderPath        = "/fapi/v1/order"
	binanceExchangeInfoPath = "/fapi/v1/exchangeInfo"
)

// ============================================================
// 一、BinanceFuturesTestSuite - 继承 base test suite
// ============================================================

// BinanceFuturesTestSuite 币安合约交易器测试套件
// 继承 TraderTestSuite 并添加 Binance Futures 特定的 mock 逻辑
type BinanceFuturesTestSuite struct {
	*TraderTestSuite // 嵌入基础测试套件
	mockServer       *httptest.Server
}

// NewBinanceFuturesTestSuite 创建币安合约测试套件
func NewBinanceFuturesTestSuite(t *testing.T) *BinanceFuturesTestSuite {
	// 创建 mock HTTP 服务器
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 根据不同的 URL 路径返回不同的 mock 响应
		path := r.URL.Path

		var respBody interface{}

		switch {
		// Mock GetBalance - /fapi/v2/balance
		case path == "/fapi/v2/balance":
			respBody = []map[string]interface{}{
				{
					"accountAlias":       "test",
					"asset":              "USDT",
					"balance":            "10000.00",
					"crossWalletBalance": "10000.00",
					"crossUnPnl":         "100.50",
					"availableBalance":   "8000.00",
					"maxWithdrawAmount":  "8000.00",
				},
			}

		// Mock GetAccount - /fapi/v2/account
		case path == "/fapi/v2/account":
			respBody = map[string]interface{}{
				"totalWalletBalance":    "10000.00",
				"availableBalance":      "8000.00",
				"totalUnrealizedProfit": "100.50",
				"assets": []map[string]interface{}{
					{
						"asset":                  "USDT",
						"walletBalance":          "10000.00",
						"unrealizedProfit":       "100.50",
						"marginBalance":          "10100.50",
						"maintMargin":            "200.00",
						"initialMargin":          "2000.00",
						"positionInitialMargin":  "2000.00",
						"openOrderInitialMargin": "0.00",
						"crossWalletBalance":     "10000.00",
						"crossUnPnl":             "100.50",
						"availableBalance":       "8000.00",
						"maxWithdrawAmount":      "8000.00",
					},
				},
			}

		// Mock GetPositions - /fapi/v2/positionRisk
		case path == "/fapi/v2/positionRisk":
			respBody = []map[string]interface{}{
				{
					"symbol":           "BTCUSDT",
					"positionAmt":      "0.5",
					"entryPrice":       "50000.00",
					"markPrice":        "50500.00",
					"unRealizedProfit": "250.00",
					"liquidationPrice": "45000.00",
					"leverage":         "10",
					"positionSide":     "LONG",
				},
			}

		// Mock GetMarketPrice - /fapi/v1/ticker/price and /fapi/v2/ticker/price
		case path == "/fapi/v1/ticker/price" || path == "/fapi/v2/ticker/price":
			symbol := r.URL.Query().Get("symbol")
			if symbol == "" {
				// 返回所有价格
				respBody = []map[string]interface{}{
					{"Symbol": "BTCUSDT", "Price": "50000.00", "Time": 1234567890},
					{"Symbol": "ETHUSDT", "Price": "3000.00", "Time": 1234567890},
				}
			} else if symbol == "INVALIDUSDT" {
				// 返回错误
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"code": -1121,
					"msg":  "Invalid symbol.",
				})
				return
			} else {
				// 返回单个价格（注意：即使有 symbol 参数，也要返回数组）
				price := "50000.00"
				if symbol == "ETHUSDT" {
					price = "3000.00"
				}
				respBody = []map[string]interface{}{
					{
						"Symbol": symbol,
						"Price":  price,
						"Time":   1234567890,
					},
				}
			}

		// Mock ExchangeInfo - /fapi/v1/exchangeInfo
		case path == binanceExchangeInfoPath:
			respBody = map[string]interface{}{
				"symbols": []map[string]interface{}{
					{
						"symbol":             "BTCUSDT",
						"status":             "TRADING",
						"baseAsset":          "BTC",
						"quoteAsset":         "USDT",
						"pricePrecision":     2,
						"quantityPrecision":  3,
						"baseAssetPrecision": 8,
						"quotePrecision":     8,
						"filters": []map[string]interface{}{
							{
								"filterType": "PRICE_FILTER",
								"minPrice":   "0.01",
								"maxPrice":   "1000000",
								"tickSize":   "0.01",
							},
							{
								"filterType": "LOT_SIZE",
								"minQty":     "0.001",
								"maxQty":     "10000",
								"stepSize":   "0.001",
							},
						},
					},
					{
						"symbol":             "ETHUSDT",
						"status":             "TRADING",
						"baseAsset":          "ETH",
						"quoteAsset":         "USDT",
						"pricePrecision":     2,
						"quantityPrecision":  3,
						"baseAssetPrecision": 8,
						"quotePrecision":     8,
						"filters": []map[string]interface{}{
							{
								"filterType": "PRICE_FILTER",
								"minPrice":   "0.01",
								"maxPrice":   "100000",
								"tickSize":   "0.01",
							},
							{
								"filterType": "LOT_SIZE",
								"minQty":     "0.001",
								"maxQty":     "10000",
								"stepSize":   "0.001",
							},
						},
					},
				},
			}

		// Mock CreateOrder - /fapi/v1/order (POST)
		case path == binanceOrderPath && r.Method == "POST":
			symbol := r.FormValue("symbol")
			if symbol == "" {
				symbol = "BTCUSDT"
			}

			// 模拟币安的参数验证：STOP/TAKE_PROFIT 不能使用 closePosition=true
			orderType := r.FormValue("type")
			closePosition := r.FormValue("closePosition")

			if (orderType == "STOP" || orderType == "TAKE_PROFIT") && closePosition == "true" {
				// 返回币安的 -4136 错误
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"code": -4136,
					"msg":  "Target strategy invalid for orderType " + orderType + ",closePosition true",
				})
				return
			}

			respBody = map[string]interface{}{
				"orderId":       123456,
				"symbol":        symbol,
				"status":        "FILLED",
				"clientOrderId": r.FormValue("newClientOrderId"),
				"price":         r.FormValue("price"),
				"avgPrice":      r.FormValue("price"),
				"origQty":       r.FormValue("quantity"),
				"executedQty":   r.FormValue("quantity"),
				"cumQty":        r.FormValue("quantity"),
				"cumQuote":      "1000.00",
				"timeInForce":   r.FormValue("timeInForce"),
				"type":          r.FormValue("type"),
				"reduceOnly":    r.FormValue("reduceOnly") == "true",
				"side":          r.FormValue("side"),
				"positionSide":  r.FormValue("positionSide"),
				"stopPrice":     r.FormValue("stopPrice"),
				"workingType":   r.FormValue("workingType"),
			}

		// Mock CancelOrder - /fapi/v1/order (DELETE)
		case path == binanceOrderPath && r.Method == "DELETE":
			respBody = map[string]interface{}{
				"orderId": 123456,
				"symbol":  r.URL.Query().Get("symbol"),
				"status":  "CANCELED",
			}

		// Mock ListOpenOrders - /fapi/v1/openOrders
		case path == "/fapi/v1/openOrders":
			respBody = []map[string]interface{}{}

		// Mock CancelAllOrders - /fapi/v1/allOpenOrders (DELETE)
		case path == "/fapi/v1/allOpenOrders" && r.Method == "DELETE":
			respBody = map[string]interface{}{
				"code": 200,
				"msg":  "The operation of cancel all open order is done.",
			}

		// Mock SetLeverage - /fapi/v1/leverage
		case path == "/fapi/v1/leverage":
			// 将字符串转换为整数
			leverageStr := r.FormValue("leverage")
			leverage := 10 // 默认值
			if leverageStr != "" {
				// 注意：这里我们直接返回整数，而不是字符串
				fmt.Sscanf(leverageStr, "%d", &leverage)
			}
			respBody = map[string]interface{}{
				"leverage":         leverage,
				"maxNotionalValue": "1000000",
				"symbol":           r.FormValue("symbol"),
			}

		// Mock SetMarginType - /fapi/v1/marginType
		case path == "/fapi/v1/marginType":
			respBody = map[string]interface{}{
				"code": 200,
				"msg":  "success",
			}

		// Mock ChangePositionMode - /fapi/v1/positionSide/dual
		case path == "/fapi/v1/positionSide/dual":
			respBody = map[string]interface{}{
				"code": 200,
				"msg":  "success",
			}

		// Mock ServerTime - /fapi/v1/time
		case path == "/fapi/v1/time":
			respBody = map[string]interface{}{
				"serverTime": 1234567890000,
			}

		// Default: empty response
		default:
			respBody = map[string]interface{}{}
		}

		// 序列化响应
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody)
	}))

	// 创建 futures.Client 并设置为使用 mock 服务器
	client := futures.NewClient("test_api_key", "test_secret_key")
	client.BaseURL = mockServer.URL
	client.HTTPClient = mockServer.Client()

	// 创建 FuturesTrader
	trader := &FuturesTrader{
		client:        client,
		cacheDuration: 0, // 禁用缓存以便测试
	}

	// 创建基础套件
	baseSuite := NewTraderTestSuite(t, trader)

	return &BinanceFuturesTestSuite{
		TraderTestSuite: baseSuite,
		mockServer:      mockServer,
	}
}

// Cleanup 清理资源
func (s *BinanceFuturesTestSuite) Cleanup() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
	s.TraderTestSuite.Cleanup()
}

// ============================================================
// 二、使用 BinanceFuturesTestSuite 运行通用测试
// ============================================================

// TestFuturesTrader_InterfaceCompliance 测试接口兼容性
func TestFuturesTrader_InterfaceCompliance(t *testing.T) {
	var _ Trader = (*FuturesTrader)(nil)
}

// TestFuturesTrader_CommonInterface 使用测试套件运行所有通用接口测试
func TestFuturesTrader_CommonInterface(t *testing.T) {
	// 创建测试套件
	suite := NewBinanceFuturesTestSuite(t)
	defer suite.Cleanup()

	// 运行所有通用接口测试
	suite.RunAllTests()
}

// ============================================================
// 三、币安合约特定功能的单元测试
// ============================================================

// TestNewFuturesTrader 测试创建币安合约交易器
func TestNewFuturesTrader(t *testing.T) {
	// 创建 mock HTTP 服务器
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		var respBody interface{}

		switch path {
		case "/fapi/v1/time":
			respBody = map[string]interface{}{
				"serverTime": 1234567890000,
			}
		case "/fapi/v1/positionSide/dual":
			respBody = map[string]interface{}{
				"code": 200,
				"msg":  "success",
			}
		default:
			respBody = map[string]interface{}{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody)
	}))
	defer mockServer.Close()

	// 测试成功创建
	trader := NewFuturesTrader("test_api_key", "test_secret_key", "test_user")

	// 修改 client 使用 mock server
	trader.client.BaseURL = mockServer.URL
	trader.client.HTTPClient = mockServer.Client()

	assert.NotNil(t, trader)
	assert.NotNil(t, trader.client)
	assert.Equal(t, 15*time.Second, trader.cacheDuration)
}

// TestCalculatePositionSize 测试仓位计算
func TestCalculatePositionSize(t *testing.T) {
	trader := &FuturesTrader{}

	tests := []struct {
		name         string
		balance      float64
		riskPercent  float64
		price        float64
		leverage     int
		wantQuantity float64
	}{
		{
			name:         "正常计算",
			balance:      10000,
			riskPercent:  2,
			price:        50000,
			leverage:     10,
			wantQuantity: 0.04, // (10000 * 0.02 * 10) / 50000 = 0.04
		},
		{
			name:         "高杠杆",
			balance:      10000,
			riskPercent:  1,
			price:        3000,
			leverage:     20,
			wantQuantity: 0.6667, // (10000 * 0.01 * 20) / 3000 = 0.6667
		},
		{
			name:         "低风险",
			balance:      5000,
			riskPercent:  0.5,
			price:        50000,
			leverage:     5,
			wantQuantity: 0.0025, // (5000 * 0.005 * 5) / 50000 = 0.0025
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quantity := trader.CalculatePositionSize(tt.balance, tt.riskPercent, tt.price, tt.leverage)
			assert.InDelta(t, tt.wantQuantity, quantity, 0.0001, "计算的仓位数量不正确")
		})
	}
}

// TestGetBrOrderID 测试订单ID生成
func TestGetBrOrderID(t *testing.T) {
	// 测试3次，确保每次生成的ID都不同
	ids := make(map[string]bool)
	for i := 0; i < 3; i++ {
		id := getBrOrderID()

		// 检查格式
		assert.True(t, strings.HasPrefix(id, "x-KzrpZaP9"), "订单ID应以x-KzrpZaP9开头")

		// 检查长度（应该 <= 32）
		assert.LessOrEqual(t, len(id), 32, "订单ID长度不应超过32字符")

		// 检查唯一性
		assert.False(t, ids[id], "订单ID应该唯一")
		ids[id] = true
	}
}
// ============================================================
// 专项测试：Issue #94 修复验证
// ============================================================

// setupMockServerWithParamCapture 创建能捕获请求参数的 mock server (helper)
func setupMockServerWithParamCapture(capturedParams *map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == binanceOrderPath && r.Method == "POST" {
			r.ParseForm()

			// 捕获所有表单参数
			*capturedParams = make(map[string]string)
			for key := range r.Form {
				(*capturedParams)[key] = r.FormValue(key)
			}

			// 模拟币安的参数验证：STOP/TAKE_PROFIT 不能使用 closePosition=true
			orderType := r.FormValue("type")
			closePosition := r.FormValue("closePosition")

			if (orderType == "STOP" || orderType == "TAKE_PROFIT") && closePosition == "true" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"code": -4136,
					"msg":  "Target strategy invalid for orderType " + orderType + ",closePosition true",
				})
				return
			}

			// 正常响应
			json.NewEncoder(w).Encode(map[string]interface{}{
				"orderId": 123456,
				"symbol":  "BTCUSDT",
				"status":  "FILLED",
			})
		} else if r.URL.Path == binanceExchangeInfoPath {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"symbols": []map[string]interface{}{
					{
						"symbol":             "BTCUSDT",
						"pricePrecision":     2,
						"quantityPrecision":  3,
						"baseAssetPrecision": 8,
						"quotePrecision":     8,
						"filters": []map[string]interface{}{
							{"filterType": "PRICE_FILTER", "tickSize": "0.01"},
							{"filterType": "LOT_SIZE", "stepSize": "0.001"},
						},
					},
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}
	}))
}

// createTestTrader 创建测试用的 trader (helper)
func createTestTrader(mockServerURL string) *FuturesTrader {
	client := futures.NewClient("test_key", "test_secret")
	futures.UseTestnet = true
	client.BaseURL = mockServerURL

	return &FuturesTrader{
		client:        client,
		cacheDuration: 15 * time.Second,
	}
}

// TestStopLossAndTakeProfitNoClosePosition 验证修复：STOP/TAKE_PROFIT 不发送 closePosition
// Issue #94: 币安 API 限制，STOP/TAKE_PROFIT 类型不支持 closePosition=true
func TestStopLossAndTakeProfitNoClosePosition(t *testing.T) {
	tests := []struct {
		name              string
		testFunc          func(*FuturesTrader) error
		expectedOrderType string
		expectedSide      string
	}{
		{
			name: "SetStopLoss 不发送 closePosition",
			testFunc: func(trader *FuturesTrader) error {
				return trader.SetStopLoss("BTCUSDT", "LONG", 0.01, 45000.0)
			},
			expectedOrderType: "STOP",
			expectedSide:      "SELL",
		},
		{
			name: "SetTakeProfit 不发送 closePosition",
			testFunc: func(trader *FuturesTrader) error {
				return trader.SetTakeProfit("BTCUSDT", "LONG", 0.01, 55000.0)
			},
			expectedOrderType: "TAKE_PROFIT",
			expectedSide:      "SELL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedParams map[string]string
			mockServer := setupMockServerWithParamCapture(&capturedParams)
			defer mockServer.Close()

			trader := createTestTrader(mockServer.URL)

			// 执行测试函数
			err := tt.testFunc(trader)

			// 验证结果
			assert.NoError(t, err, "调用应该成功")
			assert.NotNil(t, capturedParams, "应该捕获到请求参数")

			// ✅ 关键验证：确保没有发送 closePosition=true
			closePositionValue, hasClosePosition := capturedParams["closePosition"]
			assert.False(t, hasClosePosition && closePositionValue == "true",
				"不应该发送 closePosition=true 参数（Issue #94 修复）")

			// ✅ 验证必需参数
			assert.Equal(t, tt.expectedOrderType, capturedParams["type"], "订单类型")
			assert.Equal(t, tt.expectedSide, capturedParams["side"], "订单方向")
			assert.Equal(t, "LONG", capturedParams["positionSide"], "持仓方向")
			assert.NotEmpty(t, capturedParams["quantity"], "应该有 quantity")
			assert.NotEmpty(t, capturedParams["price"], "应该有 price（限价）")
			assert.NotEmpty(t, capturedParams["stopPrice"], "应该有 stopPrice")
		})
	}
}

// TestSetStopLossWithClosePositionWouldFail 验证修复前的代码会失败
// 证明：STOP + closePosition=true 会导致 -4136 错误
func TestSetStopLossWithClosePositionWouldFail(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == binanceOrderPath && r.Method == "POST" {
			r.ParseForm()
			orderType := r.FormValue("type")
			closePosition := r.FormValue("closePosition")

			if orderType == "STOP" && closePosition == "true" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"code": -4136,
					"msg":  "Target strategy invalid for orderType STOP,closePosition true",
				})
				return
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"orderId": 123456,
				"status":  "FILLED",
			})
		}
	}))
	defer mockServer.Close()

	client := futures.NewClient("test_key", "test_secret")
	client.BaseURL = mockServer.URL

	// 模拟旧版本实现（发送 closePosition=true）
	_, err := client.NewCreateOrderService().
		Symbol("BTCUSDT").
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeStop).
		StopPrice("45000").
		Quantity("0.01").
		ClosePosition(true). // ❌ 导致 -4136 错误
		Do(context.Background())

	// 验证错误
	assert.Error(t, err, "STOP + closePosition=true 应该失败")
	assert.Contains(t, err.Error(), "-4136", "错误应包含 -4136 代码")
}
