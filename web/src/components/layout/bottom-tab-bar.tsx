import { Link, useMatchRoute, useNavigate } from '@tanstack/react-router'
import {
  BriefcaseIcon,
  CheckIcon,
  ChevronRightIcon,
  ClockIcon,
  EllipsisIcon,
  LayoutDashboardIcon,
  LogOutIcon,
  PlusIcon,
  SmartphoneIcon,
  type LucideIcon,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { CreateAccountDialog } from '@/components/account/create-account-dialog'
import { useInstallAction } from '@/components/install/use-install-action'
import { ThemeSegmented } from '@/components/theme'
import { NewsBadge } from '@/components/news/news-badge'
import { TodoCountBadge } from '@/components/todos/todo-count-badge'
import { Button } from '@/components/ui/button'
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { cn } from '@/lib/utils'
import { AccountAvatar } from './app-sidebar'
import { adminNav, moreNav, settingsNav } from './nav'
import { useLogoutAction } from './use-logout-action'

const tabClass =
  'flex h-full min-w-0 flex-1 flex-col items-center justify-center gap-0.5 text-[11px] font-medium text-muted-foreground transition-colors active:text-foreground data-[active=true]:text-primary'

/**
 * Spodní navigace na mobilu (< md): Přehled, Zakázky, + Nová, Docházka, Více (dodavatel: Moje práce, Více).
 * Respektuje safe area (home indicator) a při otevřené klávesnici se skryje.
 */
export function BottomTabBar() {
  const { slug, canEdit, role } = useCurrentAccount()
  const matchRoute = useMatchRoute()
  const [moreOpen, setMoreOpen] = useState(false)
  const is = (to: Parameters<typeof matchRoute>[0]['to']) =>
    Boolean(matchRoute({ to, params: { slug }, fuzzy: true }))

  return (
    <>
      <nav
        aria-label="Hlavní navigace"
        className="fixed inset-x-0 bottom-0 z-40 border-t bg-background/90 pb-safe backdrop-blur-md supports-backdrop-filter:bg-background/75 keyboard-open:hidden md:hidden"
      >
        <div className="mx-auto flex h-tabbar max-w-lg items-stretch px-1">
          {role === 'contractor' ? (
            <Tab to="/a/$slug/me" slug={slug} icon={ClockIcon} label="Moje práce" active={is('/a/$slug/me')} />
          ) : (
            <>
              <Tab to="/a/$slug/dashboard" slug={slug} icon={LayoutDashboardIcon} label="Přehled" active={is('/a/$slug/dashboard')} />
              <Tab to="/a/$slug/jobs" slug={slug} icon={BriefcaseIcon} label="Zakázky" active={is('/a/$slug/jobs')} />
              {canEdit && (
                <div className="flex flex-1 items-center justify-center">
                  <Link
                    to="/a/$slug/jobs"
                    params={{ slug }}
                    search={{ new: 0 }}
                    aria-label="Nová zakázka"
                    className="flex size-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground shadow-lg shadow-primary/25 transition-transform active:scale-95"
                  >
                    <PlusIcon className="size-6" strokeWidth={2.25} />
                  </Link>
                </div>
              )}
              <Tab to="/a/$slug/attendance" slug={slug} icon={ClockIcon} label="Docházka" active={is('/a/$slug/attendance')} />
            </>
          )}
          <button
            type="button"
            className={tabClass}
            data-active={is('/a/$slug/settings') || moreNav.some((i) => is(i.to)) || moreOpen}
            onClick={() => setMoreOpen(true)}
          >
            <span className="relative flex w-full justify-center">
              <EllipsisIcon className="size-6" />
              <TodoCountBadge slug={slug} variant="dot" />
              {role !== 'contractor' && <NewsBadge variant="dot" />}
            </span>
            Více
          </button>
        </div>
      </nav>
      <MoreDrawer open={moreOpen} onOpenChange={setMoreOpen} />
    </>
  )
}

function Tab({
  to,
  slug,
  icon: Icon,
  label,
  active,
}: {
  to: '/a/$slug/dashboard' | '/a/$slug/jobs' | '/a/$slug/attendance' | '/a/$slug/me'
  slug: string
  icon: LucideIcon
  label: string
  active: boolean
}) {
  return (
    <Link to={to} params={{ slug }} className={tabClass} data-active={active} aria-current={active ? 'page' : undefined}>
      <Icon className="size-6" strokeWidth={active ? 2.25 : 1.75} />
      {label}
    </Link>
  )
}

/** Menu „Více“: přepínač účtů, nastavení, motiv, odhlášení. */
function MoreDrawer({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { slug, accounts, user, me, role } = useCurrentAccount()
  const navigate = useNavigate()
  const logout = useLogoutAction()
  const [createOpen, setCreateOpen] = useState(false)
  const installApp = useInstallAction()
  const close = () => onOpenChange(false)

  return (
    <>
      <Drawer open={open} onOpenChange={onOpenChange} showSwipeHandle>
        <DrawerContent>
          <DrawerHeader className="group-data-[swipe-axis=y]/drawer-popup:text-left">
            <DrawerTitle className="text-lg">Více</DrawerTitle>
            {user && <p className="truncate text-sm text-muted-foreground">{user.email}</p>}
          </DrawerHeader>
          <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-4 pt-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
            <Section title="Účet">
              {accounts.map((a) => (
                <Row
                  key={a.slug}
                  onClick={() => {
                    close()
                    void navigate({ to: '/a/$slug/dashboard', params: { slug: a.slug } })
                  }}
                  leading={<AccountAvatar name={a.name} className="size-8 rounded-lg" />}
                  trailing={a.slug === slug ? <CheckIcon className="size-5 text-primary" /> : null}
                >
                  {a.name}
                </Row>
              ))}
              <Row
                onClick={() => setCreateOpen(true)}
                leading={
                  <span className="flex size-8 items-center justify-center rounded-lg border border-dashed">
                    <PlusIcon className="size-4" />
                  </span>
                }
              >
                Nový účet
              </Row>
            </Section>

            <Section title="Další">
              {(role === 'contractor' ? [] : moreNav).map((item) => (
                <Row
                  key={item.to}
                  onClick={() => {
                    close()
                    void navigate({ to: item.to, params: { slug } })
                  }}
                  leading={<item.icon className="size-5 text-muted-foreground" />}
                  trailing={
                    <>
                      {item.to === '/a/$slug/todos' && <TodoCountBadge slug={slug} />}
                      {item.to === '/a/$slug/news' && <NewsBadge />}
                      <ChevronRightIcon className="size-4 text-muted-foreground" />
                    </>
                  }
                >
                  {item.label}
                </Row>
              ))}
            </Section>

            <Section title="Nastavení">
              {settingsNav.map((item) => (
                <Row
                  key={item.to}
                  onClick={() => {
                    close()
                    void navigate({ to: item.to, params: { slug } })
                  }}
                  leading={<item.icon className="size-5 text-muted-foreground" />}
                  trailing={<ChevronRightIcon className="size-4 text-muted-foreground" />}
                >
                  {item.label}
                </Row>
              ))}
              {me?.instance_admin && (
                <Row
                  onClick={() => {
                    close()
                    void navigate({ to: adminNav.to, params: { slug } })
                  }}
                  leading={<adminNav.icon className="size-5 text-muted-foreground" />}
                  trailing={<ChevronRightIcon className="size-4 text-muted-foreground" />}
                >
                  {adminNav.label}
                </Row>
              )}
            </Section>

            {installApp.available && (
              <Row
                onClick={() => {
                  close()
                  installApp.run()
                }}
                leading={<SmartphoneIcon className="size-5 text-muted-foreground" />}
                trailing={<ChevronRightIcon className="size-4 text-muted-foreground" />}
              >
                Nainstalovat aplikaci
              </Row>
            )}

            <Section title="Motiv">
              <ThemeSegmented />
            </Section>

            <Button variant="outline" onClick={logout.run} disabled={logout.isPending} className="text-destructive">
              <LogOutIcon data-icon="inline-start" />
              Odhlásit se
            </Button>
          </div>
        </DrawerContent>
      </Drawer>
      <CreateAccountDialog
        open={createOpen}
        onOpenChange={(o) => {
          setCreateOpen(o)
          if (!o) close()
        }}
      />
      {installApp.dialog}
    </>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-1">
      <h3 className="px-1 pb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">{title}</h3>
      {children}
    </section>
  )
}

function Row({
  children,
  leading,
  trailing,
  onClick,
}: {
  children: ReactNode
  leading?: ReactNode
  trailing?: ReactNode
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn('flex min-h-12 w-full items-center gap-3 rounded-lg px-2 text-left text-base active:bg-muted')}
    >
      {leading}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {trailing}
    </button>
  )
}
