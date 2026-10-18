import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CopyIcon, MailIcon, QrCodeIcon, Share2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { api, unwrap } from '@/api/client'
import { keys } from '@/api/queries/keys'
import { useInviteContractor } from '@/api/queries/garden'
import type { Contractor } from '@/api/types'
import { TextField } from '@/components/garden/form'
import { QrCode } from '@/components/qr-code'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatDateTime } from '@/lib/date'

export function useLoginLink(slug: string, contractorId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.POST('/api/accounts/{slug}/contractors/{id}/login-link', { params: { path: { slug, id: contractorId } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: [...keys.account(slug), 'contractors'] }),
  })
}

/** Přihlašovací odkaz pro dodavatele bez e-mailu: QR kód k naskenování, sdílení (SMS, WhatsApp) nebo kopírování. */
export function LoginLinkDialog({ slug, contractor, onClose }: { slug: string; contractor: Pick<Contractor, 'id' | 'name' | 'login'>; onClose: () => void }) {
  const create = useLoginLink(slug, contractor.id)
  const link = create.data
  const share = async () => {
    if (!link) return
    const text = `Přihlášení do aplikace Zahrada (platí jednou, do ${formatDateTime(link.expires_at)}): ${link.url}`
    if (navigator.share) {
      try {
        await navigator.share({ title: 'Přihlášení do Zahrady', text, url: link.url })
      } catch {
        /* zrušeno */
      }
    } else {
      await navigator.clipboard.writeText(text)
      toast.success('Zkopírováno, vložte do SMS nebo zprávy')
    }
  }
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={`Přihlášení pro ${contractor.name}`}
      description="Dodavatel otevře odkaz v telefonu a je přihlášený, e-mail ani heslo nepotřebuje. Odkaz funguje jednou a platí 7 dní."
      footer={
        link ? (
          <Button onClick={() => void share()}>
            <Share2Icon data-icon="inline-start" /> Poslat odkaz
          </Button>
        ) : (
          <Button disabled={create.isPending} onClick={() => create.mutate()}>
            <QrCodeIcon data-icon="inline-start" /> Vytvořit odkaz
          </Button>
        )
      }
    >
      {contractor.login === 'link' && !link && (
        <p className="text-sm text-amber-700 dark:text-amber-400">
          Dodavatel už přihlášení má. Nový odkaz ho odhlásí ze všech zařízení — hodí se při ztrátě nebo výměně telefonu.
        </p>
      )}
      {link && (
        <div className="grid grid-cols-[minmax(0,1fr)] gap-3">
          <div className="mx-auto rounded-lg bg-white p-3">
            <QrCode value={link.url} label="QR kód pro přihlášení" className="size-56" />
          </div>
          <p className="text-center text-sm text-muted-foreground">Nechte dodavatele naskenovat QR kód fotoaparátem telefonu, nebo mu odkaz pošlete.</p>
          <div className="flex min-w-0 gap-2">
            <code className="min-w-0 flex-1 truncate rounded-md bg-muted px-2 py-1.5 text-xs">{link.url}</code>
            <Button
              variant="outline"
              size="sm"
              onClick={() => void navigator.clipboard.writeText(link.url).then(() => toast.success('Odkaz zkopírován'))}
            >
              <CopyIcon data-icon="inline-start" /> Kopírovat
            </Button>
          </div>
          <p className="text-center text-xs text-muted-foreground">Platí do {formatDateTime(link.expires_at)}, jen jednou.</p>
        </div>
      )}
    </ResponsiveDialog>
  )
}

function InviteByEmailDialog({ slug, contractor, onClose }: { slug: string; contractor: Contractor; onClose: () => void }) {
  const invite = useInviteContractor(slug, contractor.id)
  const [email, setEmail] = useState(contractor.email)
  return (
    <ResponsiveDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title="Pozvat e-mailem"
      description="Dodavatel dostane e-mail s odkazem a založí si přihlášení s heslem."
      footer={
        <Button disabled={!email.includes('@') || invite.isPending} onClick={() => invite.mutate(email, { onSuccess: () => { toast.success('Pozvánka odeslána'); onClose() } })}>
          Odeslat pozvánku
        </Button>
      }
    >
      <TextField label="E-mail" name="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
    </ResponsiveDialog>
  )
}

/** Karta „Přihlášení do aplikace“ v detailu dodavatele. */
export function ContractorAccessCard({ slug, contractor, canManage }: { slug: string; contractor: Contractor; canManage: boolean }) {
  const [dialog, setDialog] = useState<'link' | 'email' | null>(null)
  const status =
    contractor.login === 'email'
      ? `Přihlašuje se e-mailem ${contractor.user_email}.`
      : contractor.login === 'link'
        ? 'Přihlašuje se odkazem z telefonu (bez e-mailu).'
        : 'Zatím nemá přístup do aplikace. Bez něj mu docházku zapisujete vy.'
  return (
    <Card>
      <CardHeader>
        <CardTitle>Přihlášení do aplikace</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-3 text-sm">
        <p className="text-muted-foreground">{status}</p>
        {canManage && contractor.login !== 'email' && (
          <div className="flex flex-wrap gap-2">
            <Button size="sm" onClick={() => setDialog('link')}>
              <QrCodeIcon data-icon="inline-start" /> {contractor.login === 'link' ? 'Nový přihlašovací odkaz' : 'Přihlásit přes QR / odkaz'}
            </Button>
            {contractor.login === 'none' && (
              <Button size="sm" variant="outline" onClick={() => setDialog('email')}>
                <MailIcon data-icon="inline-start" /> Pozvat e-mailem
              </Button>
            )}
          </div>
        )}
      </CardContent>
      {dialog === 'link' && <LoginLinkDialog slug={slug} contractor={contractor} onClose={() => setDialog(null)} />}
      {dialog === 'email' && <InviteByEmailDialog slug={slug} contractor={contractor} onClose={() => setDialog(null)} />}
    </Card>
  )
}
