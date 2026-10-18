import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { api, unwrap } from '@/api/client'
import { errorMessage } from '@/api/errors'
import { keys } from '@/api/queries/keys'
import { Logo } from '@/components/logo'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

// Přihlášení dodavatele jednorázovým odkazem (QR kód / SMS od vedoucího).
export const Route = createFileRoute('/prihlaseni/$token')({
  head: () => ({ meta: [{ title: 'Přihlášení · Zahrada' }] }),
  component: LoginLinkPage,
})

function LoginLinkPage() {
  const { token } = Route.useParams()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [error, setError] = useState<string | null>(null)
  const started = useRef(false)
  useEffect(() => {
    if (started.current) return // StrictMode: odkaz je jednorázový, volat jen jednou
    started.current = true
    unwrap(api.POST('/api/auth/login-link', { body: { token } }))
      .then((me) => {
        qc.setQueryData(keys.me(), me)
        const slug = me.accounts[0]?.slug
        void navigate({ to: slug ? '/a/$slug/me' : '/', params: { slug: slug ?? '' }, replace: true })
      })
      .catch((e: unknown) => setError(errorMessage(e)))
  }, [token, qc, navigate])
  return (
    <div className="mx-auto grid min-h-dvh max-w-md content-center gap-4 p-4">
      <Logo />
      <Card>
        <CardHeader>
          <CardTitle>{error ? 'Odkaz už neplatí' : 'Přihlašuji…'}</CardTitle>
          {error && (
            <CardDescription>
              Přihlašovací odkaz funguje jen jednou a platí 7 dní. Požádejte vedoucího o nový.
            </CardDescription>
          )}
        </CardHeader>
        {error && (
          <CardContent className="text-sm">
            <Link to="/login" className="text-primary">Přihlásit se e-mailem a heslem</Link>
          </CardContent>
        )}
      </Card>
    </div>
  )
}
