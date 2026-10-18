import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { toast } from 'sonner'
import { accountQueries, useUpdateAccount } from '@/api/queries/accounts'
import type { Account, UpdateAccountInput } from '@/api/types'
import { FormGrid, SelectField, TextAreaField, TextField } from '@/components/garden/form'
import { SettingsPage } from '@/components/settings-page'
import { FormSkeleton } from '@/components/skeletons'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatMoneyInput, parseMoney } from '@/lib/money'

export const Route = createFileRoute('/a/$slug/settings/company')({
  loader: ({ context, params }) => context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
  head: () => ({ meta: [{ title: 'Firma · Zahrada' }] }),
  component: CompanySettings,
})

function CompanySettings() {
  const { slug } = Route.useParams()
  const q = useQuery(accountQueries.detail(slug))
  if (!q.data) return <FormSkeleton />
  return <CompanyForm key={q.data.updated_at} account={q.data} />
}

type Rounding = UpdateAccountInput['attendance_rounding_min']

function CompanyForm({ account: a }: { account: Account }) {
  const { slug, canManageSettings } = useCurrentAccount()
  const update = useUpdateAccount(slug)
  const [f, setF] = useState({
    name: a.name, registration_no: a.registration_no, vat_no: a.vat_no, street: a.street, city: a.city, zip: a.zip,
    email: a.email, phone: a.phone, web: a.web, vat_payer: a.vat_payer, default_vat_rate_bps: String(a.default_vat_rate_bps),
    default_hourly_rate: formatMoneyInput(a.default_hourly_rate), default_km_rate: formatMoneyInput(a.default_km_rate),
    attendance_rounding_min: String(a.attendance_rounding_min), attendance_rounding_mode: a.attendance_rounding_mode,
    attendance_gps: a.attendance_gps, plan_horizon_weeks: String(a.plan_horizon_weeks), quote_validity_days: String(a.quote_validity_days),
    quote_terms: a.quote_terms, time_zone: a.time_zone,
  })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  const save = () => {
    const hourly = parseMoney(f.default_hourly_rate)
    const km = parseMoney(f.default_km_rate)
    if (hourly === null || km === null) return toast.error('Zkontrolujte sazby.')
    update.mutate(
      {
        name: f.name, registration_no: f.registration_no, vat_no: f.vat_no, street: f.street, city: f.city, zip: f.zip,
        email: f.email, phone: f.phone, web: f.web, vat_payer: f.vat_payer, default_vat_rate_bps: Number(f.default_vat_rate_bps),
        default_hourly_rate: hourly, default_km_rate: km, attendance_rounding_min: Number(f.attendance_rounding_min) as Rounding,
        attendance_rounding_mode: f.attendance_rounding_mode, attendance_gps: f.attendance_gps,
        plan_horizon_weeks: Number(f.plan_horizon_weeks) || 8, quote_validity_days: Number(f.quote_validity_days) || 30,
        quote_terms: f.quote_terms, time_zone: f.time_zone,
      },
      { onSuccess: () => toast.success('Nastavení uloženo') },
    )
  }
  return (
    <SettingsPage title="Firma" description="Údaje firmy, sazby a pravidla docházky.">
      <fieldset disabled={!canManageSettings} className="grid gap-4">
        <Card>
          <CardHeader><CardTitle>Údaje firmy</CardTitle></CardHeader>
          <CardContent>
            <FormGrid>
              <TextField label="Název" name="name" value={f.name} onChange={set('name')} className="sm:col-span-2" />
              <TextField label="IČO" name="registration_no" value={f.registration_no} onChange={set('registration_no')} />
              <TextField label="DIČ" name="vat_no" value={f.vat_no} onChange={set('vat_no')} />
              <TextField label="Ulice" name="street" value={f.street} onChange={set('street')} className="sm:col-span-2" />
              <TextField label="Obec" name="city" value={f.city} onChange={set('city')} />
              <TextField label="PSČ" name="zip" value={f.zip} onChange={set('zip')} />
              <TextField label="E-mail" name="email" type="email" value={f.email} onChange={set('email')} />
              <TextField label="Telefon" name="phone" type="tel" value={f.phone} onChange={set('phone')} />
              <TextField label="Web" name="web" value={f.web} onChange={set('web')} />
            </FormGrid>
          </CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle>Ceny a sazby</CardTitle></CardHeader>
          <CardContent className="grid gap-4">
            <label className="flex items-center gap-2 text-sm">
              <Switch checked={f.vat_payer} onCheckedChange={(v) => setF({ ...f, vat_payer: v })} /> Plátce DPH
            </label>
            <FormGrid>
              <SelectField
                label="Výchozí sazba DPH"
                name="default_vat_rate_bps"
                value={f.default_vat_rate_bps}
                onChange={set('default_vat_rate_bps')}
                options={[{ value: '2100', label: '21 %' }, { value: '1200', label: '12 %' }, { value: '0', label: '0 %' }]}
              />
              <div />
              <TextField label="Výchozí hodinová sazba dodavatele (Kč)" name="default_hourly_rate" value={f.default_hourly_rate} onChange={set('default_hourly_rate')} inputMode="decimal" />
              <TextField label="Výchozí sazba za km (Kč)" name="default_km_rate" value={f.default_km_rate} onChange={set('default_km_rate')} inputMode="decimal" />
              <TextField label="Platnost nabídek (dní)" name="quote_validity_days" value={f.quote_validity_days} onChange={set('quote_validity_days')} inputMode="numeric" />
              <TextAreaField label="Podmínky do nabídek" name="quote_terms" value={f.quote_terms} onChange={set('quote_terms')} className="sm:col-span-2" />
            </FormGrid>
          </CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle>Docházka a plán</CardTitle></CardHeader>
          <CardContent className="grid gap-4">
            <FormGrid>
              <SelectField
                label="Zaokrouhlení odpracované doby"
                name="attendance_rounding_min"
                value={f.attendance_rounding_min}
                onChange={set('attendance_rounding_min')}
                options={[{ value: '0', label: 'Nezaokrouhlovat' }, ...['5', '10', '15', '30'].map((m) => ({ value: m, label: `Na ${m} minut` }))]}
              />
              <SelectField
                label="Směr zaokrouhlení"
                name="attendance_rounding_mode"
                value={f.attendance_rounding_mode}
                onChange={set('attendance_rounding_mode')}
                options={[{ value: 'nearest', label: 'Na nejbližší' }, { value: 'up', label: 'Nahoru' }, { value: 'down', label: 'Dolů' }]}
              />
              <TextField label="Plánovat údržbu dopředu (týdnů)" name="plan_horizon_weeks" value={f.plan_horizon_weeks} onChange={set('plan_horizon_weeks')} inputMode="numeric" />
              <TextField label="Časové pásmo" name="time_zone" value={f.time_zone} onChange={set('time_zone')} placeholder="Europe/Prague" hint="Prázdné = výchozí nastavení serveru." />
            </FormGrid>
            <label className="flex items-center gap-2 text-sm">
              <Switch checked={f.attendance_gps} onCheckedChange={(v) => setF({ ...f, attendance_gps: v })} />
              Ukládat polohu u příchodu a odchodu (dodavatele o tom informujte)
            </label>
            {a.attendance_locked_through && <p className="text-sm text-muted-foreground">Docházka je uzavřená do {a.attendance_locked_through}.</p>}
          </CardContent>
        </Card>
        {canManageSettings && (
          <div>
            <Button disabled={!f.name.trim() || update.isPending} onClick={save}>Uložit</Button>
          </div>
        )}
      </fieldset>
    </SettingsPage>
  )
}
