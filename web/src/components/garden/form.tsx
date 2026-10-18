import type { ComponentProps, ReactNode } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

// Jednoduché ovládací prvky formulářů Zahrady (řízené komponenty, bez react-hook-form).

export function TextField({
  label,
  hint,
  className,
  ...props
}: { label: ReactNode; hint?: ReactNode } & ComponentProps<typeof Input>) {
  return (
    <div className={cn('grid gap-1.5', className)}>
      <Label htmlFor={props.id ?? props.name}>{label}</Label>
      <Input id={props.id ?? props.name} {...props} />
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

export function TextAreaField({
  label,
  className,
  ...props
}: { label: ReactNode } & ComponentProps<typeof Textarea>) {
  return (
    <div className={cn('grid gap-1.5', className)}>
      <Label htmlFor={props.id ?? props.name}>{label}</Label>
      <Textarea id={props.id ?? props.name} rows={3} {...props} />
    </div>
  )
}

export function SelectField({
  label,
  options,
  className,
  ...props
}: {
  label: ReactNode
  options: { value: string; label: string }[]
} & ComponentProps<'select'>) {
  return (
    <div className={cn('grid gap-1.5', className)}>
      <Label htmlFor={props.id ?? props.name}>{label}</Label>
      <select
        id={props.id ?? props.name}
        className="h-11 w-full rounded-lg border border-input bg-transparent px-2 text-base outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 md:h-8 md:text-sm dark:bg-input/30"
        {...props}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </div>
  )
}

/** Odznak se stavem (barvu dodá `tone` z labels.ts). */
export function StatusPill({ tone, children }: { tone: string; children: ReactNode }) {
  return (
    <span className={cn('inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium', tone)}>
      {children}
    </span>
  )
}

/** Mřížka formuláře: 1 sloupec na mobilu, 2 od sm. */
export function FormGrid({ children }: { children: ReactNode }) {
  return <div className="grid gap-4 sm:grid-cols-2">{children}</div>
}
