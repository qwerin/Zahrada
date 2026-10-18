import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { BriefcaseIcon, ChevronRightIcon, PlusIcon, SearchIcon } from 'lucide-react'
import { useState } from 'react'
import { z } from 'zod'
import { gardenQueries, useCreateClient, useCreateJob } from '@/api/queries/garden'
import type { CreateJobInput, Job, JobKind } from '@/api/types'
import { EmptyState } from '@/components/empty-state'
import { FormGrid, SelectField, StatusPill, TextAreaField, TextField } from '@/components/garden/form'
import { jobKindLabels, jobStatusLabels, jobStatusTone } from '@/components/garden/labels'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { ResponsiveList } from '@/components/responsive-list'
import { Button } from '@/components/ui/button'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDate } from '@/lib/date'

const searchSchema = z.object({
  query: z.string().optional().catch(undefined),
  status: z.enum(['open', 'done', 'all']).optional().catch(undefined),
  /** ID klienta → otevře dialog nové zakázky s předvyplněným klientem */
  new: z.number().optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/jobs/')({
  validateSearch: searchSchema,
  head: () => ({ meta: [{ title: 'Zakázky · Zahrada' }] }),
  component: JobsPage,
})

const statusFilter = { open: 'open', done: 'done,invoiced', all: '' } as const

function plannedRange(j: Job): string {
  if (!j.planned_from && !j.planned_to) return ''
  if (j.planned_from === j.planned_to || !j.planned_to) return formatDate(j.planned_from)
  return `${formatDate(j.planned_from)} – ${formatDate(j.planned_to)}`
}

function JobsPage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const { canEdit } = useCurrentAccount()
  const tab = search.status ?? 'open'
  const [creating, setCreating] = useState(search.new !== undefined)
  const list = useQuery(gardenQueries.jobs(slug, { query: search.query, status: statusFilter[tab], per_page: 200 }))
  const open = (j: Job) => void navigate({ to: '/a/$slug/jobs/$jobId', params: { slug, jobId: j.id } })

  return (
    <>
      <PageHeader
        title="Zakázky"
        description="Realizace, údržba a jednorázové práce."
        actions={canEdit ? [{ label: 'Nová zakázka', icon: PlusIcon, primary: true, onClick: () => setCreating(true) }] : []}
      >
        <div className="flex flex-col gap-2 pt-1 sm:flex-row sm:items-center">
          <InputGroup className="sm:max-w-xs">
            <InputGroupAddon>
              <SearchIcon />
            </InputGroupAddon>
            <InputGroupInput
              placeholder="Číslo nebo název…"
              defaultValue={search.query ?? ''}
              onChange={(e) => void navigate({ search: (p) => ({ ...p, query: e.target.value || undefined }), replace: true })}
            />
          </InputGroup>
          <Tabs value={tab} onValueChange={(v) => void navigate({ search: (p) => ({ ...p, status: v as 'open' | 'done' | 'all' }), replace: true })}>
            <TabsList className="h-11! w-full sm:w-auto md:h-8!">
              <TabsTrigger value="open" className="px-3">Rozpracované</TabsTrigger>
              <TabsTrigger value="done" className="px-3">Dokončené</TabsTrigger>
              <TabsTrigger value="all" className="px-3">Vše</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      </PageHeader>
      <PageBody>
        {list.isError ? (
          <PageError error={list.error} reset={() => void list.refetch()} />
        ) : (
          <ResponsiveList
            items={list.data?.items}
            isLoading={list.isPending}
            getKey={(j) => j.id}
            onRowClick={open}
            columns={[
              { id: 'number', header: 'Číslo', cell: (j) => <span className="font-mono text-xs">{j.number}</span> },
              { id: 'title', header: 'Zakázka', cell: (j) => <span className="font-medium">{j.title}</span> },
              { id: 'client', header: 'Klient', cell: (j) => [j.client_name, j.site_name].filter(Boolean).join(' · ') },
              { id: 'kind', header: 'Druh', cell: (j) => jobKindLabels[j.kind] },
              { id: 'planned', header: 'Termín', cell: plannedRange },
              { id: 'status', header: 'Stav', cell: (j) => <StatusPill tone={jobStatusTone[j.status]}>{jobStatusLabels[j.status]}</StatusPill> },
            ]}
            renderCard={(j) => (
              <div className="flex items-center gap-3">
                <div className="min-w-0 flex-1">
                  <p className="flex items-center gap-2">
                    <span className="font-mono text-xs text-muted-foreground">{j.number}</span>
                    <StatusPill tone={jobStatusTone[j.status]}>{jobStatusLabels[j.status]}</StatusPill>
                  </p>
                  <p className="truncate font-medium">{j.title}</p>
                  <p className="truncate text-sm text-muted-foreground">{[j.client_name, plannedRange(j)].filter(Boolean).join(' · ')}</p>
                </div>
                <ChevronRightIcon className="size-4 text-muted-foreground" />
              </div>
            )}
            empty={
              <EmptyState
                icon={BriefcaseIcon}
                title="Žádné zakázky"
                description={tab === 'open' ? 'Rozpracované zakázky se zobrazí tady.' : 'Nic nenalezeno.'}
                action={canEdit && <Button onClick={() => setCreating(true)}>Nová zakázka</Button>}
              />
            }
          />
        )}
      </PageBody>
      {creating && (
        <NewJobDialog
          clientId={search.new}
          onClose={() => {
            setCreating(false)
            if (search.new !== undefined) void navigate({ search: (p) => ({ ...p, new: undefined }), replace: true })
          }}
          onCreated={(id) => void navigate({ to: '/a/$slug/jobs/$jobId', params: { slug, jobId: id } })}
        />
      )}
    </>
  )
}

function NewJobDialog({ clientId, onClose, onCreated }: { clientId?: number; onClose: () => void; onCreated: (id: number) => void }) {
  const { slug } = useCurrentAccount()
  const clients = useQuery(gardenQueries.clients(slug))
  const contractors = useQuery(gardenQueries.contractors(slug, 'true'))
  const create = useCreateJob(slug)
  const createClient = useCreateClient(slug)
  const [newClient, setNewClient] = useState({ name: '', phone: '', street: '', city: '' })
  const [f, setF] = useState({
    client_id: clientId ? String(clientId) : '', site_id: '', kind: 'realization' as JobKind, title: '',
    status: 'ordered' as CreateJobInput['status'], planned_from: '', planned_to: '', note: '', contractor_ids: [] as number[],
  })
  const client = useQuery({ ...gardenQueries.client(slug, Number(f.client_id) || 0), enabled: Number(f.client_id) > 0 })
  const noClients = clients.data?.items.length === 0
  const isNewClient = f.client_id === 'new' || (noClients && !f.client_id)
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  const submit = async () => {
    let cid = Number(f.client_id)
    let siteId = f.site_id ? Number(f.site_id) : undefined
    if (isNewClient) {
      // nový klient i se zahradou na jeho adrese, pak zakázka
      const c = await createClient.mutateAsync({ name: newClient.name.trim(), kind: 'person', phone: newClient.phone, street: newClient.street, city: newClient.city })
      cid = c.id
      if (newClient.street || newClient.city) {
        const { api, unwrap } = await import('@/api/client')
        const site = await unwrap(api.POST('/api/accounts/{slug}/sites', { params: { path: { slug } }, body: { client_id: cid, name: 'Zahrada', street: newClient.street, city: newClient.city } }))
        siteId = site.id
      }
    }
    create.mutate(
      {
        client_id: cid, site_id: siteId, kind: f.kind, title: f.title,
        status: f.status, planned_from: f.planned_from || undefined, planned_to: f.planned_to || undefined, note: f.note,
        contractor_ids: f.contractor_ids,
      },
      { onSuccess: (j) => { onClose(); onCreated(j.id) } },
    )
  }
  const canSubmit = f.title.trim() && (isNewClient ? newClient.name.trim() : f.client_id)
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title="Nová zakázka"
      footer={<Button disabled={!canSubmit || create.isPending || createClient.isPending} onClick={() => void submit()}>Vytvořit</Button>}
    >
      <FormGrid>
        <SelectField
          label="Klient"
          name="client_id"
          value={f.client_id}
          onChange={(e) => setF({ ...f, client_id: e.target.value, site_id: '' })}
          options={[
            { value: '', label: noClients ? 'Zatím žádní klienti' : 'Vyberte klienta…' },
            { value: 'new', label: '+ Nový klient' },
            ...(clients.data?.items ?? []).map((c) => ({ value: String(c.id), label: c.name })),
          ]}
          className="sm:col-span-2"
        />
        {isNewClient && (
          <>
            <TextField label="Jméno nového klienta" name="new_client_name" value={newClient.name} onChange={(e) => setNewClient({ ...newClient, name: e.target.value })} autoFocus />
            <TextField label="Telefon" name="new_client_phone" type="tel" value={newClient.phone} onChange={(e) => setNewClient({ ...newClient, phone: e.target.value })} />
            <TextField label="Ulice zahrady" name="new_client_street" value={newClient.street} onChange={(e) => setNewClient({ ...newClient, street: e.target.value })} />
            <TextField label="Obec" name="new_client_city" value={newClient.city} onChange={(e) => setNewClient({ ...newClient, city: e.target.value })} />
          </>
        )}
        <SelectField
          label="Zahrada"
          name="site_id"
          value={f.site_id}
          onChange={set('site_id')}
          disabled={isNewClient}
          options={[{ value: '', label: isNewClient ? 'Založí se z adresy' : '—' }, ...(client.data?.sites ?? []).filter((s) => !s.archived).map((s) => ({ value: String(s.id), label: s.name }))]}
        />
        <SelectField
          label="Druh"
          name="kind"
          value={f.kind}
          onChange={set('kind')}
          options={Object.entries(jobKindLabels).map(([value, label]) => ({ value, label }))}
        />
        <TextField label="Název zakázky" name="title" value={f.title} onChange={set('title')} placeholder="Např. Založení trávníku" className="sm:col-span-2" />
        <SelectField
          label="Stav"
          name="status"
          value={f.status}
          onChange={set('status')}
          options={(['inquiry', 'quoted', 'ordered', 'in_progress'] as const).map((s) => ({ value: s, label: jobStatusLabels[s] }))}
        />
        <div />
        <TextField label="Termín od" name="planned_from" type="date" value={f.planned_from} onChange={set('planned_from')} />
        <TextField label="Termín do" name="planned_to" type="date" value={f.planned_to} onChange={set('planned_to')} />
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
            {contractors.data?.items.length === 0 && <p className="text-sm text-muted-foreground">Zatím žádní dodavatelé.</p>}
          </div>
        </fieldset>
        <TextAreaField label="Poznámka" name="note" value={f.note} onChange={set('note')} className="sm:col-span-2" />
      </FormGrid>
    </ResponsiveDialog>
  )
}
