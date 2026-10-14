import { useEffect, useState } from 'react'

export type Theme = 'light' | 'dark'

const themeKey = 'console-theme'
const darkQuery = '(prefers-color-scheme: dark)'

function storedTheme(): Theme | null {
  try {
    const stored = localStorage.getItem(themeKey)
    return stored === 'light' || stored === 'dark' ? stored : null
  } catch {
    return null
  }
}

function systemTheme(): Theme {
  return window.matchMedia(darkQuery).matches ? 'dark' : 'light'
}

// The system preference applies until the user chooses a theme explicitly. public/theme.js
// applies the same theme before the first paint; keep the two in sync.
export function useTheme(): [Theme, (theme: Theme) => void] {
  const [preference, setPreference] = useState(storedTheme)
  const [system, setSystem] = useState(systemTheme)
  useEffect(() => {
    const media = window.matchMedia(darkQuery)
    const listener = () => {
      setSystem(systemTheme())
    }
    media.addEventListener('change', listener)
    return () => {
      media.removeEventListener('change', listener)
    }
  }, [])
  const theme = preference ?? system
  useEffect(() => {
    document.documentElement.dataset['theme'] = theme
  }, [theme])
  return [
    theme,
    (next) => {
      setPreference(next)
      try {
        localStorage.setItem(themeKey, next)
      } catch {
        // The choice still applies for this page view.
      }
    },
  ]
}
