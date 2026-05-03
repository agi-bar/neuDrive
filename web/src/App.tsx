import { Suspense, lazy, useCallback, useEffect, useState } from 'react'
import { Navigate, NavLink, Outlet, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { api, BILLING_REDIRECT_EVENT, type BillingRedirectDetail, type BillingStatus, type DashboardStats, type PublicConfig } from './api'
import LanguageToggle from './components/LanguageToggle'
import { useI18n } from './i18n'

const LoginPage = lazy(() => import('./pages/LoginPage'))
const DashboardPage = lazy(() => import('./pages/DashboardPage'))
const BillingPage = lazy(() => import('./pages/BillingPage'))
const BillingSuccessPage = lazy(() => import('./pages/BillingSuccessPage'))
const ConnectionsPage = lazy(() => import('./pages/ConnectionsPage'))
const InfoPage = lazy(() => import('./pages/InfoPage'))
const ProjectsPage = lazy(() => import('./pages/ProjectsPage'))
const SetupPage = lazy(() => import('./pages/SetupPage'))
const OAuthAuthorizePage = lazy(() => import('./pages/OAuthAuthorizePage'))
const SetupWebAppsPage = lazy(() => import('./pages/setup/SetupWebAppsPage'))
const SetupCloudPage = lazy(() => import('./pages/setup/SetupCloudPage'))
const SetupLocalPage = lazy(() => import('./pages/setup/SetupLocalPage'))
const SetupAdvancedPage = lazy(() => import('./pages/setup/SetupAdvancedPage'))
const SetupGptActionsPage = lazy(() => import('./pages/setup/SetupGptActionsPage'))
const FilesBrowserPage = lazy(() => import('./pages/data/FilesBrowserPage'))
const DataFileEditorPage = lazy(() => import('./pages/data/DataFileEditorPage'))
const DataSkillsPage = lazy(() => import('./pages/data/DataSkillsPage'))
const DataMemoryPage = lazy(() => import('./pages/data/DataMemoryPage'))
const DataConversationsPage = lazy(() => import('./pages/data/DataConversationsPage'))
const ClaudeImportPage = lazy(() => import('./pages/ClaudeImportPage'))
const SystemSettingsPage = lazy(() => import('./pages/SystemSettingsPage'))
const SyncLoginPage = lazy(() => import('./pages/SyncLoginPage'))
const SkillsImportPage = lazy(() => import('./pages/SkillsImportPage'))
const GitMirrorPage = lazy(() => import('./pages/GitMirrorPage'))
const ClaudeMigrationPage = lazy(() => import('./pages/ClaudeMigrationPage'))
const PlanGatePage = lazy(() => import('./pages/PlanGatePage'))
const OnboardingPage = lazy(() => import('./pages/OnboardingPage'))
const DeveloperAccessPage = lazy(() => import('./pages/DeveloperAccessPage'))
const MarketingHomePage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.MarketingHomePage })))
const PricingPage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.PricingPage })))
const IntegrationsPage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.IntegrationsPage })))
const IntegrationDetailPage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.IntegrationDetailPage })))
const DocsLandingPage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.DocsLandingPage })))
const GuidePage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.GuidePage })))
const SignupPage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.SignupPage })))
const PrivacyPage = lazy(() => import('./pages/PublicPages').then(({ LegalPage }) => ({ default: () => <LegalPage kind="privacy" /> })))
const TermsPage = lazy(() => import('./pages/PublicPages').then(({ LegalPage }) => ({ default: () => <LegalPage kind="terms" /> })))

const emptyStats: DashboardStats = {
  connections: 0,
  files: 0,
  projects: 0,
  conversations: 0,
  skills: 0,
  memory: 0,
  profile: 0,
  weekly_activity: [],
  pending: [],
}

function App() {
  const [user, setUser] = useState<any>(null)
  const [publicConfig, setPublicConfig] = useState<PublicConfig>({})
  const [shellStats, setShellStats] = useState<DashboardStats>(emptyStats)
  const [shellBilling, setShellBilling] = useState<BillingStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const { tx } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const systemSettingsEnabled = !!publicConfig?.system_settings_enabled
  const localMode = !!publicConfig?.local_mode
  const billingEnabled = !!publicConfig?.billing_enabled
  const importsHomePath = localMode ? '/imports/claude' : '/imports/claude-export'

  const checkAuth = useCallback(async () => {
    const clearAuthParamsFromURL = () => {
      const nextURL = new URL(window.location.href)
      nextURL.searchParams.delete('auth_token')
      nextURL.searchParams.delete('auth_refresh')
      nextURL.searchParams.delete('local_token')
      const next = `${nextURL.pathname}${nextURL.search}${nextURL.hash}`
      window.history.replaceState({}, '', next || nextURL.pathname)
    }

    const params = new URLSearchParams(window.location.search)
    const authToken = params.get('auth_token')
    const authRefresh = params.get('auth_refresh')
    const localToken = params.get('local_token')
    if (authToken) {
      localStorage.setItem('token', authToken)
      if (authRefresh) localStorage.setItem('refresh_token', authRefresh)
      else localStorage.removeItem('refresh_token')
      clearAuthParamsFromURL()
    }
    if (localToken) {
      localStorage.setItem('token', localToken)
      localStorage.removeItem('refresh_token')
      clearAuthParamsFromURL()
    }

    let cfg: PublicConfig = {}
    try {
      cfg = await api.getPublicConfig()
      setPublicConfig(cfg || {})
    } catch {
      setPublicConfig({})
    }

    const bootstrapLocalOwner = async (): Promise<string | null> => {
      if (!cfg?.local_mode) return null
      try {
        const created = await api.bootstrapLocalOwnerToken()
        if (!created?.token) return null
        localStorage.setItem('token', created.token)
        localStorage.removeItem('refresh_token')
        return created.token
      } catch {
        return null
      }
    }

    let token = localStorage.getItem('token')
    if (!token) token = await bootstrapLocalOwner() || ''
    if (!token) {
      setUser(null)
      setLoading(false)
      return
    }

    try {
      setUser(await api.getMe())
    } catch {
      localStorage.removeItem('token')
      localStorage.removeItem('refresh_token')
      const fallbackToken = await bootstrapLocalOwner()
      if (!fallbackToken) {
        setUser(null)
        setLoading(false)
        return
      }
      try {
        setUser(await api.getMe())
      } catch {
        localStorage.removeItem('token')
        localStorage.removeItem('refresh_token')
        setUser(null)
      }
    }
    setLoading(false)
  }, [])

  useEffect(() => {
    void checkAuth()
  }, [checkAuth])

  useEffect(() => {
    if (!user) return
    let cancelled = false
    const loadShellState = async () => {
      const [statsResult, billingResult] = await Promise.allSettled([
        api.getStats(),
        billingEnabled ? api.getBillingStatus() : Promise.resolve(null),
      ])
      if (cancelled) return
      if (statsResult.status === 'fulfilled') setShellStats(statsResult.value)
      if (billingResult.status === 'fulfilled') setShellBilling(billingResult.value)
    }
    void loadShellState()
    return () => {
      cancelled = true
    }
  }, [billingEnabled, user])

  useEffect(() => {
    if (!user) return
    const isSignupReturn = localStorage.getItem('neudrive.postSignupIntent') === '1'
    if (!isSignupReturn) return
    if (location.pathname.startsWith('/plan') || location.pathname.startsWith('/onboarding') || location.pathname.startsWith('/billing')) return
    navigate(billingEnabled ? '/plan' : '/onboarding', { replace: true })
  }, [billingEnabled, location.pathname, navigate, user])

  useEffect(() => {
    const onBillingRedirect = (event: Event) => {
      if (!billingEnabled) return
      const detail = (event as CustomEvent<BillingRedirectDetail>).detail
      const params = new URLSearchParams()
      if (detail?.code) params.set('reason', detail.code)
      params.set('ts', String(Date.now()))
      navigate(`/settings/billing?${params.toString()}`)
    }
    window.addEventListener(BILLING_REDIRECT_EVENT, onBillingRedirect as EventListener)
    return () => window.removeEventListener(BILLING_REDIRECT_EVENT, onBillingRedirect as EventListener)
  }, [billingEnabled, navigate])

  useEffect(() => {
    if (!user) return
    const path = location.pathname
    const pageTitle =
      path === '/' ? 'Home' :
      path.startsWith('/plan') ? 'Choose Plan' :
      path.startsWith('/onboarding') ? 'Get Started' :
      path.startsWith('/connections') ? 'Connections' :
      path.startsWith('/sync-backup') || path.startsWith('/git-mirror') ? 'Sync & Backup' :
      path.startsWith('/settings/billing') || path.startsWith('/billing') ? 'Plan & Billing' :
      path.startsWith('/settings/developer-access') || path.startsWith('/settings/developer') ? 'Developer Access' :
      path.startsWith('/settings/security') ? 'Security' :
      path.startsWith('/settings/profile') || path.startsWith('/info') ? 'My Profile' :
      path.startsWith('/setup') ? 'Setup Guide' :
      path.startsWith('/data/conversations') ? 'Conversations' :
      path.startsWith('/data/projects') || path.startsWith('/projects') ? 'Projects' :
      path.startsWith('/data') ? 'Data Explorer' :
      path.startsWith('/memory') ? 'Memory' :
      path.startsWith('/skills') ? 'Skills' :
      path.startsWith('/imports') ? 'Data Imports' :
      'neuDrive'
    document.title = pageTitle === 'neuDrive' ? 'neuDrive' : `${pageTitle} — neuDrive`
  }, [location.pathname, user])

  const routeFallback = (
    <div className="loading-screen">
      <div className="loading-spinner" />
      <p>{tx('页面加载中...', 'Loading page...')}</p>
    </div>
  )

  const handleLogout = async () => {
    await api.logout()
    setUser(null)
    navigate('/login')
  }

  if (loading) {
    return (
      <div className="loading-screen">
        <div className="loading-spinner" />
        <p>{tx('加载中...', 'Loading...')}</p>
      </div>
    )
  }

  if (location.pathname === '/oauth/authorize') {
    return <Suspense fallback={routeFallback}><OAuthAuthorizePage /></Suspense>
  }

  if (location.pathname === '/import/skills') {
    return <Suspense fallback={routeFallback}><SkillsImportPage /></Suspense>
  }

  if (location.pathname.startsWith('/guides/') || location.pathname.startsWith('/integrations/') || location.pathname === '/privacy' || location.pathname === '/terms') {
    return (
      <Suspense fallback={routeFallback}>
        <Routes>
          <Route path="/guides/:platform" element={<GuidePage />} />
          <Route path="/integrations/:platform" element={<IntegrationDetailPage />} />
          <Route path="/privacy" element={<PrivacyPage />} />
          <Route path="/terms" element={<TermsPage />} />
        </Routes>
      </Suspense>
    )
  }

  if (!user) {
    const protectedSignupRedirect = `/signup?redirect=${encodeURIComponent(location.pathname + location.search)}`
    const protectedLoginRedirect = `/login?redirect=${encodeURIComponent(location.pathname + location.search)}`
    return (
      <Suspense fallback={routeFallback}>
        <Routes>
          <Route path="/" element={<MarketingHomePage />} />
          <Route path="/pricing" element={<PricingPage />} />
          <Route path="/integrations" element={<IntegrationsPage />} />
          <Route path="/integrations/:platform" element={<IntegrationDetailPage />} />
          <Route path="/docs" element={<DocsLandingPage />} />
          <Route path="/guides/:platform" element={<GuidePage />} />
          <Route path="/privacy" element={<PrivacyPage />} />
          <Route path="/terms" element={<TermsPage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/signup" element={<SignupPage />} />
          <Route path="/onboarding/*" element={<Navigate to={protectedSignupRedirect} replace />} />
          <Route path="/plan" element={<Navigate to={protectedSignupRedirect} replace />} />
          <Route path="*" element={<Navigate to={protectedLoginRedirect} replace />} />
        </Routes>
      </Suspense>
    )
  }

  const hasConnection = shellStats.connections > 0
  const showUpgrade = billingEnabled && (!shellBilling || shellBilling.current_plan === 'free')
  const isSyncLoginRoute = location.pathname === '/sync/login'
  const isLegacySyncLoginRoute =
    location.pathname === '/data/sync' &&
    new URLSearchParams(location.search).get('cli_login') === '1'

  if (isLegacySyncLoginRoute) {
    return <Navigate to={`/sync/login${location.search}`} replace />
  }

  if (isSyncLoginRoute) {
    return <Suspense fallback={routeFallback}><SyncLoginPage systemSettingsEnabled={systemSettingsEnabled} /></Suspense>
  }

  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <h1>neuDrive</h1>
        </div>

        <nav className="sidebar-nav">
          <NavLink to="/" end className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">■</span>
            <span>Home</span>
          </NavLink>

          {!hasConnection && (
            <NavLink to="/onboarding" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
              <span className="nav-icon">◆</span>
              <span>{tx('开始接入', 'Get Started')}</span>
            </NavLink>
          )}

          <NavLink to="/connections" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">◇</span>
            <span>Connections</span>
          </NavLink>

          <NavLink to="/data/files" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">▤</span>
            <span>Data</span>
          </NavLink>

          {hasConnection && (
            <>
              <NavLink to="/memory" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
                <span className="nav-icon">●</span>
                <span>Memory</span>
              </NavLink>
              <NavLink to="/skills" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
                <span className="nav-icon">✦</span>
                <span>Skills</span>
              </NavLink>
              <NavLink to="/sync-backup" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
                <span className="nav-icon">↕</span>
                <span>{tx('同步备份', 'Sync & Backup')}</span>
              </NavLink>
            </>
          )}

          <div className="nav-group settings-group">
            <NavLink to="/settings/profile" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
              <span className="nav-icon">⚙</span>
              <span>Settings</span>
            </NavLink>
            <div className="nav-submenu">
              <NavLink to="/settings/profile" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>Profile</NavLink>
              {billingEnabled && <NavLink to="/settings/billing" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>Plan & Billing</NavLink>}
              <NavLink to="/settings/developer-access" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>Developer Access</NavLink>
              {systemSettingsEnabled && <NavLink to="/settings/security" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>Security</NavLink>}
            </div>
          </div>

          {localMode && (
            <div className="nav-group">
              <NavLink to={importsHomePath} className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
                <span className="nav-icon">⇩</span>
                <span>{tx('数据导入', 'Data Imports')}</span>
              </NavLink>
            </div>
          )}
        </nav>

        <div className="sidebar-footer">
          {showUpgrade && <NavLink to="/plan" className="sidebar-upgrade">Upgrade to Pro</NavLink>}
          <LanguageToggle compact />
          <div className="sidebar-footer-row">
            <div className="user-info">
              <span className="user-name">{user.name || user.slug || tx('用户', 'User')}</span>
            </div>
            <button className="btn-text" onClick={handleLogout}>{tx('退出', 'Sign out')}</button>
          </div>
        </div>
      </aside>

      <main className="main-content">
        <Suspense fallback={routeFallback}>
          <Routes>
            <Route path="/" element={<DashboardPage systemSettingsEnabled={systemSettingsEnabled} localMode={localMode} billingEnabled={billingEnabled} />} />
            <Route path="/plan" element={<PlanGatePage billingEnabled={billingEnabled} />} />
            <Route path="/onboarding" element={<OnboardingPage />} />
            <Route path="/onboarding/:platform" element={<OnboardingPage />} />
            <Route path="/connections" element={<ConnectionsPage />} />
            <Route path="/sync-backup" element={<GitMirrorPage />} />
            <Route path="/git-mirror" element={<Navigate to="/sync-backup" replace />} />
            <Route path="/billing" element={<Navigate to="/settings/billing" replace />} />
            <Route path="/billing/success" element={billingEnabled ? <BillingSuccessPage /> : <Navigate to="/onboarding" replace />} />
            <Route path="/settings" element={<Navigate to="/settings/profile" replace />} />
            <Route path="/settings/profile" element={<InfoPage title="My Profile" />} />
            <Route path="/settings/billing" element={billingEnabled ? <BillingPage /> : <Navigate to="/settings/profile" replace />} />
            <Route path="/settings/developer-access" element={<DeveloperAccessPage />} />
            <Route path="/settings/security" element={systemSettingsEnabled ? <SystemSettingsPage /> : <Navigate to="/settings/profile" replace />} />
            <Route path="/settings/developer" element={<Navigate to="/settings/developer-access" replace />} />

            <Route path="/setup" element={<SetupPage />}>
              <Route index element={<Navigate to="web-apps" replace />} />
              <Route path="web-apps" element={<SetupWebAppsPage />} />
              <Route path="cloud" element={<SetupCloudPage />} />
              <Route path="adapters" element={<Navigate to="/setup/web-apps" replace />} />
              <Route path="local" element={<SetupLocalPage />} />
              <Route path="advanced" element={<SetupAdvancedPage />} />
              <Route path="gpt-actions" element={<SetupGptActionsPage />} />
              <Route path="tokens" element={<Navigate to="/settings/developer-access" replace />} />
            </Route>
            <Route path="/setup/tokens" element={<Navigate to="/settings/developer-access" replace />} />

            <Route path="/connections/import/claude" element={<ClaudeImportPage />} />
            <Route path="/data" element={<Outlet />}>
              <Route index element={<Navigate to="files" replace />} />
              <Route path="files/edit/*" element={<DataFileEditorPage />} />
              <Route path="files/browse/*" element={<FilesBrowserPage />} />
              <Route path="files/recent" element={<Navigate to="/data/files" replace />} />
              <Route path="files/*" element={<FilesBrowserPage />} />
              <Route path="projects" element={<ProjectsPage />} />
              <Route path="projects/:projectName" element={<ProjectsPage />} />
              <Route path="conversations" element={<DataConversationsPage />} />
              <Route path="conversations/*" element={<DataConversationsPage />} />
              <Route path="skills" element={<Navigate to="/skills" replace />} />
              <Route path="skills/:bundleKey" element={<Navigate to="/skills" replace />} />
              <Route path="memory" element={<Navigate to="/memory" replace />} />
              <Route path="profile" element={<Navigate to="/settings/profile" replace />} />
              <Route path="roles" element={<Navigate to="/data/files" replace />} />
              <Route path="inbox" element={<Navigate to="/data/files" replace />} />
              <Route path="settings" element={<Navigate to="/settings/profile" replace />} />
              <Route path="sync" element={<Navigate to="/sync-backup" replace />} />
            </Route>
            <Route path="/memory" element={<DataMemoryPage />} />
            <Route path="/skills" element={<DataSkillsPage />} />
            <Route path="/skills/:bundleKey" element={<DataSkillsPage />} />

            <Route path="/imports" element={<Navigate to={importsHomePath} replace />} />
            <Route path="/imports/claude" element={localMode ? <ClaudeMigrationPage localMode={localMode} officialExportPath="/imports/claude-export" /> : <Navigate to="/imports/claude-export" replace />} />
            <Route path="/imports/codex" element={localMode ? <ClaudeMigrationPage localMode={localMode} platform="codex" displayName="Codex CLI" /> : <Navigate to="/imports/claude-export" replace />} />
            <Route path="/imports/claude-export" element={<ClaudeImportPage localMode={localMode} />} />

            <Route path="/login" element={<Navigate to="/" replace />} />
            <Route path="/signup" element={<Navigate to="/" replace />} />
            <Route path="/pricing" element={<Navigate to="/settings/billing" replace />} />
            <Route path="/integrations" element={<Navigate to="/connections" replace />} />
            <Route path="/docs" element={<Navigate to="/onboarding" replace />} />
            <Route path="/info" element={<Navigate to="/settings/profile" replace />} />
            <Route path="/projects" element={<Navigate to="/data/projects" replace />} />
            <Route path="/collaborations" element={<Navigate to="/" replace />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Suspense>
      </main>
    </div>
  )
}

export default App
