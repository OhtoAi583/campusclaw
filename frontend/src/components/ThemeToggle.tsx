import { useEffect, useState } from 'react'

type Theme = 'light' | 'dark'

function initialTheme(): Theme {
  const stored = window.localStorage.getItem('campusclaw-theme')
  if (stored === 'light' || stored === 'dark') return stored
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(initialTheme)

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    window.localStorage.setItem('campusclaw-theme', theme)
  }, [theme])

  return (
    <button
      type="button"
      className="icon-button"
      onClick={() => setTheme((current) => (current === 'dark' ? 'light' : 'dark'))}
      aria-label="切换深浅色主题"
      title="切换深浅色主题"
    >
      {theme === 'dark' ? '☀︎' : '☾'}
    </button>
  )
}
