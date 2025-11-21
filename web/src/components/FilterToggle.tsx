import { Activity } from 'lucide-react'
import { type Language } from '../i18n/translations'
import { t } from '../i18n/translations'

interface FilterToggleProps {
  // 当前是否启用过滤
  enabled: boolean

  // 过滤状态变化时的回调函数
  onChange: (enabled: boolean) => void

  // 语言
  language: Language

  // 可选：自定义样式类名
  className?: string
}

function FilterToggle({
  enabled,
  onChange,
  language,
  className = '',
}: FilterToggleProps) {
  return (
    <button
      onClick={() => onChange(!enabled)}
      className={`flex items-center gap-2 px-3 py-1.5 rounded text-xs font-medium cursor-pointer transition-all ${className}`}
      style={{
        background: enabled ? '#F0B90B' : '#1E2329',
        border: `1px solid ${enabled ? '#F0B90B' : '#2B3139'}`,
        color: enabled ? '#000000' : '#848E9C',
      }}
      title={t('filterOnlyWithActions', language)}
    >
      <Activity className="w-3.5 h-3.5" />
      <span>{t('filterOnlyWithActions', language)}</span>
    </button>
  )
}

export default FilterToggle
