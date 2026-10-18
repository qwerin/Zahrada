// Typované testovací fixtury odpovídající src/api/schema.gen.ts (komponentové testy).
import type { Account, AccountMembership, Me } from '@/api/types'

export const SLUG = 'firma'
export const TS = '2026-03-01T10:00:00Z'

type Role = AccountMembership['role']

export function capabilitiesFor(role: Role): Account['capabilities'] {
  const manager = role === 'owner' || role === 'admin'
  const editor = manager || role === 'member'
  return {
    staff: role !== 'contractor',
    edit: editor,
    manage_members: manager,
    manage_owners: role === 'owner',
    manage_settings: manager,
    view_reports: editor || role === 'accountant',
  }
}

export function makeMe(overrides: Partial<Me> = {}, role: Role = 'owner'): Me {
  return {
    user: { id: 1, email: 'jan@example.cz', name: 'Jan Novák' },
    accounts: [{ slug: SLUG, name: 'Firma s.r.o.', role }],
    email_verified: true,
    instance_admin: false,
    instance_admin_pending: false,
    ...overrides,
  }
}

export function makeAccount(overrides: Partial<Account> = {}): Account {
  const role = overrides.role ?? 'owner'
  return {
    slug: SLUG,
    name: 'Firma s.r.o.',
    role,
    capabilities: capabilitiesFor(role),
    registration_no: '12345678',
    vat_no: '',
    street: 'Hlavní 1',
    city: 'Praha',
    zip: '11000',
    country: 'CZ',
    email: 'firma@example.cz',
    phone: '',
    web: '',
    vat_payer: false,
    default_vat_rate_bps: 2100,
    default_hourly_rate: 35000,
    default_km_rate: 0,
    attendance_rounding_min: 15,
    attendance_rounding_mode: 'nearest',
    attendance_locked_through: '',
    attendance_gps: false,
    plan_horizon_weeks: 8,
    quote_validity_days: 30,
    quote_terms: '',
    time_zone: '',
    onboarded_at: TS,
    created_at: TS,
    updated_at: TS,
    ...overrides,
  }
}

export function emptyList<T>() {
  return { items: [] as T[], page: 1, per_page: 50, total: 0 }
}
