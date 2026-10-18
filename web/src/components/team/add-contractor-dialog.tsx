import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '@/api/client'
import { useCreateContractor } from '@/api/queries/garden'
import type { Contractor } from '@/api/types'
import { FormGrid, TextField } from '@/components/garden/form'
import { LoginLinkDialog } from '@/components/garden/login-link'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { parseMoney } from '@/lib/money'

/**
 * Přidání dodavatele z nastavení týmu: založí dodavatele a rovnou mu dá přístup —
 * s e-mailem pozvánkou, bez e-mailu přihlašovacím odkazem / QR kódem.
 */
export function AddContractorDialog({ onClose }: { onClose: () => void }) {
  const { slug } = useCurrentAccount()
  const navigate = useNavigate()
  const create = useCreateContractor(slug)
  const [created, setCreated] = useState<Contractor | null>(null)
  const qc = useQueryClient()
  const [inviting, setInviting] = useState(false)
  const [f, setF] = useState({ name: '', phone: '', email: '', hourly: '' })
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })

  if (created) return <LoginLinkDialog slug={slug} contractor={created} onClose={onClose} />

  const submit = () =>
    create.mutate(
      { name: f.name, kind: 'person', phone: f.phone, email: f.email.trim(), hourly_rate: f.hourly ? (parseMoney(f.hourly) ?? undefined) : undefined },
      {
        onSuccess: (c) => {
          if (f.email.trim()) {
            // s e-mailem: klasická pozvánka
            setInviting(true)
            unwrap(api.POST('/api/accounts/{slug}/contractors/{id}/invite', { params: { path: { slug, id: c.id } }, body: { email: f.email.trim() } }))
              .then(() => {
                toast.success(`Pozvánka odeslána na ${f.email.trim()}`)
                void qc.invalidateQueries({ queryKey: ['a', slug] })
                onClose()
                void navigate({ to: '/a/$slug/contractors/$contractorId', params: { slug, contractorId: c.id } })
              })
              .catch((e: unknown) => toast.error((e as Error).message))
              .finally(() => setInviting(false))
          } else {
            setCreated(c) // bez e-mailu: hned přihlašovací odkaz
          }
        },
      },
    )

  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title="Přidat dodavatele"
      description="Bez e-mailu dostane přihlašovací odkaz nebo QR kód do telefonu. S e-mailem mu přijde pozvánka."
      footer={
        <Button disabled={!f.name.trim() || create.isPending || inviting} onClick={submit}>
          {f.email.trim() ? 'Přidat a pozvat e-mailem' : 'Přidat a vytvořit přihlášení'}
        </Button>
      }
    >
      <FormGrid>
        <TextField label="Jméno" name="name" value={f.name} onChange={set('name')} className="sm:col-span-2" autoFocus />
        <TextField label="Telefon" name="phone" type="tel" value={f.phone} onChange={set('phone')} />
        <TextField label="E-mail (nepovinný)" name="email" type="email" value={f.email} onChange={set('email')} />
        <TextField label="Hodinová sazba (Kč)" name="hourly" inputMode="decimal" value={f.hourly} onChange={set('hourly')} hint="Prázdné = výchozí sazba firmy." />
      </FormGrid>
    </ResponsiveDialog>
  )
}
