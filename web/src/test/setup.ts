import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// jsdom does not implement scrolling, which the router calls after navigation, or media
// queries, which the theme follows. Tests use the light theme.
window.scrollTo = () => undefined
window.matchMedia = (query: string) => {
  const media = new EventTarget()
  return Object.assign(media, {
    matches: false,
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
  })
}

afterEach(() => {
  cleanup()
})
