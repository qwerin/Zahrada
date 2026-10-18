import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { CheckIcon, ChevronLeftIcon, ChevronRightIcon, ClockIcon, DownloadIcon, LockIcon, PlusIcon, Trash2Icon, UndoIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { z } from 'zod'
import { accountQueries } from '@/api/queries/accounts'
import {
  gardenQueries,
  useAttendanceAction,
  useCloseAttendance,
  useDeleteAttendance,
  useSaveAttendance,
} from '@/api/queries/garden'
import type { AttendanceEntry } from '@/api/types'
import { EmptyState } from '@/components/empty-state'
import { FormGrid, SelectField, StatusPill, TextAreaField, TextField } from '@/components/garden/form'
import {
  attendanceStatusLabels,
  attendanceStatusTone,
  currentMonth,
  formatMinutes,
  monthEnd,
  monthLabel,
  shiftMonth,
} from '@/components/garden/labels'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDate, todayISO } from '@/lib/date'
import { formatMoney } from '@/lib/money'

const searchSchema = z.object({
  month: z.string().regex(/^\d{4}-\d{2}$/).optional().catch(undefined),
  contractor: z.number().optional().catch(undefined),
  status: z.enum(['all', 'submitted', 'draft', 'open']).optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/attendance/')({
  validateSearch: searchSchema,
  head: () => ({ meta: [{ title: 'Docházka · Zahrada' }] }),
  component: AttendancePage,
})

const statusQuery = { all: undefined, submitted: 'submitted', draft: 'draft,rejected', open: 'open' } as const

function AttendancePage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const { canEdit, canManageSettings } = useCurrentAccount()
  const month = search.month ?? currentMonth()
  const tab = search.status ?? 'all'
  const account = useQuery(accountQueries.detail(slug))
  const contractors = useQuery(gardenQueries.contractors(slug))
  const summary = useQuery(gardenQueries.summary(slug, month))
  const list = useQuery(
    gardenQueries.attendance(slug, { from: `${month}-01`, to: monthEnd(month), contractor_id: search.contractor, status: statusQuery[tab] }),
  )
  const action = useAttendanceAction(slug)
  const close = useCloseAttendance(slug)
  const [selected, setSelected] = useState<number[]>([])
  const [editing, setEditing] = useState<AttendanceEntry | 'new' | null>(null)
  const go = (patch: Partial<z.infer<typeof searchSchema>>) => {
    setSelected([])
    void navigate({ search: (p) => ({ ...p, ...patch }), replace: true })
  }
  const items = list.data?.items ?? []
  const selectable = items.filter((e) => e.editable && e.end && e.status !== 'approved')
  const locked = summary.data?.locked ?? false

  const run = (kind: 'approve' | 'reject' | 'reopen', ids: number[]) => {
    const reason = kind === 'reject' ? (prompt('Důvod vrácení (uvidí ho dodavatel):') ?? undefined) : undefined
    if (kind === 'reject' && reason === undefined) return
    action.mutate({ action: kind, ids, reason }, { onSuccess: () => { setSelected([]); toast.success('Hotovo') } })
  }

  const actions: PageAction[] = [
    ...(canEdit && !locked ? [{ label: 'Zapsat docházku', icon: PlusIcon, primary: true, onClick: () => setEditing('new' as const) }] : []),
    {
      label: 'Export do Excelu',
      icon: DownloadIcon,
      render: <a href={`/api/accounts/${slug}/attendance/export.csv?month=${month}${search.contractor ? `&contractor_id=${search.contractor}` : ''}`} />,
    },
    ...(canManageSettings && !locked
      ? [{
          label: `Uzavřít ${monthLabel(month)}`,
          icon: LockIcon,
          overflow: true,
          onClick: () =>
            confirm(`Uzavřít docházku do ${formatDate(monthEnd(month))}? Uzavřené záznamy už nepůjde měnit.`) &&
            close.mutate(monthEnd(month), { onSuccess: () => toast.success('Docházka uzavřena') }),
        }]
      : []),
  ]

  return (
    <>
      <PageHeader title="Docházka" description="Odpracované hodiny dodavatelů, schvalování a podklady k proplacení." actions={actions}>
        <div className="flex flex-col gap-2 pt-1 sm:flex-row sm:items-center">
          <div className="flex items-center gap-1">
            <Button variant="outline" size="icon" aria-label="Předchozí měsíc" onClick={() => go({ month: shiftMonth(month, -1) })}>
              <ChevronLeftIcon />
            </Button>
            <span className="min-w-36 text-center font-medium capitalize">{monthLabel(month)}</span>
            <Button variant="outline" size="icon" aria-label="Další měsíc" onClick={() => go({ month: shiftMonth(month, 1) })}>
              <ChevronRightIcon />
            </Button>
          </div>
          <select
            aria-label="Dodavatel"
            className="h-11 rounded-lg border border-input bg-transparent px-2 text-base md:h-8 md:text-sm dark:bg-input/30"
            value={search.contractor ?? ''}
            onChange={(e) => go({ contractor: e.target.value ? Number(e.target.value) : undefined })}
          >
            <option value="">Všichni dodavatelé</option>
            {(contractors.data?.items ?? []).map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
          <Tabs value={tab} onValueChange={(v) => go({ status: v as 'all' })}>
            <TabsList className="h-11! w-full sm:w-auto md:h-8!">
              <TabsTrigger value="all" className="px-3">Vše</TabsTrigger>
              <TabsTrigger value="submitted" className="px-3">
                Ke schválení{summary.data && summary.data.pending_count > 0 ? ` (${summary.data.pending_count})` : ''}
              </TabsTrigger>
              <TabsTrigger value="draft" className="px-3">Rozpracované</TabsTrigger>
              <TabsTrigger value="open" className="px-3">V práci</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      </PageHeader>
      <PageBody className="grid gap-4">
        {summary.data && (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Card>
              <CardContent className="py-3">
                <p className="text-sm text-muted-foreground">Schváleno</p>
                <p className="text-xl font-semibold tabular-nums">{formatMinutes(summary.data.total_minutes)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-3">
                <p className="text-sm text-muted-foreground">K proplacení vč. cestovného</p>
                <p className="text-xl font-semibold tabular-nums">{formatMoney(summary.data.total_cost)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-3">
                <p className="text-sm text-muted-foreground">Neschválené záznamy</p>
                <p className="text-xl font-semibold tabular-nums">{summary.data.pending_count}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-3">
                <p className="text-sm text-muted-foreground">Měsíc</p>
                <p className="flex items-center gap-1 text-xl font-semibold">
                  {locked ? <><LockIcon className="size-4" /> Uzavřený</> : 'Otevřený'}
                </p>
              </CardContent>
            </Card>
          </div>
        )}
        {summary.data && summary.data.contractors.length > 0 && (
          <Card>
            <CardContent className="grid gap-1 py-3 text-sm">
              <p className="mb-1 font-medium">Souhrn po dodavatelích (schválené, zaokrouhlení {summary.data.rounding_min || 'žádné'} {summary.data.rounding_min ? 'min' : ''})</p>
              {summary.data.contractors.map((c) => (
                <details key={c.contractor_id} className="rounded-md px-2 py-1 open:bg-muted/40">
                  <summary className="flex cursor-pointer items-center gap-3">
                    <span className="flex-1 font-medium">{c.contractor_name}</span>
                    <span className="tabular-nums text-muted-foreground">{c.days} {c.days === 1 ? "den" : c.days < 5 ? "dny" : "dní"}</span>
                    <span className="w-24 text-right tabular-nums">{formatMinutes(c.minutes)}</span>
                    <span className="w-28 text-right tabular-nums font-medium">{formatMoney(c.total)}</span>
                  </summary>
                  <div className="mt-1 grid gap-0.5 pl-4 text-muted-foreground">
                    {c.jobs.map((j) => (
                      <div key={j.job_id} className="flex gap-3">
                        <span className="flex-1 truncate">{j.job_id ? `${j.job_number} ${j.job_title}` : 'Režie'}</span>
                        <span className="w-24 text-right tabular-nums">{formatMinutes(j.minutes)}</span>
                        <span className="w-28 text-right tabular-nums">{formatMoney(j.cost + j.km_cost)}</span>
                      </div>
                    ))}
                  </div>
                </details>
              ))}
            </CardContent>
          </Card>
        )}

        {canEdit && selected.length > 0 && (
          <div className="sticky top-2 z-10 flex flex-wrap items-center gap-2 rounded-lg border bg-background p-2 shadow-sm">
            <span className="px-2 text-sm">Vybráno {selected.length}</span>
            <Button size="sm" onClick={() => run('approve', selected)}>
              <CheckIcon data-icon="inline-start" /> Schválit
            </Button>
            <Button size="sm" variant="outline" onClick={() => run('reject', selected)}>
              <UndoIcon data-icon="inline-start" /> Vrátit k opravě
            </Button>
          </div>
        )}

        {list.isError ? (
          <PageError error={list.error} reset={() => void list.refetch()} />
        ) : items.length === 0 && !list.isPending ? (
          <EmptyState icon={ClockIcon} title="Žádné záznamy" description="V tomto měsíci zatím nikdo nic nezapsal." />
        ) : (
          <Card>
            <CardContent className="grid gap-0 py-1">
              {canEdit && selectable.length > 0 && (
                <label className="flex items-center gap-2 border-b py-2 text-sm text-muted-foreground">
                  <Checkbox
                    checked={selected.length === selectable.length}
                    onCheckedChange={(v) => setSelected(v ? selectable.map((e) => e.id) : [])}
                  />
                  Vybrat vše ke schválení
                </label>
              )}
              {items.map((e) => (
                <div key={e.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b py-2 text-sm last:border-b-0">
                  {canEdit && (
                    <Checkbox
                      disabled={!selectable.includes(e)}
                      checked={selected.includes(e.id)}
                      onCheckedChange={(v) => setSelected(v ? [...selected, e.id] : selected.filter((x) => x !== e.id))}
                      aria-label="Vybrat záznam"
                    />
                  )}
                  <button
                    type="button"
                    className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-0.5 text-left"
                    onClick={() => canEdit && e.editable && setEditing(e)}
                  >
                    <span className="w-20 tabular-nums">{formatDate(e.work_on)}</span>
                    <span className="w-32 truncate font-medium">{e.contractor_name}</span>
                    <span className="tabular-nums">{e.start}–{e.end ?? '…'}</span>
                    <span className="min-w-0 flex-1 truncate text-muted-foreground">
                      {e.job_id ? `${e.job_number} ${e.job_title}` : 'Režie'}
                      {e.note && ` · ${e.note}`}
                    </span>
                    <span className="w-20 text-right tabular-nums">{e.end ? formatMinutes(e.worked_min) : 'v práci'}</span>
                    <span className="w-24 text-right tabular-nums">{formatMoney(e.cost + e.km_cost)}</span>
                  </button>
                  <StatusPill tone={attendanceStatusTone[e.status]}>{attendanceStatusLabels[e.status]}</StatusPill>
                  {canEdit && e.status === 'approved' && e.editable && (
                    <Button size="sm" variant="ghost" onClick={() => run('reopen', [e.id])}>Znovu otevřít</Button>
                  )}
                </div>
              ))}
            </CardContent>
          </Card>
        )}
        {account.data?.attendance_locked_through && (
          <p className="text-xs text-muted-foreground">Docházka je uzavřená do {formatDate(account.data.attendance_locked_through)}.</p>
        )}
      </PageBody>
      {editing && <EntryDialog entry={editing === 'new' ? null : editing} defaultContractor={search.contractor} onClose={() => setEditing(null)} />}
    </>
  )
}

function EntryDialog({ entry, defaultContractor, onClose }: { entry: AttendanceEntry | null; defaultContractor?: number; onClose: () => void }) {
  const { slug } = useCurrentAccount()
  const contractors = useQuery(gardenQueries.contractors(slug, 'true'))
  const jobs = useQuery(gardenQueries.jobs(slug, { status: 'open', per_page: 200 }))
  const save = useSaveAttendance(slug)
  const del = useDeleteAttendance(slug)
  const [f, setF] = useState({
    contractor_id: String(entry?.contractor_id ?? defaultContractor ?? ''),
    job_id: entry?.job_id ? String(entry.job_id) : '',
    work_on: entry?.work_on ?? todayISO(),
    start: entry?.start ?? '07:00',
    end: entry?.end ?? '15:30',
    break_min: String(entry?.break_min ?? 30),
    km: String(entry?.km ?? 0),
    note: entry?.note ?? '',
  })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  const submit = () =>
    save.mutate(
      {
        id: entry?.id, contractor_id: Number(f.contractor_id), job_id: f.job_id ? Number(f.job_id) : 0, work_on: f.work_on,
        start: f.start, end: f.end, break_min: Number(f.break_min) || 0, km: Number(f.km) || 0, note: f.note,
      },
      { onSuccess: () => { toast.success('Uloženo'); onClose() } },
    )
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={entry ? 'Upravit záznam' : 'Zapsat docházku'}
      description={entry ? undefined : 'Záznam zapsaný vedoucím je rovnou schválený.'}
      footer={
        <div className="flex w-full justify-between gap-2">
          {entry ? (
            <Button variant="ghost" onClick={() => confirm('Smazat záznam?') && del.mutate(entry.id, { onSuccess: onClose })}>
              <Trash2Icon data-icon="inline-start" /> Smazat
            </Button>
          ) : (
            <span />
          )}
          <Button disabled={!f.contractor_id || save.isPending} onClick={submit}>Uložit</Button>
        </div>
      }
    >
      <FormGrid>
        <SelectField
          label="Dodavatel"
          name="contractor_id"
          value={f.contractor_id}
          onChange={set('contractor_id')}
          options={[{ value: '', label: 'Vyberte…' }, ...(contractors.data?.items ?? []).map((c) => ({ value: String(c.id), label: c.name }))]}
        />
        <SelectField
          label="Zakázka"
          name="job_id"
          value={f.job_id}
          onChange={set('job_id')}
          options={[{ value: '', label: 'Režie (bez zakázky)' }, ...(jobs.data?.items ?? []).map((j) => ({ value: String(j.id), label: `${j.number} ${j.title}` }))]}
        />
        <TextField label="Datum" name="work_on" type="date" value={f.work_on} onChange={set('work_on')} />
        <div className="grid grid-cols-2 gap-2">
          <TextField label="Od" name="start" type="time" value={f.start} onChange={set('start')} />
          <TextField label="Do" name="end" type="time" value={f.end} onChange={set('end')} />
        </div>
        <TextField label="Přestávka (min)" name="break_min" inputMode="numeric" value={f.break_min} onChange={set('break_min')} />
        <TextField label="Najeto (km)" name="km" inputMode="numeric" value={f.km} onChange={set('km')} />
        <TextAreaField label="Co se dělalo" name="note" value={f.note} onChange={set('note')} className="sm:col-span-2" />
      </FormGrid>
    </ResponsiveDialog>
  )
}
