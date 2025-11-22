interface IconProps {
  width?: number
  height?: number
  className?: string
}

// AI模型品牌颜色映射
const MODEL_COLORS: Record<string, string> = {
  deepseek: '#60a5fa', // 蓝色
  qwen: '#c084fc', // 紫色
  openai: '#10a37f', // OpenAI 绿色
  gemini: '#4285f4', // Google 蓝色
  groq: '#f55036', // Groq 红色
}

// 获取模型品牌颜色
export const getModelColor = (modelType: string): string => {
  const type = modelType.includes('_') ? modelType.split('_').pop() : modelType
  return MODEL_COLORS[type || ''] || '#848E9C' // 默认灰色
}

// 获取AI模型图标的函数
export const getModelIcon = (modelType: string, props: IconProps = {}) => {
  // 支持完整ID或类型名
  const type = modelType.includes('_') ? modelType.split('_').pop() : modelType

  let iconPath: string | null = null

  switch (type) {
    case 'deepseek':
      iconPath = '/icons/deepseek.svg'
      break
    case 'qwen':
      iconPath = '/icons/qwen.svg'
      break
    case 'openai':
      iconPath = '/icons/openai.svg'
      break
    case 'gemini':
      iconPath = '/icons/gemini.svg'
      break
    case 'groq':
      iconPath = '/icons/groq.svg'
      break
    default:
      return null
  }

  return (
    <img
      src={iconPath}
      alt={`${type} icon`}
      width={props.width || 24}
      height={props.height || 24}
      className={props.className}
      style={{ borderRadius: '50%' }}
    />
  )
}
