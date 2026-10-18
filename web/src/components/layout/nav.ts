import {
  BriefcaseIcon,
  BuildingIcon,
  ClockIcon,
  HardHatIcon,
  HistoryIcon,
  KeyRoundIcon,
  LayoutDashboardIcon,
  ListTodoIcon,
  ServerCogIcon,
  ShieldCheckIcon,
  TagsIcon,
  UserCogIcon,
  UserIcon,
  UsersIcon,
} from 'lucide-react'

// Jediný zdroj pravdy pro navigaci (sidebar, tab bar, „Více“, nastavení).

/** Položky, které na mobilu nejsou v tab baru — zobrazují se v menu „Více“ (na desktopu v sidebaru). */
export const moreNav = [
  { label: 'Klienti', to: '/a/$slug/clients', icon: UsersIcon },
  { label: 'Dodavatelé', to: '/a/$slug/contractors', icon: HardHatIcon },
  { label: 'Ceník', to: '/a/$slug/price-items', icon: TagsIcon },
  { label: 'Úkoly', to: '/a/$slug/todos', icon: ListTodoIcon },
  { label: 'Aktivita', to: '/a/$slug/activity', icon: HistoryIcon },
] as const

export const mainNav = [
  { label: 'Přehled', to: '/a/$slug/dashboard', icon: LayoutDashboardIcon },
  { label: 'Zakázky', to: '/a/$slug/jobs', icon: BriefcaseIcon },
  { label: 'Docházka', to: '/a/$slug/attendance', icon: ClockIcon },
  ...moreNav,
] as const

/** Navigace dodavatele: jen vlastní práce. */
export const contractorNav = [{ label: 'Moje práce', to: '/a/$slug/me', icon: ClockIcon }] as const

/** Skupiny sekcí nastavení (pořadí = pořadí v menu). */
export const settingsGroups = [
  { id: 'company', label: 'Firma' },
  { id: 'team', label: 'Tým a integrace' },
  { id: 'me', label: 'Můj účet' },
] as const

export const settingsNav = [
  { label: 'Firma', to: '/a/$slug/settings/company', icon: BuildingIcon, description: 'Údaje firmy, sazby, docházka', group: 'company' },
  { label: 'Tým', to: '/a/$slug/settings/members', icon: UserCogIcon, description: 'Uživatelé účtu, role a pozvánky', group: 'team' },
  { label: 'API tokeny', to: '/a/$slug/settings/tokens', icon: KeyRoundIcon, description: 'Přístup pro integrace', group: 'team' },
  { label: 'Můj profil', to: '/a/$slug/settings/profile', icon: UserIcon, description: 'Jméno a heslo', group: 'me' },
  { label: 'Zabezpečení', to: '/a/$slug/settings/security', icon: ShieldCheckIcon, description: 'Dvoufázové ověření a bezpečnostní klíče', group: 'me' },
] as const

/** Správa instance — jen pro správce instance (`me.instance_admin`). */
export const adminNav = { label: 'Správa instance', to: '/a/$slug/admin', icon: ServerCogIcon } as const

/** Iniciály pro avatar účtu/uživatele: „Jan Novák“ → „JN“. */
export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  const first = parts[0]?.[0] ?? '?'
  const second = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? '') : (parts[0]?.[1] ?? '')
  return (first + second).toUpperCase()
}
