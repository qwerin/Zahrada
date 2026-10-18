import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { accountQueries, useUpdateAccount } from '@/api/queries/accounts'
import { FormGrid, TextField } from '@/components/garden/form'
import { Logo } from '@/components/logo'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { markOnboarded } from '@/lib/onboarding'
import { parseMoney } from '@/lib/money'

export const Route = createFileRoute('/a/$slug/onboarding')({
  head: () => ({ meta: [{ title: 'Vítejte · Zahrada' }] }),
  component: Onboarding,
})

/** Jednokrokový průvodce po registraci: základní údaje firmy a sazba dodavatelů. */
function Onboarding() {
  const { slug } = Route.useParams()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const account = useQuery(accountQueries.detail(slug))
  const update = useUpdateAccount(slug)
  const [f, setF] = useState({ name: '', registration_no: '', street: '', city: '', zip: '', email: '', phone: '', hourly: '', vat_payer: false })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })
  const done = async () => {
    await markOnboarded(qc, slug)
    await navigate({ to: '/a/$slug/dashboard', params: { slug } })
  }
  const save = () =>
    update.mutate(
      {
        name: f.name.trim() || account.data?.name, registration_no: f.registration_no, street: f.street, city: f.city, zip: f.zip,
        email: f.email, phone: f.phone, vat_payer: f.vat_payer, default_hourly_rate: parseMoney(f.hourly) ?? undefined, onboarded: true,
      },
      { onSuccess: () => void navigate({ to: '/a/$slug/dashboard', params: { slug } }) },
    )
  return (
    <div className="mx-auto grid min-h-dvh max-w-xl content-center gap-4 p-4">
      <Logo />
      <Card>
        <CardHeader>
          <CardTitle>Vítejte v Zahradě</CardTitle>
          <CardDescription>Doplňte základní údaje firmy. Vše jde později změnit v nastavení.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          <FormGrid>
            <TextField label="Název firmy" name="name" placeholder={account.data?.name} value={f.name} onChange={set('name')} className="sm:col-span-2" />
            <TextField label="IČO" name="registration_no" value={f.registration_no} onChange={set('registration_no')} />
            <TextField label="Telefon" name="phone" type="tel" value={f.phone} onChange={set('phone')} />
            <TextField label="Ulice" name="street" value={f.street} onChange={set('street')} className="sm:col-span-2" />
            <TextField label="Obec" name="city" value={f.city} onChange={set('city')} />
            <TextField label="PSČ" name="zip" value={f.zip} onChange={set('zip')} />
            <TextField label="E-mail" name="email" type="email" value={f.email} onChange={set('email')} />
            <TextField label="Hodinová sazba dodavatelů (Kč)" name="hourly" value={f.hourly} onChange={set('hourly')} inputMode="decimal" />
          </FormGrid>
          <label className="flex items-center gap-2 text-sm">
            <Switch checked={f.vat_payer} onCheckedChange={(v) => setF({ ...f, vat_payer: v })} /> Jsme plátci DPH
          </label>
          <div className="flex justify-between gap-2">
            <Button variant="ghost" onClick={() => void done()}>Přeskočit</Button>
            <Button disabled={update.isPending} onClick={save}>Uložit a pokračovat</Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
