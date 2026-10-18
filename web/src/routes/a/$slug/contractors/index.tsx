import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { ChevronRightIcon, HardHatIcon, PlusIcon } from 'lucide-react'
import { useState } from 'react'
import { gardenQueries, useCreateContractor } from '@/api/queries/garden'
import { EmptyState } from '@/components/empty-state'
import { FormGrid, StatusPill, TextField } from '@/components/garden/form'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { ResponsiveList } from '@/components/responsive-list'
import { Button } from '@/components/ui/button'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatMoney, parseMoney } from '@/lib/money'

export const Route = createFileRoute('/a/$slug/contractors/')({
  head: () => ({ meta: [{ title: 'Dodavatelé · Zahrada' }] }),
  component: ContractorsPage,
})

function ContractorsPage() {
  const { slug } = Route.useParams()
  const navigate = useNavigate()
  const { canEdit } = useCurrentAccount()
  const [creating, setCreating] = useState(false)
  const list = useQuery(gardenQueries.contractors(slug))
  return (
    <>
      <PageHeader
        title="Dodavatelé"
        description="Kdo pro vás pracuje a eviduje docházku."
        actions={canEdit ? [{ label: 'Nový dodavatel', icon: PlusIcon, primary: true, onClick: () => setCreating(true) }] : []}
      />
      <PageBody>
        {list.isError ? (
          <PageError error={list.error} reset={() => void list.refetch()} />
        ) : (
          <ResponsiveList
            items={list.data?.items}
            isLoading={list.isPending}
            getKey={(c) => c.id}
            onRowClick={(c) => void navigate({ to: '/a/$slug/contractors/$contractorId', params: { slug, contractorId: c.id } })}
            columns={[
              { id: 'name', header: 'Jméno', cell: (c) => <span className="font-medium">{c.name}</span> },
              { id: 'phone', header: 'Telefon', cell: (c) => c.phone },
              { id: 'rate', header: 'Sazba', align: 'right', cell: (c) => `${formatMoney(c.hourly_rate)} / h` },
              { id: 'login', header: 'Přihlášení', cell: (c) => (c.user_email ? c.user_email : <span className="text-muted-foreground">nepozván</span>) },
              { id: 'active', header: '', cell: (c) => !c.active && <StatusPill tone="bg-muted text-muted-foreground">neaktivní</StatusPill> },
            ]}
            renderCard={(c) => (
              <div className="flex items-center gap-3">
                <span className="size-3 shrink-0 rounded-full" style={{ background: c.color || 'var(--muted-foreground)' }} />
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium">
                    {c.name} {!c.active && <span className="text-xs text-muted-foreground">(neaktivní)</span>}
                  </p>
                  <p className="truncate text-sm text-muted-foreground">
                    {[c.phone, `${formatMoney(c.hourly_rate)} / h`].filter(Boolean).join(' · ')}
                  </p>
                </div>
                <ChevronRightIcon className="size-4 text-muted-foreground" />
              </div>
            )}
            empty={
              <EmptyState
                icon={HardHatIcon}
                title="Zatím žádní dodavatelé"
                description="Přidejte lidi, kteří pro vás pracují. Pak je můžete pozvat, aby si sami zapisovali docházku."
                action={canEdit && <Button onClick={() => setCreating(true)}>Nový dodavatel</Button>}
              />
            }
          />
        )}
      </PageBody>
      {creating && (
        <NewContractorDialog
          onClose={() => setCreating(false)}
          onCreated={(id) => void navigate({ to: '/a/$slug/contractors/$contractorId', params: { slug, contractorId: id } })}
        />
      )}
    </>
  )
}

function NewContractorDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (id: number) => void }) {
  const { slug } = useCurrentAccount()
  const create = useCreateContractor(slug)
  const [f, setF] = useState({ name: '', phone: '', email: '', registration_no: '', hourly_rate: '', km_rate: '' })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  const submit = () =>
    create.mutate(
      {
        name: f.name, kind: 'person', phone: f.phone, email: f.email, registration_no: f.registration_no,
        hourly_rate: f.hourly_rate ? (parseMoney(f.hourly_rate) ?? undefined) : undefined,
        km_rate: f.km_rate ? (parseMoney(f.km_rate) ?? undefined) : undefined,
      },
      { onSuccess: (c) => { onClose(); onCreated(c.id) } },
    )
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title="Nový dodavatel"
      footer={<Button disabled={!f.name.trim() || create.isPending} onClick={submit}>Vytvořit</Button>}
    >
      <FormGrid>
        <TextField label="Jméno / firma" name="name" value={f.name} onChange={set('name')} className="sm:col-span-2" autoFocus />
        <TextField label="Telefon" name="phone" type="tel" value={f.phone} onChange={set('phone')} />
        <TextField label="E-mail" name="email" type="email" value={f.email} onChange={set('email')} hint="Na něj půjde pozvánka do aplikace." />
        <TextField label="IČO" name="registration_no" value={f.registration_no} onChange={set('registration_no')} />
        <div />
        <TextField label="Hodinová sazba (Kč)" name="hourly_rate" value={f.hourly_rate} onChange={set('hourly_rate')} inputMode="decimal" hint="Prázdné = výchozí sazba firmy." />
        <TextField label="Sazba za km (Kč)" name="km_rate" value={f.km_rate} onChange={set('km_rate')} inputMode="decimal" />
      </FormGrid>
    </ResponsiveDialog>
  )
}
