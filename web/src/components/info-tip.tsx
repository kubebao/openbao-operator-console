import { useId } from 'react'
import type { ReactNode } from 'react'
import { Info } from 'lucide-react'

// Secondary explanations stay one hover or focus away instead of filling the page.
// The tooltip stays open while hovered and closes with Escape.
export function InfoTip({ label, children }: { label: string; children: ReactNode }) {
  const id = useId()
  return (
    <span className="info-tip">
      <button
        type="button"
        aria-label={label}
        aria-describedby={id}
        onKeyDown={(event) => {
          if (event.key === 'Escape') event.currentTarget.blur()
        }}
      >
        <Info size={14} />
      </button>
      <span role="tooltip" id={id}>
        {children}
      </span>
    </span>
  )
}

export function SectionTitle({
  icon,
  tip,
  tipLabel,
  children,
}: {
  icon?: ReactNode
  tip?: ReactNode
  tipLabel?: string
  children: ReactNode
}) {
  return (
    <div className="section-title">
      <h2>
        {icon}
        {children}
      </h2>
      {tip && <InfoTip label={tipLabel ?? 'More information'}>{tip}</InfoTip>}
    </div>
  )
}
