// Popisky a řazení úkolů (čistá logika, testovatelná).

import type { Todo, TodoRelatedType } from '@/api/types'
import { daysBetween, todayISO } from '@/lib/date'

/** Filtry seznamu úkolů podle typu vazby (hodnota `related_type` v API). */
export const todoTypeFilters: { value: TodoRelatedType; label: string }[] = [
  { value: 'job', label: 'Zakázky' },
  { value: 'client', label: 'Klienti' },
  { value: 'contractor', label: 'Dodavatelé' },
]

/** Text odkazu na záznam úkolu: „Otevřít fakturu“ … */
export const relatedLinkLabels: Record<string, string> = {
  job: 'Otevřít zakázku',
  client: 'Otevřít klienta',
  site: 'Otevřít zahradu',
  contractor: 'Otevřít dodavatele',
}

export type DueState = 'none' | 'overdue' | 'today' | 'soon' | 'later'

/** Stav termínu otevřeného úkolu vzhledem k dnešku (hotové úkoly → `none`). */
export function todoDueState(todo: Pick<Todo, 'due_on' | 'completed'>, today: string = todayISO()): DueState {
  if (!todo.due_on || todo.completed) return 'none'
  const diff = daysBetween(today, todo.due_on)
  if (diff < 0) return 'overdue'
  if (diff === 0) return 'today'
  if (diff <= 3) return 'soon'
  return 'later'
}
