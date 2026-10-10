import { Check } from 'lucide-react'
import type { Step } from '../../lib/onboarding'
import { steps } from '../../lib/onboarding'

function stepClass(index: number, current: number): string {
  if (index < current) return 'complete'
  return index === current ? 'current' : ''
}

export function StepProgress({ step }: { step: Step }) {
  const current = steps.indexOf(step)
  return (
    <ol className="onboarding-steps" aria-label="Add cluster steps">
      {steps.map((name, index) => {
        return (
          <li
            key={name}
            aria-current={name === step ? 'step' : undefined}
            className={stepClass(index, current)}
          >
            <span>{index < current ? <Check size={14} /> : index + 1}</span>
            {name}
          </li>
        )
      })}
    </ol>
  )
}
