import { Fragment } from 'react'
import type { ReactNode } from 'react'
import { ChevronRight } from 'lucide-react'

// Breadcrumbs show where a nested view sits. The last item is the current page.
export function Breadcrumbs({ children }: { children: readonly ReactNode[] }) {
  return (
    <nav className="breadcrumbs" aria-label="Breadcrumb">
      <ol>
        {children.map((item, index) => (
          <Fragment key={index}>
            {index > 0 && (
              <li aria-hidden="true" className="breadcrumb-separator">
                <ChevronRight size={12} />
              </li>
            )}
            <li aria-current={index === children.length - 1 ? 'page' : undefined}>{item}</li>
          </Fragment>
        ))}
      </ol>
    </nav>
  )
}
