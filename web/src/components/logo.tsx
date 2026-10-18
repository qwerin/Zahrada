import { cn } from '@/lib/utils'

/** Značka aplikace: zaoblený čtverec s lístkem. Barvy z tokenů (funguje ve světlém i tmavém režimu). */
export function LogoMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" aria-hidden="true" className={cn('size-8 shrink-0', className)}>
      <rect width="32" height="32" rx="8" className="fill-primary" />
      <path d="M9 23c0-8 5-13 14-14-1 9-6 14-14 14Z" className="fill-primary-foreground" />
      <path d="M9.5 22.5 18 14" strokeWidth="1.6" strokeLinecap="round" className="stroke-primary" />
    </svg>
  )
}

export function Logo({ className }: { className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-2', className)}>
      <LogoMark />
      <span className="text-base font-semibold tracking-tight">Zahrada</span>
    </span>
  )
}
