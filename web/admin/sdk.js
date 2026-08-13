export class MomobaseAPIError extends Error {
  constructor(status, code, message, details) { super(message); Object.assign(this, { name: 'MomobaseAPIError', status, code, details }) }
  static async fromResponse(r) { let b; try { b = await r.json() } catch { b = {} }; const e = b?.error || b; return new this(r.status, e?.code || 'HTTP_ERROR', e?.message || `HTTP ${r.status}`, b) }
}
const query = (o = {}) => { const q = new URLSearchParams(); if (o.page) q.set('page', o.page); if (o.perPage) q.set('per_page', o.perPage); return q.size ? `?${q}` : '' }
const endpoint = (path, id) => `${path}/${encodeURIComponent(id)}`
async function unwrap(r) { if (!r.ok) throw await MomobaseAPIError.fromResponse(r); const b = await r.json(); if (b && typeof b === 'object' && 'success' in b) { if (!b.success) throw new MomobaseAPIError(r.status, b.error?.code || 'API_ERROR', b.error?.message || b.message || 'API error', b); return b.data } return b }
const cached = (t, skew) => ({ accessToken: t.access_token, refreshToken: t.refresh_token, expiresAt: Date.now() + Math.max(Number(t.expires_in) - skew, 1) * 1000 })
class SessionClient {
  constructor(baseUrl, skew = 30) { this.baseUrl = baseUrl.replace(/\/$/, ''); this.skew = skew }
  clearToken() { this.token = null }
  setToken(t) { this.token = cached(t, this.skew); return t }
  async bearer(signal) { if (!this.token) await this.authenticate(signal); else if (this.token.expiresAt <= Date.now()) await this.refresh(signal); return this.token.accessToken }
  async form(path, values, signal) { const r = await fetch(this.baseUrl + path, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body: new URLSearchParams(values), signal }); if (!r.ok) throw await MomobaseAPIError.fromResponse(r); return this.setToken(await r.json()) }
  async request(method, path, payload, options = {}) { const headers = { Authorization: `Bearer ${await this.bearer(options.signal)}` }; if (method !== 'GET') headers['Content-Type'] = 'application/json'; if (options.idempotencyKey) headers['Idempotency-Key'] = options.idempotencyKey; return unwrap(await fetch(this.baseUrl + path, { method, headers, body: method === 'GET' ? undefined : JSON.stringify(payload || {}), signal: options.signal })) }
  get(path, o) { return this.request('GET', path, null, o) }
  post(path, p, o) { return this.request('POST', path, p, o) }
  patch(path, p, o) { return this.request('PATCH', path, p, o) }
}
export class MomobaseClient extends SessionClient {
  constructor(o) { super(o.baseUrl, o.tokenSkewSeconds); this.options = o; this.collections = { create: (p, x) => this.payment('collection', p, x) }; this.disbursements = { create: (p, x) => this.payment('disbursement', p, x) }; this.transactions = { get: (id, x) => this.get(endpoint('/api/v1/transactions', id), x), getByReference: (ref, x) => this.get(endpoint('/api/v1/transactions/by-reference', ref), x) } }
  authenticate(signal) { return this.form('/api/v1/token', { grant_type: 'client_credentials', client_id: this.options.clientId, client_secret: this.options.clientSecret }, signal) }
  async refresh(signal) { if (!this.token?.refreshToken) return this.authenticate(signal); try { return await this.form('/api/v1/token/refresh', { grant_type: 'refresh_token', refresh_token: this.token.refreshToken }, signal) } catch { this.clearToken(); return this.authenticate(signal) } }
  payment(kind, p, o = {}) { if (p.payment_method !== 'momo' || !p.momo?.phone || p.country?.length !== 2) throw new Error('valid momo payment, phone, and 2-letter country are required'); return this.post(`/api/v1/${kind === 'collection' ? 'collections' : 'disbursements'}`, p, o) }
}
export class MomobaseAdminClient extends SessionClient {
  constructor(o) { super(o.baseUrl, o.tokenSkewSeconds); this.email = o.email; this.password = o.password; if (o.accessToken) this.token = { accessToken: o.accessToken, refreshToken: o.refreshToken, expiresAt: Number.MAX_SAFE_INTEGER }; this.resources() }
  setCredentials(email, password) { this.email = email; this.password = password; this.clearToken() }
  setAccessToken(accessToken, refreshToken) { this.token = { accessToken, refreshToken, expiresAt: Number.MAX_SAFE_INTEGER } }
  authenticate(signal) { if (!this.email || !this.password) return Promise.reject(new Error('Admin email and password are required')); return this.form('/api/admin/token', { grant_type: 'password', username: this.email, password: this.password }, signal) }
  async refresh(signal) { if (!this.token?.refreshToken) return this.authenticate(signal); try { return await this.form('/api/admin/token/refresh', { grant_type: 'refresh_token', refresh_token: this.token.refreshToken }, signal) } catch { this.clearToken(); return this.authenticate(signal) } }
  logout() { return this.post('/api/admin/logout') }
  resources() {
    this.system = { info: () => this.get('/api/admin/system/info'), health: () => this.get('/api/admin/system/health'), workers: o => this.get(`/api/admin/workers${query(o)}`), runtimeProviders: o => this.get(`/api/admin/runtime/providers${query(o)}`) }
    this.users = { me: () => this.get('/api/admin/me'), list: o => this.get(`/api/admin/users${query(o)}`), create: p => this.post('/api/admin/users', p), changePassword: (id, password) => this.patch(endpoint('/api/admin/users', id) + '/password', { password }), changeStatus: (id, status) => this.patch(endpoint('/api/admin/users', id) + '/status', { status }) }
    this.apps = { list: o => this.get(`/api/admin/apps${query(o)}`), create: p => this.post('/api/admin/apps', p), get: id => this.get(endpoint('/api/admin/apps', id)), update: (id, p) => this.patch(endpoint('/api/admin/apps', id), p), changeStatus: (id, status) => this.patch(endpoint('/api/admin/apps', id) + '/status', { status }), credentials: (id, o) => this.get(`${endpoint('/api/admin/apps', id)}/credentials${query(o)}`), createCredential: (id, p) => this.post(endpoint('/api/admin/apps', id) + '/credentials', p), revokeCredential: (id, c) => this.patch(`${endpoint('/api/admin/apps', id)}/credentials/${encodeURIComponent(c)}/revoke`), rotateCredential: (id, c) => this.post(`${endpoint('/api/admin/apps', id)}/credentials/${encodeURIComponent(c)}/rotate`) }
    this.providers = { list: o => this.get(`/api/admin/providers${query(o)}`), registry: () => this.get('/api/admin/providers/registry'), createAccount: p => this.post('/api/admin/providers/accounts', p), updateCountries: (id, countries) => this.patch(endpoint('/api/admin/providers/accounts', id) + '/countries', { countries }), updateConfig: (id, config) => this.patch(endpoint('/api/admin/providers/accounts', id) + '/config', { config }), activate: id => this.patch(endpoint('/api/admin/providers/accounts', id) + '/activate'), deactivate: id => this.patch(endpoint('/api/admin/providers/accounts', id) + '/deactivate'), test: id => this.post(endpoint('/api/admin/providers/accounts', id) + '/test'), balance: (id, country) => this.get(endpoint('/api/admin/providers/accounts', id) + '/balance' + (country ? `?country=${encodeURIComponent(country)}` : '')), activeBalances: o => this.get(`/api/admin/balances/providers${query(o)}`), health: o => this.get(`/api/admin/health/providers${query(o)}`) }
    this.routes = { list: o => this.get(`/api/admin/routes${query(o)}`), create: p => this.post('/api/admin/routes', p), update: (id, p) => this.patch(endpoint('/api/admin/routes', id), p) }
    this.transactions = { list: o => this.get(`/api/admin/transactions${query(o)}`), auditLogs: o => this.get(`/api/admin/audit-logs${query(o)}`) }
  }
}
