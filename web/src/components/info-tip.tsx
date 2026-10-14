import { useEffect, useId, useState } from 'react'
import type { ReactNode } from 'react'
import { Info } from 'lucide-react'

// Secondary explanations stay one hover or focus away instead of filling the page.
// The tooltip stays open while hovered or focused and closes with Escape, also when it was
// opened by hover and focus is elsewhere (WCAG 1.4.13).
export function InfoTip({ label, children }: { label: string; children: ReactNode }) {
  const id = useId()
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  const [dismissed, setDismissed] = useState(false)
  const active = hovered || focused
  useEffect(() => {
    if (!active) return
    const listener = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setDismissed(true)
    }
    document.addEventListener('keydown', listener)
    return () => {
      document.removeEventListener('keydown', listener)
    }
  }, [active])
  // Showing the tip again after leaving it clears an earlier dismissal.
  const track = (set: (value: boolean) => void, value: boolean) => () => {
    set(value)
    if (value) setDismissed(false)
  }
  return (
    <span
      className={dismissed ? 'info-tip info-tip-dismissed' : 'info-tip'}
      onMouseEnter={track(setHovered, true)}
      onMouseLeave={track(setHovered, false)}
      onFocus={track(setFocused, true)}
      onBlur={track(setFocused, false)}
    >
      <button type="button" aria-label={label} aria-describedby={id}>
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
