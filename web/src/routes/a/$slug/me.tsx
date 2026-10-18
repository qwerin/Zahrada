import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { KeyRoundIcon, LogInIcon, LogOutIcon, MapPinIcon, PlusIcon, SendIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import {
  gardenQueries,
  useClockIn,
  useClockOut,
  useDeleteMyAttendance,
  useSaveMyAttendance,
  useSubmitMyAttendance,
} from '@/api/queries/garden'
import type { AttendanceEntry } from '@/api/types'
import { FormGrid, SelectField, StatusPill, TextAreaField, TextField } from '@/components/garden/form'
import { attendanceStatusLabels, attendanceStatusTone, currentMonth, formatMinutes, monthLabel, shiftMonth } from '@/components/garden/labels'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { formatDate } from '@/lib/date'
import { formatMoney } from '@/lib/money'

export const Route = createFileRoute('/a/$slug/me')({
  head: () => ({ meta: [{ title: 'Moje práce · Zahrada' }] }),
  component: MyWorkPage,
})

function mapsHref(address: string) {
  return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(address)}`
}

function MyWorkPage() {
  const { slug } = Route.useParams()
  const me = useQuery(gardenQueries.meContractor(slug))
  const jobs = useQuery(gardenQueries.meJobs(slug))
  const [month, setMonth] = useState(currentMonth())
  const entries = useQuery(gardenQueries.meAttendance(slug, month))
  const clockIn = useClockIn(slug)
  const clockOut = useClockOut(slug)
  const submit = useSubmitMyAttendance(slug)
  const [jobId, setJobId] = useState('')
  const [out, setOut] = useState({ break_min: '30', km: '0', note: '' })
  const [editing, setEditing] = useState<AttendanceEntry | 'new' | null>(null)

  if (me.isError) return <PageError error={me.error} reset={() => void me.refetch()} />
  const open = me.data?.open
  const items = entries.data?.items ?? []
  const toSubmit = items.filter((e) => e.end && (e.status === 'draft' || e.status === 'rejected'))
  const totalMin = items.filter((e) => e.end).reduce((s, e) => s + e.worked_min, 0)

  return (
    <>
      <PageHeader title="Moje práce" description={me.data ? `${me.data.name} · ${formatDate(me.data.today)}` : undefined} />
      <PageBody className="grid gap-4">
        <Card className={open ? 'border-emerald-500/50' : undefined}>
          <CardContent className="grid gap-4 py-5">
            {open ? (
              <>
                <div>
                  <p className="text-sm text-muted-foreground">V práci od</p>
                  <p className="text-4xl font-semibold tabular-nums">{open.start}</p>
                  <p className="text-sm text-muted-foreground">{open.job_id ? `${open.job_number} ${open.job_title}` : 'Režie'}</p>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <TextField label="Přestávka (min)" name="break_min" inputMode="numeric" value={out.break_min} onChange={(e) => setOut({ ...out, break_min: e.target.value })} />
                  <TextField label="Najeto (km)" name="km" inputMode="numeric" value={out.km} onChange={(e) => setOut({ ...out, km: e.target.value })} />
                  <TextField label="Co se dělalo" name="note" value={out.note} onChange={(e) => setOut({ ...out, note: e.target.value })} className="col-span-2" />
                </div>
                <Button
                  size="lg"
                  className="h-16 text-lg"
                  disabled={clockOut.isPending}
                  onClick={() =>
                    clockOut.mutate(
                      { break_min: Number(out.break_min) || 0, km: Number(out.km) || 0, note: out.note },
                      { onSuccess: (e) => toast.success(`Odchod zapsán: ${formatMinutes(e.worked_min)}`) },
                    )
                  }
                >
                  <LogOutIcon className="size-6" /> Odchod
                </Button>
              </>
            ) : (
              <>
                <SelectField
                  label="Na čem pracuji"
                  name="job"
                  value={jobId}
                  onChange={(e) => setJobId(e.target.value)}
                  options={[{ value: '', label: 'Režie (bez zakázky)' }, ...(jobs.data ?? []).map((j) => ({ value: String(j.id), label: `${j.number} ${j.title}` }))]}
                />
                <Button
                  size="lg"
                  className="h-16 text-lg"
                  disabled={clockIn.isPending || !me.data}
                  onClick={() => clockIn.mutate(jobId ? Number(jobId) : undefined, { onSuccess: (e) => toast.success(`Příchod zapsán v ${e.start}`) })}
                >
                  <LogInIcon className="size-6" /> Příchod
                </Button>
              </>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Moje zakázky</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-3">
            {jobs.data?.length === 0 && <p className="text-sm text-muted-foreground">Nemáte přidělenou žádnou rozpracovanou zakázku.</p>}
            {jobs.data?.map((j) => (
              <div key={j.id} className="rounded-lg border p-3">
                <p className="font-medium">{j.title}</p>
                <p className="text-sm text-muted-foreground">
                  <span className="font-mono">{j.number}</span> · {j.client_name}
                  {j.planned_from && ` · ${formatDate(j.planned_from)}`}
                  {j.planned_to && j.planned_to !== j.planned_from && ` – ${formatDate(j.planned_to)}`}
                </p>
                {j.address && (
                  <a href={mapsHref(j.address)} target="_blank" rel="noreferrer" className="mt-1 flex items-center gap-1 text-sm text-primary">
                    <MapPinIcon className="size-4" /> {j.address}
                  </a>
                )}
                {j.access_note && (
                  <p className="mt-1 flex items-start gap-1 text-sm">
                    <KeyRoundIcon className="mt-0.5 size-4 shrink-0" /> {j.access_note}
                  </p>
                )}
                {j.note && <p className="mt-1 whitespace-pre-wrap text-sm text-muted-foreground">{j.note}</p>}
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
            <CardTitle className="flex items-center gap-2">
              <Button variant="ghost" size="sm" onClick={() => setMonth(shiftMonth(month, -1))} aria-label="Předchozí měsíc">‹</Button>
              <span className="capitalize">{monthLabel(month)}</span>
              <Button variant="ghost" size="sm" onClick={() => setMonth(shiftMonth(month, 1))} aria-label="Další měsíc">›</Button>
            </CardTitle>
            <div className="flex gap-2">
              <Button size="sm" variant="outline" onClick={() => setEditing('new')}>
                <PlusIcon data-icon="inline-start" /> Zapsat ručně
              </Button>
              {toSubmit.length > 0 && (
                <Button
                  size="sm"
                  disabled={submit.isPending}
                  onClick={() => submit.mutate(toSubmit.map((e) => e.id), { onSuccess: () => toast.success('Odesláno ke schválení') })}
                >
                  <SendIcon data-icon="inline-start" /> Odeslat ke schválení ({toSubmit.length})
                </Button>
              )}
            </div>
          </CardHeader>
          <CardContent className="grid gap-0">
            <p className="mb-2 text-sm text-muted-foreground">Celkem {formatMinutes(totalMin)}</p>
            {items.length === 0 && <p className="text-sm text-muted-foreground">V tomto měsíci zatím nic.</p>}
            {items.map((e) => (
              <button
                key={e.id}
                type="button"
                disabled={!e.editable || !e.end}
                onClick={() => setEditing(e)}
                className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b py-2 text-left text-sm last:border-b-0 enabled:hover:bg-muted/40"
              >
                <span className="w-20 tabular-nums">{formatDate(e.work_on)}</span>
                <span className="tabular-nums">{e.start}–{e.end ?? '…'}</span>
                <span className="min-w-0 flex-1 truncate text-muted-foreground">{e.job_id ? e.job_title : 'Režie'}</span>
                <span className="tabular-nums">{e.end ? formatMinutes(e.worked_min) : 'v práci'}</span>
                <span className="w-24 text-right tabular-nums">{formatMoney(e.cost + e.km_cost)}</span>
                <StatusPill tone={attendanceStatusTone[e.status]}>{attendanceStatusLabels[e.status]}</StatusPill>
                {e.reject_reason && <span className="w-full text-xs text-destructive">Vráceno: {e.reject_reason}</span>}
              </button>
            ))}
          </CardContent>
        </Card>
      </PageBody>
      {editing && <MyEntryDialog entry={editing === 'new' ? null : editing} today={me.data?.today} onClose={() => setEditing(null)} />}
    </>
  )
}

function MyEntryDialog({ entry, today, onClose }: { entry: AttendanceEntry | null; today?: string; onClose: () => void }) {
  const { slug } = Route.useParams()
  const jobs = useQuery(gardenQueries.meJobs(slug))
  const save = useSaveMyAttendance(slug)
  const del = useDeleteMyAttendance(slug)
  const [f, setF] = useState({
    job_id: entry?.job_id ? String(entry.job_id) : '',
    work_on: entry?.work_on ?? today ?? '',
    start: entry?.start ?? '07:00',
    end: entry?.end ?? '15:30',
    break_min: String(entry?.break_min ?? 30),
    km: String(entry?.km ?? 0),
    note: entry?.note ?? '',
  })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={entry ? 'Upravit záznam' : 'Zapsat ručně'}
      footer={
        <div className="flex w-full justify-between gap-2">
          {entry ? (
            <Button variant="ghost" onClick={() => confirm('Smazat záznam?') && del.mutate(entry.id, { onSuccess: onClose })}>Smazat</Button>
          ) : (
            <span />
          )}
          <Button
            disabled={save.isPending || !f.work_on}
            onClick={() =>
              save.mutate(
                { id: entry?.id, job_id: f.job_id ? Number(f.job_id) : 0, work_on: f.work_on, start: f.start, end: f.end, break_min: Number(f.break_min) || 0, km: Number(f.km) || 0, note: f.note },
                { onSuccess: onClose },
              )
            }
          >
            Uložit
          </Button>
        </div>
      }
    >
      <FormGrid>
        <SelectField
          label="Zakázka"
          name="job_id"
          value={f.job_id}
          onChange={set('job_id')}
          options={[{ value: '', label: 'Režie (bez zakázky)' }, ...(jobs.data ?? []).map((j) => ({ value: String(j.id), label: `${j.number} ${j.title}` }))]}
          className="sm:col-span-2"
        />
        <TextField label="Datum" name="work_on" type="date" value={f.work_on} onChange={set('work_on')} />
        <div className="grid grid-cols-2 gap-2">
          <div className="grid gap-1.5">
            <label className="text-sm font-medium" htmlFor="start">Od</label>
            <Input id="start" type="time" value={f.start} onChange={set('start')} />
          </div>
          <div className="grid gap-1.5">
            <label className="text-sm font-medium" htmlFor="end">Do</label>
            <Input id="end" type="time" value={f.end} onChange={set('end')} />
          </div>
        </div>
        <TextField label="Přestávka (min)" name="break_min" inputMode="numeric" value={f.break_min} onChange={set('break_min')} />
        <TextField label="Najeto (km)" name="km" inputMode="numeric" value={f.km} onChange={set('km')} />
        <TextAreaField label="Co se dělalo" name="note" value={f.note} onChange={set('note')} className="sm:col-span-2" />
      </FormGrid>
    </ResponsiveDialog>
  )
}
