import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { ArchiveIcon, BriefcaseIcon, KeyRoundIcon, MapPinIcon, PencilIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import {
  gardenQueries,
  useCreateSite,
  useDeleteClient,
  useDeleteSite,
  useUpdateClient,
  useUpdateSite,
} from '@/api/queries/garden'
import type { ClientDetail, Site } from '@/api/types'
import { HistorySection } from '@/components/events/timeline'
import { FormGrid, SelectField, StatusPill, TextAreaField, TextField } from '@/components/garden/form'
import { jobStatusLabels, jobStatusTone } from '@/components/garden/labels'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { FormSkeleton } from '@/components/skeletons'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/clients/$clientId')({
  params: {
    parse: ({ clientId }) => ({ clientId: parseId(clientId) }),
    stringify: ({ clientId }) => ({ clientId: String(clientId) }),
  },
  head: () => ({ meta: [{ title: 'Klient · Zahrada' }] }),
  component: ClientPage,
})

function ClientPage() {
  const { slug, clientId } = Route.useParams()
  const { canEdit } = useCurrentAccount()
  const navigate = useNavigate()
  const q = useQuery(gardenQueries.client(slug, clientId))
  const jobs = useQuery(gardenQueries.jobs(slug, { client_id: clientId, per_page: 50 }))
  const update = useUpdateClient(slug, clientId)
  const del = useDeleteClient(slug)
  const [editing, setEditing] = useState(false)
  const [site, setSite] = useState<Site | 'new' | null>(null)

  if (q.isError) return <PageError error={q.error} reset={() => void q.refetch()} />
  if (!q.data) return <FormSkeleton />
  const c = q.data

  const actions: PageAction[] = canEdit
    ? [
        { label: 'Nová zakázka', icon: BriefcaseIcon, primary: true, render: <Link to="/a/$slug/jobs" params={{ slug }} search={{ new: clientId }} /> },
        { label: 'Upravit', icon: PencilIcon, onClick: () => setEditing(true) },
        {
          label: c.archived ? 'Obnovit z archivu' : 'Archivovat',
          icon: ArchiveIcon,
          overflow: true,
          onClick: () => update.mutate({ archived: !c.archived }, { onSuccess: () => toast.success(c.archived ? 'Klient obnoven' : 'Klient archivován') }),
        },
        {
          label: 'Smazat',
          icon: Trash2Icon,
          overflow: true,
          variant: 'destructive',
          onClick: () => {
            if (!confirm(`Smazat klienta ${c.name} včetně jeho zahrad?`)) return
            del.mutate(c.id, { onSuccess: () => { toast.success('Klient smazán'); void navigate({ to: '/a/$slug/clients', params: { slug } }) } })
          },
        },
      ]
    : []

  return (
    <>
      <PageHeader title={c.name} description={[c.archived && 'Archivovaný', c.city, c.phone, c.email].filter(Boolean).join(' · ')} back={{ to: '/a/$slug/clients', params: { slug } }} actions={actions} />
      <PageBody className="grid gap-4 lg:grid-cols-[1fr_20rem]">
        <div className="grid gap-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <CardTitle>Zahrady</CardTitle>
              {canEdit && (
                <Button size="sm" variant="outline" onClick={() => setSite('new')}>
                  <PlusIcon data-icon="inline-start" /> Přidat zahradu
                </Button>
              )}
            </CardHeader>
            <CardContent className="grid gap-3">
              {c.sites.length === 0 && <p className="text-sm text-muted-foreground">Klient zatím nemá žádnou zahradu.</p>}
              {c.sites.map((s) => (
                <button
                  key={s.id}
                  type="button"
                  disabled={!canEdit}
                  onClick={() => setSite(s)}
                  className="rounded-lg border p-3 text-left transition-colors enabled:hover:bg-muted/50"
                >
                  <p className="font-medium">
                    {s.name} {s.archived && <span className="text-xs text-muted-foreground">(archiv)</span>}
                  </p>
                  <p className="flex items-center gap-1 text-sm text-muted-foreground">
                    <MapPinIcon className="size-3.5" /> {[s.street, [s.zip, s.city].filter(Boolean).join(' ')].filter(Boolean).join(', ') || 'Bez adresy'}
                  </p>
                  {s.access_note && (
                    <p className="flex items-center gap-1 text-sm text-muted-foreground">
                      <KeyRoundIcon className="size-3.5" /> {s.access_note}
                    </p>
                  )}
                  <p className="text-xs text-muted-foreground">
                    {[s.area_m2 && `plocha ${s.area_m2} m²`, s.lawn_m2 && `trávník ${s.lawn_m2} m²`, s.hedge_bm && `živý plot ${s.hedge_bm} bm`].filter(Boolean).join(' · ')}
                  </p>
                </button>
              ))}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Zakázky</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-2">
              {jobs.data?.items.length === 0 && <p className="text-sm text-muted-foreground">Zatím žádné zakázky.</p>}
              {jobs.data?.items.map((j) => (
                <Link key={j.id} to="/a/$slug/jobs/$jobId" params={{ slug, jobId: j.id }} className="flex items-center gap-2 rounded-md p-2 hover:bg-muted/50">
                  <span className="font-mono text-xs text-muted-foreground">{j.number}</span>
                  <span className="min-w-0 flex-1 truncate">{j.title}</span>
                  <StatusPill tone={jobStatusTone[j.status]}>{jobStatusLabels[j.status]}</StatusPill>
                </Link>
              ))}
            </CardContent>
          </Card>
        </div>
        <div className="grid content-start gap-4">
          <Card>
            <CardHeader>
              <CardTitle>Kontakt</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-1 text-sm">
              {c.phone && <a href={`tel:${c.phone.replace(/\s/g, '')}`} className="text-primary">{c.phone}</a>}
              {c.email && <a href={`mailto:${c.email}`} className="text-primary">{c.email}</a>}
              <p>{[c.street, [c.zip, c.city].filter(Boolean).join(' ')].filter(Boolean).join(', ')}</p>
              {c.registration_no && <p className="text-muted-foreground">IČO {c.registration_no}</p>}
              {c.note && <p className="mt-2 whitespace-pre-wrap text-muted-foreground">{c.note}</p>}
            </CardContent>
          </Card>
          <HistorySection subjectType="client" subjectId={c.id} />
        </div>
      </PageBody>
      {editing && <EditClientDialog client={c} onClose={() => setEditing(false)} />}
      {site && <SiteDialog clientId={c.id} site={site === 'new' ? null : site} onClose={() => setSite(null)} />}
    </>
  )
}

function EditClientDialog({ client, onClose }: { client: ClientDetail; onClose: () => void }) {
  const { slug } = useCurrentAccount()
  const update = useUpdateClient(slug, client.id)
  const [f, setF] = useState({
    name: client.name, kind: client.kind, phone: client.phone, email: client.email, street: client.street,
    city: client.city, zip: client.zip, registration_no: client.registration_no, vat_no: client.vat_no, note: client.note,
  })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title="Upravit klienta"
      footer={<Button disabled={!f.name.trim() || update.isPending} onClick={() => update.mutate(f, { onSuccess: onClose })}>Uložit</Button>}
    >
      <FormGrid>
        <TextField label="Jméno / název" name="name" value={f.name} onChange={set('name')} className="sm:col-span-2" />
        <SelectField label="Typ" name="kind" value={f.kind} onChange={set('kind')} options={[{ value: 'person', label: 'Soukromá osoba' }, { value: 'company', label: 'Firma' }]} />
        <TextField label="IČO" name="registration_no" value={f.registration_no} onChange={set('registration_no')} />
        <TextField label="DIČ" name="vat_no" value={f.vat_no} onChange={set('vat_no')} />
        <TextField label="Telefon" name="phone" type="tel" value={f.phone} onChange={set('phone')} />
        <TextField label="E-mail" name="email" type="email" value={f.email} onChange={set('email')} />
        <TextField label="Ulice" name="street" value={f.street} onChange={set('street')} />
        <TextField label="Obec" name="city" value={f.city} onChange={set('city')} />
        <TextField label="PSČ" name="zip" value={f.zip} onChange={set('zip')} />
        <TextAreaField label="Poznámka" name="note" value={f.note} onChange={set('note')} className="sm:col-span-2" />
      </FormGrid>
    </ResponsiveDialog>
  )
}

const numOrUndef = (v: string) => (v.trim() === '' ? undefined : Math.max(0, Math.round(Number(v.replace(',', '.')) || 0)))

function SiteDialog({ clientId, site, onClose }: { clientId: number; site: Site | null; onClose: () => void }) {
  const { slug } = useCurrentAccount()
  const create = useCreateSite(slug)
  const update = useUpdateSite(slug)
  const del = useDeleteSite(slug)
  const [f, setF] = useState({
    name: site?.name ?? 'Zahrada', street: site?.street ?? '', city: site?.city ?? '', zip: site?.zip ?? '',
    access_note: site?.access_note ?? '', note: site?.note ?? '',
    area_m2: site?.area_m2?.toString() ?? '', lawn_m2: site?.lawn_m2?.toString() ?? '', hedge_bm: site?.hedge_bm?.toString() ?? '',
  })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  const body = {
    name: f.name, street: f.street, city: f.city, zip: f.zip, access_note: f.access_note, note: f.note,
    area_m2: numOrUndef(f.area_m2), lawn_m2: numOrUndef(f.lawn_m2), hedge_bm: numOrUndef(f.hedge_bm),
  }
  const save = () =>
    site
      ? update.mutate({ id: site.id, ...body }, { onSuccess: onClose })
      : create.mutate({ client_id: clientId, ...body }, { onSuccess: onClose })
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={site ? 'Upravit zahradu' : 'Nová zahrada'}
      footer={
        <div className="flex w-full flex-wrap justify-between gap-2">
          {site ? (
            <Button
              variant="ghost"
              onClick={() => confirm('Smazat zahradu?') && del.mutate(site.id, { onSuccess: onClose })}
            >
              <Trash2Icon data-icon="inline-start" /> Smazat
            </Button>
          ) : (
            <span />
          )}
          <Button disabled={!f.name.trim() || create.isPending || update.isPending} onClick={save}>
            Uložit
          </Button>
        </div>
      }
    >
      <FormGrid>
        <TextField label="Název" name="name" value={f.name} onChange={set('name')} className="sm:col-span-2" />
        <TextField label="Ulice" name="street" value={f.street} onChange={set('street')} className="sm:col-span-2" />
        <TextField label="Obec" name="city" value={f.city} onChange={set('city')} />
        <TextField label="PSČ" name="zip" value={f.zip} onChange={set('zip')} inputMode="numeric" />
        <TextField label="Plocha (m²)" name="area_m2" value={f.area_m2} onChange={set('area_m2')} inputMode="numeric" />
        <TextField label="Trávník (m²)" name="lawn_m2" value={f.lawn_m2} onChange={set('lawn_m2')} inputMode="numeric" />
        <TextField label="Živý plot (bm)" name="hedge_bm" value={f.hedge_bm} onChange={set('hedge_bm')} inputMode="numeric" />
        <TextAreaField label="Přístup (klíč, kód brány, pes…)" name="access_note" value={f.access_note} onChange={set('access_note')} className="sm:col-span-2" />
        <TextAreaField label="Poznámka" name="note" value={f.note} onChange={set('note')} className="sm:col-span-2" />
      </FormGrid>
    </ResponsiveDialog>
  )
}
