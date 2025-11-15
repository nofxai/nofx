package backtest

import (
	"strings"
	"testing"
)

func TestBacktestAccount_RiskLimits(t *testing.T) {
	t.Run("should reject leverage exceeding maximum", func(t *testing.T) {
		acc := NewBacktestAccount(10000, 5, 2) // 10000 USDT, 5bps fee, 2bps slippage

		_, _, _, err := acc.Open("BTCUSDT", "long", 0.1, 150, 50000, 0)

		if err == nil {
			t.Fatal("Expected error for leverage > 100, got nil")
		}
		if !strings.Contains(err.Error(), "exceeds maximum allowed leverage") {
			t.Errorf("Expected leverage limit error, got: %v", err)
		}
	})

	t.Run("should reject when max positions reached", func(t *testing.T) {
		acc := NewBacktestAccount(100000, 5, 2)

		// Open 20 positions (the maximum)
		for i := 1; i <= 20; i++ {
			symbol := "COIN" + string(rune('A'+i-1)) + "USDT"
			_, _, _, err := acc.Open(symbol, "long", 0.1, 10, 100, 0)
			if err != nil {
				t.Fatalf("Failed to open position %d: %v", i, err)
			}
		}

		// Try to open 21st position
		_, _, _, err := acc.Open("NEWCOINUSDT", "long", 0.1, 10, 100, 0)

		if err == nil {
			t.Fatal("Expected error for exceeding max positions, got nil")
		}
		if !strings.Contains(err.Error(), "maximum position count") {
			t.Errorf("Expected max positions error, got: %v", err)
		}
	})

	t.Run("should reject excessive notional value", func(t *testing.T) {
		acc := NewBacktestAccount(1000, 5, 2) // Small account: 1000 USDT

		// Try to open a position with notional value > 50x equity (50,000 USDT)
		// With 10x leverage, this would need quantity = 5.0 BTC at 50000 USDT
		_, _, _, err := acc.Open("BTCUSDT", "long", 1.1, 10, 50000, 0)

		if err == nil {
			t.Fatal("Expected error for excessive notional value, got nil")
		}
		if !strings.Contains(err.Error(), "exceeds maximum allowed") {
			t.Errorf("Expected notional limit error, got: %v", err)
		}
	})

	t.Run("should allow valid positions within limits", func(t *testing.T) {
		acc := NewBacktestAccount(10000, 5, 2)

		// Open a reasonable position
		pos, _, _, err := acc.Open("BTCUSDT", "long", 0.1, 20, 50000, 0)

		if err != nil {
			t.Fatalf("Expected successful open, got error: %v", err)
		}
		if pos == nil {
			t.Fatal("Expected position to be created")
		}
		if pos.Symbol != "BTCUSDT" {
			t.Errorf("Expected symbol BTCUSDT, got %s", pos.Symbol)
		}
		if pos.Leverage != 20 {
			t.Errorf("Expected leverage 20, got %d", pos.Leverage)
		}
	})
}

func TestBacktestAccount_BasicOperations(t *testing.T) {
	t.Run("should open and close position correctly", func(t *testing.T) {
		acc := NewBacktestAccount(10000, 5, 2)

		// Open long position
		pos, _, _, err := acc.Open("ETHUSDT", "long", 1.0, 10, 3000, 0)
		if err != nil {
			t.Fatalf("Failed to open position: %v", err)
		}

		initialCash := acc.cash

		// Close position at profit (symbol, side, quantity, price)
		realizedPnL, _, _, err := acc.Close("ETHUSDT", "long", 1.0, 3300)
		if err != nil {
			t.Fatalf("Failed to close position: %v", err)
		}

		if realizedPnL <= 0 {
			t.Errorf("Expected positive PnL, got %f", realizedPnL)
		}

		// Cash should increase after profitable close
		if acc.cash <= initialCash {
			t.Errorf("Expected cash to increase after profit, initial=%f, final=%f", initialCash, acc.cash)
		}

		// Position should be removed
		if pos.Quantity > 0 {
			t.Error("Position should be fully closed")
		}
	})

	t.Run("should calculate equity correctly", func(t *testing.T) {
		acc := NewBacktestAccount(10000, 5, 2)

		// Initial equity should equal initial balance
		prices := make(map[string]float64)
		equity, _, _ := acc.TotalEquity(prices)
		if equity != 10000 {
			t.Errorf("Expected initial equity 10000, got %f", equity)
		}

		// Open position
		acc.Open("BTCUSDT", "long", 0.1, 10, 50000, 0)

		// Equity should change with position value
		prices["BTCUSDT"] = 51000
		equity, _, _ = acc.TotalEquity(prices)

		// Equity = cash + unrealized PnL
		// Should be greater than initial if price went up
		if equity <= 10000 {
			t.Errorf("Expected equity to increase with profitable position, got %f", equity)
		}
	})
}
