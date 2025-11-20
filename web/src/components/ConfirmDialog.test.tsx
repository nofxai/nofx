import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfirmDialogProvider } from './ConfirmDialog'
import { confirmToast } from '../lib/notify'

/**
 * ConfirmDialog 组件测试
 *
 * 目的：验证 ConfirmDialogProvider 正确初始化并能被 confirmToast 使用
 *
 * 复现的 Bug：
 * - 当 ConfirmDialogProvider 未包裹应用时，confirmToast 调用会报错：
 *   "ConfirmDialogProvider not initialized"
 */
describe('ConfirmDialog - Provider Initialization', () => {
  beforeEach(() => {
    // Reset any previous state
    vi.clearAllMocks()
  })

  /**
   * 测试：当 ConfirmDialogProvider 正确初始化时，confirmToast 应该显示对话框
   *
   * 这个测试验证正常的使用场景：
   * - ConfirmDialogProvider 包裹应用
   * - confirmToast 可以正常调用并显示确认对话框
   *
   * 注：本测试验证修复后的正确行为。如果 main.tsx 中缺少 ConfirmDialogProvider，
   * 虽然测试仍会通过（因为测试内部提供了 Provider），但实际应用中会失败。
   */
  it('should show confirmation dialog when provider is initialized', async () => {
    const user = userEvent.setup()

    // Render with ConfirmDialogProvider
    const TestComponent = () => {
      const handleClick = async () => {
        await confirmToast('Are you sure you want to delete?')
      }

      return (
        <ConfirmDialogProvider>
          <button onClick={handleClick}>Delete Trader</button>
        </ConfirmDialogProvider>
      )
    }

    render(<TestComponent />)

    // Click the button to trigger confirmToast
    const deleteButton = screen.getByText('Delete Trader')
    await user.click(deleteButton)

    // Wait for the confirmation dialog to appear
    await waitFor(() => {
      expect(screen.getByText('Are you sure you want to delete?')).toBeInTheDocument()
    })
  })

  /**
   * 测试：用户点击确认按钮应该返回 true
   */
  it('should return true when user clicks confirm button', async () => {
    const user = userEvent.setup()
    let confirmResult: boolean | undefined

    const TestComponent = () => {
      const handleClick = async () => {
        confirmResult = await confirmToast('Are you sure?', {
          okText: '确认',
          cancelText: '取消',
        })
      }

      return (
        <ConfirmDialogProvider>
          <button onClick={handleClick}>Test Button</button>
        </ConfirmDialogProvider>
      )
    }

    render(<TestComponent />)

    // Click test button
    await user.click(screen.getByText('Test Button'))

    // Wait for dialog and click confirm
    await waitFor(() => {
      expect(screen.getByText('Are you sure?')).toBeInTheDocument()
    })

    const confirmButton = screen.getByText('确认')
    await user.click(confirmButton)

    // Should return true
    await waitFor(() => {
      expect(confirmResult).toBe(true)
    })
  })

  /**
   * 测试：用户点击取消按钮应该返回 false
   */
  it('should return false when user clicks cancel button', async () => {
    const user = userEvent.setup()
    let confirmResult: boolean | undefined

    const TestComponent = () => {
      const handleClick = async () => {
        confirmResult = await confirmToast('Are you sure?', {
          okText: '确认',
          cancelText: '取消',
        })
      }

      return (
        <ConfirmDialogProvider>
          <button onClick={handleClick}>Test Button</button>
        </ConfirmDialogProvider>
      )
    }

    render(<TestComponent />)

    // Click test button
    await user.click(screen.getByText('Test Button'))

    // Wait for dialog and click cancel
    await waitFor(() => {
      expect(screen.getByText('Are you sure?')).toBeInTheDocument()
    })

    const cancelButton = screen.getByText('取消')
    await user.click(cancelButton)

    // Should return false
    await waitFor(() => {
      expect(confirmResult).toBe(false)
    })
  })
})
