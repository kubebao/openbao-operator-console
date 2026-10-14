import { useEffect } from 'react'

const product = 'OpenBao Operator Console'

// useDocumentTitle names the browser tab and history entry after the current view.
export function useDocumentTitle(title: string) {
  useEffect(() => {
    document.title = title ? `${title} · ${product}` : product
  }, [title])
}
