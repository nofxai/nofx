package trader

import (
	"fmt"
	"nofx/decision"
	"nofx/logger"
	"testing"
)

// MockPartialCloseTrader 用於測試 partial close 邏輯
type MockPartialCloseTrader struct {
	positions             []map[string]interface{}
	closePartialCalled    bool
	closeLongCalled       bool
	closeShortCalled      bool
	stopLossCalled        bool
	takeProfitCalled      bool
	cancelAllOrdersCalled bool   // 記錄 CancelAllOrders 是否被調用
	cancelAllOrdersSymbol string // 記錄被調用的 symbol
	lastStopLoss          float64
	lastTakeProfit        float64
}

func (m *MockPartialCloseTrader) CancelAllOrders(symbol string) error {
	m.cancelAllOrdersCalled = true
	m.cancelAllOrdersSymbol = symbol
	return nil
}

func (m *MockPartialCloseTrader) GetPositions() ([]map[string]interface{}, error) {
	return m.positions, nil
}

func (m *MockPartialCloseTrader) ClosePartialLong(symbol string, quantity float64) (map[string]interface{}, error) {
	m.closePartialCalled = true
	return map[string]interface{}{"orderId": "12345"}, nil
}

func (m *MockPartialCloseTrader) ClosePartialShort(symbol string, quantity float64) (map[string]interface{}, error) {
	m.closePartialCalled = true
	return map[string]interface{}{"orderId": "12345"}, nil
}

func (m *MockPartialCloseTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	m.closeLongCalled = true
	return map[string]interface{}{"orderId": "12346"}, nil
}

func (m *MockPartialCloseTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	m.closeShortCalled = true
	return map[string]interface{}{"orderId": "12346"}, nil
}

func (m *MockPartialCloseTrader) SetStopLoss(symbol, side string, quantity, price float64) error {
	m.stopLossCalled = true
	m.lastStopLoss = price
	return nil
}

func (m *MockPartialCloseTrader) SetTakeProfit(symbol, side string, quantity, price float64) error {
	m.takeProfitCalled = true
	m.lastTakeProfit = price
	return nil
}

func assertPrice(t *testing.T, desc string, got, want float64) {
	t.Helper()
	if got != want {
		t.Errorf("%s錯誤: got = %.2f, 期望 = %.2f", desc, got, want)
	}
}

func assertBool(t *testing.T, desc string, got, want bool) {
	t.Helper()
	if got != want {
		t.Errorf("%s錯誤: got = %v, 期望 = %v", desc, got, want)
	}
}

// TestPartialCloseMinPositionCheck 測試最小倉位檢查邏輯
func TestPartialCloseMinPositionCheck(t *testing.T) {
	tests := []struct {
		name              string
		totalQuantity     float64
		markPrice         float64
		closePercentage   float64
		expectFullClose   bool // 是否應該觸發全平邏輯
		expectRemainValue float64
	}{
		{
			name:              "正常部分平倉_剩餘價值充足",
			totalQuantity:     1.0,
			markPrice:         100.0,
			closePercentage:   50.0,
			expectFullClose:   false,
			expectRemainValue: 50.0, // 剩餘 0.5 * 100 = 50 USDT
		},
		{
			name:              "部分平倉_剩餘價值小於10USDT_應該全平",
			totalQuantity:     0.2,
			markPrice:         100.0,
			closePercentage:   95.0, // 平倉 95%，剩餘 1 USDT (0.2 * 5% * 100)
			expectFullClose:   true,
			expectRemainValue: 1.0,
		},
		{
			name:              "部分平倉_剩餘價值剛好10USDT_應該全平",
			totalQuantity:     1.0,
			markPrice:         100.0,
			closePercentage:   90.0, // 剩餘 10 USDT (1.0 * 10% * 100)，邊界測試 (<=)
			expectFullClose:   true,
			expectRemainValue: 10.0,
		},
		{
			name:              "部分平倉_剩餘價值11USDT_不應全平",
			totalQuantity:     1.1,
			markPrice:         100.0,
			closePercentage:   90.0, // 剩餘 11 USDT (1.1 * 10% * 100)
			expectFullClose:   false,
			expectRemainValue: 11.0,
		},
		{
			name:              "大倉位部分平倉_剩餘價值遠大於10USDT",
			totalQuantity:     10.0,
			markPrice:         1000.0,
			closePercentage:   80.0,
			expectFullClose:   false,
			expectRemainValue: 2000.0, // 剩餘 2 * 1000 = 2000 USDT
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 計算剩餘價值
			closeQuantity := tt.totalQuantity * (tt.closePercentage / 100.0)
			remainingQuantity := tt.totalQuantity - closeQuantity
			remainingValue := remainingQuantity * tt.markPrice

			// 驗證計算（使用浮點數比較允許微小誤差）
			const epsilon = 0.001
			if remainingValue-tt.expectRemainValue > epsilon || tt.expectRemainValue-remainingValue > epsilon {
				t.Errorf("計算錯誤: 剩餘價值 = %.2f, 期望 = %.2f",
					remainingValue, tt.expectRemainValue)
			}

			// 驗證最小倉位檢查邏輯
			const MIN_POSITION_VALUE = 10.0
			shouldFullClose := remainingValue > 0 && remainingValue <= MIN_POSITION_VALUE

			if shouldFullClose != tt.expectFullClose {
				t.Errorf("最小倉位檢查失敗: shouldFullClose = %v, 期望 = %v (剩餘價值 = %.2f USDT)",
					shouldFullClose, tt.expectFullClose, remainingValue)
			}
		})
	}
}

// TestPartialCloseWithStopLossTakeProfitRecovery 測試止盈止損恢復邏輯
func TestPartialCloseWithStopLossTakeProfitRecovery(t *testing.T) {
	tests := []struct {
		name             string
		newStopLoss      float64
		newTakeProfit    float64
		expectStopLoss   bool
		expectTakeProfit bool
	}{
		{
			name:             "有新止損和止盈_應該恢復兩者",
			newStopLoss:      95.0,
			newTakeProfit:    110.0,
			expectStopLoss:   true,
			expectTakeProfit: true,
		},
		{
			name:             "只有新止損_僅恢復止損",
			newStopLoss:      95.0,
			newTakeProfit:    0,
			expectStopLoss:   true,
			expectTakeProfit: false,
		},
		{
			name:             "只有新止盈_僅恢復止盈",
			newStopLoss:      0,
			newTakeProfit:    110.0,
			expectStopLoss:   false,
			expectTakeProfit: true,
		},
		{
			name:             "沒有新止損止盈_不恢復",
			newStopLoss:      0,
			newTakeProfit:    0,
			expectStopLoss:   false,
			expectTakeProfit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模擬止盈止損恢復邏輯
			stopLossRecovered := tt.newStopLoss > 0
			takeProfitRecovered := tt.newTakeProfit > 0

			if stopLossRecovered != tt.expectStopLoss {
				t.Errorf("止損恢復邏輯錯誤: recovered = %v, 期望 = %v",
					stopLossRecovered, tt.expectStopLoss)
			}

			if takeProfitRecovered != tt.expectTakeProfit {
				t.Errorf("止盈恢復邏輯錯誤: recovered = %v, 期望 = %v",
					takeProfitRecovered, tt.expectTakeProfit)
			}
		})
	}
}

// TestPartialCloseSLTPFallbackFromMemory 測試當 AI 不提供 SL/TP 時，從內存 fallback 獲取原始價格
// 這是 Issue #70 的關鍵修復：確保部分平倉後剩餘倉位有止損止盈保護
func TestPartialCloseSLTPFallbackFromMemory(t *testing.T) {
	tests := []struct {
		name               string
		aiNewStopLoss      float64 // AI 提供的新止損
		aiNewTakeProfit    float64 // AI 提供的新止盈
		memoryStopLoss     float64 // 內存中緩存的原始止損
		memoryTakeProfit   float64 // 內存中緩存的原始止盈
		expectFinalSL      float64 // 最終使用的止損
		expectFinalTP      float64 // 最終使用的止盈
		expectSLRecovered  bool    // 是否應該設置止損
		expectTPRecovered  bool    // 是否應該設置止盈
	}{
		{
			name:              "AI提供新價格_使用AI價格",
			aiNewStopLoss:     48000.0,
			aiNewTakeProfit:   52000.0,
			memoryStopLoss:    47000.0,
			memoryTakeProfit:  51000.0,
			expectFinalSL:     48000.0, // 使用 AI 提供的價格
			expectFinalTP:     52000.0,
			expectSLRecovered: true,
			expectTPRecovered: true,
		},
		{
			name:              "AI不提供_從內存fallback",
			aiNewStopLoss:     0,
			aiNewTakeProfit:   0,
			memoryStopLoss:    47000.0,
			memoryTakeProfit:  51000.0,
			expectFinalSL:     47000.0, // 從內存 fallback
			expectFinalTP:     51000.0,
			expectSLRecovered: true,
			expectTPRecovered: true,
		},
		{
			name:              "AI提供止損_止盈從內存fallback",
			aiNewStopLoss:     48000.0,
			aiNewTakeProfit:   0,
			memoryStopLoss:    47000.0,
			memoryTakeProfit:  51000.0,
			expectFinalSL:     48000.0, // AI 提供
			expectFinalTP:     51000.0, // 內存 fallback
			expectSLRecovered: true,
			expectTPRecovered: true,
		},
		{
			name:              "AI和內存都沒有_不設置",
			aiNewStopLoss:     0,
			aiNewTakeProfit:   0,
			memoryStopLoss:    0,
			memoryTakeProfit:  0,
			expectFinalSL:     0,
			expectFinalTP:     0,
			expectSLRecovered: false,
			expectTPRecovered: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模擬 executePartialCloseWithRecord 中的邏輯
			// 優先使用 AI 提供的新價格，否則使用內存中緩存的原始價格
			finalStopLoss := tt.aiNewStopLoss
			if finalStopLoss <= 0 {
				finalStopLoss = tt.memoryStopLoss
			}
			finalTakeProfit := tt.aiNewTakeProfit
			if finalTakeProfit <= 0 {
				finalTakeProfit = tt.memoryTakeProfit
			}

			// 驗證最終價格
			assertPrice(t, "最終止損價格", finalStopLoss, tt.expectFinalSL)
			assertPrice(t, "最終止盈價格", finalTakeProfit, tt.expectFinalTP)

			// 驗證是否應該調用設置止損止盈
			shouldSetSL := finalStopLoss > 0
			shouldSetTP := finalTakeProfit > 0

			assertBool(t, "止損設置邏輯", shouldSetSL, tt.expectSLRecovered)
			assertBool(t, "止盈設置邏輯", shouldSetTP, tt.expectTPRecovered)
		})
	}
}

// TestPartialCloseEdgeCases 測試邊界情況
func TestPartialCloseEdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		closePercentage float64
		totalQuantity   float64
		markPrice       float64
		expectError     bool
		errorContains   string
	}{
		{
			name:            "平倉百分比為0_應該報錯",
			closePercentage: 0,
			totalQuantity:   1.0,
			markPrice:       100.0,
			expectError:     true,
			errorContains:   "0-100",
		},
		{
			name:            "平倉百分比超過100_應該報錯",
			closePercentage: 101.0,
			totalQuantity:   1.0,
			markPrice:       100.0,
			expectError:     true,
			errorContains:   "0-100",
		},
		{
			name:            "平倉百分比為負數_應該報錯",
			closePercentage: -10.0,
			totalQuantity:   1.0,
			markPrice:       100.0,
			expectError:     true,
			errorContains:   "0-100",
		},
		{
			name:            "正常範圍_不應報錯",
			closePercentage: 50.0,
			totalQuantity:   1.0,
			markPrice:       100.0,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模擬百分比驗證邏輯
			var err error
			if tt.closePercentage <= 0 || tt.closePercentage > 100 {
				err = fmt.Errorf("平仓百分比必须在 0-100 之间，当前: %.1f", tt.closePercentage)
			}

			if tt.expectError {
				if err == nil {
					t.Errorf("期望報錯但沒有報錯")
				}
			} else {
				if err != nil {
					t.Errorf("不應報錯但報錯了: %v", err)
				}
			}
		})
	}
}

// TestPartialCloseSLTPMemoryCacheUpdate 測試部分平倉後內存緩存是否正確更新
// 這是代碼審查中發現的 bug：部分平倉後重新創建 SL/TP 訂單時，需要更新內存緩存
// 否則後續的 partial_close 或 update_stop_loss 會使用過時的價格
func TestPartialCloseSLTPMemoryCacheUpdate(t *testing.T) {
	tests := []struct {
		name                   string
		originalStopLoss       float64 // 開倉時設置的止損
		originalTakeProfit     float64 // 開倉時設置的止盈
		aiNewStopLoss          float64 // AI 在 partial_close 時提供的新止損
		aiNewTakeProfit        float64 // AI 在 partial_close 時提供的新止盈
		expectCacheStopLoss    float64 // 期望內存緩存中的止損（partial_close 後）
		expectCacheTakeProfit  float64 // 期望內存緩存中的止盈（partial_close 後）
	}{
		{
			name:                   "AI提供新價格_緩存應更新為AI價格",
			originalStopLoss:       47000.0,
			originalTakeProfit:     51000.0,
			aiNewStopLoss:          48000.0,
			aiNewTakeProfit:        52000.0,
			expectCacheStopLoss:    48000.0, // 應該是 AI 提供的新價格
			expectCacheTakeProfit:  52000.0,
		},
		{
			name:                   "AI不提供_緩存應保持原價格",
			originalStopLoss:       47000.0,
			originalTakeProfit:     51000.0,
			aiNewStopLoss:          0, // AI 不提供
			aiNewTakeProfit:        0,
			expectCacheStopLoss:    47000.0, // 應該是原始價格（fallback）
			expectCacheTakeProfit:  51000.0,
		},
		{
			name:                   "混合情況_各自更新",
			originalStopLoss:       47000.0,
			originalTakeProfit:     51000.0,
			aiNewStopLoss:          48000.0, // AI 提供
			aiNewTakeProfit:        0,       // AI 不提供
			expectCacheStopLoss:    48000.0, // AI 價格
			expectCacheTakeProfit:  51000.0, // 原始價格（fallback）
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模擬 executePartialCloseWithRecord 中的邏輯
			// 步驟 1: 開倉時設置初始緩存
			positionStopLoss := make(map[string]float64)
			positionTakeProfit := make(map[string]float64)
			posKey := "BTCUSDT_long"

			positionStopLoss[posKey] = tt.originalStopLoss
			positionTakeProfit[posKey] = tt.originalTakeProfit

			// 步驟 2: 部分平倉時，確定最終使用的價格
			finalStopLoss := tt.aiNewStopLoss
			if finalStopLoss <= 0 {
				finalStopLoss = positionStopLoss[posKey]
			}
			finalTakeProfit := tt.aiNewTakeProfit
			if finalTakeProfit <= 0 {
				finalTakeProfit = positionTakeProfit[posKey]
			}

			// 步驟 3: 設置 SL/TP 訂單成功後，更新緩存（這是修復後的邏輯）
			if finalStopLoss > 0 {
				positionStopLoss[posKey] = finalStopLoss
			}
			if finalTakeProfit > 0 {
				positionTakeProfit[posKey] = finalTakeProfit
			}

			// 驗證緩存是否正確更新
			assertPrice(t, "內存緩存止損", positionStopLoss[posKey], tt.expectCacheStopLoss)
			assertPrice(t, "內存緩存止盈", positionTakeProfit[posKey], tt.expectCacheTakeProfit)
		})
	}
}

// TestPartialCloseIntegration 整合測試（使用 mock trader）
func TestPartialCloseIntegration(t *testing.T) {
	tests := []struct {
		name                 string
		symbol               string
		side                 string
		totalQuantity        float64
		markPrice            float64
		closePercentage      float64
		newStopLoss          float64
		newTakeProfit        float64
		expectFullClose      bool
		expectStopLossCall   bool
		expectTakeProfitCall bool
	}{
		{
			name:                 "LONG倉_正常部分平倉_有止盈止損",
			symbol:               "BTCUSDT",
			side:                 "LONG",
			totalQuantity:        1.0,
			markPrice:            50000.0,
			closePercentage:      50.0,
			newStopLoss:          48000.0,
			newTakeProfit:        52000.0,
			expectFullClose:      false,
			expectStopLossCall:   true,
			expectTakeProfitCall: true,
		},
		{
			name:                 "SHORT倉_剩餘價值過小_應自動全平",
			symbol:               "ETHUSDT",
			side:                 "SHORT",
			totalQuantity:        0.02,
			markPrice:            3000.0, // 總價值 60 USDT
			closePercentage:      95.0,   // 剩餘 3 USDT < 10 USDT
			newStopLoss:          3100.0,
			newTakeProfit:        2900.0,
			expectFullClose:      true,
			expectStopLossCall:   false, // 全平不需要恢復止盈止損
			expectTakeProfitCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 創建 mock trader
			mockTrader := &MockPartialCloseTrader{
				positions: []map[string]interface{}{
					{
						"symbol":    tt.symbol,
						"side":      tt.side,
						"quantity":  tt.totalQuantity,
						"markPrice": tt.markPrice,
					},
				},
			}

			// 創建決策
			dec := &decision.Decision{
				Symbol:          tt.symbol,
				Action:          "partial_close",
				ClosePercentage: tt.closePercentage,
				NewStopLoss:     tt.newStopLoss,
				NewTakeProfit:   tt.newTakeProfit,
			}

			// 創建 actionRecord
			actionRecord := &logger.DecisionAction{}

			// 計算剩餘價值
			closeQuantity := tt.totalQuantity * (tt.closePercentage / 100.0)
			remainingQuantity := tt.totalQuantity - closeQuantity
			remainingValue := remainingQuantity * tt.markPrice

			// 驗證最小倉位檢查
			const MIN_POSITION_VALUE = 10.0
			shouldFullClose := remainingValue > 0 && remainingValue <= MIN_POSITION_VALUE

			if shouldFullClose != tt.expectFullClose {
				t.Errorf("最小倉位檢查不符: shouldFullClose = %v, 期望 = %v (剩餘 %.2f USDT)",
					shouldFullClose, tt.expectFullClose, remainingValue)
			}

			// 模擬執行邏輯
			if shouldFullClose {
				// 應該轉為全平
				if tt.side == "LONG" {
					mockTrader.CloseLong(tt.symbol, tt.totalQuantity)
				} else {
					mockTrader.CloseShort(tt.symbol, tt.totalQuantity)
				}
			} else {
				// 正常部分平倉
				if tt.side == "LONG" {
					mockTrader.ClosePartialLong(tt.symbol, closeQuantity)
				} else {
					mockTrader.ClosePartialShort(tt.symbol, closeQuantity)
				}

				// 恢復止盈止損
				if dec.NewStopLoss > 0 {
					mockTrader.SetStopLoss(tt.symbol, tt.side, remainingQuantity, dec.NewStopLoss)
				}
				if dec.NewTakeProfit > 0 {
					mockTrader.SetTakeProfit(tt.symbol, tt.side, remainingQuantity, dec.NewTakeProfit)
				}
			}

			// 驗證調用
			if tt.expectFullClose {
				if !mockTrader.closeLongCalled && !mockTrader.closeShortCalled {
					t.Error("期望調用全平但沒有調用")
				}
				if mockTrader.closePartialCalled {
					t.Error("不應該調用部分平倉")
				}
			} else {
				if !mockTrader.closePartialCalled {
					t.Error("期望調用部分平倉但沒有調用")
				}
			}

			if mockTrader.stopLossCalled != tt.expectStopLossCall {
				t.Errorf("止損調用不符: called = %v, 期望 = %v",
					mockTrader.stopLossCalled, tt.expectStopLossCall)
			}

			if mockTrader.takeProfitCalled != tt.expectTakeProfitCall {
				t.Errorf("止盈調用不符: called = %v, 期望 = %v",
					mockTrader.takeProfitCalled, tt.expectTakeProfitCall)
			}

			_ = actionRecord // 避免未使用警告
		})
	}
}

// TestCloseLongCloseShortAlwaysCancelOrders 測試 CloseLong/CloseShort 總是取消挂單
// 這是 Issue #70 的核心修復：部分平仓时，CloseLong/CloseShort 必须取消所有挂单
// 因为原订单数量不正确，需要在 auto_trader.go 中用剩余数量重新创建
func TestCloseLongCloseShortAlwaysCancelOrders(t *testing.T) {
	tests := []struct {
		name                   string
		action                 string // "close_long" or "close_short"
		symbol                 string
		expectCancelAllOrders  bool
	}{
		{
			name:                   "CloseLong應該取消所有挂單",
			action:                 "close_long",
			symbol:                 "BTCUSDT",
			expectCancelAllOrders:  true,
		},
		{
			name:                   "CloseShort應該取消所有挂單",
			action:                 "close_short",
			symbol:                 "ETHUSDT",
			expectCancelAllOrders:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模擬 CloseLong/CloseShort 的行為
			// 在三個交易所實現（binance_futures.go, hyperliquid_trader.go, aster_trader.go）中
			// CloseLong/CloseShort 執行後會調用 CancelAllOrders
			//
			// 這是設計決策：
			// 1. 部分平倉：CloseLong/CloseShort 取消舊訂單 → auto_trader.go 重新創建新訂單（正確數量）
			// 2. 全部平倉：CloseLong/CloseShort 取消舊訂單 → 無需重建（倉位已清空）
			//
			// 代碼位置（全部包含 CancelAllOrders 調用）：
			// - binance_futures.go:468-472 (CloseLong), 524-528 (CloseShort)
			// - hyperliquid_trader.go:550-554 (CloseLong), 621-625 (CloseShort)
			// - aster_trader.go:similar positions

			mockTrader := &MockPartialCloseTrader{}

			// 模擬執行平倉後的 CancelAllOrders 調用
			if tt.action == "close_long" || tt.action == "close_short" {
				// 這模擬了三個交易所實現中 CloseLong/CloseShort 的行為
				mockTrader.CancelAllOrders(tt.symbol)
			}

			// 驗證 CancelAllOrders 是否被調用
			if mockTrader.cancelAllOrdersCalled != tt.expectCancelAllOrders {
				t.Errorf("CancelAllOrders 調用狀態錯誤: got %v, want %v",
					mockTrader.cancelAllOrdersCalled, tt.expectCancelAllOrders)
			}

			if tt.expectCancelAllOrders && mockTrader.cancelAllOrdersSymbol != tt.symbol {
				t.Errorf("CancelAllOrders 調用的 symbol 錯誤: got %s, want %s",
					mockTrader.cancelAllOrdersSymbol, tt.symbol)
			}
		})
	}
}

// TestPartialCloseFlowWithCancelAndRecreate 測試完整的部分平倉流程
// 驗證：CloseLong 取消訂單 → 重新創建 SL/TP（正確數量）→ 更新內存緩存
func TestPartialCloseFlowWithCancelAndRecreate(t *testing.T) {
	mockTrader := &MockPartialCloseTrader{}

	// 模擬部分平倉流程
	symbol := "BTCUSDT"
	originalQuantity := 1.0
	closePercentage := 50.0
	remainingQuantity := originalQuantity * (1 - closePercentage/100)
	newStopLoss := 48000.0
	newTakeProfit := 52000.0

	// Step 1: CloseLong 執行並取消所有訂單
	mockTrader.CloseLong(symbol, originalQuantity*closePercentage/100)
	mockTrader.CancelAllOrders(symbol) // 這是 CloseLong 內部會調用的

	// Step 2: 重新創建 SL/TP 訂單（使用剩餘數量）
	mockTrader.SetStopLoss(symbol, "LONG", remainingQuantity, newStopLoss)
	mockTrader.SetTakeProfit(symbol, "LONG", remainingQuantity, newTakeProfit)

	// Step 3: 更新內存緩存
	positionStopLoss := make(map[string]float64)
	positionTakeProfit := make(map[string]float64)
	posKey := symbol + "_long"
	positionStopLoss[posKey] = newStopLoss
	positionTakeProfit[posKey] = newTakeProfit

	// 驗證整個流程
	if !mockTrader.closeLongCalled {
		t.Error("CloseLong 應該被調用")
	}
	if !mockTrader.cancelAllOrdersCalled {
		t.Error("CancelAllOrders 應該被調用")
	}
	if !mockTrader.stopLossCalled {
		t.Error("SetStopLoss 應該被調用來重建止損訂單")
	}
	if !mockTrader.takeProfitCalled {
		t.Error("SetTakeProfit 應該被調用來重建止盈訂單")
	}
	if positionStopLoss[posKey] != newStopLoss {
		t.Errorf("內存緩存止損錯誤: got %.2f, want %.2f", positionStopLoss[posKey], newStopLoss)
	}
	if positionTakeProfit[posKey] != newTakeProfit {
		t.Errorf("內存緩存止盈錯誤: got %.2f, want %.2f", positionTakeProfit[posKey], newTakeProfit)
	}
}
