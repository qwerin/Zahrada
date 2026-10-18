// SPA adresy záznamů podle typu (události, úkoly). Typy odpovídají `subject_type` / `related_type` v API.

export type RecordKind = 'client' | 'site' | 'contractor' | 'job' | 'attendance' | 'webhook'

/** `recordHref("acme", "job", 12)` → `"/a/acme/jobs/12"`; neznámý typ → `null`. */
export function recordHref(slug: string, type: string | undefined, id: number | undefined): string | null {
  if (!type) return null
  const base = `/a/${encodeURIComponent(slug)}`
  const withId = (path: string) => (id ? `${base}/${path}/${id}` : null)
  switch (type) {
    case 'client':
      return withId('clients')
    case 'contractor':
      return withId('contractors')
    case 'job':
      return withId('jobs')
    case 'attendance':
      return `${base}/attendance`
    default:
      return null
  }
}

export const recordKindLabels: Record<RecordKind, string> = {
  client: 'Klient',
  site: 'Zahrada',
  contractor: 'Dodavatel',
  job: 'Zakázka',
  attendance: 'Docházka',
  webhook: 'Webhook',
}
