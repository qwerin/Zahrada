import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import {
  ChevronDownIcon,
  ClipboardPasteIcon,
  CopyPlusIcon,
  PencilIcon,
  PlusIcon,
  PrinterIcon,
  SaveIcon,
  SendIcon,
  TagsIcon,
  ThumbsDownIcon,
  ThumbsUpIcon,
  Trash2Icon,
} from 'lucide-react'
import { useDeferredValue, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { api, unwrap } from '@/api/client'
import { quoteQueries, useCreateQuote, useDeleteQuote, useQuoteStatus, useSaveQuote } from '@/api/queries/garden'
import type { CalcItem, PriceItem, Quote, QuoteLine } from '@/api/types'
import { FormGrid, SelectField, StatusPill, TextAreaField, TextField } from '@/components/garden/form'
import { itemKindLabels as kindLabels, quoteStatusLabels, quoteStatusTone } from '@/components/garden/labels'
import { PasteItemsDialog } from '@/components/garden/paste-dialog'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { FormSkeleton } from '@/components/skeletons'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatMoney, formatMoneyInput, formatVatRate, parseMoney } from '@/lib/money'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/quotes/$quoteId')({
  params: {
    parse: ({ quoteId }) => ({ quoteId: parseId(quoteId) }),
    stringify: ({ quoteId }) => ({ quoteId: String(quoteId) }),
  },
  head: () => ({ meta: [{ title: 'Kalkulace · Zahrada' }] }),
  component: QuotePage,
})

/** Řádek editoru: položka + lokální klíč a cena v Kč jako text. */
type Row = QuoteLine & { key: number; priceText: string; costText: string; open?: boolean }

let seq = 1
const toRow = (l: QuoteLine): Row => ({
  ...l,
  key: seq++,
  priceText: l.unit_price != null ? formatMoneyInput(l.unit_price) : '',
  costText: l.unit_cost ? formatMoneyInput(l.unit_cost) : '',
})

const pct = (bps?: number) => (bps ? String(bps / 100).replace('.', ',') : '')
const toBps = (s: string) => Math.round((Number(s.replace(',', '.')) || 0) * 100)

function rowToLine(r: Row): QuoteLine {
  const price = r.priceText.trim() ? parseMoney(r.priceText) : null
  const cost = r.costText.trim() ? parseMoney(r.costText) : 0
  return {
    kind: r.kind, section: r.section, name: r.name, unit: r.unit, price_item_id: r.price_item_id, note: r.note,
    quantity: (r.quantity || '0').replace(',', '.'), waste_bps: r.waste_bps, unit_cost: cost ?? 0,
    unit_price: price ?? undefined, markup_bps: r.markup_bps, discount_bps: r.discount_bps,
    vat_rate_bps: r.vat_rate_bps, norm_hours: (r.norm_hours || '').replace(',', '.'),
  }
}

function QuotePage() {
  const { slug, quoteId } = Route.useParams()
  const q = useQuery(quoteQueries.quote(slug, quoteId))
  if (q.isError) return <PageError error={q.error} reset={() => void q.refetch()} />
  if (!q.data) return <FormSkeleton />
  return <QuoteEditor key={`${q.data.id}-${q.data.status}`} quote={q.data} />
}

function QuoteEditor({ quote }: { quote: Quote }) {
  const { slug, canEdit } = useCurrentAccount()
  const navigate = useNavigate()
  const editable = canEdit && quote.status === 'draft'
  const save = useSaveQuote(slug, quote.id)
  const status = useQuoteStatus(slug, quote.id)
  const newVersion = useCreateQuote(slug, quote.job_id)
  const del = useDeleteQuote(slug)
  const [rows, setRows] = useState<Row[]>(() => quote.items.map(toRow))
  const [head, setHead] = useState({ title: quote.title, valid_until: quote.valid_until ?? '', discount: pct(quote.discount_bps), intro: quote.intro, note: quote.note })
  const [dirty, setDirty] = useState(false)
  const [picking, setPicking] = useState(false)
  const [pasting, setPasting] = useState(false)
  const defaultVat = 2100

  const lines = rows.map(rowToLine)
  const preview = useDeferredValue({ discount_bps: toBps(head.discount), items: lines.map(({ name: _n, unit: _u, price_item_id: _p, note: _no, ...c }) => c as CalcItem) })
  const live = useQuery({
    queryKey: ['a', slug, 'calc-preview', preview],
    queryFn: () => unwrap(api.POST('/api/accounts/{slug}/calc/preview', { params: { path: { slug } }, body: preview })),
    placeholderData: (p) => p,
    enabled: editable,
    retry: false,
    meta: { silent: true },
  })
  const result = editable ? (live.data ?? quote.result) : quote.result

  useEffect(() => {
    if (!dirty) return
    const h = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', h)
    return () => window.removeEventListener('beforeunload', h)
  }, [dirty])

  const patchRow = (key: number, patch: Partial<Row>) => {
    setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)))
    setDirty(true)
  }
  const addRows = (more: Row[]) => {
    setRows((rs) => [...rs, ...more])
    setDirty(true)
  }
  const lastSection = rows.at(-1)?.section ?? ''
  const blank = (): Row => toRow({ kind: 'work', section: lastSection, name: '', unit: 'm²', quantity: '1', vat_rate_bps: defaultVat })

  const doSave = (then?: () => void) =>
    save.mutate(
      { title: head.title || quote.title, valid_until: head.valid_until || undefined, discount_bps: toBps(head.discount), intro: head.intro, note: head.note, items: rows.filter((r) => r.name.trim()).map(rowToLine) },
      { onSuccess: () => { setDirty(false); toast.success('Kalkulace uložena'); then?.() } },
    )
  const setStatus = (s: 'draft' | 'sent' | 'accepted' | 'rejected', msg: string) => {
    const go = () => status.mutate(s, { onSuccess: () => toast.success(msg) })
    if (dirty && editable) doSave(go)
    else go()
  }

  const actions: PageAction[] = []
  if (editable) {
    actions.push({ label: 'Uložit', icon: SaveIcon, primary: true, onClick: () => doSave() })
    actions.push({ label: 'Odeslat klientovi', icon: SendIcon, onClick: () => setStatus('sent', 'Nabídka označena jako odeslaná') })
    actions.push({ label: 'Přijato klientem', icon: ThumbsUpIcon, overflow: true, onClick: () => setStatus('accepted', 'Nabídka přijata, zakázka objednána') })
  }
  if (canEdit && quote.status === 'sent') {
    actions.push({ label: 'Přijato klientem', icon: ThumbsUpIcon, primary: true, onClick: () => setStatus('accepted', 'Nabídka přijata, zakázka objednána') })
    actions.push({ label: 'Odmítnuto', icon: ThumbsDownIcon, onClick: () => setStatus('rejected', 'Nabídka odmítnuta') })
    actions.push({ label: 'Upravit (vrátit do rozpracovaných)', icon: PencilIcon, overflow: true, onClick: () => setStatus('draft', 'Nabídku lze upravit') })
  }
  if (canEdit && quote.status === 'rejected') actions.push({ label: 'Upravit', icon: PencilIcon, onClick: () => setStatus('draft', 'Nabídku lze upravit') })
  actions.push({ label: 'Tisk / PDF', icon: PrinterIcon, render: <a href={`/tisk/${slug}/nabidka/${quote.id}`} target="_blank" rel="noreferrer" /> })
  if (canEdit) {
    actions.push({
      label: 'Nová verze',
      icon: CopyPlusIcon,
      overflow: true,
      onClick: () => newVersion.mutate(quote.id, { onSuccess: (n) => void navigate({ to: '/a/$slug/quotes/$quoteId', params: { slug, quoteId: n.id } }) }),
    })
    if (quote.status !== 'accepted')
      actions.push({
        label: 'Smazat',
        icon: Trash2Icon,
        overflow: true,
        variant: 'destructive',
        onClick: () =>
          confirm('Smazat tuto verzi kalkulace?') &&
          del.mutate(quote.id, { onSuccess: () => void navigate({ to: '/a/$slug/jobs/$jobId', params: { slug, jobId: quote.job_id } }) }),
      })
  }

  let prevSection: string | undefined
  return (
    <>
      <PageHeader
        title={head.title || quote.title}
        description={
          <span className="flex flex-wrap items-center gap-2">
            <span className="font-mono">{quote.number}</span>
            <StatusPill tone={quoteStatusTone[quote.status]}>{quoteStatusLabels[quote.status]}</StatusPill>
            <Link to="/a/$slug/jobs/$jobId" params={{ slug, jobId: quote.job_id }} className="text-primary">
              {quote.job.title} · {quote.job.client_name}
            </Link>
            {dirty && <span className="text-amber-600">neuloženo</span>}
          </span>
        }
        back={{ to: '/a/$slug/jobs/$jobId', params: { slug, jobId: quote.job_id } }}
        actions={actions}
      />
      <PageBody className="grid gap-4 lg:grid-cols-[1fr_20rem]">
        <div className="grid content-start gap-3">
          {editable && (
            <Card>
              <CardContent className="py-3">
                <FormGrid>
                  <TextField label="Název nabídky" name="title" value={head.title} onChange={(e) => { setHead({ ...head, title: e.target.value }); setDirty(true) }} className="sm:col-span-2" />
                  <TextField label="Platnost do" name="valid_until" type="date" value={head.valid_until} onChange={(e) => { setHead({ ...head, valid_until: e.target.value }); setDirty(true) }} />
                  <TextField label="Sleva na celou nabídku (%)" name="discount" inputMode="decimal" value={head.discount} onChange={(e) => { setHead({ ...head, discount: e.target.value }); setDirty(true) }} />
                </FormGrid>
              </CardContent>
            </Card>
          )}
          {rows.length === 0 && (
            <Card>
              <CardContent className="grid justify-items-start gap-2 py-6 text-sm text-muted-foreground">
                Kalkulace je prázdná. Přidejte položky z ceníku, vložte je z textu (třeba z chatu s Claude), nebo je napište ručně.
              </CardContent>
            </Card>
          )}
          {rows.map((r, i) => {
            const res = result.items[i]
            const header = r.section !== prevSection && r.section ? <h3 className="mt-2 text-sm font-semibold">{r.section}</h3> : null
            prevSection = r.section
            return (
              <div key={r.key} className="grid gap-1">
                {header}
                <Card className="py-0">
                  <CardContent className="grid gap-2 py-3">
                    {editable ? (
                      <>
                        <div className="flex gap-2">
                          <Input value={r.name} placeholder="Název položky" aria-label="Název" onChange={(e) => patchRow(r.key, { name: e.target.value })} className="flex-1" />
                          <Button variant="ghost" size="icon" aria-label="Odebrat" onClick={() => { setRows(rows.filter((x) => x.key !== r.key)); setDirty(true) }}>
                            <Trash2Icon />
                          </Button>
                        </div>
                        <div className="grid grid-cols-[1fr_4.5rem_1fr] items-end gap-2 sm:grid-cols-[7rem_5rem_9rem_1fr_auto]">
                          <Input value={r.quantity.replace('.', ',')} inputMode="decimal" aria-label="Množství" className="text-right" onChange={(e) => patchRow(r.key, { quantity: e.target.value })} />
                          <Input value={r.unit ?? ''} aria-label="Jednotka" onChange={(e) => patchRow(r.key, { unit: e.target.value })} />
                          <InputGroup>
                            <InputGroupInput
                              value={r.priceText}
                              inputMode="decimal"
                              aria-label="Cena za jednotku"
                              className="text-right"
                              placeholder={res ? formatMoneyInput(res.unit_price) : ''}
                              onChange={(e) => patchRow(r.key, { priceText: e.target.value })}
                            />
                            <InputGroupAddon align="inline-end">Kč</InputGroupAddon>
                          </InputGroup>
                          <span className="col-span-2 text-right font-medium tabular-nums sm:col-span-1">{res ? formatMoney(res.net) : '–'}</span>
                          <Button variant="ghost" size="sm" onClick={() => patchRow(r.key, { open: !r.open })} aria-expanded={!!r.open}>
                            Více <ChevronDownIcon className={r.open ? 'rotate-180' : ''} />
                          </Button>
                        </div>
                        {r.open && (
                          <div className="grid grid-cols-2 gap-2 border-t pt-2 sm:grid-cols-4">
                            <TextField label="Oddíl" name={`sec-${r.key}`} value={r.section ?? ''} onChange={(e) => patchRow(r.key, { section: e.target.value })} />
                            <SelectField label="Druh" name={`kind-${r.key}`} value={r.kind} onChange={(e) => patchRow(r.key, { kind: e.target.value as Row['kind'] })} options={Object.entries(kindLabels).map(([value, label]) => ({ value, label }))} />
                            <TextField label="Nákup / j (Kč)" name={`cost-${r.key}`} inputMode="decimal" value={r.costText} onChange={(e) => patchRow(r.key, { costText: e.target.value })} />
                            <TextField label="Přirážka %" name={`mk-${r.key}`} inputMode="decimal" value={pct(r.markup_bps)} onChange={(e) => patchRow(r.key, { markup_bps: toBps(e.target.value) })} />
                            <TextField label="Ztratné %" name={`ws-${r.key}`} inputMode="decimal" value={pct(r.waste_bps)} onChange={(e) => patchRow(r.key, { waste_bps: toBps(e.target.value) })} />
                            <TextField label="Sleva %" name={`dc-${r.key}`} inputMode="decimal" value={pct(r.discount_bps)} onChange={(e) => patchRow(r.key, { discount_bps: toBps(e.target.value) })} />
                            <TextField label="Pracnost h/j" name={`nh-${r.key}`} inputMode="decimal" value={(r.norm_hours ?? '').replace('.', ',')} onChange={(e) => patchRow(r.key, { norm_hours: e.target.value })} />
                            <SelectField label="DPH" name={`vat-${r.key}`} value={String(r.vat_rate_bps)} onChange={(e) => patchRow(r.key, { vat_rate_bps: Number(e.target.value) })} options={[{ value: '2100', label: '21 %' }, { value: '1200', label: '12 %' }, { value: '0', label: '0 %' }]} />
                          </div>
                        )}
                      </>
                    ) : (
                      <div className="flex flex-wrap items-baseline gap-x-3 text-sm">
                        <span className="min-w-0 flex-1 font-medium">{r.name}</span>
                        <span className="tabular-nums text-muted-foreground">
                          {r.quantity.replace('.', ',')} {r.unit} × {res ? formatMoney(res.unit_price) : ''}
                        </span>
                        <span className="w-28 text-right font-medium tabular-nums">{res ? formatMoney(res.net) : ''}</span>
                      </div>
                    )}
                  </CardContent>
                </Card>
              </div>
            )
          })}
          {editable && (
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" size="sm" onClick={() => addRows([blank()])}>
                <PlusIcon data-icon="inline-start" /> Položka
              </Button>
              <Button variant="outline" size="sm" onClick={() => setPicking(true)}>
                <TagsIcon data-icon="inline-start" /> Z ceníku
              </Button>
              <Button variant="outline" size="sm" onClick={() => setPasting(true)}>
                <ClipboardPasteIcon data-icon="inline-start" /> Vložit z textu
              </Button>
            </div>
          )}
          {editable ? (
            <Card>
              <CardContent className="grid gap-3 py-3">
                <TextAreaField label="Úvodní text pro klienta" name="intro" value={head.intro} onChange={(e) => { setHead({ ...head, intro: e.target.value }); setDirty(true) }} />
                <TextAreaField label="Poznámka a podmínky" name="note" value={head.note} onChange={(e) => { setHead({ ...head, note: e.target.value }); setDirty(true) }} />
              </CardContent>
            </Card>
          ) : (
            (quote.intro || quote.note) && (
              <Card>
                <CardContent className="grid gap-2 whitespace-pre-wrap py-3 text-sm">
                  {quote.intro && <p>{quote.intro}</p>}
                  {quote.note && <p className="text-muted-foreground">{quote.note}</p>}
                </CardContent>
              </Card>
            )
          )}
        </div>
        <div className="lg:sticky lg:top-4 lg:self-start">
          <Card>
            <CardHeader>
              <CardTitle>Souhrn</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-3 text-sm">
              {result.cost > 0 && (
                <div className="flex h-2.5 overflow-hidden rounded-full bg-amber-500">
                  <div className="bg-primary" style={{ width: `${result.net > 0 ? Math.min(100, (result.cost / result.net) * 100) : 0}%` }} />
                </div>
              )}
              <div className="grid grid-cols-2 gap-2">
                <div><p className="text-muted-foreground">Náklady</p><p className="font-semibold tabular-nums">{result.cost > 0 ? formatMoney(result.cost) : '–'}</p></div>
                <div><p className="text-amber-600">Marže</p><p className="font-semibold tabular-nums">{result.cost > 0 ? `${formatMoney(result.margin)} (${(result.margin_bps / 100).toLocaleString('cs-CZ')} %)` : '–'}</p></div>
                <div><p className="text-muted-foreground">Odhad práce</p><p className="font-semibold tabular-nums">{Number(result.hours).toLocaleString('cs-CZ')} h</p></div>
              </div>
              {result.sections.length > 1 && (
                <div className="grid gap-0.5 border-t pt-2">
                  {result.sections.map((s) => (
                    <div key={s.name} className="flex justify-between gap-2"><span className="truncate text-muted-foreground">{s.name}</span><span className="tabular-nums">{formatMoney(s.net)}</span></div>
                  ))}
                </div>
              )}
              <div className="grid gap-0.5 border-t pt-2">
                {result.quote_discount > 0 && <div className="flex justify-between"><span className="text-muted-foreground">Sleva</span><span className="tabular-nums">−{formatMoney(result.quote_discount)}</span></div>}
                <div className="flex justify-between"><span className="text-muted-foreground">Bez DPH</span><span className="tabular-nums">{formatMoney(result.net)}</span></div>
                {result.vat.filter((v) => v.rate_bps > 0).map((v) => (
                  <div key={v.rate_bps} className="flex justify-between"><span className="text-muted-foreground">DPH {formatVatRate(v.rate_bps)}</span><span className="tabular-nums">{formatMoney(v.vat)}</span></div>
                ))}
                <div className="flex items-baseline justify-between pt-1">
                  <span className="font-medium">{result.vat_payer ? 'Celkem s DPH' : 'Celkem'}</span>
                  <span className="text-xl font-semibold tabular-nums">{formatMoney(result.gross)}</span>
                </div>
              </div>
              {editable && (
                <Button disabled={save.isPending} onClick={() => doSave()}>
                  <SaveIcon data-icon="inline-start" /> Uložit
                </Button>
              )}
            </CardContent>
          </Card>
        </div>
      </PageBody>
      {picking && (
        <PriceItemPicker
          onClose={() => setPicking(false)}
          onPick={(items) => {
            addRows(items.map((p) => toRow({
              kind: p.kind, section: p.category || lastSection, name: p.name, unit: p.unit, price_item_id: p.id, quantity: '1',
              unit_cost: p.unit_cost, unit_price: p.unit_price ?? undefined, markup_bps: p.markup_bps, waste_bps: p.waste_bps,
              norm_hours: p.norm_hours, vat_rate_bps: p.vat_rate_bps,
            })))
            setPicking(false)
          }}
        />
      )}
      <PasteItemsDialog
        open={pasting}
        onOpenChange={setPasting}
        onImport={(items) => {
          addRows(items.map((i) => toRow({ kind: i.kind, section: i.section || lastSection, name: i.name, unit: i.unit, quantity: i.quantity, unit_price: i.unitPrice, vat_rate_bps: defaultVat })))
          setPasting(false)
          toast.success(`Vloženo ${items.length} položek. Nezapomeňte uložit.`)
        }}
      />
    </>
  )
}

function PriceItemPicker({ onClose, onPick }: { onClose: () => void; onPick: (items: PriceItem[]) => void }) {
  const { slug } = useCurrentAccount()
  const [query, setQuery] = useState('')
  const [chosen, setChosen] = useState<PriceItem[]>([])
  const list = useQuery(quoteQueries.priceItems(slug, query))
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title="Přidat z ceníku"
      footer={<Button disabled={chosen.length === 0} onClick={() => onPick(chosen)}>Přidat {chosen.length || ''}</Button>}
    >
      <div className="grid gap-2">
        <Input placeholder="Hledat…" value={query} onChange={(e) => setQuery(e.target.value)} autoFocus />
        <div className="max-h-80 overflow-auto">
          {list.data?.items.length === 0 && (
            <p className="p-2 text-sm text-muted-foreground">
              Nic nenalezeno. Ceník doplníte v sekci <Link to="/a/$slug/price-items" params={{ slug }} className="text-primary">Ceník</Link>.
            </p>
          )}
          {list.data?.items.map((p) => {
            const on = chosen.some((c) => c.id === p.id)
            return (
              <button
                key={p.id}
                type="button"
                onClick={() => setChosen(on ? chosen.filter((c) => c.id !== p.id) : [...chosen, p])}
                className={`flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm ${on ? 'bg-primary/10' : 'hover:bg-muted/50'}`}
              >
                <span className="min-w-0 flex-1 truncate">
                  {p.name} {p.category && <span className="text-muted-foreground">· {p.category}</span>}
                </span>
                <span className="tabular-nums text-muted-foreground">{formatMoney(p.sale_price)} / {p.unit}</span>
              </button>
            )
          })}
        </div>
      </div>
    </ResponsiveDialog>
  )
}
