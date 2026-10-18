import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { useDeferredValue, useState } from 'react'
import { api, unwrap } from '@/api/client'
import type { CalcItem } from '@/api/types'
import { PageBody, PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { formatMoney, formatVatRate } from '@/lib/money'

export const Route = createFileRoute('/a/$slug/calc')({
  head: () => ({ meta: [{ title: 'Kalkulace · Zahrada' }] }),
  component: CalcPage,
})

type Row = { id: number; name: string; unit: string; kind: CalcItem['kind']; quantity: string; price: string; unit_cost: number; markup_bps?: number; waste_bps?: number; norm_hours?: string }

const SAMPLE: Row[] = [
  { id: 1, name: 'Frézování půdy', unit: 'm²', kind: 'work', quantity: '250', price: '15', unit_cost: 600, norm_hours: '0.02' },
  { id: 2, name: 'Travní osivo (ztratné 10 %)', unit: 'kg', kind: 'material', quantity: '7,5', price: '', unit_cost: 18000, markup_bps: 3000, waste_bps: 1000 },
  { id: 3, name: 'Výsev a zaválcování', unit: 'm²', kind: 'work', quantity: '250', price: '22', unit_cost: 900, norm_hours: '0.03' },
  { id: 4, name: 'Odvoz zeminy', unit: 'fůra', kind: 'transport', quantity: '2', price: '', unit_cost: 120000 },
]

const toHalere = (kc: string) => {
  const n = Number(kc.replace(/\s/g, '').replace(',', '.'))
  return kc.trim() === '' || !Number.isFinite(n) ? undefined : Math.round(n * 100)
}

// Rychlá orientační kalkulace přes /calc/preview (bez ukládání). Ukládané kalkulace s verzemi přijdou později.
function CalcPage() {
  const { slug } = Route.useParams()
  const [rows, setRows] = useState(SAMPLE)
  const [discount, setDiscount] = useState('0')
  const body = useDeferredValue({
    discount_bps: Math.round((Number(discount.replace(',', '.')) || 0) * 100),
    items: rows.map((r) => ({
      kind: r.kind, quantity: r.quantity || '0', unit_cost: r.unit_cost, unit_price: toHalere(r.price), markup_bps: r.markup_bps,
      waste_bps: r.waste_bps, norm_hours: r.norm_hours, vat_rate_bps: 2100,
    })),
  })
  const res = useQuery({
    queryKey: ['a', slug, 'calc', body],
    queryFn: () => unwrap(api.POST('/api/accounts/{slug}/calc/preview', { params: { path: { slug } }, body })),
    placeholderData: (p) => p,
    retry: false,
    meta: { silent: true },
  })
  const update = (id: number, patch: Partial<Row>) => setRows((rs) => rs.map((r) => (r.id === id ? { ...r, ...patch } : r)))
  const r = res.data
  const costShare = r && r.net > 0 ? Math.min(100, (r.cost / r.net) * 100) : 0

  return (
    <>
      <PageHeader title="Kalkulace" description="Orientační výpočet ceny, nákladů, marže a pracnosti." />
      <PageBody className="grid gap-4">
        <Card>
          <CardContent className="grid gap-2 py-3">
            {rows.map((row, i) => (
              <div key={row.id} className="grid grid-cols-[1fr_auto] items-center gap-2 border-b pb-2 sm:grid-cols-[1fr_7rem_8rem_8rem_auto]">
                <Input value={row.name} onChange={(e) => update(row.id, { name: e.target.value })} aria-label="Položka" className="col-span-2 sm:col-span-1" />
                <label className="flex items-center gap-1 text-sm text-muted-foreground">
                  <Input value={row.quantity} onChange={(e) => update(row.id, { quantity: e.target.value })} inputMode="decimal" className="text-right" aria-label="Množství" />
                  {row.unit}
                </label>
                <label className="flex items-center gap-1 text-sm text-muted-foreground">
                  <Input value={row.price} placeholder={r?.items[i] ? String(r.items[i].unit_price / 100) : ''} onChange={(e) => update(row.id, { price: e.target.value })} inputMode="decimal" className="text-right" aria-label="Cena za jednotku" />
                  Kč
                </label>
                <span className="text-right font-medium tabular-nums">{r?.items[i] ? formatMoney(r.items[i].net) : '–'}</span>
                <Button variant="ghost" size="icon" aria-label="Odebrat" onClick={() => setRows(rows.filter((x) => x.id !== row.id))}>
                  <Trash2Icon />
                </Button>
              </div>
            ))}
            <div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setRows([...rows, { id: Date.now(), name: 'Nová položka', unit: 'ks', kind: 'other', quantity: '1', price: '0', unit_cost: 0 }])}
              >
                <PlusIcon data-icon="inline-start" /> Přidat položku
              </Button>
            </div>
            {res.isError && <p className="text-sm text-destructive">Zkontrolujte množství a ceny (čísla, nejvýš 3 desetinná místa).</p>}
          </CardContent>
        </Card>
        {r && (
          <Card>
            <CardContent className="grid gap-3 py-4">
              <div className="flex h-3 overflow-hidden rounded-full bg-amber-500">
                <div className="bg-primary transition-all" style={{ width: `${costShare}%` }} />
              </div>
              <div className="grid grid-cols-3 gap-3 text-sm">
                <div><p className="text-muted-foreground">Náklady</p><p className="font-semibold tabular-nums">{formatMoney(r.cost)}</p></div>
                <div><p className="text-amber-600">Marže {(r.margin_bps / 100).toLocaleString('cs-CZ')} %</p><p className="font-semibold tabular-nums">{formatMoney(r.margin)}</p></div>
                <div><p className="text-muted-foreground">Odhad práce</p><p className="font-semibold tabular-nums">{Number(r.hours).toLocaleString('cs-CZ')} h</p></div>
              </div>
              <div className="grid gap-1 border-t pt-3 text-sm">
                <label className="flex items-center justify-between gap-2">
                  <span className="text-muted-foreground">Sleva na zakázku (%)</span>
                  <Input value={discount} onChange={(e) => setDiscount(e.target.value)} inputMode="decimal" className="w-20 text-right" />
                </label>
                <div className="flex justify-between"><span className="text-muted-foreground">Bez DPH</span><span className="tabular-nums">{formatMoney(r.net)}</span></div>
                {r.vat.filter((v) => v.rate_bps > 0).map((v) => (
                  <div key={v.rate_bps} className="flex justify-between"><span className="text-muted-foreground">DPH {formatVatRate(v.rate_bps)}</span><span className="tabular-nums">{formatMoney(v.vat)}</span></div>
                ))}
                <div className="flex items-baseline justify-between pt-1">
                  <span className="font-medium">{r.vat_payer ? 'Celkem s DPH' : 'Celkem'}</span>
                  <span className="text-2xl font-semibold tabular-nums">{formatMoney(r.gross)}</span>
                </div>
              </div>
            </CardContent>
          </Card>
        )}
      </PageBody>
    </>
  )
}
