import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { Button } from './ui/button'

export function CodeBlock({
  code,
  label,
  onCopyError,
}: {
  code: string
  label: string
  onCopyError: (message: string) => void
}) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="code-block">
      <div className="code-block-header">
        <span>{label}</span>
        <Button
          type="button"
          size="small"
          variant="ghost"
          aria-label={`Copy ${label.toLocaleLowerCase()}`}
          onClick={() => {
            void navigator.clipboard
              .writeText(code)
              .then(() => {
                setCopied(true)
                window.setTimeout(() => {
                  setCopied(false)
                }, 2000)
              })
              .catch(() => {
                onCopyError('Copy failed. Select the text in the block instead.')
              })
          }}
        >
          {copied ? <Check size={14} /> : <Copy size={14} />}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>
      <pre tabIndex={0} role="region" aria-label={label}>
        <code>{code}</code>
      </pre>
    </div>
  )
}
