import { type Language } from '../i18n/translations'

interface RecordLimitSelectorProps {
  // 当前选中的条数
  limit: number

  // 条数变化时的回调函数
  onLimitChange: (newLimit: number) => void

  // 可选的条数选项（默认 [5, 10, 20, 50]）
  options?: number[]

  // 语言（用于显示 "显示"/"Show" 和 "条"）
  language: Language

  // 可选：自定义样式类名
  className?: string
}

export default function RecordLimitSelector({
  limit,
  onLimitChange,
  options = [5, 10, 20, 50],
  language,
  className = '',
}: RecordLimitSelectorProps) {
  return (
    <div className={`flex items-center gap-2 ${className}`}>
      <span className="text-xs" style={{ color: '#848E9C' }}>
        {language === 'zh' ? '显示' : 'Show'}:
      </span>
      <select
        value={limit}
        onChange={(e) => onLimitChange(parseInt(e.target.value, 10))}
        className="rounded px-2 py-1 text-xs font-medium cursor-pointer transition-colors"
        style={{
          background: '#1E2329',
          border: '1px solid #2B3139',
          color: '#EAECEF',
        }}
      >
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
      <span className="text-xs" style={{ color: '#848E9C' }}>
        {language === 'zh' ? '条' : ''}
      </span>
    </div>
  )
}
