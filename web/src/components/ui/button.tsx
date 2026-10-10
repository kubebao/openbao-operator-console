import { Slot } from '@radix-ui/react-slot'
import type { ComponentProps } from 'react'

type Variant = 'default' | 'outline' | 'ghost' | 'danger'
type Size = 'default' | 'icon' | 'small'

const variants: Record<Variant, string> = {
  default: 'button-primary',
  outline: 'button-outline',
  ghost: 'button-ghost',
  danger: 'button-danger',
}

const sizes: Record<Size, string | undefined> = {
  default: undefined,
  icon: 'button-icon',
  small: 'button-small',
}

export function Button({
  className,
  variant = 'outline',
  size = 'default',
  asChild = false,
  ...props
}: ComponentProps<'button'> & {
  variant?: Variant
  size?: Size
  asChild?: boolean
}) {
  const Component = asChild ? Slot : 'button'
  const classes = ['button', variants[variant], sizes[size], className]
  return <Component className={classes.filter(Boolean).join(' ')} {...props} />
}
