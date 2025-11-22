import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import FilterToggle from '../FilterToggle'

describe('FilterToggle', () => {
  it('should render with correct label in Chinese', () => {
    const mockOnChange = vi.fn()
    render(
      <FilterToggle
        enabled={false}
        onChange={mockOnChange}
        language="zh"
      />
    )

    expect(screen.getByText('有操作')).toBeInTheDocument()
  })

  it('should render with correct label in English', () => {
    const mockOnChange = vi.fn()
    render(
      <FilterToggle
        enabled={false}
        onChange={mockOnChange}
        language="en"
      />
    )

    expect(screen.getByText('Has Actions')).toBeInTheDocument()
  })

  it('should show enabled state with yellow background', () => {
    const mockOnChange = vi.fn()
    const { container } = render(
      <FilterToggle
        enabled={true}
        onChange={mockOnChange}
        language="zh"
      />
    )

    const button = container.querySelector('button')
    expect(button).toHaveStyle({ background: '#F0B90B' })
  })

  it('should show disabled state with dark background', () => {
    const mockOnChange = vi.fn()
    const { container } = render(
      <FilterToggle
        enabled={false}
        onChange={mockOnChange}
        language="zh"
      />
    )

    const button = container.querySelector('button')
    expect(button).toHaveStyle({ background: '#1E2329' })
  })

  it('should call onChange with inverted value when clicked', () => {
    const mockOnChange = vi.fn()
    render(
      <FilterToggle
        enabled={false}
        onChange={mockOnChange}
        language="zh"
      />
    )

    const button = screen.getByRole('button')
    fireEvent.click(button)

    expect(mockOnChange).toHaveBeenCalledWith(true)
  })

  it('should toggle from enabled to disabled', () => {
    const mockOnChange = vi.fn()
    render(
      <FilterToggle
        enabled={true}
        onChange={mockOnChange}
        language="zh"
      />
    )

    const button = screen.getByRole('button')
    fireEvent.click(button)

    expect(mockOnChange).toHaveBeenCalledWith(false)
  })

  it('should display Filter icon', () => {
    const mockOnChange = vi.fn()
    const { container } = render(
      <FilterToggle
        enabled={false}
        onChange={mockOnChange}
        language="zh"
      />
    )

    const svg = container.querySelector('svg')
    expect(svg).toBeInTheDocument()
  })
})
