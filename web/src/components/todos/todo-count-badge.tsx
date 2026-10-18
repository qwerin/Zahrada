import { useQuery } from '@tanstack/react-query'
import { todoQueries } from '@/api/queries/todos'
import { SidebarMenuBadge } from '@/components/ui/sidebar'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { cn } from '@/lib/utils'

/** Počet otevřených úkolů u položky navigace „Úkoly“ (nic při nule). */
export function TodoCountBadge({ slug, variant = 'pill' }: { slug: string; variant?: 'sidebar' | 'pill' | 'dot' }) {
  const { role } = useCurrentAccount()
  // dodavatel úkoly nevidí (403) — dotaz vůbec neposílat
  const { data: count } = useQuery({ ...todoQueries.openCount(slug), enabled: role !== undefined && role !== 'contractor' })
  if (!count) return null
  const label = count > 99 ? '99+' : String(count)
  if (variant === 'sidebar') {
    return (
      <SidebarMenuBadge className="rounded-full bg-primary/10 px-1.5 text-primary peer-data-active/menu-button:text-primary">
        {label}
        <span className="sr-only"> otevřených úkolů</span>
      </SidebarMenuBadge>
    )
  }
  if (variant === 'dot') {
    return <span aria-label={`${label} otevřených úkolů`} className="absolute top-2 right-[calc(50%-14px)] size-2 rounded-full bg-primary ring-2 ring-background" />
  }
  return (
    <span className={cn('inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-primary px-1.5 text-xs font-medium text-primary-foreground tabular-nums')}>
      {label}
      <span className="sr-only"> otevřených úkolů</span>
    </span>
  )
}
