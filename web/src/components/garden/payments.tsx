import { useQuery } from '@tanstack/react-query'
import { BanknoteIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { paymentQueries, useDeletePayment, usePayContractor } from '@/api/queries/garden'
import type { ContractorPayments } from '@/api/types'
import { FormGrid, SelectField, TextField } from '@/components/garden/form'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatDate, todayISO } from '@/lib/date'
import { formatMoney, formatMoneyInput, parseMoney } from '@/lib/money'

export const paymentKindLabels = { advance: 'Záloha', payout: 'Výplata' } as const
const methodLabels: Record<string, string> = { cash: 'hotově', bank: 'převodem', '': '' }

/** Zůstatek: kolik ještě dlužíme dodavateli (nebo kolik dostal navíc). */
export function BalanceLine({ balance }: { balance: ContractorPayments['balance'] }) {
  const b = balance.balance
  return (
    <div className="grid gap-1 text-sm">
      <div className="flex items-baseline justify-between gap-2">
        <span className="font-medium">{b >= 0 ? 'Zbývá vyplatit' : 'Vyplaceno navíc (záloha dopředu)'}</span>
        <span className={`text-xl font-semibold tabular-nums ${b < 0 ? 'text-amber-600' : ''}`}>{formatMoney(Math.abs(b))}</span>
      </div>
      <p className="text-muted-foreground">
        Odpracováno a schváleno {formatMoney(balance.earned)} · vyplaceno {formatMoney(balance.paid)}
        {balance.pending_count > 0 && ` · ${balance.pending_count} záznamů docházky čeká na schválení`}
      </p>
    </div>
  )
}

export function PaymentList({ data, onDelete }: { data: ContractorPayments; onDelete?: (id: number) => void }) {
  if (data.items.length === 0) return <p className="text-sm text-muted-foreground">Zatím žádné zálohy ani výplaty.</p>
  return (
    <div className="grid">
      {data.items.map((p) => (
        <div key={p.id} className="flex flex-wrap items-center gap-x-3 gap-y-0.5 border-b py-2 text-sm last:border-b-0">
          <span className="w-20 tabular-nums">{formatDate(p.paid_on)}</span>
          <span className="w-16 font-medium">{paymentKindLabels[p.kind]}</span>
          <span className="min-w-0 flex-1 truncate text-muted-foreground">{[methodLabels[p.method], p.note].filter(Boolean).join(' · ')}</span>
          <span className="tabular-nums font-medium">{formatMoney(p.amount)}</span>
          {onDelete && p.editable && (
            <Button variant="ghost" size="icon" aria-label="Smazat" onClick={() => confirm('Smazat platbu?') && onDelete(p.id)}>
              <Trash2Icon />
            </Button>
          )}
        </div>
      ))}
    </div>
  )
}

/** Karta „Zálohy a výplaty“ v detailu dodavatele. */
export function ContractorPaymentsCard({ slug, contractorId, canEdit }: { slug: string; contractorId: number; canEdit: boolean }) {
  const q = useQuery(paymentQueries.contractor(slug, contractorId))
  const del = useDeletePayment(slug)
  const [paying, setPaying] = useState<'advance' | 'payout' | null>(null)
  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle>Zálohy a výplaty</CardTitle>
        {canEdit && (
          <div className="flex gap-2">
            <Button size="sm" onClick={() => setPaying('advance')}>
              <BanknoteIcon data-icon="inline-start" /> Vyplatit zálohu
            </Button>
            <Button size="sm" variant="outline" onClick={() => setPaying('payout')}>
              Doplatit
            </Button>
          </div>
        )}
      </CardHeader>
      <CardContent className="grid gap-3">
        {q.data && <BalanceLine balance={q.data.balance} />}
        {q.data && <PaymentList data={q.data} onDelete={canEdit ? (id) => del.mutate(id, { onSuccess: () => toast.success('Platba smazána') }) : undefined} />}
      </CardContent>
      {paying && (
        <PayDialog
          slug={slug}
          contractorId={contractorId}
          kind={paying}
          suggested={paying === 'payout' && q.data && q.data.balance.balance > 0 ? q.data.balance.balance : undefined}
          onClose={() => setPaying(null)}
        />
      )}
    </Card>
  )
}

export function PayDialog({
  slug,
  contractorId,
  kind,
  suggested,
  onClose,
}: {
  slug: string
  contractorId: number
  kind: 'advance' | 'payout'
  suggested?: number
  onClose: () => void
}) {
  const pay = usePayContractor(slug, contractorId)
  const [f, setF] = useState({ kind, paid_on: todayISO(), amount: suggested ? formatMoneyInput(suggested) : '', method: 'cash', note: '' })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  const submit = () => {
    const amount = parseMoney(f.amount)
    if (!amount || amount <= 0) return toast.error('Zadejte částku.')
    pay.mutate(
      { kind: f.kind, paid_on: f.paid_on, amount, method: f.method as 'cash' | 'bank', note: f.note },
      { onSuccess: () => { toast.success(f.kind === 'advance' ? 'Záloha zapsána' : 'Výplata zapsána'); onClose() } },
    )
  }
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={f.kind === 'advance' ? 'Vyplatit zálohu' : 'Vyplatit doplatek'}
      description="Platba se odečte od odpracovaných a schválených hodin dodavatele."
      footer={<Button disabled={pay.isPending} onClick={submit}>Zapsat</Button>}
    >
      <FormGrid>
        <SelectField label="Druh" name="kind" value={f.kind} onChange={set('kind')} options={[{ value: 'advance', label: 'Záloha' }, { value: 'payout', label: 'Výplata / doplatek' }]} />
        <TextField label="Datum" name="paid_on" type="date" value={f.paid_on} onChange={set('paid_on')} />
        <TextField label="Částka (Kč)" name="amount" inputMode="decimal" value={f.amount} onChange={set('amount')} autoFocus />
        <SelectField label="Způsob" name="method" value={f.method} onChange={set('method')} options={[{ value: 'cash', label: 'Hotově' }, { value: 'bank', label: 'Převodem' }]} />
        <TextField label="Poznámka" name="note" value={f.note} onChange={set('note')} className="sm:col-span-2" />
      </FormGrid>
    </ResponsiveDialog>
  )
}
