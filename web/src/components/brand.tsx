// The OpenBao Operator mark with the product name. The mark is decorative; the name is text.
export function Brand({ className = 'brand-lockup' }: { className?: string }) {
  return (
    <span className={className}>
      <img src="/logo.svg" alt="" width={42} height={20} />
      <span>
        OpenBao Operator<small>Console</small>
      </span>
    </span>
  )
}
