import { X } from 'lucide-react'
import { t, type Language } from '../../i18n/translations'

interface PromptModalProps {
  prompt: string
  onClose: () => void
  language: Language
}

export function PromptModal({ prompt, onClose, language }: PromptModalProps) {
  return (
    <div
      className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4"
      onClick={onClose}
    >
      <div
        className="bg-gray-800 rounded-lg w-full max-w-4xl relative"
        style={{
          background: '#1E2329',
          maxHeight: 'calc(100vh - 4rem)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div
          className="flex items-center justify-between p-4 border-b"
          style={{ borderColor: '#2B3139' }}
        >
          <h3 className="text-lg font-bold" style={{ color: '#EAECEF' }}>
            {t('currentPrompt', language)}
          </h3>
          <button
            onClick={onClose}
            className="p-1 rounded hover:bg-gray-700 transition-colors"
            style={{ color: '#848E9C' }}
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content */}
        <div
          className="p-4 overflow-y-auto"
          style={{ maxHeight: 'calc(100vh - 12rem)' }}
        >
          <pre
            className="text-sm whitespace-pre-wrap font-mono"
            style={{
              color: '#EAECEF',
              background: '#0B0E11',
              padding: '16px',
              borderRadius: '8px',
              border: '1px solid #2B3139',
            }}
          >
            {prompt}
          </pre>
        </div>

        {/* Footer */}
        <div
          className="flex justify-end p-4 border-t"
          style={{ borderColor: '#2B3139' }}
        >
          <button
            onClick={onClose}
            className="px-4 py-2 rounded font-medium transition-colors"
            style={{
              background: '#2B3139',
              color: '#EAECEF',
            }}
          >
            {t('close', language)}
          </button>
        </div>
      </div>
    </div>
  )
}
