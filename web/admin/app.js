import { MomobaseAdminClient, MomobaseClient } from './sdk.js'

const countries = ['UG', 'KE', 'TZ', 'RW', 'ET', 'GH', 'NG']

// Provider codes come from the server registry, so any registered provider is
// offered. Configuration is provider-specific, so every code starts from the
// same generic template rather than a hardcoded per-adapter preset.
const providerPresets = {
  dummy: {
    name: 'Sandbox provider',
    config: '{\n  "currency": "UGX",\n  "webhook_secret": "replace-with-provider-webhook-secret",\n  "outcome": "succeed"\n}'
  }
}
const genericProviderConfig = '{\n  "currency": "UGX",\n  "webhook_secret": "replace-with-provider-webhook-secret"\n}'

function titleize(code) {
  return String(code || '').split(/[_-]+/).filter(Boolean).map(w => w[0].toUpperCase() + w.slice(1)).join(' ')
}
function providerPreset(code) {
  if (providerPresets[code]) return providerPresets[code]
  const label = titleize(code)
  return { name: label ? `${label} Main` : '', config: genericProviderConfig }
}

function pretty(v) { return JSON.stringify(v ?? null, null, 2) }
function items(page) { return page?.items || [] }
function nowRef(prefix) { return `${prefix}-${Date.now()}` }
function maskSecrets(text) {
  if (!text) return ''
  return text
    .replace(/("(?:api_key|client_secret|subscription_key|primary_key|collection_api_key|disbursement_api_key|webhook_secret)"\s*:\s*")[^"]*(")/gi, '$1********$2')
    .replace(/("(?:client_id|api_user|user_id)"\s*:\s*")[^"]{8,}(")/gi, '$1********$2')
}

window.adminApp = function () {
  return {
    baseUrl: localStorage.getItem('momobase.baseUrl') || window.location.origin,
    email: localStorage.getItem('momobase.admin.email') || 'admin@momobase.local',
    password: '',
    token: '',
    refreshToken: '',
    client: null,
    appClient: null,
    ready: false,
    loading: false,
    loadingMap: {},
    error: '',
    notice: '',
    secretNotice: '',
    showProviderSecrets: false,
    page: 'dashboard',
    providerCodes: [],
    selectedAppId: '',
    selectedProviderId: '',
    selectedTransaction: null,
    drawerOpen: false,
    confirmState: { open: false, title: '', message: '', confirmText: 'Confirm', action: null },
    appCred: { clientId: '', clientSecret: '' },
    forms: {
      admin: { name: '', email: '', password: '', role: 'operations' },
      app: { name: '', description: '', environment: 'sandbox', status: 'active' },
      credential: { name: 'SDK Client', scopes: 'collections:create disbursements:create transactions:read', expires_at: '' },
      provider: { provider_code: '', name: '', environment: 'sandbox', countryList: 'UG', config: genericProviderConfig },
      route: { service_type: 'collection', payment_method: 'momo', provider_account_id: '', priority: 1, active: true },
      collection: { amount: 5000, reference: nowRef('COLL'), country: 'UG', account: '256771111111', scheme: '' },
      disbursement: { amount: 2500, reference: nowRef('DISB'), country: 'UG', account: '256772222222', scheme: '' },
      lookup: { id: '', reference: '' }
    },
    data: {
      me: null, systemInfo: null, systemHealth: null, workers: null, runtimeProviders: null,
      users: null, apps: null, credentials: null, providers: null, routes: null, health: null,
      balances: null, transactions: null, auditLogs: null, appLookup: null, providerBalance: null,
      appPaymentResult: null, appTransaction: null, appTransactionByReference: null
    },
    nav: [
      ['dashboard', 'Dashboard'], ['apps', 'Apps'], ['credentials', 'Credentials'], ['providers', 'Providers'],
      ['routes', 'Routes'], ['transactions', 'Transactions'], ['users', 'Users'], ['operations', 'Operations'],
      ['api-tester', 'API tester'], ['audit', 'Audit']
    ],
    init() {
      this.client = new MomobaseAdminClient({ baseUrl: this.baseUrl, accessToken: this.token || undefined, refreshToken: this.refreshToken || undefined, email: this.email, password: this.password })
      if (this.token) this.bootstrap().catch(e => this.fail(e))
    },
    setBaseUrl() { localStorage.setItem('momobase.baseUrl', this.baseUrl); this.client = new MomobaseAdminClient({ baseUrl: this.baseUrl, accessToken: this.token || undefined, refreshToken: this.refreshToken || undefined, email: this.email, password: this.password }) },
    setPage(p) { this.page = p; this.notice = ''; this.error = ''; this.refreshPage() },
    isLoading(k) { return !!this.loadingMap[k] },
    async run(label, fn, key = 'global') {
      this.loading = true; this.loadingMap[key] = true; this.error = ''; this.notice = ''
      try { const out = await fn(); if (label) this.notice = label; return out }
      catch (e) { this.fail(e); throw e }
      finally { this.loading = false; this.loadingMap[key] = false }
    },
    fail(e) { this.error = e?.message || String(e); console.error(e) },
    badgeClass(value) {
      const v = String(value ?? '').toLowerCase()
      if (['active','healthy','succeeded','success','closed','true'].includes(v)) return 'bg-black text-white'
      if (['processing','pending','degraded','half_open'].includes(v)) return 'border border-black bg-white text-black'
      if (['failed','down','disabled','inactive','suspended','misconfigured','open','false'].includes(v)) return 'border border-black bg-white text-black line-through'
      return 'border border-black bg-white text-black'
    },
    maskedConfig() { return this.showProviderSecrets ? this.forms.provider.config : maskSecrets(this.forms.provider.config) },
    providerCode(provider) {
      const runtime = items(this.data.runtimeProviders).find((p) => p.provider_account_id === provider.id)
      return provider.provider_code || runtime?.provider_code || '-'
    },
    providerConfigForSubmit() {
      const cfg = JSON.parse(this.forms.provider.config || '{}')
      cfg.webhook_secret = cfg.webhook_secret || 'replace-with-provider-webhook-secret'
      delete cfg.country
      delete cfg.supports_global
      return cfg
    },
    syncProviderConfig() { try { this.forms.provider.config = pretty(this.providerConfigForSubmit()) } catch (e) { this.fail(e) } },
    providerCountries() { return [...new Set(this.forms.provider.countryList.split(',').map(v => v.trim().toUpperCase()).filter(Boolean))] },
    loadProviderCountries() {
      const provider = items(this.data.providers).find(p => p.id === this.selectedProviderId)
      if (provider) this.forms.provider.countryList = (provider.countries || []).join(', ')
    },
    async copy(text) { await navigator.clipboard.writeText(String(text || '')); this.notice = 'Copied to clipboard' },
    ask(title, message, action, confirmText = 'Confirm') { this.confirmState = { open: true, title, message, confirmText, action } },
    async confirmNow() { const action = this.confirmState.action; this.confirmState.open = false; if (action) await action() },
    cancelConfirm() { this.confirmState.open = false; this.confirmState.action = null },
    openTransaction(t) { this.selectedTransaction = t; this.drawerOpen = true },
    closeDrawer() { this.drawerOpen = false },
    async login() {
      await this.run('Logged in', async () => {
        this.setBaseUrl(); this.client.setCredentials(this.email, this.password)
        const token = await this.client.authenticate(); this.token = token.access_token; this.refreshToken = token.refresh_token || ''
        localStorage.setItem('momobase.admin.email', this.email); this.ready = true; await this.refreshAll()
      }, 'login')
    },
    async logout() { try { if (this.client) await this.client.logout() } catch {}; this.token=''; this.refreshToken=''; this.ready=false },
    async bootstrap() { this.ready = true; await this.refreshAll() },
    async refreshPage() {
      if (!this.ready) return
      const map = { dashboard:()=>this.refreshOps(), apps:()=>this.refreshApps(), credentials:()=>this.refreshCredentials(), providers:()=>this.refreshProviders(), routes:()=>this.refreshRoutes(), transactions:()=>this.refreshTransactions(), users:()=>this.refreshUsers(), operations:()=>this.refreshOps(), audit:()=>this.refreshAudit(), 'api-tester':()=>this.refreshApps() }
      await (map[this.page] || (()=>Promise.resolve()))()
    },
    async refreshAll() { await this.run('Refreshed', async () => Promise.all([this.refreshOps(), this.refreshUsers(), this.refreshApps(), this.refreshProviders(), this.refreshRoutes(), this.refreshTransactions(), this.refreshAudit()]), 'refresh') },
    async refreshOps() { const [me, systemInfo, systemHealth, workers, runtimeProviders, health, balances] = await Promise.all([this.client.users.me(), this.client.system.info(), this.client.system.health(), this.client.system.workers(), this.client.system.runtimeProviders(), this.client.providers.health(), this.client.providers.activeBalances()]); Object.assign(this.data, { me, systemInfo, systemHealth, workers, runtimeProviders, health, balances }) },
    async refreshUsers() { this.data.users = await this.client.users.list() },
    async refreshApps() { this.data.apps = await this.client.apps.list(); if (!this.selectedAppId && items(this.data.apps)[0]) this.selectedAppId = items(this.data.apps)[0].id },
    async refreshCredentials() { await this.refreshApps(); if (this.selectedAppId) this.data.credentials = await this.client.apps.credentials(this.selectedAppId) },
    async refreshProviders() { const [providers, runtimeProviders] = await Promise.all([this.client.providers.list(), this.client.system.runtimeProviders().catch(() => this.data.runtimeProviders), this.refreshProviderRegistry()]); this.data.providers = providers; if (runtimeProviders) this.data.runtimeProviders = runtimeProviders; if (!this.selectedProviderId && items(this.data.providers)[0]) this.selectedProviderId = items(this.data.providers)[0].id; this.loadProviderCountries(); if (!this.forms.route.provider_account_id && this.selectedProviderId) this.forms.route.provider_account_id = this.selectedProviderId },
    // The selectable provider codes are whatever the server has registered,
    // including providers added by an application embedding Momobase.
    async refreshProviderRegistry() { const registry = await this.client.providers.registry(); this.providerCodes = registry?.providers || []; if (!this.providerCodes.includes(this.forms.provider.provider_code)) { this.forms.provider.provider_code = this.providerCodes[0] || ''; this.providerConfigTemplate() } },
    async refreshRoutes() { this.data.routes = await this.client.routes.list() },
    async refreshTransactions() { this.data.transactions = await this.client.transactions.list() },
    async refreshAudit() { this.data.auditLogs = await this.client.transactions.auditLogs() },
    async createAdmin() { await this.run('Admin created', async()=>{ await this.client.users.create(this.forms.admin); await this.refreshUsers() }, 'createAdmin') },
    async changeAdminStatus(id, status) { this.ask('Change admin status', `Set admin to ${status}?`, () => this.run('Admin status updated', async()=>{ await this.client.users.changeStatus(id, status); await this.refreshUsers() }, 'adminStatus'), 'Update') },
    async changeAdminPassword(id) { const password = prompt('New password'); if (password) await this.run('Password updated', () => this.client.users.changePassword(id, password), 'adminPassword') },
    async createApp() { await this.run('App created', async()=>{ const app = await this.client.apps.create(this.forms.app); this.selectedAppId = app.id; await this.refreshApps() }, 'createApp') },
    async getApp() { if (!this.selectedAppId) return; this.data.appLookup = await this.run('App loaded', () => this.client.apps.get(this.selectedAppId), 'getApp') },
    async updateApp() { if (!this.selectedAppId) return; await this.run('App updated', async()=>{ await this.client.apps.update(this.selectedAppId, this.forms.app); await this.refreshApps(); await this.getApp() }, 'updateApp') },
    async changeAppStatus(status) { if (!this.selectedAppId) return; this.ask('Change app status', `Set selected app to ${status}?`, () => this.run('App status updated', async()=>{ await this.client.apps.changeStatus(this.selectedAppId, status); await this.refreshApps() }, 'appStatus'), 'Update') },
    async createCredential() { if (!this.selectedAppId) return; const created = await this.run('Credential created', () => this.client.apps.createCredential(this.selectedAppId, this.forms.credential), 'createCredential'); this.secretNotice = 'Secret shown once. Copy it now.'; this.appCred.clientId = created.credential.client_id; this.appCred.clientSecret = created.client_secret; await this.refreshCredentials() },
    async revokeCredential(id) { this.ask('Revoke credential', 'This credential will stop working immediately. Continue?', () => this.run('Credential revoked', async()=>{ await this.client.apps.revokeCredential(this.selectedAppId, id); await this.refreshCredentials() }, 'revokeCredential'), 'Revoke') },
    async rotateCredential(id) { this.ask('Rotate credential', 'The old secret will stop working. Continue?', async()=>{ const created = await this.run('Credential rotated', () => this.client.apps.rotateCredential(this.selectedAppId, id), 'rotateCredential'); this.secretNotice='New secret shown once. Copy it now.'; this.appCred.clientId=created.credential.client_id; this.appCred.clientSecret=created.client_secret; await this.refreshCredentials() }, 'Rotate') },
    providerConfigTemplate() { const preset = providerPreset(this.forms.provider.provider_code); this.forms.provider.config = preset.config; this.forms.provider.name = preset.name; this.syncProviderConfig() },
    async createProvider() { await this.run('Provider created', async()=>{ const payload = { provider_code: this.forms.provider.provider_code, name: this.forms.provider.name, environment: this.forms.provider.environment, countries: this.providerCountries(), config: this.providerConfigForSubmit() }; const p = await this.client.providers.createAccount(payload); this.selectedProviderId = p.id; this.forms.route.provider_account_id = p.id; await this.refreshProviders() }, 'createProvider') },
    async updateProviderCountries(id=this.selectedProviderId) { if (!id) return; await this.run('Provider countries updated', async()=>{ await this.client.providers.updateCountries(id, this.providerCountries()); await this.refreshProviders() }, 'providerCountries') },
    async updateProviderConfig(id=this.selectedProviderId) { if (!id) return; await this.run('Provider config updated', () => this.client.providers.updateConfig(id, this.providerConfigForSubmit()), 'providerConfig') },
    async activateProvider(id) { this.ask('Activate provider', 'Provider will be loaded into runtime and may become routable if healthy. Continue?', () => this.run('Provider activated', async()=>{ await this.client.providers.activate(id); await this.refreshProviders(); await this.refreshOps() }, 'activateProvider'), 'Activate') },
    async deactivateProvider(id) { this.ask('Deactivate provider', 'New transactions will stop using this provider immediately. Continue?', () => this.run('Provider deactivated', async()=>{ await this.client.providers.deactivate(id); await this.refreshProviders(); await this.refreshOps() }, 'deactivateProvider'), 'Deactivate') },
    async testProvider(id) { await this.run('Provider tested', () => this.client.providers.test(id), 'testProvider') },
    async providerBalance(id) { const provider = items(this.data.providers).find(p => p.id === id); this.data.providerBalance = await this.run('Provider balance queried', () => this.client.providers.balance(id, provider?.countries?.[0]), 'providerBalance') },
    async createRoute() { await this.run('Route created', async()=>{ await this.client.routes.create(this.forms.route); await this.refreshRoutes() }, 'createRoute') },
    async updateRoute(route) { this.ask('Update route', 'Save route priority/active changes?', () => this.run('Route updated', async()=>{ await this.client.routes.update(route.id, { priority: Number(route.priority), active: Boolean(route.active) }); await this.refreshRoutes() }, 'updateRoute'), 'Save') },
    makeAppClient() { this.appClient = new MomobaseClient({ baseUrl: this.baseUrl, clientId: this.appCred.clientId, clientSecret: this.appCred.clientSecret }); return this.appClient },
    async appToken() { await this.run('App token created', async()=>{ this.data.appPaymentResult = await this.makeAppClient().authenticate() }, 'appToken') },
    async appCollection() { const p=this.forms.collection; const payload={payment_method:'momo',amount:Number(p.amount),currency:'UGX',country:p.country,reference:p.reference,description:'Admin panel test collection',account:{account:p.account,scheme:p.scheme},customer:{name:'Test User'}}; const res=await this.run('Collection created',()=>this.makeAppClient().collections.create(payload,{idempotencyKey:`ui-coll-${p.reference}`}), 'appCollection'); this.data.appPaymentResult=res; this.forms.lookup.id=res.transaction_id; this.forms.lookup.reference=p.reference; await this.refreshTransactions() },
    async appDisbursement() { const p=this.forms.disbursement; const payload={payment_method:'momo',amount:Number(p.amount),currency:'UGX',country:p.country,reference:p.reference,description:'Admin panel test disbursement',account:{account:p.account,scheme:p.scheme},recipient:{name:'Test Recipient'}}; const res=await this.run('Disbursement created',()=>this.makeAppClient().disbursements.create(payload,{idempotencyKey:`ui-disb-${p.reference}`}), 'appDisbursement'); this.data.appPaymentResult=res; this.forms.lookup.id=res.transaction_id; this.forms.lookup.reference=p.reference; await this.refreshTransactions() },
    async appGetTransaction() { this.data.appTransaction = await this.run('Transaction loaded', () => this.makeAppClient().transactions.get(this.forms.lookup.id), 'appGetTransaction') },
    async appGetByReference() { this.data.appTransactionByReference = await this.run('Transaction loaded', () => this.makeAppClient().transactions.getByReference(this.forms.lookup.reference), 'appGetByReference') },
    countries, items, pretty
  }
}
