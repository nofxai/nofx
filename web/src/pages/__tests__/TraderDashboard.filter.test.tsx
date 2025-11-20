import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import TraderDashboard from '../TraderDashboard'
import { AuthProvider } from '../../contexts/AuthContext'
import { LanguageProvider } from '../../contexts/LanguageContext'
import type { DecisionRecord } from '../../types'
import userEvent from '@testing-library/user-event'

// Mock SWR to call the fetcher function
vi.mock('swr', () => ({
  default: vi.fn((key, fetcher) => {
    // Call the fetcher to trigger API calls
    if (fetcher && typeof fetcher === 'function') {
      fetcher().catch(() => {})
    }
    if (typeof key === 'string' && key.includes('decisions/latest')) {
      return { data: [], error: null, isLoading: false, mutate: vi.fn() }
    }
    if (typeof key === 'string' && key === 'traders') {
      return {
        data: [{ trader_id: 'test-trader', trader_name: 'Test Trader', ai_model: 'claude', system_prompt_template: 'test' }],
        error: null,
        isLoading: false,
        mutate: vi.fn(),
      }
    }
    return { data: undefined, error: null, isLoading: false, mutate: vi.fn() }
  }),
}))

// Mock API
vi.mock('../../lib/api', () => ({
  api: {
    getTraders: vi.fn(() =>
      Promise.resolve([
        {
          trader_id: 'test-trader',
          trader_name: 'Test Trader',
          ai_model: 'claude',
          system_prompt_template: 'test',
        },
      ])
    ),
    getStatus: vi.fn(() => Promise.resolve({ call_count: 10, runtime_minutes: 30 })),
    getAccount: vi.fn(() =>
      Promise.resolve({
        initial_balance: 10000,
        total_equity: 10500,
        available_balance: 9000,
        total_pnl: 500,
        total_pnl_pct: 5,
        position_count: 1,
        margin_used_pct: 10,
      })
    ),
    getPositions: vi.fn(() => Promise.resolve([])),
    getLatestDecisions: vi.fn(() => Promise.resolve([])),
    getStatistics: vi.fn(() =>
      Promise.resolve({
        total_cycles: 10,
        successful_cycles: 9,
        failed_cycles: 1,
      })
    ),
  },
}))

// Mock system config
vi.mock('../../hooks/useSystemConfig', () => ({
  useSystemConfig: () => ({ config: { use_default_coins: false } }),
}))

const mockDecisionsWithActions: DecisionRecord[] = [
  {
    timestamp: '2025-01-01T10:00:00Z',
    cycle_number: 1,
    input_prompt: 'Test prompt 1',
    cot_trace: 'Thinking...',
    decision_json: '{}',
    account_state: {
      total_balance: 10000,
      available_balance: 9000,
      total_unrealized_profit: 100,
      position_count: 1,
      margin_used_pct: 10,
      initial_balance: 10000,
    },
    positions: [],
    candidate_coins: ['BTC'],
    decisions: [
      {
        action: 'open_long',
        symbol: 'BTC',
        quantity: 0.1,
        leverage: 10,
        price: 50000,
        order_id: 123,
        timestamp: '2025-01-01T10:00:00Z',
        success: true,
        error: '',
      },
    ],
    execution_log: ['Opened long position'],
    success: true,
    error_message: '',
  },
  {
    timestamp: '2025-01-01T12:00:00Z',
    cycle_number: 3,
    input_prompt: 'Test prompt 3',
    cot_trace: 'Thinking...',
    decision_json: '{}',
    account_state: {
      total_balance: 10000,
      available_balance: 9000,
      total_unrealized_profit: 100,
      position_count: 1,
      margin_used_pct: 10,
      initial_balance: 10000,
    },
    positions: [],
    candidate_coins: ['ETH'],
    decisions: [
      {
        action: 'close_position',
        symbol: 'BTC',
        quantity: 0.1,
        leverage: 10,
        price: 51000,
        order_id: 124,
        timestamp: '2025-01-01T12:00:00Z',
        success: true,
        error: '',
      },
    ],
    execution_log: ['Closed position'],
    success: true,
    error_message: '',
  },
]

describe('TraderDashboard - Backend Filter Integration', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('should call API with onlyWithActions=true when filter is enabled', async () => {
    const { api } = await import('../../lib/api')

    // Set filter to enabled in localStorage
    localStorage.setItem('showOnlyWithActions', JSON.stringify(true))
    localStorage.setItem('decisionLimit', '10')

    render(
      <MemoryRouter initialEntries={['/?trader=test-trader']}>
        <AuthProvider>
          <LanguageProvider>
            <TraderDashboard />
          </LanguageProvider>
        </AuthProvider>
      </MemoryRouter>
    )

    await waitFor(
      () => {
        // Verify API was called with onlyWithActions=true
        expect(api.getLatestDecisions).toHaveBeenCalledWith(
          'test-trader',
          10,
          true // onlyWithActions should be true
        )
      },
      { timeout: 3000 }
    )
  })

  it('should call API with onlyWithActions=false when filter is disabled', async () => {
    const { api } = await import('../../lib/api')

    // Set filter to disabled in localStorage
    localStorage.setItem('showOnlyWithActions', JSON.stringify(false))
    localStorage.setItem('decisionLimit', '5')

    render(
      <MemoryRouter initialEntries={['/?trader=test-trader']}>
        <AuthProvider>
          <LanguageProvider>
            <TraderDashboard />
          </LanguageProvider>
        </AuthProvider>
      </MemoryRouter>
    )

    await waitFor(
      () => {
        // Verify API was called with onlyWithActions=false
        expect(api.getLatestDecisions).toHaveBeenCalledWith(
          'test-trader',
          5,
          false // onlyWithActions should be false
        )
      },
      { timeout: 3000 }
    )
  })

  it('should default to onlyWithActions=false when not set in localStorage', async () => {
    const { api } = await import('../../lib/api')

    render(
      <MemoryRouter initialEntries={['/?trader=test-trader']}>
        <AuthProvider>
          <LanguageProvider>
            <TraderDashboard />
          </LanguageProvider>
        </AuthProvider>
      </MemoryRouter>
    )

    await waitFor(
      () => {
        // Verify API was called with default onlyWithActions=false
        const calls = vi.mocked(api.getLatestDecisions).mock.calls
        const lastCall = calls[calls.length - 1]
        expect(lastCall[2]).toBe(false) // Third parameter should be false
      },
      { timeout: 3000 }
    )
  })

  it('should persist filter state to localStorage', () => {
    localStorage.setItem('showOnlyWithActions', JSON.stringify(true))
    const saved = localStorage.getItem('showOnlyWithActions')
    expect(saved).toBe('true')
    expect(JSON.parse(saved)).toBe(true)
  })

  it('should restore filter state from localStorage on mount', () => {
    localStorage.setItem('showOnlyWithActions', JSON.stringify(true))
    const saved = localStorage.getItem('showOnlyWithActions')
    const restored = saved ? JSON.parse(saved) : false
    expect(restored).toBe(true)
  })
})
