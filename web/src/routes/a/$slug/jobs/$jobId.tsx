import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { PencilIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { gardenQueries, useDeleteJob, useJobStatus, useUpdateJob } from '@/api/queries/garden'
import type { Job, JobKind, JobStatus } from '@/api/types'
import { HistorySection } from '@/components/events/timeline'
import { FormGrid, SelectField, StatusPill, TextAreaField, TextField } from '@/components/garden/form'
import {
  attendanceStatusLabels,
  attendanceStatusTone,
  formatMinutes,
  jobKindLabels,
  jobStatusLabels,
  jobStatusTone,
} from '@/components/garden/labels'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { FormSkeleton } from '@/components/skeletons'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDate } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/jobs/$jobId')({
  params: {
    parse: ({ jobId }) => ({ jobId: parseId(jobId) }),
    stringify: ({ jobId }) => ({ jobId: String(jobId) }),
  },
  head: () => ({ meta: [{ title: 'Zakázka · Zahrada' }] }),
  component: JobPage,
})

/** Popisek tlačítka přechodu do stavu. */
const statusAction: Record<string, string> = {
  inquiry: 'Vrátit do poptávky',
  quoted: 'Nabídka odeslána',
  ordered: 'Objednáno',
  in_progress: 'Zahájit realizaci',
  done: 'Dokončit',
  invoiced: 'Vyfakturováno',
  cancelled: 'Zrušit',
}

function JobPage() {
  const { slug, jobId } = Route.useParams()
  const { canEdit } = useCurrentAccount()
  const navigate = useNavigate()
  const q = useQuery(gardenQueries.job(slug, jobId))
  const contractors = useQuery(gardenQueries.contractors(slug))
  const att = useQuery(gardenQueries.attendance(slug, { job_id: jobId }))
  const status = useJobStatus(slug, jobId)
  const update = useUpdateJob(slug, jobId)
  const del = useDeleteJob(slug)
  const [editing, setEditing] = useState(false)

  if (q.isError) return <PageError error={q.error} reset={() => void q.refetch()} />
  if (!q.data) return <FormSkeleton />
  const j = q.data
  const names = new Map((contractors.data?.items ?? []).map((c) => [c.id, c.name]))
  const totalMin = (att.data?.items ?? []).filter((e) => e.status === 'approved').reduce((s, e) => s + e.worked_min, 0)
  const totalCost = (att.data?.items ?? []).filter((e) => e.status === 'approved').reduce((s, e) => s + e.cost + e.km_cost, 0)

  const actions: PageAction[] = canEdit
    ? [
        ...(j.next_statuses as JobStatus[])
          .filter((s) => s !== 'cancelled')
          .map((s, i) => ({
            label: statusAction[s] ?? jobStatusLabels[s],
            primary: i === 0,
            variant: (i === 0 ? 'default' : 'outline') as PageAction['variant'],
            onClick: () => status.mutate(s, { onSuccess: () => toast.success(`Stav: ${jobStatusLabels[s]}`) }),
          })),
        { label: 'Upravit', icon: PencilIcon, onClick: () => setEditing(true), overflow: true },
        ...((j.next_statuses as string[]).includes('cancelled')
          ? [{ label: 'Zrušit zakázku', overflow: true, onClick: () => confirm('Zrušit zakázku?') && status.mutate('cancelled') }]
          : []),
        {
          label: 'Smazat',
          icon: Trash2Icon,
          overflow: true,
          variant: 'destructive',
          onClick: () =>
            confirm(`Smazat zakázku ${j.number}?`) &&
            del.mutate(j.id, { onSuccess: () => { toast.success('Zakázka smazána'); void navigate({ to: '/a/$slug/jobs', params: { slug } }) } }),
        },
      ]
    : []

  return (
    <>
      <PageHeader
        title={j.title}
        description={
          <span className="flex flex-wrap items-center gap-2">
            <span className="font-mono">{j.number}</span>
            <StatusPill tone={jobStatusTone[j.status]}>{jobStatusLabels[j.status]}</StatusPill>
            <span>{jobKindLabels[j.kind]}</span>
          </span>
        }
        back={{ to: '/a/$slug/jobs', params: { slug } }}
        actions={actions}
      />
      <PageBody className="grid gap-4 lg:grid-cols-[1fr_20rem]">
        <div className="grid content-start gap-4">
          <Card>
            <CardHeader>
              <CardTitle>Docházka na zakázce</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-2">
              <p className="text-sm text-muted-foreground">
                Schváleno celkem {formatMinutes(totalMin)} · {formatMoney(totalCost)}
              </p>
              {att.data?.items.length === 0 && <p className="text-sm text-muted-foreground">Zatím žádná docházka.</p>}
              {att.data?.items.map((e) => (
                <div key={e.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b py-2 text-sm last:border-b-0">
                  <span className="w-24">{formatDate(e.work_on)}</span>
                  <span className="min-w-0 flex-1 truncate font-medium">{e.contractor_name}</span>
                  <span className="tabular-nums">
                    {e.start}–{e.end ?? '…'}
                  </span>
                  <span className="w-20 text-right tabular-nums">{e.end ? formatMinutes(e.worked_min) : 'v práci'}</span>
                  <StatusPill tone={attendanceStatusTone[e.status]}>{attendanceStatusLabels[e.status]}</StatusPill>
                </div>
              ))}
              <Link to="/a/$slug/attendance" params={{ slug }} className="text-sm text-primary">
                Otevřít docházku →
              </Link>
            </CardContent>
          </Card>
          {j.note && (
            <Card>
              <CardHeader>
                <CardTitle>Poznámka</CardTitle>
              </CardHeader>
              <CardContent className="whitespace-pre-wrap text-sm">{j.note}</CardContent>
            </Card>
          )}
        </div>
        <div className="grid content-start gap-4">
          <Card>
            <CardHeader>
              <CardTitle>Údaje</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-2 text-sm">
              <p>
                <span className="text-muted-foreground">Klient: </span>
                <Link to="/a/$slug/clients/$clientId" params={{ slug, clientId: j.client_id }} className="text-primary">
                  {j.client_name}
                </Link>
              </p>
              {j.site_name && <p><span className="text-muted-foreground">Zahrada: </span>{j.site_name}</p>}
              {(j.planned_from || j.planned_to) && (
                <p>
                  <span className="text-muted-foreground">Termín: </span>
                  {formatDate(j.planned_from)} {j.planned_to && j.planned_to !== j.planned_from && `– ${formatDate(j.planned_to)}`}
                </p>
              )}
              {j.monthly_fee ? <p><span className="text-muted-foreground">Paušál: </span>{formatMoney(j.monthly_fee)} / měsíc</p> : null}
              <div>
                <p className="mb-1 text-muted-foreground">Dodavatelé:</p>
                <div className="flex flex-wrap gap-1.5">
                  {j.contractor_ids.length === 0 && <span className="text-muted-foreground">nikdo přiřazen</span>}
                  {j.contractor_ids.map((id) => (
                    <span key={id} className="rounded-full bg-muted px-2 py-0.5 text-xs">{names.get(id) ?? `#${id}`}</span>
                  ))}
                </div>
              </div>
            </CardContent>
          </Card>
          <HistorySection subjectType="job" subjectId={j.id} />
        </div>
      </PageBody>
      {editing && <EditJobDialog job={j} onClose={() => setEditing(false)} save={update} />}
    </>
  )
}

function EditJobDialog({ job, onClose, save }: { job: Job; onClose: () => void; save: ReturnType<typeof useUpdateJob> }) {
  const { slug } = useCurrentAccount()
  const client = useQuery(gardenQueries.client(slug, job.client_id))
  const contractors = useQuery(gardenQueries.contractors(slug))
  const [f, setF] = useState({
    title: job.title, kind: job.kind as JobKind, site_id: job.site_id ? String(job.site_id) : '',
    planned_from: job.planned_from ?? '', planned_to: job.planned_to ?? '', note: job.note,
    monthly_fee: job.monthly_fee ? String(job.monthly_fee / 100) : '', contractor_ids: job.contractor_ids,
  })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  const submit = () =>
    save.mutate(
      {
        title: f.title, kind: f.kind, site_id: f.site_id ? Number(f.site_id) : 0, planned_from: f.planned_from, planned_to: f.planned_to,
        note: f.note, monthly_fee: f.monthly_fee ? Math.round(Number(f.monthly_fee.replace(',', '.')) * 100) : 0,
        contractor_ids: f.contractor_ids,
      },
      { onSuccess: onClose },
    )
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title="Upravit zakázku"
      footer={<Button disabled={!f.title.trim() || save.isPending} onClick={submit}>Uložit</Button>}
    >
      <FormGrid>
        <TextField label="Název" name="title" value={f.title} onChange={set('title')} className="sm:col-span-2" />
        <SelectField label="Druh" name="kind" value={f.kind} onChange={set('kind')} options={Object.entries(jobKindLabels).map(([value, label]) => ({ value, label }))} />
        <SelectField
          label="Zahrada"
          name="site_id"
          value={f.site_id}
          onChange={set('site_id')}
          options={[{ value: '', label: '—' }, ...(client.data?.sites ?? []).map((s) => ({ value: String(s.id), label: s.name }))]}
        />
        <TextField label="Termín od" name="planned_from" type="date" value={f.planned_from} onChange={set('planned_from')} />
        <TextField label="Termín do" name="planned_to" type="date" value={f.planned_to} onChange={set('planned_to')} />
        {f.kind === 'maintenance' && (
          <TextField label="Měsíční paušál (Kč)" name="monthly_fee" value={f.monthly_fee} onChange={set('monthly_fee')} inputMode="decimal" />
        )}
        <fieldset className="grid gap-1.5 sm:col-span-2">
          <legend className="mb-1 text-sm font-medium">Dodavatelé</legend>
          <div className="flex flex-wrap gap-2">
            {(contractors.data?.items ?? []).map((c) => {
              const on = f.contractor_ids.includes(c.id)
              return (
                <Button
                  key={c.id}
                  type="button"
                  size="sm"
                  variant={on ? 'default' : 'outline'}
                  onClick={() => setF({ ...f, contractor_ids: on ? f.contractor_ids.filter((x) => x !== c.id) : [...f.contractor_ids, c.id] })}
                >
                  {c.name}
                </Button>
              )
            })}
          </div>
        </fieldset>
        <TextAreaField label="Poznámka" name="note" value={f.note} onChange={set('note')} className="sm:col-span-2" />
      </FormGrid>
    </ResponsiveDialog>
  )
}
