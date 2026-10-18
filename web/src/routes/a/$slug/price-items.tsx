import { useQuery } from '@tanstack/react-query'
import { accountQueries } from '@/api/queries/accounts'
import { createFileRoute } from '@tanstack/react-router'
import { ClipboardPasteIcon, PlusIcon, SearchIcon, TagsIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { quoteQueries, useDeletePriceItem, useImportPriceItems, useSavePriceItem } from '@/api/queries/garden'
import type { PriceItem, PriceItemInput } from '@/api/types'
import { EmptyState } from '@/components/empty-state'
import { FormGrid, SelectField, TextField } from '@/components/garden/form'
import { itemKindLabels as kindLabels } from '@/components/garden/labels'
import { PasteItemsDialog } from '@/components/garden/paste-dialog'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { ResponsiveList } from '@/components/responsive-list'
import { Button } from '@/components/ui/button'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatMoney, formatMoneyInput, parseMoney } from '@/lib/money'

export const Route = createFileRoute('/a/$slug/price-items')({
  head: () => ({ meta: [{ title: 'Ceník · Zahrada' }] }),
  component: PriceListPage,
})

function PriceListPage() {
  const { slug } = Route.useParams()
  const { canEdit } = useCurrentAccount()
  const account = useQuery(accountQueries.detail(slug)).data
  const [query, setQuery] = useState('')
  const list = useQuery(quoteQueries.priceItems(slug, query))
  const importItems = useImportPriceItems(slug)
  const [editing, setEditing] = useState<PriceItem | 'new' | null>(null)
  const [pasting, setPasting] = useState(false)

  return (
    <>
      <PageHeader
        title="Ceník"
        description="Připravené práce a materiály pro rychlé kalkulace."
        actions={
          canEdit
            ? [
                { label: 'Nová položka', icon: PlusIcon, primary: true, onClick: () => setEditing('new') },
                { label: 'Vložit z textu', icon: ClipboardPasteIcon, onClick: () => setPasting(true) },
              ]
            : []
        }
      >
        <InputGroup className="pt-1 sm:max-w-xs">
          <InputGroupAddon>
            <SearchIcon />
          </InputGroupAddon>
          <InputGroupInput placeholder="Hledat v ceníku…" value={query} onChange={(e) => setQuery(e.target.value)} />
        </InputGroup>
      </PageHeader>
      <PageBody>
        {list.isError ? (
          <PageError error={list.error} reset={() => void list.refetch()} />
        ) : (
          <ResponsiveList
            items={list.data?.items}
            isLoading={list.isPending}
            getKey={(i) => i.id}
            onRowClick={canEdit ? (i) => setEditing(i) : undefined}
            columns={[
              { id: 'name', header: 'Položka', cell: (i) => <span className="font-medium">{i.name}</span> },
              { id: 'cat', header: 'Kategorie', cell: (i) => i.category || kindLabels[i.kind] },
              { id: 'unit', header: 'Jednotka', cell: (i) => i.unit },
              { id: 'cost', header: 'Nákup', align: 'right', cell: (i) => (i.unit_cost ? formatMoney(i.unit_cost) : '–') },
              { id: 'price', header: 'Prodej', align: 'right', cell: (i) => <span className="font-medium">{formatMoney(i.sale_price)}</span> },
              { id: 'norm', header: 'Pracnost', align: 'right', cell: (i) => (Number(i.norm_hours) ? `${i.norm_hours.replace('.', ',')} h/${i.unit}` : '') },
            ]}
            renderCard={(i) => (
              <div className="flex items-center gap-3">
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium">{i.name}</p>
                  <p className="truncate text-sm text-muted-foreground">{i.category || kindLabels[i.kind]}</p>
                </div>
                <span className="text-right text-sm tabular-nums">
                  {formatMoney(i.sale_price)}
                  {i.unit && <span className="text-muted-foreground"> / {i.unit}</span>}
                </span>
              </div>
            )}
            empty={
              <EmptyState
                icon={TagsIcon}
                title={query ? 'Nic nenalezeno' : 'Ceník je prázdný'}
                description="Přidejte práce a materiály, nebo vložte celý ceník z Excelu či z chatu s Claude."
                action={canEdit && !query && <Button onClick={() => setPasting(true)}>Vložit z textu</Button>}
              />
            }
          />
        )}
      </PageBody>
      {editing && <PriceItemDialog item={editing === 'new' ? null : editing} defaultVat={account?.default_vat_rate_bps ?? 2100} onClose={() => setEditing(null)} />}
      <PasteItemsDialog
        open={pasting}
        onOpenChange={setPasting}
        title="Vložit položky do ceníku"
        busy={importItems.isPending}
        onImport={(items) =>
          importItems.mutate(
            items.map((i) => ({ kind: i.kind, name: i.name, unit: i.unit, category: i.section, unit_price: i.unitPrice })),
            { onSuccess: (r) => { toast.success(`Přidáno ${r.total} položek`); setPasting(false) } },
          )
        }
      />
    </>
  )
}

function PriceItemDialog({ item, defaultVat, onClose }: { item: PriceItem | null; defaultVat: number; onClose: () => void }) {
  const { slug } = useCurrentAccount()
  const save = useSavePriceItem(slug)
  const del = useDeletePriceItem(slug)
  const [f, setF] = useState({
    kind: item?.kind ?? 'work',
    name: item?.name ?? '',
    unit: item?.unit ?? 'm²',
    category: item?.category ?? '',
    code: item?.code ?? '',
    unit_cost: item ? formatMoneyInput(item.unit_cost) : '',
    unit_price: item?.unit_price != null ? formatMoneyInput(item.unit_price) : '',
    markup: item ? String(item.markup_bps / 100) : '30',
    waste: item ? String(item.waste_bps / 100) : '0',
    norm_hours: item?.norm_hours && item.norm_hours !== '0' ? item.norm_hours.replace('.', ',') : '',
    vat: String(item?.vat_rate_bps ?? defaultVat),
  })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  const submit = () => {
    const cost = f.unit_cost ? parseMoney(f.unit_cost) : 0
    const price = f.unit_price ? parseMoney(f.unit_price) : undefined
    if (cost === null || price === null) return toast.error('Zkontrolujte ceny.')
    const body: PriceItemInput & { id?: number } = {
      id: item?.id, kind: f.kind as PriceItemInput['kind'], name: f.name, unit: f.unit, category: f.category, code: f.code,
      unit_cost: cost, unit_price: price, markup_bps: Math.round((Number(f.markup.replace(',', '.')) || 0) * 100),
      waste_bps: Math.round((Number(f.waste.replace(',', '.')) || 0) * 100), norm_hours: f.norm_hours, vat_rate_bps: Number(f.vat),
    }
    save.mutate(body, { onSuccess: onClose })
  }
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={item ? 'Upravit položku ceníku' : 'Nová položka ceníku'}
      footer={
        <div className="flex w-full justify-between gap-2">
          {item ? (
            <Button variant="ghost" onClick={() => confirm('Smazat položku z ceníku?') && del.mutate(item.id, { onSuccess: onClose })}>
              <Trash2Icon data-icon="inline-start" /> Smazat
            </Button>
          ) : (
            <span />
          )}
          <Button disabled={!f.name.trim() || save.isPending} onClick={submit}>Uložit</Button>
        </div>
      }
    >
      <FormGrid>
        <TextField label="Název" name="name" value={f.name} onChange={set('name')} className="sm:col-span-2" autoFocus />
        <SelectField label="Druh" name="kind" value={f.kind} onChange={set('kind')} options={Object.entries(kindLabels).map(([value, label]) => ({ value, label }))} />
        <TextField label="Jednotka" name="unit" value={f.unit} onChange={set('unit')} placeholder="m², ks, hod…" />
        <TextField label="Kategorie" name="category" value={f.category} onChange={set('category')} placeholder="Trávník, Výsadba…" />
        <TextField label="Kód" name="code" value={f.code} onChange={set('code')} />
        <TextField label="Nákupní cena / j (Kč)" name="unit_cost" value={f.unit_cost} onChange={set('unit_cost')} inputMode="decimal" />
        <TextField label="Prodejní cena / j (Kč)" name="unit_price" value={f.unit_price} onChange={set('unit_price')} inputMode="decimal" hint="Prázdné = nákup + přirážka." />
        <TextField label="Přirážka (%)" name="markup" value={f.markup} onChange={set('markup')} inputMode="decimal" />
        <TextField label="Ztratné (%)" name="waste" value={f.waste} onChange={set('waste')} inputMode="decimal" hint="Navýší množství materiálu." />
        <TextField label="Pracnost (h na jednotku)" name="norm_hours" value={f.norm_hours} onChange={set('norm_hours')} inputMode="decimal" />
        <SelectField label="DPH" name="vat" value={f.vat} onChange={set('vat')} options={[{ value: '2100', label: '21 %' }, { value: '1200', label: '12 %' }, { value: '0', label: '0 %' }]} />
      </FormGrid>
    </ResponsiveDialog>
  )
}
