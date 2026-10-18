import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { toast } from 'sonner'
import { errorMessage } from '@/api/errors'
import { useCreateAccount } from '@/api/queries/accounts'
import { TextField } from '@/components/garden/form'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'

/** Založení dalšího účtu (firmy). Po vytvoření přepne na jeho nastavení. */
export function CreateAccountDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const navigate = useNavigate()
  const create = useCreateAccount()
  const [name, setName] = useState('')
  const submit = async () => {
    try {
      const account = await create.mutateAsync({ name: name.trim() })
      toast.success(`Účet „${account.name}“ založen`)
      onOpenChange(false)
      setName('')
      await navigate({ to: '/a/$slug/settings/company', params: { slug: account.slug } })
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Nový účet"
      description="Další firma se samostatnými klienty, zakázkami a docházkou."
      footer={<Button disabled={!name.trim() || create.isPending} onClick={() => void submit()}>Založit</Button>}
    >
      <TextField label="Název firmy" name="account-name" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
    </ResponsiveDialog>
  )
}
