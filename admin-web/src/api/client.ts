import type { ActivitiesFilter, ActivitiesResult, AdminSummary, AIInteractionsFilter, AIInteractionsResult, AuditLog, BrowserCapturePolicy, ApplicationCapturePolicy, DataQualityFactPage, DataQualitySummary, Device, EffectivenessReport, EffectivenessSubject, Event, EventDetail, ProviderHealth, RAGFilters, RAGQueryJob, RAGStatus, RoleAssignment, Session, Subject, WorkRole, WorkEpisodesResult, AdapterHealthResult } from './types'

const API_PREFIX = '/api/v1'

function csrfToken(): string | undefined {
  const value = document.cookie.split('; ').find(item => item.startsWith('aetheris_csrf='))
  return value?.slice('aetheris_csrf='.length)
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method || 'GET').toUpperCase()
  const headers: Record<string, string> = { 'Content-Type': 'application/json', ...(init.headers as Record<string, string> || {}) }
  if (method !== 'GET' && method !== 'HEAD') {
    const token = csrfToken()
    if (token) headers['X-CSRF-Token'] = token
  }
  const response = await fetch(`${API_PREFIX}${path}`, { credentials: 'include', ...init, headers })
  if (response.status === 401) {
    window.dispatchEvent(new CustomEvent('aetheris:unauthorized'))
  }
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new Error(body.message || `请求失败（${response.status}）`)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

type WorkRoleSnapshot = { code?: WorkRole }
type WireEvent = Omit<Event, 'work_role'> & { work_role?: WorkRole | WorkRoleSnapshot | null }
type WireEventDetail = Omit<EventDetail, 'work_role'> & { work_role?: WorkRole | WorkRoleSnapshot | null }

function normalizeWorkRole(value: WorkRole | WorkRoleSnapshot | null | undefined): WorkRole | undefined {
  if (typeof value === 'string') return value
  return value?.code
}

function normalizeEvent<T extends WireEvent | WireEventDetail>(event: T): T & { work_role?: WorkRole } {
  return { ...event, work_role: normalizeWorkRole(event.work_role) }
}

export const api = {
  login: (username: string, password: string) => request<Session>('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
  me: () => request<Session>('/admin/me'),
  listDevices: () => request<Device[]>('/admin/devices'),
  listSubjects: () => request<Subject[]>('/admin/subjects'),
  getSummary: () => request<AdminSummary>('/admin/summary'),
  getProviderHealth: () => request<ProviderHealth[]>('/admin/health/providers'),
  getBrowserPolicy: () => request<BrowserCapturePolicy>('/admin/browser-policy'),
  updateBrowserPolicy: (policy: Pick<BrowserCapturePolicy, 'enabled' | 'allowed_domains'>) => request<BrowserCapturePolicy>('/admin/browser-policy', { method: 'PUT', body: JSON.stringify(policy) }),
  getApplicationCapturePolicy: () => request<ApplicationCapturePolicy>('/admin/application-capture-policy'),
  updateApplicationCapturePolicy: (policy: Pick<ApplicationCapturePolicy, 'enabled'>) => request<ApplicationCapturePolicy>('/admin/application-capture-policy', { method: 'PUT', body: JSON.stringify(policy) }),
  listWorkRoles: () => request<RoleAssignment[]>('/admin/work-roles'),
  assignWorkRole: (assignment: Omit<RoleAssignment, 'id'>) => request<RoleAssignment>('/admin/work-role-assignments', { method: 'PUT', body: JSON.stringify(assignment) }),
  listEvents: async (workRole?: WorkRole) => { const result = await request<{ events: WireEvent[]; count: number }>(`/admin/events${workRole ? `?work_role=${encodeURIComponent(workRole)}` : ''}`); return result.events.map(normalizeEvent) },
  getEvent: async (eventId: string) => normalizeEvent(await request<WireEventDetail>(`/admin/events/${encodeURIComponent(eventId)}`)),
  tombstoneEvent: (eventId: string, reason: string) => request<void>(`/admin/events/${encodeURIComponent(eventId)}/tombstone`, { method: 'POST', body: JSON.stringify({ reason }) }),
  listAuditLogs: () => request<AuditLog[]>('/admin/audit-logs'),
  createModelRun: (eventId: string) => request<{ run_id: string; status: string }>('/admin/model-runs', { method: 'POST', body: JSON.stringify({ input_event_ids: [eventId], task: 'summarize', model: 'default' }) }),
  listEffectivenessSubjects: async (from: string, to: string) => {
    const result = await request<{ subjects: EffectivenessSubject[]; count: number }>(`/admin/effectiveness/subjects?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`)
    return result.subjects
  },
  getEffectiveness: (subjectId: string, from: string, to: string) => request<EffectivenessReport>(`/admin/effectiveness?subject_id=${encodeURIComponent(subjectId)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`),
  recomputeEffectiveness: (from: string, to: string) => request<{ job_id: string; status: string }>('/admin/effectiveness/recompute', { method: 'POST', body: JSON.stringify({ from, to }) }),
  summarizeEffectiveness: (subjectId: string, from: string, to: string) => request<{ result?: string; notice: string }>('/admin/effectiveness/summary', { method: 'POST', body: JSON.stringify({ subject_id: subjectId, from, to }) }),
  listAIInteractions: (filter: AIInteractionsFilter) => {
    const params = new URLSearchParams()
    Object.entries(filter).forEach(([key, value]) => { if (value !== undefined && value !== '') params.set(key, String(value)) })
    return request<AIInteractionsResult>(`/admin/ai-interactions?${params.toString()}`)
  },
  listActivities: (filter: ActivitiesFilter) => {
    const params = new URLSearchParams()
    Object.entries(filter).forEach(([key, value]) => { if (value !== undefined && value !== '') params.set(key, String(value)) })
    return request<ActivitiesResult>(`/admin/activities?${params.toString()}`)
  },
  getRAGStatus: () => request<RAGStatus>('/admin/rag/status'),
  createRAGQuery: (input: { question: string; filters: RAGFilters }) => request<{ query_id: string; status: string; progress: number }>('/admin/rag/queries', { method: 'POST', body: JSON.stringify(input) }),
  getRAGQuery: (queryID: string) => request<RAGQueryJob>(`/admin/rag/queries/${encodeURIComponent(queryID)}`),
  getDataQualitySummary: (from: string, to: string) => request<DataQualitySummary>(`/admin/data-quality/summary?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`),
  listDataQualityFacts: (from: string, to: string, qualityState = '', offset = 0) => request<DataQualityFactPage>(`/admin/data-quality/facts?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&quality_state=${encodeURIComponent(qualityState)}&limit=50&offset=${offset}`),
  recomputeDataQuality: (from: string, to: string) => request<{ job_id: string; status: string }>('/admin/data-quality/recompute', { method: 'POST', body: JSON.stringify({ from, to }) }),
  listWorkEpisodes: (filter: { project_id?: string; subject_id?: string; device_id?: string; status?: string; limit?: number; offset?: number } = {}) => {
    const params = new URLSearchParams(); Object.entries(filter).forEach(([key, value]) => { if (value !== undefined && value !== '') params.set(key, String(value)) })
    return request<WorkEpisodesResult>(`/admin/work-episodes?${params.toString()}`)
  },
  getWorkEpisode: (id: string) => request<import('./types').WorkEpisode>(`/admin/work-episodes/${encodeURIComponent(id)}`),
  listAdapterHealth: (deviceID = '') => request<AdapterHealthResult>(`/admin/adapter-health${deviceID ? `?device_id=${encodeURIComponent(deviceID)}` : ''}`),
  listProjects: () => request<{ projects: import('./types').LogicalProject[]; count: number }>('/admin/projects'),
  startProjectBackfill: (mode: 'dry_run' | 'apply', ruleVersion: number) => request<{ id: string; state: string }>('/admin/project-backfills', { method: 'POST', body: JSON.stringify({ mode, rule_version: ruleVersion }) }),
}
