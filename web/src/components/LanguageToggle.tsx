import { useI18n, type AppLocale } from '../i18n'

type LanguageToggleProps = {
  compact?: boolean
}

const LOCALES: Array<{ value: AppLocale; label: string; ariaLabel: { zh: string; en: string } }> = [
  { value: 'en', label: 'EN', ariaLabel: { zh: '切换到英文', en: 'Switch to English' } },
  { value: 'zh-CN', label: '中文', ariaLabel: { zh: '切换到中文', en: 'Switch to Chinese' } },
]

export default function LanguageToggle({ compact = false }: LanguageToggleProps) {
  const { locale, setLocale, tx } = useI18n()

  return (
    <div
      className={`language-toggle ${compact ? 'language-toggle-compact' : ''}`}
      role="radiogroup"
      aria-label={tx('语言', 'Language')}
    >
      {LOCALES.map((entry) => (
        <button
          key={entry.value}
          type="button"
          role="radio"
          aria-checked={locale === entry.value}
          aria-label={tx(entry.ariaLabel.zh, entry.ariaLabel.en)}
          className={`language-toggle-btn ${locale === entry.value ? 'language-toggle-btn-active' : ''}`}
          onClick={() => setLocale(entry.value)}
        >
          {entry.label}
        </button>
      ))}
    </div>
  )
}
