import type { AttendanceStatus, JobKind, JobStatus } from '@/api/types'

export const jobStatusLabels: Record<JobStatus, string> = {
  inquiry: 'Poptávka',
  quoted: 'Nabídka odeslána',
  ordered: 'Objednáno',
  in_progress: 'V realizaci',
  done: 'Dokončeno',
  invoiced: 'Vyfakturováno',
  cancelled: 'Zrušeno',
}

/** Třídy odznaku stavu zakázky (barva podle fáze). */
export const jobStatusTone: Record<JobStatus, string> = {
  inquiry: 'bg-muted text-muted-foreground',
  quoted: 'bg-sky-500/15 text-sky-700 dark:text-sky-300',
  ordered: 'bg-amber-500/15 text-amber-800 dark:text-amber-300',
  in_progress: 'bg-emerald-500/15 text-emerald-800 dark:text-emerald-300',
  done: 'bg-primary/10 text-primary',
  invoiced: 'bg-violet-500/15 text-violet-800 dark:text-violet-300',
  cancelled: 'bg-muted text-muted-foreground line-through',
}

export const jobKindLabels: Record<JobKind, string> = {
  realization: 'Realizace',
  maintenance: 'Údržba',
  one_off: 'Jednorázově',
}

export const attendanceStatusLabels: Record<AttendanceStatus, string> = {
  draft: 'Rozpracováno',
  submitted: 'Ke schválení',
  approved: 'Schváleno',
  rejected: 'Vráceno',
}

export const attendanceStatusTone: Record<AttendanceStatus, string> = {
  draft: 'bg-muted text-muted-foreground',
  submitted: 'bg-amber-500/15 text-amber-800 dark:text-amber-300',
  approved: 'bg-emerald-500/15 text-emerald-800 dark:text-emerald-300',
  rejected: 'bg-destructive/10 text-destructive',
}

/** 465 → „7 h 45 min“, 480 → „8 h“. */
export function formatMinutes(min: number): string {
  const h = Math.floor(min / 60)
  const m = min % 60
  if (h === 0) return `${m} min`
  return m === 0 ? `${h} h` : `${h} h ${m} min`
}

/** Aktuální měsíc „YYYY-MM“ (místní čas). */
export function currentMonth(now: Date = new Date()): string {
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

/** Posun měsíce „2026-03“ o delta. */
export function shiftMonth(month: string, delta: number): string {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(y!, m! - 1 + delta, 1)
  return currentMonth(d)
}

/** „2026-03“ → „březen 2026“. */
export function monthLabel(month: string): string {
  const [y, m] = month.split('-').map(Number)
  return new Date(y!, m! - 1, 1).toLocaleDateString('cs-CZ', { month: 'long', year: 'numeric' })
}

/** Poslední den měsíce „YYYY-MM-DD“. */
export function monthEnd(month: string): string {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(y!, m!, 0)
  return `${month}-${String(d.getDate()).padStart(2, '0')}`
}
