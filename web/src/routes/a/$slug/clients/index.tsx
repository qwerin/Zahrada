import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { ArchiveIcon, ChevronRightIcon, SearchIcon, UserPlusIcon, UsersIcon } from 'lucide-react'
import { useState } from 'react'
import { z } from 'zod'
import { gardenQueries, useCreateClient } from '@/api/queries/garden'
import type { Client } from '@/api/types'
import { EmptyState } from '@/components/empty-state'
import { FormGrid, SelectField, TextField } from '@/components/garden/form'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { ResponsiveList } from '@/components/responsive-list'
import { Button } from '@/components/ui/button'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCurrentAccount } from '@/hooks/use-current-account'

const searchSchema = z.object({
  query: z.string().optional().catch(undefined),
  archived: z.boolean().optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/clients/')({
  validateSearch: searchSchema,
  head: () => ({ meta: [{ title: 'Klienti · Zahrada' }] }),
  component: ClientsPage,
})

function ClientsPage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const { canEdit } = useCurrentAccount()
  const [creating, setCreating] = useState(false)
  const list = useQuery(gardenQueries.clients(slug, search.query ?? '', search.archived ?? false))
  const open = (c: Client) => void navigate({ to: '/a/$slug/clients/$clientId', params: { slug, clientId: c.id } })

  return (
    <>
      <PageHeader
        title="Klienti"
        description="Zákazníci a jejich zahrady."
        actions={canEdit ? [{ label: 'Nový klient', icon: UserPlusIcon, primary: true, onClick: () => setCreating(true) }] : []}
      >
        <div className="flex flex-col gap-2 pt-1 sm:flex-row sm:items-center">
          <InputGroup className="sm:max-w-xs">
            <InputGroupAddon>
              <SearchIcon />
            </InputGroupAddon>
            <InputGroupInput
              placeholder="Hledat jméno, obec, telefon…"
              defaultValue={search.query ?? ''}
              onChange={(e) => void navigate({ search: (p) => ({ ...p, query: e.target.value || undefined }), replace: true })}
            />
          </InputGroup>
          <Tabs
            value={search.archived ? 'archived' : 'active'}
            onValueChange={(v) => void navigate({ search: (p) => ({ ...p, archived: v === 'archived' || undefined }), replace: true })}
          >
            <TabsList className="h-11! w-full sm:w-auto md:h-8!">
              <TabsTrigger value="active" className="px-3">Aktivní</TabsTrigger>
              <TabsTrigger value="archived" className="px-3">
                <ArchiveIcon /> Archiv
              </TabsTrigger>
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
            getKey={(c) => c.id}
            onRowClick={open}
            columns={[
              { id: 'name', header: 'Jméno', cell: (c) => <span className="font-medium">{c.name}</span> },
              { id: 'city', header: 'Obec', cell: (c) => c.city },
              { id: 'phone', header: 'Telefon', cell: (c) => c.phone },
              { id: 'email', header: 'E-mail', cell: (c) => c.email },
            ]}
            renderCard={(c) => (
              <div className="flex items-center gap-3">
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium">{c.name}</p>
                  <p className="truncate text-sm text-muted-foreground">{[c.city, c.phone].filter(Boolean).join(' · ')}</p>
                </div>
                <ChevronRightIcon className="size-4 text-muted-foreground" />
              </div>
            )}
            empty={
              <EmptyState
                icon={UsersIcon}
                title={search.query ? 'Nic nenalezeno' : 'Zatím žádní klienti'}
                description={search.query ? 'Zkuste jiný výraz.' : 'Přidejte prvního klienta a jeho zahradu.'}
                action={canEdit && !search.query && <Button onClick={() => setCreating(true)}>Nový klient</Button>}
              />
            }
          />
        )}
      </PageBody>
      <NewClientDialog open={creating} onOpenChange={setCreating} onCreated={(id) => void navigate({ to: '/a/$slug/clients/$clientId', params: { slug, clientId: id } })} />
    </>
  )
}

function NewClientDialog({ open, onOpenChange, onCreated }: { open: boolean; onOpenChange: (o: boolean) => void; onCreated: (id: number) => void }) {
  const { slug } = useCurrentAccount()
  const create = useCreateClient(slug)
  const [f, setF] = useState({ name: '', kind: 'person' as 'person' | 'company', phone: '', email: '', street: '', city: '', zip: '', registration_no: '' })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Nový klient"
      footer={
        <Button
          disabled={!f.name.trim() || create.isPending}
          onClick={() => create.mutate(f, { onSuccess: (c) => { onOpenChange(false); onCreated(c.id) } })}
        >
          Vytvořit
        </Button>
      }
    >
      <FormGrid>
        <TextField label="Jméno / název" name="name" value={f.name} onChange={set('name')} autoFocus className="sm:col-span-2" />
        <SelectField label="Typ" name="kind" value={f.kind} onChange={set('kind')} options={[{ value: 'person', label: 'Soukromá osoba' }, { value: 'company', label: 'Firma' }]} />
        <TextField label="IČO" name="registration_no" value={f.registration_no} onChange={set('registration_no')} inputMode="numeric" />
        <TextField label="Telefon" name="phone" type="tel" value={f.phone} onChange={set('phone')} />
        <TextField label="E-mail" name="email" type="email" value={f.email} onChange={set('email')} />
        <TextField label="Ulice" name="street" value={f.street} onChange={set('street')} />
        <TextField label="Obec" name="city" value={f.city} onChange={set('city')} />
        <TextField label="PSČ" name="zip" value={f.zip} onChange={set('zip')} inputMode="numeric" />
      </FormGrid>
    </ResponsiveDialog>
  )
}
