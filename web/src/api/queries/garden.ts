// Klienti, zahrady, dodavatelé, zakázky a docházka (SPEC §4.2–4.8).
// Klíče jsou pod prefixem ['a', slug, …], mutace zneplatňují celé zdroje.

import { keepPreviousData, queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type {
  AttendanceInput,
  ClockOutInput,
  CreateClientInput,
  CreateContractorInput,
  CreateJobInput,
  CreateSiteInput,
  JobListQuery,
  JobStatus,
  UpdateClientInput,
  UpdateContractorInput,
  UpdateJobInput,
  UpdateSiteInput,
} from '../types'
import { keys } from './keys'

const k = {
  clients: (slug: string) => [...keys.account(slug), 'clients'] as const,
  sites: (slug: string) => [...keys.account(slug), 'sites'] as const,
  contractors: (slug: string) => [...keys.account(slug), 'contractors'] as const,
  jobs: (slug: string) => [...keys.account(slug), 'jobs'] as const,
  attendance: (slug: string) => [...keys.account(slug), 'attendance'] as const,
  me: (slug: string) => [...keys.account(slug), 'me'] as const,
}

const path = (slug: string) => ({ params: { path: { slug } } })
const pathId = (slug: string, id: number) => ({ params: { path: { slug, id } } })

export const gardenQueries = {
  clients: (slug: string, query = '', archived = false) =>
    queryOptions({
      queryKey: [...k.clients(slug), 'list', query, archived],
      queryFn: () =>
        unwrap(api.GET('/api/accounts/{slug}/clients', { params: { path: { slug }, query: { query, archived, per_page: 200 } } })),
      placeholderData: keepPreviousData,
    }),
  client: (slug: string, id: number) =>
    queryOptions({
      queryKey: [...k.clients(slug), 'detail', id],
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}/clients/{id}', pathId(slug, id))),
    }),
  contractors: (slug: string, active: '' | 'true' | 'false' = '') =>
    queryOptions({
      queryKey: [...k.contractors(slug), 'list', active],
      queryFn: () =>
        unwrap(api.GET('/api/accounts/{slug}/contractors', { params: { path: { slug }, query: { active, per_page: 200 } } })),
    }),
  contractor: (slug: string, id: number) =>
    queryOptions({
      queryKey: [...k.contractors(slug), 'detail', id],
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}/contractors/{id}', pathId(slug, id))),
    }),
  jobs: (slug: string, query: JobListQuery) =>
    queryOptions({
      queryKey: [...k.jobs(slug), 'list', query],
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}/jobs', { params: { path: { slug }, query } })),
      placeholderData: keepPreviousData,
    }),
  job: (slug: string, id: number) =>
    queryOptions({
      queryKey: [...k.jobs(slug), 'detail', id],
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}/jobs/{id}', pathId(slug, id))),
    }),
  attendance: (slug: string, query: { from?: string; to?: string; contractor_id?: number; job_id?: number; status?: string }) =>
    queryOptions({
      queryKey: [...k.attendance(slug), 'list', query],
      queryFn: () =>
        unwrap(api.GET('/api/accounts/{slug}/attendance', { params: { path: { slug }, query: { ...query, per_page: 200 } } })),
      placeholderData: keepPreviousData,
    }),
  summary: (slug: string, month: string) =>
    queryOptions({
      queryKey: [...k.attendance(slug), 'summary', month],
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}/attendance/summary', { params: { path: { slug }, query: { month } } })),
      placeholderData: keepPreviousData,
    }),
  meContractor: (slug: string) =>
    queryOptions({
      queryKey: [...k.me(slug), 'contractor'],
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}/me/contractor', path(slug))),
      refetchInterval: 60_000,
      retry: false,
    }),
  meJobs: (slug: string) =>
    queryOptions({
      queryKey: [...k.me(slug), 'jobs'],
      queryFn: async () => (await unwrap(api.GET('/api/accounts/{slug}/me/jobs', path(slug)))) ?? [],
      retry: false,
    }),
  meAttendance: (slug: string, month: string) =>
    queryOptions({
      queryKey: [...k.me(slug), 'attendance', month],
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}/me/attendance', { params: { path: { slug }, query: { month } } })),
      retry: false,
    }),
}

/** Společná invalidace po změně zdrojů (zakázky ↔ docházka se ovlivňují). */
function useInvalidate(slug: string) {
  const qc = useQueryClient()
  return (...what: ('clients' | 'sites' | 'contractors' | 'jobs' | 'attendance' | 'me')[]) =>
    Promise.all(what.map((w) => qc.invalidateQueries({ queryKey: k[w](slug) })))
}

// ---- klienti a zahrady ----

export function useCreateClient(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (body: CreateClientInput) => unwrap(api.POST('/api/accounts/{slug}/clients', { ...path(slug), body })),
    onSuccess: () => inv('clients'),
  })
}

export function useUpdateClient(slug: string, id: number) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (body: UpdateClientInput) =>
      unwrap(api.PATCH('/api/accounts/{slug}/clients/{id}', { ...pathId(slug, id), body })),
    onSuccess: () => inv('clients', 'jobs'),
  })
}

export function useDeleteClient(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (id: number) => unwrap(api.DELETE('/api/accounts/{slug}/clients/{id}', pathId(slug, id))),
    onSuccess: () => inv('clients'),
  })
}

export function useCreateSite(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (body: CreateSiteInput) => unwrap(api.POST('/api/accounts/{slug}/sites', { ...path(slug), body })),
    onSuccess: () => inv('clients', 'sites'),
  })
}

export function useUpdateSite(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: ({ id, ...body }: UpdateSiteInput & { id: number }) =>
      unwrap(api.PATCH('/api/accounts/{slug}/sites/{id}', { ...pathId(slug, id), body })),
    onSuccess: () => inv('clients', 'sites', 'jobs'),
  })
}

export function useDeleteSite(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (id: number) => unwrap(api.DELETE('/api/accounts/{slug}/sites/{id}', pathId(slug, id))),
    onSuccess: () => inv('clients', 'sites'),
  })
}

// ---- dodavatelé ----

export function useCreateContractor(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (body: CreateContractorInput) => unwrap(api.POST('/api/accounts/{slug}/contractors', { ...path(slug), body })),
    onSuccess: () => inv('contractors'),
  })
}

export function useUpdateContractor(slug: string, id: number) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (body: UpdateContractorInput) =>
      unwrap(api.PATCH('/api/accounts/{slug}/contractors/{id}', { ...pathId(slug, id), body })),
    onSuccess: () => inv('contractors'),
  })
}

export function useDeleteContractor(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (id: number) => unwrap(api.DELETE('/api/accounts/{slug}/contractors/{id}', pathId(slug, id))),
    onSuccess: () => inv('contractors'),
  })
}

export function useInviteContractor(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (email: string) =>
      unwrap(api.POST('/api/accounts/{slug}/contractors/{id}/invite', { ...pathId(slug, id), body: { email } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: [...keys.account(slug), 'invitations'] }),
  })
}

// ---- zakázky ----

export function useCreateJob(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (body: CreateJobInput) => unwrap(api.POST('/api/accounts/{slug}/jobs', { ...path(slug), body })),
    onSuccess: () => inv('jobs'),
  })
}

export function useUpdateJob(slug: string, id: number) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (body: UpdateJobInput) => unwrap(api.PATCH('/api/accounts/{slug}/jobs/{id}', { ...pathId(slug, id), body })),
    onSuccess: () => inv('jobs'),
  })
}

export function useJobStatus(slug: string, id: number) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (status: JobStatus) =>
      unwrap(api.POST('/api/accounts/{slug}/jobs/{id}/status', { ...pathId(slug, id), body: { status } })),
    onSuccess: () => inv('jobs'),
  })
}

export function useDeleteJob(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (id: number) => unwrap(api.DELETE('/api/accounts/{slug}/jobs/{id}', pathId(slug, id))),
    onSuccess: () => inv('jobs'),
  })
}

// ---- docházka (vedoucí) ----

export function useSaveAttendance(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: ({ id, ...body }: AttendanceInput & { id?: number }) =>
      id
        ? unwrap(api.PATCH('/api/accounts/{slug}/attendance/{id}', { ...pathId(slug, id), body }))
        : unwrap(api.POST('/api/accounts/{slug}/attendance', { ...path(slug), body })),
    onSuccess: () => inv('attendance', 'jobs'),
  })
}

export function useDeleteAttendance(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (id: number) => unwrap(api.DELETE('/api/accounts/{slug}/attendance/{id}', pathId(slug, id))),
    onSuccess: () => inv('attendance'),
  })
}

export function useAttendanceAction(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: ({ action, ids, reason }: { action: 'approve' | 'reject' | 'reopen'; ids: number[]; reason?: string }) => {
      const body = { ids, reason }
      if (action === 'approve') return unwrap(api.POST('/api/accounts/{slug}/attendance/actions/approve', { ...path(slug), body }))
      if (action === 'reject') return unwrap(api.POST('/api/accounts/{slug}/attendance/actions/reject', { ...path(slug), body }))
      return unwrap(api.POST('/api/accounts/{slug}/attendance/actions/reopen', { ...path(slug), body }))
    },
    onSuccess: () => inv('attendance'),
  })
}

export function useCloseAttendance(slug: string) {
  const qc = useQueryClient()
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (through: string) =>
      unwrap(api.POST('/api/accounts/{slug}/attendance/actions/close', { ...path(slug), body: { through } })),
    onSuccess: (account) => {
      qc.setQueryData(keys.accountDetail(slug), account)
      return inv('attendance')
    },
  })
}

// ---- moje práce (dodavatel) ----

export function useClockIn(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (jobId?: number) =>
      unwrap(api.POST('/api/accounts/{slug}/me/attendance/clock-in', { ...path(slug), body: { job_id: jobId } })),
    onSuccess: () => inv('me'),
  })
}

export function useClockOut(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (body: ClockOutInput) => unwrap(api.POST('/api/accounts/{slug}/me/attendance/clock-out', { ...path(slug), body })),
    onSuccess: () => inv('me'),
  })
}

export function useSaveMyAttendance(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: ({ id, ...body }: AttendanceInput & { id?: number }) =>
      id
        ? unwrap(api.PATCH('/api/accounts/{slug}/me/attendance/{id}', { ...pathId(slug, id), body }))
        : unwrap(api.POST('/api/accounts/{slug}/me/attendance', { ...path(slug), body })),
    onSuccess: () => inv('me'),
  })
}

export function useDeleteMyAttendance(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (id: number) => unwrap(api.DELETE('/api/accounts/{slug}/me/attendance/{id}', pathId(slug, id))),
    onSuccess: () => inv('me'),
  })
}

export function useSubmitMyAttendance(slug: string) {
  const inv = useInvalidate(slug)
  return useMutation({
    mutationFn: (ids: number[]) =>
      unwrap(api.POST('/api/accounts/{slug}/me/attendance/actions/submit', { ...path(slug), body: { ids } })),
    onSuccess: () => inv('me'),
  })
}
