import { useEffect, useRef } from 'react'
import type { RefObject } from 'react'
import { useRouterState } from '@tanstack/react-router'

// useRouteFocus moves focus to the new page's main heading after navigation, so keyboard
// and screen reader users start at the new content. Search changes, such as filters and
// tabs, keep focus where it is.
export function useRouteFocus(main: RefObject<HTMLElement | null>) {
  // The resolved location changes once the new page has rendered.
  const pathname = useRouterState({ select: (state) => state.resolvedLocation?.pathname })
  const previous = useRef(pathname)
  useEffect(() => {
    const initial = previous.current === undefined
    if (previous.current === pathname) return
    previous.current = pathname
    if (initial) return
    const region = main.current
    if (!region) return
    const target = region.querySelector<HTMLElement>('h1') ?? region
    if (!target.hasAttribute('tabindex')) target.setAttribute('tabindex', '-1')
    target.focus({ preventScroll: true })
  }, [pathname, main])
}
