import { Slot } from '@radix-ui/react-slot'
import { cva } from 'class-variance-authority'
import { clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'
import type { ComponentProps } from 'react'

const variants = cva('button', {
  variants: {
    variant: {
      default: 'button-primary',
      outline: 'button-outline',
      ghost: 'button-ghost',
      danger: 'button-danger',
    },
    size: { default: '', icon: 'button-icon', small: 'button-small' },
  },
  defaultVariants: { variant: 'outline', size: 'default' },
})

export function Button({
  className,
  variant = 'outline',
  size = 'default',
  asChild = false,
  ...props
}: ComponentProps<'button'> & {
  variant?: 'default' | 'outline' | 'ghost' | 'danger'
  size?: 'default' | 'icon' | 'small'
  asChild?: boolean
}) {
  const Component = asChild ? Slot : 'button'
  return <Component className={twMerge(clsx(variants({ variant, size }), className))} {...props} />
}
