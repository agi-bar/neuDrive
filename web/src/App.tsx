import { Component, Suspense, lazy, useCallback, useEffect, useState, type ErrorInfo, type ReactNode } from 'react'
import { Navigate, NavLink, Outlet, Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom'
import { api, BILLING_REDIRECT_EVENT, type BillingRedirectDetail, type BillingStatus, type PublicConfig } from './api'
import LanguageToggle from './components/LanguageToggle'
import GitHubRepoLink from './components/GitHubRepoLink'
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
const CommandLineToolsPage = lazy(() => import('./pages/CommandLineToolsPage'))
const MarketingHomePage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.MarketingHomePage })))
const PricingPage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.PricingPage })))
const IntegrationsPage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.IntegrationsPage })))
const IntegrationDetailPage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.IntegrationDetailPage })))
const DocsLandingPage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.DocsLandingPage })))
const GuidePage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.GuidePage })))
const SignupPage = lazy(() => import('./pages/PublicPages').then((module) => ({ default: module.SignupPage })))
const PrivacyPage = lazy(() => import('./pages/PublicPages').then(({ LegalPage }) => ({ default: () => <LegalPage kind="privacy" /> })))
const TermsPage = lazy(() => import('./pages/PublicPages').then(({ LegalPage }) => ({ default: () => <LegalPage kind="terms" /> })))

const STALE_CHUNK_RELOAD_KEY = 'neudrive.staleChunkReloadAt'

function isChunkLoadError(error: unknown) {
  const message = error instanceof Error ? error.message : String(error || '')
  return /Failed to fetch dynamically imported module|Importing a module script failed|Loading chunk|vite:preloadError/i.test(message)
}

function reloadForFreshAssets() {
  if (typeof window === 'undefined') return false
  const now = Date.now()
  const lastReload = Number(window.sessionStorage.getItem(STALE_CHUNK_RELOAD_KEY) || '0')
  if (Number.isFinite(lastReload) && now - lastReload < 60_000) return false
  window.sessionStorage.setItem(STALE_CHUNK_RELOAD_KEY, String(now))
  const nextURL = new URL(window.location.href)
  nextURL.searchParams.set('__nd_reload', String(now))
  window.location.replace(nextURL.toString())
  return true
}

if (typeof window !== 'undefined') {
  window.addEventListener('vite:preloadError', (event) => {
    event.preventDefault()
    reloadForFreshAssets()
  })
  window.addEventListener('unhandledrejection', (event) => {
    if (isChunkLoadError(event.reason) && reloadForFreshAssets()) {
      event.preventDefault()
    }
  })
}

class RouteErrorBoundary extends Component<{ children: ReactNode; fallback: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  componentDidCatch(error: unknown, _errorInfo: ErrorInfo) {
    if (isChunkLoadError(error)) reloadForFreshAssets()
  }

  render() {
    if (this.state.failed) return this.props.fallback
    return this.props.children
  }
}

function LegacyOnboardingRedirect() {
  const { platform } = useParams()
  if (platform === 'codex' || platform === 'gemini' || platform === 'claude-code') return <Navigate to="/setup/cli" replace />
  if (platform === 'browser') return <Navigate to="/setup/web-apps" replace />
  return <Navigate to="/setup/mcp" replace />
}

function LegacyDataFilesRedirect() {
  const location = useLocation()
  const wildcard = useParams()['*']
  const params = new URLSearchParams(location.search)
  params.delete('access')
  if (wildcard) {
    const decoded = decodeURIComponent(wildcard).replace(/^\/+/, '')
    if (decoded) {
      params.set('path', `/${decoded}`)
      params.delete('type')
      params.delete('source')
      params.delete('q')
    }
  }
  const search = params.toString()
  return <Navigate to={`/${search ? `?${search}` : ''}`} replace />
}

function App() {
  const [user, setUser] = useState<any>(null)
  const [publicConfig, setPublicConfig] = useState<PublicConfig>({})
  const [shellBilling, setShellBilling] = useState<BillingStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const [feedbackLaunching, setFeedbackLaunching] = useState(false)
  const { tx } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const systemSettingsEnabled = !!publicConfig?.system_settings_enabled
  const localMode = !!publicConfig?.local_mode
  const billingEnabled = !!publicConfig?.billing_enabled
  const feedbackEnabled = !!publicConfig?.feedback_enabled
  const importsHomePath = localMode ? '/imports/local-apps' : '/imports/claude-export'

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
    const currentURL = new URL(window.location.href)
    if (currentURL.searchParams.has('__nd_reload')) {
      currentURL.searchParams.delete('__nd_reload')
      window.history.replaceState({}, '', `${currentURL.pathname}${currentURL.search}${currentURL.hash}`)
    }
    const timer = window.setTimeout(() => {
      window.sessionStorage.removeItem(STALE_CHUNK_RELOAD_KEY)
    }, 60_000)
    return () => window.clearTimeout(timer)
  }, [])

  useEffect(() => {
    if (!user) return
    let cancelled = false
    const loadShellState = async () => {
      if (!billingEnabled) {
        setShellBilling(null)
        return
      }
      try {
        const billing = await api.getBillingStatus()
        if (!cancelled) setShellBilling(billing)
      } catch {
        // Billing status is optional shell chrome; the billing page will surface detailed errors.
      }
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
      path.startsWith('/sync-backup') || path.startsWith('/git-mirror') ? 'GitHub Backup' :
      path.startsWith('/settings/billing') || path.startsWith('/billing') ? 'Plan & Billing' :
      path.startsWith('/settings/developer-access') || path.startsWith('/settings/developer') ? 'Developer Access' :
      path.startsWith('/settings/security') ? 'System Settings' :
      path.startsWith('/settings/profile') || path.startsWith('/info') ? 'My Profile' :
      path.startsWith('/cli') ? 'Command Line Tools' :
      path.startsWith('/setup') ? 'Setup Guide' :
      path.startsWith('/data/conversations') ? 'Conversations' :
      path.startsWith('/data/projects') || path.startsWith('/projects') ? 'Projects' :
      path.startsWith('/data') ? 'Data Explorer' :
      path.startsWith('/memory') ? 'Memory' :
      path.startsWith('/skills') ? 'Skills' :
      path.startsWith('/imports/local-apps') || path.startsWith('/imports/claude') || path.startsWith('/imports/codex') ? 'Local App Data Import' :
      path.startsWith('/imports') ? 'Data Imports' :
      'neuDrive'
    document.title = pageTitle === 'neuDrive' ? 'neuDrive' : `${pageTitle} — neuDrive`
  }, [location.pathname, user])

  useEffect(() => {
    setUserMenuOpen(false)
  }, [location.pathname])

  const routeFallback = (
    <div className="loading-screen">
      <div className="loading-spinner" />
      <p>{tx('页面加载中...', 'Loading page...')}</p>
    </div>
  )

  const routeErrorFallback = (
    <div className="loading-screen">
      <p>{tx('页面资源已更新，请刷新后继续。', 'The page assets were updated. Refresh to continue.')}</p>
      <button
        className="btn btn-primary"
        type="button"
        onClick={() => {
          window.sessionStorage.removeItem(STALE_CHUNK_RELOAD_KEY)
          window.location.reload()
        }}
      >
        {tx('刷新页面', 'Refresh page')}
      </button>
    </div>
  )

  const handleLogout = async () => {
    await api.logout()
    setUser(null)
    navigate('/login')
  }

  const handleFeedbackOpen = async () => {
    if (feedbackLaunching) return
    setFeedbackLaunching(true)
    try {
      const launch = await api.launchFeedback()
      if (!launch?.launch_url) throw new Error('feedback launch url missing')
      window.location.assign(launch.launch_url)
    } catch {
      setFeedbackLaunching(false)
      window.alert(tx('暂时无法打开反馈入口，请稍后再试。', 'Feedback is unavailable right now. Please try again later.'))
    }
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
    return <RouteErrorBoundary key={location.key} fallback={routeErrorFallback}><Suspense fallback={routeFallback}><OAuthAuthorizePage /></Suspense></RouteErrorBoundary>
  }

  if (location.pathname === '/import/skills') {
    return <RouteErrorBoundary key={location.key} fallback={routeErrorFallback}><Suspense fallback={routeFallback}><SkillsImportPage /></Suspense></RouteErrorBoundary>
  }

  if (location.pathname.startsWith('/guides/') || location.pathname.startsWith('/integrations/') || location.pathname === '/privacy' || location.pathname === '/terms') {
    return (
      <RouteErrorBoundary key={location.key} fallback={routeErrorFallback}>
      <Suspense fallback={routeFallback}>
        <Routes>
          <Route path="/guides/:platform" element={<GuidePage />} />
          <Route path="/integrations/:platform" element={<IntegrationDetailPage />} />
          <Route path="/privacy" element={<PrivacyPage />} />
          <Route path="/terms" element={<TermsPage />} />
        </Routes>
      </Suspense>
      </RouteErrorBoundary>
    )
  }

  if (!user) {
    const protectedSignupRedirect = `/signup?redirect=${encodeURIComponent(location.pathname + location.search)}`
    const protectedLoginRedirect = `/login?redirect=${encodeURIComponent(location.pathname + location.search)}`
    return (
      <RouteErrorBoundary key={location.key} fallback={routeErrorFallback}>
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
      </RouteErrorBoundary>
    )
  }

  const showGitHubBackup = localMode || !!publicConfig?.github_enabled || !!publicConfig?.github_app_enabled
  const showUpgrade = billingEnabled && (!shellBilling || shellBilling.current_plan === 'free')
  const isSyncLoginRoute = location.pathname === '/sync/login'
  const isLegacySyncLoginRoute =
    location.pathname === '/data/sync' &&
    new URLSearchParams(location.search).get('cli_login') === '1'

  if (isLegacySyncLoginRoute) {
    return <Navigate to={`/sync/login${location.search}`} replace />
  }

  if (isSyncLoginRoute) {
    return <RouteErrorBoundary key={location.key} fallback={routeErrorFallback}><Suspense fallback={routeFallback}><SyncLoginPage systemSettingsEnabled={systemSettingsEnabled} /></Suspense></RouteErrorBoundary>
  }

  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <div className="sidebar-brand-lockup">
            <img className="sidebar-brand-logo" src="/logo-mark.png" alt="" aria-hidden="true" />
            <h1>neuDrive</h1>
          </div>
          <GitHubRepoLink className="sidebar-github-link" />
        </div>

        <nav className="sidebar-nav">
          <NavLink to="/" end className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">■</span>
            <span>{tx('概览', 'Home')}</span>
          </NavLink>

          <NavLink to="/settings/developer-access" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">⌘</span>
            <span>{tx('开发者访问', 'Developer Access')}</span>
          </NavLink>

          {localMode && (
            <NavLink to={importsHomePath} className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
              <span className="nav-icon">⇩</span>
              <span>{tx('本地 App Data 导入', 'Local App Data Import')}</span>
            </NavLink>
          )}

          {showGitHubBackup && (
            <NavLink to="/sync-backup" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
              <span className="nav-icon">↕</span>
              <span>{tx('GitHub 备份', 'GitHub Backup')}</span>
            </NavLink>
          )}

          <NavLink to="/cli" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">›_</span>
            <span>{tx('命令行工具', 'Command Line')}</span>
          </NavLink>

          {feedbackEnabled && (
            <button
              type="button"
              className="nav-item nav-item-button"
              onClick={() => { void handleFeedbackOpen() }}
              disabled={feedbackLaunching}
            >
              <span className="nav-icon">?</span>
              <span>{feedbackLaunching ? tx('打开中...', 'Opening...') : tx('反馈', 'Feedback')}</span>
            </button>
          )}

          {billingEnabled && (
            <NavLink to="/settings/billing" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
              <span className="nav-icon">◈</span>
              <span>{tx('套餐与账单', 'Plan & Billing')}</span>
            </NavLink>
          )}

          {systemSettingsEnabled && (
            <NavLink to="/settings/security" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
              <span className="nav-icon">⚙</span>
              <span>{tx('系统设置', 'System Settings')}</span>
            </NavLink>
          )}
        </nav>

        <div className="sidebar-footer">
          {showUpgrade && <NavLink to="/plan" className="sidebar-upgrade">{tx('升级到 Pro', 'Upgrade to Pro')}</NavLink>}
          <LanguageToggle compact />
          <div className="sidebar-user-menu-wrap">
            <button
              className="sidebar-user-button"
              type="button"
              aria-haspopup="menu"
              aria-expanded={userMenuOpen}
              onClick={() => setUserMenuOpen((open) => !open)}
            >
              <span className="sidebar-user-avatar">{(user.name || user.slug || 'U').slice(0, 1).toUpperCase()}</span>
              <span className="user-info">
                <span className="user-name">{user.name || user.slug || tx('用户', 'User')}</span>
                <span className="user-menu-hint">{tx('账户菜单', 'Account menu')}</span>
              </span>
              <span className="user-menu-chevron">⌄</span>
            </button>
            {userMenuOpen && (
              <div className="sidebar-user-menu" role="menu">
                <NavLink to="/settings/profile" role="menuitem" onClick={() => setUserMenuOpen(false)}>
                  {tx('个人资料', 'Profile')}
                </NavLink>
                <button type="button" role="menuitem" onClick={() => { void handleLogout() }}>
                  {tx('退出', 'Sign out')}
                </button>
              </div>
            )}
          </div>
        </div>
      </aside>

      <main className="main-content">
        <RouteErrorBoundary key={location.pathname} fallback={routeErrorFallback}>
        <Suspense fallback={routeFallback}>
          <Routes>
            <Route path="/" element={<DashboardPage systemSettingsEnabled={systemSettingsEnabled} localMode={localMode} billingEnabled={billingEnabled} />} />
            <Route path="/plan" element={<PlanGatePage billingEnabled={billingEnabled} />} />
            <Route path="/onboarding" element={<Navigate to="/setup/mcp" replace />} />
            <Route path="/onboarding/:platform" element={<LegacyOnboardingRedirect />} />
            <Route path="/connections" element={<ConnectionsPage />} />
            <Route path="/sync-backup" element={<GitMirrorPage />} />
            <Route path="/git-mirror" element={<Navigate to="/sync-backup" replace />} />
            <Route path="/billing" element={<Navigate to="/settings/billing" replace />} />
            <Route path="/billing/success" element={billingEnabled ? <BillingSuccessPage /> : <Navigate to="/onboarding" replace />} />
            <Route path="/settings" element={<Navigate to="/settings/profile" replace />} />
            <Route path="/settings/profile" element={<InfoPage />} />
            <Route path="/settings/billing" element={billingEnabled ? <BillingPage /> : <Navigate to="/settings/profile" replace />} />
            <Route path="/settings/developer-access" element={<DeveloperAccessPage />} />
            <Route path="/settings/security" element={systemSettingsEnabled ? <SystemSettingsPage /> : <Navigate to="/settings/profile" replace />} />
            <Route path="/settings/developer" element={<Navigate to="/settings/developer-access" replace />} />
            <Route path="/cli" element={<CommandLineToolsPage />} />
            <Route path="/command-line-tools" element={<Navigate to="/cli" replace />} />

            <Route path="/setup/mcp" element={<OnboardingPage />} />
            <Route path="/setup" element={<SetupPage />}>
              <Route index element={<Navigate to="/setup/mcp" replace />} />
              <Route path="web-apps" element={<SetupWebAppsPage />} />
              <Route path="cli" element={<SetupCloudPage />} />
              <Route path="cloud" element={<Navigate to="/setup/cli" replace />} />
              <Route path="adapters" element={<Navigate to="/setup/web-apps" replace />} />
              <Route path="local" element={<SetupLocalPage />} />
              <Route path="advanced" element={<SetupAdvancedPage />} />
              <Route path="gpt-actions" element={<SetupGptActionsPage />} />
              <Route path="tokens" element={<Navigate to="/settings/developer-access" replace />} />
            </Route>
            <Route path="/setup/tokens" element={<Navigate to="/settings/developer-access" replace />} />

            <Route path="/connections/import/claude" element={<ClaudeImportPage />} />
            <Route path="/data" element={<Outlet />}>
              <Route index element={<Navigate to="/" replace />} />
              <Route path="files/edit/*" element={<DataFileEditorPage />} />
              <Route path="files/browse/*" element={<LegacyDataFilesRedirect />} />
              <Route path="files/recent" element={<Navigate to="/" replace />} />
              <Route path="files/*" element={<LegacyDataFilesRedirect />} />
              <Route path="projects" element={<ProjectsPage />} />
              <Route path="projects/:projectName" element={<ProjectsPage />} />
              <Route path="conversations" element={<DataConversationsPage />} />
              <Route path="conversations/*" element={<DataConversationsPage />} />
              <Route path="skills" element={<Navigate to="/skills" replace />} />
              <Route path="skills/:bundleKey" element={<Navigate to="/skills" replace />} />
              <Route path="memory" element={<Navigate to="/memory" replace />} />
              <Route path="profile" element={<Navigate to="/settings/profile" replace />} />
              <Route path="roles" element={<Navigate to="/" replace />} />
              <Route path="inbox" element={<Navigate to="/" replace />} />
              <Route path="settings" element={<Navigate to="/settings/profile" replace />} />
              <Route path="sync" element={<Navigate to="/sync-backup" replace />} />
            </Route>
            <Route path="/memory" element={<DataMemoryPage />} />
            <Route path="/skills" element={<DataSkillsPage />} />
            <Route path="/skills/:bundleKey" element={<DataSkillsPage />} />

            <Route path="/imports" element={<Navigate to={importsHomePath} replace />} />
            <Route path="/imports/local-apps" element={localMode ? <ClaudeMigrationPage localMode={localMode} officialExportPath="/imports/claude-export" /> : <Navigate to="/" replace />} />
            <Route path="/imports/claude" element={<Navigate to={localMode ? "/imports/local-apps?platform=claude" : "/"} replace />} />
            <Route path="/imports/codex" element={<Navigate to={localMode ? "/imports/local-apps?platform=codex" : "/"} replace />} />
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
        </RouteErrorBoundary>
      </main>
    </div>
  )
}

export default App
