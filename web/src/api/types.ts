// Doménové typy odvozené z vygenerovaného `schema.gen.ts` přes `paths`.
// Aplikace importuje typy odsud, ne z `components["schemas"]` — názvy schémat
// generuje backend (huma) a mohou se měnit, cesty a metody jsou kontrakt (SPEC).

import type { paths } from './schema.gen'

type HttpMethod = 'get' | 'post' | 'put' | 'patch' | 'delete'

type Operation<P extends keyof paths, M extends HttpMethod> = NonNullable<paths[P][M]>

type JsonContent<R> = R extends { content: { 'application/json': infer T } } ? T : never

/** JSON tělo úspěšné odpovědi (200/201). */
export type ResponseBody<P extends keyof paths, M extends HttpMethod> =
  Operation<P, M> extends { responses: infer R }
    ? R extends { 200: infer OK }
      ? JsonContent<OK>
      : R extends { 201: infer Created }
        ? JsonContent<Created>
        : never
    : never

/** JSON tělo požadavku. */
export type RequestBody<P extends keyof paths, M extends HttpMethod> =
  Operation<P, M> extends { requestBody?: infer B } ? JsonContent<NonNullable<B>> : never

type ListItem<L> = L extends { items: (infer T)[] | null } ? T : never

// --- Auth ---
export type AuthStatus = ResponseBody<'/api/auth/status', 'get'>
export type Me = ResponseBody<'/api/auth/me', 'get'>
export type MeUser = Me['user']
export type AccountMembership = NonNullable<Me['accounts']>[number]
export type LoginInput = RequestBody<'/api/auth/login', 'post'>
export type RegisterInput = RequestBody<'/api/auth/register', 'post'>
export type UpdateMeInput = RequestBody<'/api/auth/me', 'patch'>
export type ApiToken = ListItem<ResponseBody<'/api/auth/tokens', 'get'>>
export type ApiTokenCreated = ResponseBody<'/api/auth/tokens', 'post'>
export type CreateTokenInput = RequestBody<'/api/auth/tokens', 'post'>
export type LoginResult = ResponseBody<'/api/auth/login', 'post'>
export type EmailVerificationInfo = ResponseBody<'/api/auth/verify-email', 'post'>

export type OAuthConsent = ResponseBody<'/api/oauth/authorize', 'get'>
export type OAuthDecision = RequestBody<'/api/oauth/authorize', 'post'>
export type OAuthGrant = ListItem<ResponseBody<'/api/auth/oauth-grants', 'get'>>

// --- Správa instance ---
export type InstanceStatus = ResponseBody<'/api/admin/status', 'get'>
export type EmailTestInput = RequestBody<'/api/admin/email-test', 'post'>
export type EmailTestReport = ResponseBody<'/api/admin/email-test', 'post'>
export type EmailTestCheck = NonNullable<EmailTestReport['smtp']>[number]
export type AdminUser = ListItem<ResponseBody<'/api/admin/users', 'get'>>
export type TwoFactorChallenge = NonNullable<LoginResult['two_factor']>
export type TwoFactorMethod = TwoFactorChallenge['methods'][number]
export type TwoFactorStatus = ResponseBody<'/api/auth/2fa', 'get'>
export type WebAuthnKey = TwoFactorStatus['webauthn'][number]
export type TOTPSetup = ResponseBody<'/api/auth/2fa/totp/setup', 'post'>
export type PasswordResetInfo = ResponseBody<'/api/auth/password-reset/{token}', 'get'>

// --- Accounts ---
export type Account = ResponseBody<'/api/accounts/{slug}', 'get'>
export type UpdateAccountInput = RequestBody<'/api/accounts/{slug}', 'patch'>
export type CreateAccountInput = RequestBody<'/api/accounts', 'post'>
export type AresSubject = ResponseBody<'/api/ares/{ico}', 'get'>

// --- Klienti a zahrady ---
export type ClientList = ResponseBody<'/api/accounts/{slug}/clients', 'get'>
export type Client = ListItem<ClientList>
export type ClientDetail = ResponseBody<'/api/accounts/{slug}/clients/{id}', 'get'>
export type CreateClientInput = RequestBody<'/api/accounts/{slug}/clients', 'post'>
export type UpdateClientInput = RequestBody<'/api/accounts/{slug}/clients/{id}', 'patch'>
export type Site = ResponseBody<'/api/accounts/{slug}/sites/{id}', 'get'>
export type CreateSiteInput = RequestBody<'/api/accounts/{slug}/sites', 'post'>
export type UpdateSiteInput = RequestBody<'/api/accounts/{slug}/sites/{id}', 'patch'>

// --- Dodavatelé ---
export type ContractorList = ResponseBody<'/api/accounts/{slug}/contractors', 'get'>
export type Contractor = ListItem<ContractorList>
export type CreateContractorInput = RequestBody<'/api/accounts/{slug}/contractors', 'post'>
export type UpdateContractorInput = RequestBody<'/api/accounts/{slug}/contractors/{id}', 'patch'>

// --- Zakázky ---
export type JobList = ResponseBody<'/api/accounts/{slug}/jobs', 'get'>
export type Job = ListItem<JobList>
export type JobStatus = Job['status']
export type JobKind = Job['kind']
export type CreateJobInput = RequestBody<'/api/accounts/{slug}/jobs', 'post'>
export type UpdateJobInput = RequestBody<'/api/accounts/{slug}/jobs/{id}', 'patch'>
export type JobListQuery = NonNullable<Operation<'/api/accounts/{slug}/jobs', 'get'>['parameters']['query']>

// --- Docházka ---
export type AttendanceList = ResponseBody<'/api/accounts/{slug}/attendance', 'get'>
export type AttendanceEntry = ListItem<AttendanceList>
export type AttendanceStatus = AttendanceEntry['status']
export type AttendanceInput = RequestBody<'/api/accounts/{slug}/attendance', 'post'>
export type AttendanceSummary = ResponseBody<'/api/accounts/{slug}/attendance/summary', 'get'>
export type AttendanceContractorSummary = AttendanceSummary['contractors'][number]
export type MeContractor = ResponseBody<'/api/accounts/{slug}/me/contractor', 'get'>
export type MeJob = NonNullable<ResponseBody<'/api/accounts/{slug}/me/jobs', 'get'>>[number]
export type ClockOutInput = RequestBody<'/api/accounts/{slug}/me/attendance/clock-out', 'post'>

// --- Ceník a nabídky ---
export type PriceItemList = ResponseBody<'/api/accounts/{slug}/price-items', 'get'>
export type PriceItem = ListItem<PriceItemList>
export type PriceItemInput = RequestBody<'/api/accounts/{slug}/price-items', 'post'>
export type Quote = ResponseBody<'/api/accounts/{slug}/quotes/{id}', 'get'>
export type QuoteLine = Quote['items'][number]
export type QuoteInput = RequestBody<'/api/accounts/{slug}/quotes/{id}', 'put'>
export type QuoteSummary = NonNullable<ResponseBody<'/api/accounts/{slug}/jobs/{id}/quotes', 'get'>>[number]
export type QuoteStatus = QuoteSummary['status']

// --- Kalkulace ---
export type CalcPreviewInput = RequestBody<'/api/accounts/{slug}/calc/preview', 'post'>
export type CalcItem = CalcPreviewInput['items'][number]
export type CalcResult = ResponseBody<'/api/accounts/{slug}/calc/preview', 'post'>

// --- Attachments (přílohy) ---
export type Attachment = ResponseBody<'/api/accounts/{slug}/attachments/{id}', 'get'>
export type AttachmentOwnerType = Attachment['owner_type']

// --- Tým: členové a pozvánky ---
export type Member = ListItem<ResponseBody<'/api/accounts/{slug}/members', 'get'>>
export type MemberRole = Member['role']
export type UpdateMemberInput = RequestBody<'/api/accounts/{slug}/members/{user_id}', 'patch'>
export type Invitation = ListItem<ResponseBody<'/api/accounts/{slug}/invitations', 'get'>>
export type CreateInvitationInput = RequestBody<'/api/accounts/{slug}/members/invite', 'post'>
export type InvitationInfo = ResponseBody<'/api/invitations/{token}', 'get'>

// --- Události, úkoly, hledání ---
export type EventList = ResponseBody<'/api/accounts/{slug}/events', 'get'>
export type AppEvent = ListItem<EventList>
export type EventFilters = Omit<
  NonNullable<paths['/api/accounts/{slug}/events']['get']['parameters']['query']>,
  'page' | 'per_page'
>
export type EventCatalogEntry = NonNullable<ResponseBody<'/api/accounts/{slug}/events/catalog', 'get'>>[number]
export type TodoList = ResponseBody<'/api/accounts/{slug}/todos', 'get'>
export type Todo = ListItem<TodoList>
export type TodoFilters = Omit<
  NonNullable<paths['/api/accounts/{slug}/todos']['get']['parameters']['query']>,
  'page' | 'per_page'
>
export type CreateTodoInput = RequestBody<'/api/accounts/{slug}/todos', 'post'>
export type UpdateTodoInput = RequestBody<'/api/accounts/{slug}/todos/{id}', 'patch'>
export type TodoRelatedType = NonNullable<CreateTodoInput['related_type']>
export type SearchResults = ResponseBody<'/api/accounts/{slug}/search', 'get'>
export type SearchHit = SearchResults['jobs'][number]
