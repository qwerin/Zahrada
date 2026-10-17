import { useEffect, useMemo, useState } from 'react'

// Zaváděcí obrazovka (fáze B): náhled kalkulace nad /api/calc/preview a stav serveru.
// Ve fázi F0 ji nahradí aplikační shell převzatý z NanoFaktury (TanStack Router, Tailwind, shadcn/ui).

type Kind = 'work' | 'material' | 'machine' | 'transport' | 'subcontract' | 'other'

type Row = {
  id: number
  section: string
  name: string
  unit: string
  kind: Kind
  quantity: string
  price: string // prodejní cena v Kč za jednotku; prázdná = nákupní cena + přirážka
  unit_cost: number // haléře
  markup_bps?: number
  waste_bps?: number
  discount_bps?: number
  norm_hours?: string
}

type Subtotal = { name: string; cost: number; net: number }
type Result = {
  items: { effective_quantity: string; unit_price: number; net: number; hours: string }[]
  sections: Subtotal[]
  vat: { rate_bps: number; base: number; vat: number }[]
  subtotal: number
  quote_discount: number
  net: number
  vat_total: number
  gross: number
  cost: number
  margin: number
  margin_bps: number
  hours: string
}
type Problem = { detail?: string; field?: string }

const SAMPLE: Row[] = [
  { id: 1, section: 'Příprava terénu', name: 'Frézování půdy', unit: 'm²', kind: 'work', quantity: '250', price: '15', unit_cost: 600, norm_hours: '0.02' },
  { id: 2, section: 'Příprava terénu', name: 'Odvoz kamení a kořenů', unit: 'fůra', kind: 'transport', quantity: '2', price: '', unit_cost: 120000 },
  { id: 3, section: 'Trávník', name: 'Travní osivo, sportovní směs', unit: 'kg', kind: 'material', quantity: '7,5', price: '', unit_cost: 18000, markup_bps: 3000, waste_bps: 1000 },
  { id: 4, section: 'Trávník', name: 'Trávníkový substrát', unit: 'm³', kind: 'material', quantity: '12,5', price: '1290', unit_cost: 95000, discount_bps: 500 },
  { id: 5, section: 'Trávník', name: 'Výsev a zaválcování', unit: 'm²', kind: 'work', quantity: '250', price: '22', unit_cost: 900, norm_hours: '0.03' },
  { id: 6, section: 'Živý plot', name: "Thuja occidentalis 'Smaragd' 80–100 cm", unit: 'ks', kind: 'material', quantity: '24', price: '', unit_cost: 29000, markup_bps: 4000 },
  { id: 7, section: 'Živý plot', name: 'Výsadba keře vč. hnojení a mulče', unit: 'ks', kind: 'work', quantity: '24', price: '180', unit_cost: 8750, norm_hours: '0.25' },
]

const czk = new Intl.NumberFormat('cs-CZ', { style: 'currency', currency: 'CZK' })
const money = (h: number) => czk.format(h / 100)
const pct = (bps: number) => `${(bps / 100).toLocaleString('cs-CZ', { maximumFractionDigits: 1 })} %`
const hours = (s: string) => `${Number(s).toLocaleString('cs-CZ', { maximumFractionDigits: 2 })} h`

function toHalere(kc: string): number | undefined {
  const n = Number(kc.replace(/\s/g, '').replace(',', '.'))
  return kc.trim() === '' || !Number.isFinite(n) ? undefined : Math.round(n * 100)
}

function useHealth() {
  const [state, setState] = useState<{ ok: boolean; version?: string } | null>(null)
  useEffect(() => {
    fetch('/api/health')
      .then((r) => (r.ok ? r.json() : Promise.reject()))
      .then((j: { version: string }) => setState({ ok: true, version: j.version }))
      .catch(() => setState({ ok: false }))
  }, [])
  return state
}

export default function App() {
  const health = useHealth()
  const [rows, setRows] = useState(SAMPLE)
  const [vatPayer, setVatPayer] = useState(true)
  const [discount, setDiscount] = useState('3')
  const [result, setResult] = useState<Result | null>(null)
  const [problem, setProblem] = useState<string | null>(null)

  const body = useMemo(
    () => ({
      vat_payer: vatPayer,
      discount_bps: Math.round((Number(discount.replace(',', '.')) || 0) * 100),
      items: rows.map((r) => ({
        kind: r.kind, section: r.section, quantity: r.quantity, unit_cost: r.unit_cost,
        unit_price: toHalere(r.price), markup_bps: r.markup_bps, waste_bps: r.waste_bps,
        discount_bps: r.discount_bps, norm_hours: r.norm_hours, vat_rate_bps: 2100,
      })),
    }),
    [rows, vatPayer, discount],
  )

  useEffect(() => {
    const ctrl = new AbortController()
    const t = setTimeout(async () => {
      try {
        const res = await fetch('/api/calc/preview', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body), signal: ctrl.signal,
        })
        if (res.ok) {
          setResult(await res.json())
          setProblem(null)
          return
        }
        const p: Problem = await res.json().catch(() => ({}))
        const idx = Number(p.field?.match(/^items\[(\d+)\]/)?.[1])
        const where = Number.isInteger(idx) && rows[idx] ? `${rows[idx].name}: ` : ''
        setProblem(`${where}${p.field?.endsWith('quantity') ? 'zadejte množství jako číslo, nejvýš 3 desetinná místa.' : p.detail ?? 'neplatný údaj.'}`)
      } catch (e) {
        if ((e as Error).name !== 'AbortError') setProblem('Server neodpovídá. Zkontrolujte, že běží backend.')
      }
    }, 250)
    return () => { clearTimeout(t); ctrl.abort() }
  }, [body, rows])

  const update = (id: number, patch: Partial<Row>) =>
    setRows((rs) => rs.map((r) => (r.id === id ? { ...r, ...patch } : r)))

  const sections = [...new Set(rows.map((r) => r.section))]
  const costShare = result && result.net > 0 ? Math.min(100, Math.max(0, (result.cost / result.net) * 100)) : 0

  return (
    <div className="page">
      <header className="top">
        <span className="brand">Zahrada</span>
        <span className={`health ${health?.ok ? 'up' : health ? 'down' : ''}`} role="status">
          {health === null ? 'Ověřuji server…' : health.ok ? `Server běží, verze ${health.version}` : 'Server nedostupný'}
        </span>
      </header>

      <main>
        <h1>Realizace trávníku a živého plotu</h1>
        <p className="lede">Náhled kalkulace pro zahradu o 250 m². Upravte množství nebo cenu, součty se přepočítají na serveru.</p>

        <div className="options">
          <label className="check">
            <input type="checkbox" checked={vatPayer} onChange={(e) => setVatPayer(e.target.checked)} />
            Plátce DPH
          </label>
          <label className="inline">
            Sleva na zakázku
            <input className="num short" inputMode="decimal" value={discount} onChange={(e) => setDiscount(e.target.value)} />
            %
          </label>
        </div>

        {sections.map((s) => {
          const sub = result?.sections.find((x) => x.name === s)
          return (
            <section key={s} className="section">
              <h2>
                <span>{s}</span>
                {sub && <span className="num">{money(sub.net)}</span>}
              </h2>
              {rows.map((r, i) => r.section !== s ? null : (
                <div key={r.id} className="row">
                  <div className="name">
                    {r.name}
                    {r.waste_bps ? <small> vč. ztratného {pct(r.waste_bps)}</small> : null}
                    {r.discount_bps ? <small> sleva {pct(r.discount_bps)}</small> : null}
                  </div>
                  <label className="qty">
                    <input className="num" inputMode="decimal" aria-label={`Množství: ${r.name}`}
                      value={r.quantity} onChange={(e) => update(r.id, { quantity: e.target.value })} />
                    <span>{r.unit}</span>
                  </label>
                  <label className="price">
                    <input className="num" inputMode="decimal" aria-label={`Cena za ${r.unit}: ${r.name}`}
                      placeholder={result?.items[i] ? (result.items[i].unit_price / 100).toLocaleString('cs-CZ') : ''}
                      value={r.price} onChange={(e) => update(r.id, { price: e.target.value })} />
                    <span>Kč/{r.unit}</span>
                  </label>
                  <div className="net num">{result?.items[i] ? money(result.items[i].net) : '–'}</div>
                </div>
              ))}
            </section>
          )
        })}

        {problem && <p className="problem" role="alert">{problem}</p>}

        {result && (
          <section className="summary" aria-label="Souhrn kalkulace">
            <div className="bar" aria-hidden="true">
              <div className="cost" style={{ width: `${costShare}%` }} />
              <div className="margin" />
            </div>
            <dl className="split">
              <div><dt>Náklady</dt><dd className="num">{money(result.cost)}</dd></div>
              <div className="m"><dt>Marže {pct(result.margin_bps)}</dt><dd className="num">{money(result.margin)}</dd></div>
              <div><dt>Odhad práce</dt><dd className="num">{hours(result.hours)}</dd></div>
            </dl>
            <dl className="totals">
              {result.quote_discount > 0 && <div><dt>Sleva na zakázku</dt><dd className="num">−{money(result.quote_discount)}</dd></div>}
              <div><dt>Bez DPH</dt><dd className="num">{money(result.net)}</dd></div>
              {result.vat.filter((v) => v.rate_bps > 0).map((v) => (
                <div key={v.rate_bps}><dt>DPH {pct(v.rate_bps)}</dt><dd className="num">{money(v.vat)}</dd></div>
              ))}
              <div className="gross"><dt>{vatPayer ? 'Celkem s DPH' : 'Celkem'}</dt><dd className="num">{money(result.gross)}</dd></div>
            </dl>
          </section>
        )}
      </main>
    </div>
  )
}
