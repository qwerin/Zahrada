import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { gardenQueries, useDeleteContractor, useUpdateContractor } from '@/api/queries/garden'
import type { Contractor } from '@/api/types'
import { HistorySection } from '@/components/events/timeline'
import { FormGrid, TextAreaField, TextField } from '@/components/garden/form'
import { ContractorJobsCard } from '@/components/garden/job-contractors'
import { ContractorAccessCard } from '@/components/garden/login-link'
import { ContractorPaymentsCard } from '@/components/garden/payments'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { FormSkeleton } from '@/components/skeletons'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatMoneyInput, parseMoney } from '@/lib/money'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/contractors/$contractorId')({
  params: {
    parse: ({ contractorId }) => ({ contractorId: parseId(contractorId) }),
    stringify: ({ contractorId }) => ({ contractorId: String(contractorId) }),
  },
  head: () => ({ meta: [{ title: 'Dodavatel · Zahrada' }] }),
  component: ContractorPage,
})

function ContractorPage() {
  const { slug, contractorId } = Route.useParams()
  const { canEdit, canManageSettings } = useCurrentAccount()
  const navigate = useNavigate()
  const q = useQuery(gardenQueries.contractor(slug, contractorId))
  const del = useDeleteContractor(slug)
  if (q.isError) return <PageError error={q.error} reset={() => void q.refetch()} />
  if (!q.data) return <FormSkeleton />
  const c = q.data
  const actions: PageAction[] = [
    ...(canEdit
      ? [{
          label: 'Smazat', icon: Trash2Icon, overflow: true, variant: 'destructive' as const,
          onClick: () => confirm(`Smazat dodavatele ${c.name}?`) &&
            del.mutate(c.id, { onSuccess: () => { toast.success('Dodavatel smazán'); void navigate({ to: '/a/$slug/contractors', params: { slug } }) } }),
        }]
      : []),
  ]
  return (
    <>
      <PageHeader
        title={c.name}
        description={c.login === 'email' ? `Přihlašuje se jako ${c.user_email}` : c.login === 'link' ? 'Přihlašuje se odkazem z telefonu' : 'Zatím nemá přihlášení do aplikace'}
        back={{ to: '/a/$slug/contractors', params: { slug } }}
        actions={actions}
      />
      <PageBody className="grid gap-4 lg:grid-cols-[1fr_20rem]">
        <div className="grid content-start gap-4">
          <ContractorJobsCard slug={slug} contractor={c} canEdit={canEdit} />
          <ContractorPaymentsCard slug={slug} contractorId={c.id} canEdit={canEdit} />
          <ContractorAccessCard slug={slug} contractor={c} canManage={canManageSettings} />
          <ContractorForm key={c.updated_at} contractor={c} disabled={!canEdit} />
        </div>
        <HistorySection subjectType="contractor" subjectId={c.id} />
      </PageBody>
    </>
  )
}

function ContractorForm({ contractor: c, disabled }: { contractor: Contractor; disabled: boolean }) {
  const { slug } = useCurrentAccount()
  const update = useUpdateContractor(slug, c.id)
  const [f, setF] = useState({
    name: c.name, phone: c.phone, email: c.email, registration_no: c.registration_no, note: c.note, color: c.color || '#4d7c0f',
    hourly_rate: formatMoneyInput(c.hourly_rate), km_rate: formatMoneyInput(c.km_rate), active: c.active,
  })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  const save = () => {
    const hourly = parseMoney(f.hourly_rate)
    const km = parseMoney(f.km_rate)
    if (hourly === null || km === null) return toast.error('Zkontrolujte sazby.')
    update.mutate(
      { name: f.name, phone: f.phone, email: f.email, registration_no: f.registration_no, note: f.note, color: f.color, hourly_rate: hourly, km_rate: km, active: f.active },
      { onSuccess: () => toast.success('Uloženo') },
    )
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Údaje a sazby</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-4">
        <fieldset disabled={disabled} className="grid gap-4">
          <FormGrid>
            <TextField label="Jméno / firma" name="name" value={f.name} onChange={set('name')} className="sm:col-span-2" />
            <TextField label="Telefon" name="phone" type="tel" value={f.phone} onChange={set('phone')} />
            <TextField label="E-mail" name="email" type="email" value={f.email} onChange={set('email')} />
            <TextField label="IČO" name="registration_no" value={f.registration_no} onChange={set('registration_no')} />
            <TextField label="Barva v plánu" name="color" type="color" value={f.color} onChange={set('color')} className="max-w-32" />
            <TextField label="Hodinová sazba (Kč)" name="hourly_rate" value={f.hourly_rate} onChange={set('hourly_rate')} inputMode="decimal" hint="Platí pro nově zapsanou docházku." />
            <TextField label="Sazba za km (Kč)" name="km_rate" value={f.km_rate} onChange={set('km_rate')} inputMode="decimal" />
            <TextAreaField label="Poznámka" name="note" value={f.note} onChange={set('note')} className="sm:col-span-2" />
          </FormGrid>
          <label className="flex items-center gap-2 text-sm">
            <Switch checked={f.active} onCheckedChange={(v) => setF({ ...f, active: v })} /> Aktivní (nabízí se u zakázek a v docházce)
          </label>
        </fieldset>
        {!disabled && (
          <div>
            <Button disabled={!f.name.trim() || update.isPending} onClick={save}>Uložit</Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
