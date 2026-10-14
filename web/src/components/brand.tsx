// Both KubeBao variants remain intact. CSS selects the theme and frames the mark or full logo.
// The artwork is decorative; the product name is text.
export function BrandLogo({ className }: { className: string }) {
  return (
    <span className={className} aria-hidden="true">
      <img
        className="brand-logo-light"
        src="/brand/kubebao-logo.webp"
        alt=""
        width={768}
        height={768}
      />
      <img
        className="brand-logo-dark"
        src="/brand/kubebao-logo-dark.webp"
        alt=""
        width={768}
        height={768}
      />
    </span>
  )
}

export function Brand({ className = 'brand-lockup' }: { className?: string }) {
  return (
    <span className={className}>
      <BrandLogo className="brand-mark" />
      <span className="brand-name">
        OpenBao Operator<small>Console</small>
      </span>
    </span>
  )
}
