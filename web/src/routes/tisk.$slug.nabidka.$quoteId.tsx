import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { PrinterIcon } from 'lucide-react'
import { accountQueries } from '@/api/queries/accounts'
import { quoteQueries } from '@/api/queries/garden'
import { Button } from '@/components/ui/button'
import { formatDate, todayISO } from '@/lib/date'
import { formatMoney, formatVatRate } from '@/lib/money'
import { parseId } from '@/lib/params'

// Tisková podoba nabídky pro klienta (tisk / uložení do PDF z prohlížeče). Bez nákladů a marže.
export const Route = createFileRoute('/tisk/$slug/nabidka/$quoteId')({
  params: {
    parse: ({ slug, quoteId }) => ({ slug, quoteId: parseId(quoteId) }),
    stringify: ({ slug, quoteId }) => ({ slug, quoteId: String(quoteId) }),
  },
  head: () => ({ meta: [{ title: 'Cenová nabídka' }] }),
  component: PrintQuote,
})

function PrintQuote() {
  const { slug, quoteId } = Route.useParams()
  const q = useQuery(quoteQueries.quote(slug, quoteId))
  const acc = useQuery(accountQueries.detail(slug))
  if (!q.data || !acc.data) return <p className="p-8 text-muted-foreground">Načítám…</p>
  const quote = q.data
  const a = acc.data
  const r = quote.result
  let prev: string | undefined
  return (
    <div className="mx-auto max-w-3xl bg-white p-8 text-[13px] text-black print:p-0 dark:bg-white dark:text-black">
      <div className="mb-6 flex justify-end gap-2 print:hidden">
        <Button onClick={() => window.print()}>
          <PrinterIcon data-icon="inline-start" /> Vytisknout / uložit PDF
        </Button>
      </div>
      <header className="flex justify-between gap-6 border-b pb-4">
        <div>
          <p className="text-lg font-semibold">{a.name}</p>
          <p>{[a.street, [a.zip, a.city].filter(Boolean).join(' ')].filter(Boolean).join(', ')}</p>
          <p>{[a.registration_no && `IČO ${a.registration_no}`, a.vat_no && `DIČ ${a.vat_no}`].filter(Boolean).join(' · ')}</p>
          <p>{[a.phone, a.email, a.web].filter(Boolean).join(' · ')}</p>
        </div>
        <div className="text-right">
          <p className="text-xl font-semibold">Cenová nabídka</p>
          <p className="font-mono">{quote.number}</p>
          <p>Vystaveno {formatDate(todayISO())}</p>
          {quote.valid_until && <p>Platnost do {formatDate(quote.valid_until)}</p>}
        </div>
      </header>
      <section className="grid grid-cols-2 gap-6 py-4">
        <div>
          <p className="text-xs uppercase text-neutral-500">Pro</p>
          <p className="font-medium">{quote.job.client_name}</p>
          <p>{quote.job.client_address}</p>
          {quote.job.client_registration_no && <p>IČO {quote.job.client_registration_no}</p>}
        </div>
        {quote.job.site_address && (
          <div>
            <p className="text-xs uppercase text-neutral-500">Místo realizace</p>
            <p className="font-medium">{quote.job.site_name}</p>
            <p>{quote.job.site_address}</p>
          </div>
        )}
      </section>
      <h1 className="mb-2 text-base font-semibold">{quote.title}</h1>
      {quote.intro && <p className="mb-4 whitespace-pre-wrap">{quote.intro}</p>}
      <table className="w-full border-collapse">
        <thead>
          <tr className="border-b text-left text-xs text-neutral-500">
            <th className="py-1 font-normal">Položka</th>
            <th className="py-1 text-right font-normal">Množství</th>
            <th className="py-1 text-right font-normal">Cena / j</th>
            <th className="py-1 text-right font-normal">Celkem</th>
          </tr>
        </thead>
        <tbody>
          {quote.items.map((it, i) => {
            const res = r.items[i]!
            const head = it.section && it.section !== prev
            prev = it.section
            return [
              head && (
                <tr key={`s${i}`} className="break-inside-avoid">
                  <td colSpan={4} className="pt-3 pb-1 font-semibold">
                    {it.section}
                    <span className="float-right tabular-nums font-normal text-neutral-500">
                      {formatMoney(r.sections.find((s) => s.name === it.section)?.net ?? 0)}
                    </span>
                  </td>
                </tr>
              ),
              <tr key={i} className="break-inside-avoid border-b border-neutral-200">
                <td className="py-1 pr-2">{it.name}</td>
                <td className="py-1 text-right tabular-nums whitespace-nowrap">{res.effective_quantity.replace('.', ',')} {it.unit}</td>
                <td className="py-1 text-right tabular-nums whitespace-nowrap">{formatMoney(res.unit_price)}</td>
                <td className="py-1 text-right tabular-nums whitespace-nowrap">{formatMoney(res.net)}</td>
              </tr>,
            ]
          })}
        </tbody>
      </table>
      <div className="ml-auto mt-4 grid w-72 gap-0.5 break-inside-avoid">
        {r.quote_discount > 0 && <Line label="Sleva" value={`−${formatMoney(r.quote_discount)}`} />}
        <Line label={r.vat_payer ? 'Celkem bez DPH' : 'Celkem'} value={formatMoney(r.net)} />
        {r.vat.filter((v) => v.rate_bps > 0).map((v) => (
          <Line key={v.rate_bps} label={`DPH ${formatVatRate(v.rate_bps)} ze ${formatMoney(v.base)}`} value={formatMoney(v.vat)} />
        ))}
        {r.vat_payer && <Line label="Celkem s DPH" value={formatMoney(r.gross)} strong />}
        {!r.vat_payer && <p className="text-xs text-neutral-500">Nejsme plátci DPH.</p>}
      </div>
      {quote.note && <p className="mt-6 whitespace-pre-wrap border-t pt-3 text-neutral-700">{quote.note}</p>}
    </div>
  )
}

function Line({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <div className={`flex justify-between gap-4 ${strong ? 'border-t pt-1 text-base font-semibold' : ''}`}>
      <span>{label}</span>
      <span className="tabular-nums">{value}</span>
    </div>
  )
}
