import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { BriefcaseIcon, ClockIcon, HardHatIcon, UsersIcon } from 'lucide-react'
import { gardenQueries } from '@/api/queries/garden'
import { StatusPill } from '@/components/garden/form'
import { currentMonth, formatMinutes, jobStatusLabels, jobStatusTone, monthLabel } from '@/components/garden/labels'
import { PageBody, PageHeader } from '@/components/page-header'
import { ButtonLink } from '@/components/button-link'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatDate } from '@/lib/date'

export const Route = createFileRoute('/a/$slug/dashboard')({
  beforeLoad: ({ context, params }) => {
    // Dodavatel má jen „Moje práce“.
    const ctx = context as { membership?: { role?: string } }
    if (ctx.membership?.role === 'contractor') throw redirect({ to: '/a/$slug/me', params: { slug: params.slug }, replace: true })
  },
  head: () => ({ meta: [{ title: 'Přehled · Zahrada' }] }),
  component: Dashboard,
})

function Dashboard() {
  const { slug } = Route.useParams()
  const month = currentMonth()
  const jobs = useQuery(gardenQueries.jobs(slug, { status: 'ordered,in_progress', per_page: 50 }))
  const inquiries = useQuery(gardenQueries.jobs(slug, { status: 'inquiry,quoted', per_page: 1 }))
  const atWork = useQuery(gardenQueries.attendance(slug, { status: 'open' }))
  const summary = useQuery(gardenQueries.summary(slug, month))

  return (
    <>
      <PageHeader
        title="Přehled"
        actions={[
          { label: 'Nová zakázka', icon: BriefcaseIcon, primary: true, render: <Link to="/a/$slug/jobs" params={{ slug }} search={{ new: 0 }} /> },
          { label: 'Klienti', icon: UsersIcon, render: <Link to="/a/$slug/clients" params={{ slug }} /> },
        ]}
      />
      <PageBody className="grid gap-4">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Tile icon={BriefcaseIcon} label="Rozpracované zakázky" value={jobs.data?.total ?? '–'} to="/a/$slug/jobs" />
          <Tile icon={UsersIcon} label="Poptávky a nabídky" value={inquiries.data?.total ?? '–'} to="/a/$slug/jobs" />
          <Tile icon={HardHatIcon} label="Právě v práci" value={atWork.data?.total ?? '–'} to="/a/$slug/attendance" />
          <Tile icon={ClockIcon} label="Docházka ke schválení" value={summary.data?.pending_count ?? '–'} to="/a/$slug/attendance" />
        </div>
        <div className="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Rozpracované zakázky</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-1">
              {jobs.data?.items.length === 0 && (
                <div className="grid justify-items-start gap-2 text-sm text-muted-foreground">
                  Žádná zakázka není objednaná ani v realizaci.
                  <ButtonLink to="/a/$slug/jobs" params={{ slug }} search={{ new: 0 }} size="sm">Nová zakázka</ButtonLink>
                </div>
              )}
              {jobs.data?.items.map((j) => (
                <Link key={j.id} to="/a/$slug/jobs/$jobId" params={{ slug, jobId: j.id }} className="flex items-center gap-2 rounded-md p-2 text-sm hover:bg-muted/50">
                  <span className="min-w-0 flex-1 truncate">
                    <span className="font-medium">{j.title}</span>
                    <span className="text-muted-foreground"> · {j.client_name}{j.planned_from && ` · ${formatDate(j.planned_from)}`}</span>
                  </span>
                  <StatusPill tone={jobStatusTone[j.status]}>{jobStatusLabels[j.status]}</StatusPill>
                </Link>
              ))}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="capitalize">Docházka · {monthLabel(month)}</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-1 text-sm">
              {atWork.data && atWork.data.items.length > 0 && (
                <div className="mb-2 rounded-md bg-emerald-500/10 p-2">
                  <p className="font-medium">Právě v práci</p>
                  {atWork.data.items.map((e) => (
                    <p key={e.id}>{e.contractor_name} od {e.start}{e.job_id ? ` · ${e.job_title}` : ''}</p>
                  ))}
                </div>
              )}
              {summary.data?.contractors.length === 0 && <p className="text-muted-foreground">Zatím žádná schválená docházka.</p>}
              {summary.data?.contractors.map((c) => (
                <div key={c.contractor_id} className="flex gap-3">
                  <span className="flex-1">{c.contractor_name}</span>
                  <span className="tabular-nums text-muted-foreground">{formatMinutes(c.minutes)}</span>
                </div>
              ))}
            </CardContent>
          </Card>
        </div>
      </PageBody>
    </>
  )
}

function Tile({ icon: Icon, label, value, to }: { icon: typeof ClockIcon; label: string; value: number | string; to: '/a/$slug/jobs' | '/a/$slug/attendance' }) {
  const { slug } = Route.useParams()
  return (
    <Link to={to} params={{ slug }}>
      <Card className="transition-colors hover:bg-muted/40">
        <CardContent className="flex items-center gap-3 py-4">
          <Icon className="size-5 text-primary" />
          <div>
            <p className="text-2xl font-semibold tabular-nums">{value}</p>
            <p className="text-sm text-muted-foreground">{label}</p>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}
