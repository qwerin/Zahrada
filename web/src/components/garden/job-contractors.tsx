import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { CheckIcon, PhoneIcon, PlusIcon, SearchIcon, UserPlusIcon, XIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { api, unwrap } from '@/api/client'
import { gardenQueries, useUpdateJob } from '@/api/queries/garden'
import type { AttendanceEntry, Contractor, Job } from '@/api/types'
import { StatusPill } from '@/components/garden/form'
import { formatMinutes, jobStatusLabels, jobStatusTone } from '@/components/garden/labels'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { formatDate } from '@/lib/date'
import { formatMoney } from '@/lib/money'

/** Barevná tečka dodavatele (barva z jeho karty). */
export function ContractorDot({ color }: { color?: string }) {
  return <span className="inline-block size-2.5 shrink-0 rounded-full" style={{ background: color || 'var(--muted-foreground)' }} />
}

/** Čipy dodavatelů (seznam zakázek, přehled). */
export function ContractorChips({ ids, contractors }: { ids: number[]; contractors: Map<number, Contractor> }) {
  if (ids.length === 0) return <span className="text-xs text-muted-foreground">nepřiřazeno</span>
  return (
    <span className="flex flex-wrap gap-1">
      {ids.map((id) => {
        const c = contractors.get(id)
        return (
          <span key={id} className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs">
            <ContractorDot color={c?.color} /> {c?.name ?? `#${id}`}
          </span>
        )
      })}
    </span>
  )
}

/** Karta „Dodavatelé“ v detailu zakázky: kdo na ní dělá, kolik odpracoval, přiřazení jedním klepnutím. */
export function JobContractorsCard({
  slug,
  job,
  canEdit,
  attendance,
}: {
  slug: string
  job: Job
  canEdit: boolean
  attendance: AttendanceEntry[]
}) {
  const contractors = useQuery(gardenQueries.contractors(slug))
  const update = useUpdateJob(slug, job.id)
  const [picking, setPicking] = useState(false)
  const byId = new Map((contractors.data?.items ?? []).map((c) => [c.id, c]))
  const worked = new Map<number, number>()
  for (const e of attendance) if (e.status === 'approved') worked.set(e.contractor_id, (worked.get(e.contractor_id) ?? 0) + e.worked_min)
  const save = (ids: number[], msg: string) => update.mutate({ contractor_ids: ids }, { onSuccess: () => toast.success(msg) })

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2">
        <CardTitle>Dodavatelé</CardTitle>
        {canEdit && (
          <Button size="sm" variant={job.contractor_ids.length ? 'outline' : 'default'} onClick={() => setPicking(true)}>
            <UserPlusIcon data-icon="inline-start" /> Přiřadit
          </Button>
        )}
      </CardHeader>
      <CardContent className="grid gap-1">
        {job.contractor_ids.length === 0 && (
          <p className="text-sm text-muted-foreground">
            Nikdo není přiřazen. Přiřazený dodavatel vidí zakázku v „Moje práce“ (adresu, přístup, fotky) a může na ni zapisovat docházku.
          </p>
        )}
        {job.contractor_ids.map((id) => {
          const c = byId.get(id)
          return (
            <div key={id} className="flex items-center gap-2 rounded-md py-1.5 text-sm">
              <ContractorDot color={c?.color} />
              <Link to="/a/$slug/contractors/$contractorId" params={{ slug, contractorId: id }} className="min-w-0 flex-1 truncate font-medium hover:underline">
                {c?.name ?? `#${id}`}
              </Link>
              {c?.phone && (
                <a href={`tel:${c.phone.replace(/\s/g, '')}`} className="text-muted-foreground hover:text-foreground" aria-label={`Zavolat ${c.name}`}>
                  <PhoneIcon className="size-4" />
                </a>
              )}
              <span className="w-20 text-right tabular-nums text-muted-foreground">{worked.get(id) ? formatMinutes(worked.get(id)!) : ''}</span>
              {canEdit && (
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`Odebrat ${c?.name ?? ''}`}
                  disabled={update.isPending}
                  onClick={() => save(job.contractor_ids.filter((x) => x !== id), `${c?.name ?? 'Dodavatel'} odebrán ze zakázky`)}
                >
                  <XIcon />
                </Button>
              )}
            </div>
          )
        })}
      </CardContent>
      {picking && (
        <AssignContractorsDialog
          slug={slug}
          selected={job.contractor_ids}
          busy={update.isPending}
          onClose={() => setPicking(false)}
          onSave={(ids) => update.mutate({ contractor_ids: ids }, { onSuccess: () => { toast.success('Dodavatelé uloženi'); setPicking(false) } })}
        />
      )}
    </Card>
  )
}

/** Výběr dodavatelů se zaškrtáváním, hledáním a vytížením (počet rozpracovaných zakázek). */
export function AssignContractorsDialog({
  slug,
  selected,
  busy,
  onClose,
  onSave,
}: {
  slug: string
  selected: number[]
  busy?: boolean
  onClose: () => void
  onSave: (ids: number[]) => void
}) {
  const contractors = useQuery(gardenQueries.contractors(slug, 'true'))
  const openJobs = useQuery(gardenQueries.jobs(slug, { status: 'ordered,in_progress', per_page: 200 }))
  const [chosen, setChosen] = useState<number[]>(selected)
  const [query, setQuery] = useState('')
  const load = useMemo(() => {
    const m = new Map<number, number>()
    for (const j of openJobs.data?.items ?? []) for (const id of j.contractor_ids) m.set(id, (m.get(id) ?? 0) + 1)
    return m
  }, [openJobs.data])
  const q = query.trim().toLowerCase()
  const list = (contractors.data?.items ?? []).filter((c) => !q || c.name.toLowerCase().includes(q) || c.phone.includes(q))
  const toggle = (id: number) => setChosen(chosen.includes(id) ? chosen.filter((x) => x !== id) : [...chosen, id])
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title="Přiřadit dodavatele"
      description="Zaškrtněte, kdo na zakázce pracuje. U jména vidíte, na kolika rozpracovaných zakázkách už je."
      footer={<Button disabled={busy} onClick={() => onSave(chosen)}>Uložit ({chosen.length})</Button>}
    >
      <div className="grid gap-2">
        {(contractors.data?.items.length ?? 0) > 6 && (
          <div className="relative">
            <SearchIcon className="absolute top-1/2 left-2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input className="pl-8" placeholder="Hledat dodavatele…" value={query} onChange={(e) => setQuery(e.target.value)} />
          </div>
        )}
        {contractors.data?.items.length === 0 && (
          <p className="text-sm text-muted-foreground">
            Zatím nemáte žádné dodavatele. Přidáte je v sekci <Link to="/a/$slug/contractors" params={{ slug }} className="text-primary">Dodavatelé</Link>.
          </p>
        )}
        <div className="grid max-h-[50vh] gap-1 overflow-auto">
          {list.map((c) => {
            const on = chosen.includes(c.id)
            const n = load.get(c.id) ?? 0
            return (
              <button
                key={c.id}
                type="button"
                aria-pressed={on}
                onClick={() => toggle(c.id)}
                className={`flex min-h-12 items-center gap-3 rounded-lg border px-3 py-2 text-left text-sm transition-colors ${on ? 'border-primary bg-primary/5' : 'hover:bg-muted/50'}`}
              >
                <span className={`flex size-5 shrink-0 items-center justify-center rounded border ${on ? 'border-primary bg-primary text-primary-foreground' : ''}`}>
                  {on && <CheckIcon className="size-3.5" />}
                </span>
                <ContractorDot color={c.color} />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{c.name}</span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {[c.phone, `${formatMoney(c.hourly_rate)}/h`].filter(Boolean).join(' · ')}
                  </span>
                </span>
                <span className={`shrink-0 text-xs ${n >= 3 ? 'text-amber-600' : 'text-muted-foreground'}`}>
                  {n === 0 ? 'volný' : `${n} ${n === 1 ? 'zakázka' : n < 5 ? 'zakázky' : 'zakázek'}`}
                </span>
              </button>
            )
          })}
        </div>
      </div>
    </ResponsiveDialog>
  )
}

/** Karta „Zakázky“ v detailu dodavatele: kde pracuje + přiřazení k další zakázce. */
export function ContractorJobsCard({ slug, contractor, canEdit }: { slug: string; contractor: Contractor; canEdit: boolean }) {
  const qc = useQueryClient()
  const jobs = useQuery(gardenQueries.jobs(slug, { contractor_id: contractor.id, status: 'open', per_page: 100 }))
  const [adding, setAdding] = useState(false)
  const [busy, setBusy] = useState(false)
  const setOnJob = async (job: Job, on: boolean) => {
    setBusy(true)
    try {
      const ids = on ? [...new Set([...job.contractor_ids, contractor.id])] : job.contractor_ids.filter((x) => x !== contractor.id)
      await unwrap(api.PATCH('/api/accounts/{slug}/jobs/{id}', { params: { path: { slug, id: job.id } }, body: { contractor_ids: ids } }))
      await qc.invalidateQueries({ queryKey: ['a', slug, 'jobs'] })
      toast.success(on ? `Přiřazeno k zakázce ${job.number}` : `Odebráno ze zakázky ${job.number}`)
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2">
        <CardTitle>Rozpracované zakázky</CardTitle>
        {canEdit && (
          <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
            <PlusIcon data-icon="inline-start" /> Přiřadit k zakázce
          </Button>
        )}
      </CardHeader>
      <CardContent className="grid gap-1">
        {jobs.data?.items.length === 0 && <p className="text-sm text-muted-foreground">Momentálně není přiřazen k žádné rozpracované zakázce.</p>}
        {jobs.data?.items.map((j) => (
          <div key={j.id} className="flex items-center gap-2 text-sm">
            <Link to="/a/$slug/jobs/$jobId" params={{ slug, jobId: j.id }} className="flex min-w-0 flex-1 items-center gap-2 rounded-md py-1.5 hover:underline">
              <span className="font-mono text-xs text-muted-foreground">{j.number}</span>
              <span className="truncate">{j.title}</span>
              <span className="truncate text-muted-foreground">· {j.client_name}</span>
            </Link>
            <StatusPill tone={jobStatusTone[j.status]}>{jobStatusLabels[j.status]}</StatusPill>
            {canEdit && (
              <Button variant="ghost" size="icon" aria-label="Odebrat ze zakázky" disabled={busy} onClick={() => void setOnJob(j, false)}>
                <XIcon />
              </Button>
            )}
          </div>
        ))}
      </CardContent>
      {adding && <PickJobDialog slug={slug} contractorId={contractor.id} busy={busy} onClose={() => setAdding(false)} onPick={(j) => void setOnJob(j, true).then(() => setAdding(false))} />}
    </Card>
  )
}

function PickJobDialog({ slug, contractorId, busy, onClose, onPick }: { slug: string; contractorId: number; busy: boolean; onClose: () => void; onPick: (j: Job) => void }) {
  const [query, setQuery] = useState('')
  const jobs = useQuery(gardenQueries.jobs(slug, { status: 'open', query: query || undefined, per_page: 100 }))
  const list = (jobs.data?.items ?? []).filter((j) => !j.contractor_ids.includes(contractorId))
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title="Přiřadit k zakázce">
      <div className="grid gap-2">
        <Input placeholder="Číslo nebo název zakázky…" value={query} onChange={(e) => setQuery(e.target.value)} autoFocus />
        <div className="grid max-h-[50vh] gap-1 overflow-auto">
          {list.length === 0 && <p className="p-2 text-sm text-muted-foreground">Žádná další rozpracovaná zakázka.</p>}
          {list.map((j) => (
            <button
              key={j.id}
              type="button"
              disabled={busy}
              onClick={() => onPick(j)}
              className="flex min-h-12 items-center gap-2 rounded-lg border px-3 py-2 text-left text-sm hover:bg-muted/50"
            >
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium">{j.title}</span>
                <span className="block truncate text-xs text-muted-foreground">
                  {j.number} · {j.client_name}
                  {j.planned_from && ` · ${formatDate(j.planned_from)}`}
                </span>
              </span>
              <StatusPill tone={jobStatusTone[j.status]}>{jobStatusLabels[j.status]}</StatusPill>
            </button>
          ))}
        </div>
      </div>
    </ResponsiveDialog>
  )
}
