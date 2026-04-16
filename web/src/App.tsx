import { Suspense, lazy, useState, useEffect, useCallback } from 'react'
import { Routes, Route, NavLink, Navigate, Outlet, useNavigate, useLocation } from 'react-router-dom'
import { api } from './api'
import LanguageToggle from './components/LanguageToggle'
import { useI18n } from './i18n'

const LoginPage = lazy(() => import('./pages/LoginPage'))
const DashboardPage = lazy(() => import('./pages/DashboardPage'))
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
const SetupTokensPage = lazy(() => import('./pages/setup/SetupTokensPage'))
const FilesBrowserPage = lazy(() => import('./pages/data/FilesBrowserPage'))
const DataFileEditorPage = lazy(() => import('./pages/data/DataFileEditorPage'))
const DataSkillsPage = lazy(() => import('./pages/data/DataSkillsPage'))
const DataMemoryPage = lazy(() => import('./pages/data/DataMemoryPage'))
const SystemSettingsPage = lazy(() => import('./pages/SystemSettingsPage'))
const SyncLoginPage = lazy(() => import('./pages/SyncLoginPage'))
const SkillsImportPage = lazy(() => import('./pages/SkillsImportPage'))
const GitMirrorPage = lazy(() => import('./pages/GitMirrorPage'))
const ClaudeMigrationPage = lazy(() => import('./pages/ClaudeMigrationPage'))

function App() {
  const [user, setUser] = useState<any>(null)
  const [publicConfig, setPublicConfig] = useState<any>({})
  const [loading, setLoading] = useState(true)
  const { tx } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const [isDataNavOpen, setIsDataNavOpen] = useState(false)

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
      if (authRefresh) {
        localStorage.setItem('refresh_token', authRefresh)
      } else {
        localStorage.removeItem('refresh_token')
      }
      clearAuthParamsFromURL()
    }
    if (localToken) {
      localStorage.setItem('token', localToken)
      localStorage.removeItem('refresh_token')
      clearAuthParamsFromURL()
    }

    let cfg: any = {}
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
    if (!token) {
      token = await bootstrapLocalOwner() || ''
    }
    if (!token) {
      setUser(null)
      setLoading(false)
      return
    }

    try {
      const me = await api.getMe()
      setUser(me)
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
        const me = await api.getMe()
        setUser(me)
      } catch {
        localStorage.removeItem('token')
        localStorage.removeItem('refresh_token')
        setUser(null)
      }
    }
    setLoading(false)
  }, [])

  useEffect(() => {
    checkAuth()
  }, [checkAuth])

  const handleLogout = async () => {
    await api.logout()
    setUser(null)
    navigate('/login')
  }

  const handleDataNavToggle = () => {
    setIsDataNavOpen((current) => {
      const next = !current
      if (next) {
        navigate('/data/files')
      }
      return next
    })
  }

  const isProfileRoute = location.pathname === '/data/profile'
  const isDataRoute = location.pathname.startsWith('/data') && !isProfileRoute
  const isSyncLoginRoute = location.pathname === '/sync/login'
  const isSkillsImportRoute = location.pathname === '/import/skills'
  const isLegacySyncLoginRoute =
    location.pathname === '/data/sync' &&
    new URLSearchParams(location.search).get('cli_login') === '1'
  const systemSettingsEnabled = !!publicConfig?.system_settings_enabled
  const localMode = !!publicConfig?.local_mode
  const routeFallback = (
    <div className="loading-screen">
      <div className="loading-spinner" />
      <p>{tx('页面加载中...', 'Loading page...')}</p>
    </div>
  )

  useEffect(() => {
    setIsDataNavOpen(isDataRoute)
  }, [isDataRoute])

  if (loading) {
    return (
      <div className="loading-screen">
        <div className="loading-spinner" />
        <p>{tx('加载中...', 'Loading...')}</p>
      </div>
    )
  }

  // OAuth authorize is a standalone page (no sidebar), regardless of login state
  if (location.pathname === '/oauth/authorize') {
    return <Suspense fallback={routeFallback}><OAuthAuthorizePage /></Suspense>
  }

  if (isSkillsImportRoute) {
    return <Suspense fallback={routeFallback}><SkillsImportPage /></Suspense>
  }

  if (!user) {
    if (location.pathname !== '/login') {
      return <Navigate to={`/login?redirect=${encodeURIComponent(window.location.href)}`} replace />
    }
    return (
      <Suspense fallback={routeFallback}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </Suspense>
    )
  }

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
          <span className="sidebar-version">v0.0.1</span>
        </div>

        <nav className="sidebar-nav">
          <NavLink to="/" end className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">&#9632;</span>
            <span>{tx('概览', 'Overview')}</span>
          </NavLink>
          <div className="nav-group">
            <button
              type="button"
              className={isDataRoute ? 'nav-item nav-item-parent nav-item-button active' : 'nav-item nav-item-parent nav-item-button'}
              aria-expanded={isDataNavOpen}
              aria-controls="data-nav-submenu"
              onClick={handleDataNavToggle}
            >
              <span className="nav-icon">&#9776;</span>
              <span>{tx('数据文件', 'Data')}</span>
              <span className={`nav-group-caret ${isDataNavOpen ? 'nav-group-caret-open' : ''}`} aria-hidden="true">
                &#9654;
              </span>
            </button>
            {isDataNavOpen && (
              <div
                id="data-nav-submenu"
                className="nav-submenu"
                aria-label={tx('数据文件子菜单', 'Data navigation submenu')}
              >
                <NavLink to="/data/files" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  {tx('所有文件', 'All Files')}
                </NavLink>
                <NavLink to="/data/projects" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  {tx('项目', 'Projects')}
                </NavLink>
                <NavLink to="/data/skills" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  {tx('技能', 'Skills')}
                </NavLink>
                <NavLink to="/data/memory" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  Memory
                </NavLink>
              </div>
            )}
          </div>
          <NavLink to="/connections" end className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">&#9670;</span>
            <span>{tx('平台连接', 'Connections')}</span>
          </NavLink>
          <NavLink to="/setup/tokens" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">&#9670;</span>
            <span>{tx('Token 管理', 'Token Manager')}</span>
          </NavLink>
          <NavLink to="/git-mirror" end className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">&#8645;</span>
            <span>{tx('Git Mirror', 'Git Mirror')}</span>
          </NavLink>
          {systemSettingsEnabled && (
            <NavLink to="/settings" end className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
              <span className="nav-icon">&#9881;</span>
              <span>{tx('系统设置', 'System Settings')}</span>
            </NavLink>
          )}
        </nav>

        <div className="sidebar-footer">
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
            <Route path="/" element={<DashboardPage systemSettingsEnabled={systemSettingsEnabled} localMode={localMode} />} />
            <Route path="/setup" element={<SetupPage />}>
              <Route index element={<Navigate to="web-apps" replace />} />
              <Route path="web-apps" element={<SetupWebAppsPage />} />
              <Route path="cloud" element={<SetupCloudPage />} />
              <Route path="adapters" element={<Navigate to="/setup/web-apps" replace />} />
              <Route path="local" element={<SetupLocalPage />} />
              <Route path="advanced" element={<SetupAdvancedPage />} />
              <Route path="gpt-actions" element={<SetupGptActionsPage />} />
              <Route path="tokens" element={<SetupTokensPage />} />
            </Route>
            <Route path="/git-mirror" element={<GitMirrorPage />} />
            <Route path="/migrations/claude" element={<ClaudeMigrationPage localMode={localMode} />} />
            <Route path="/settings" element={systemSettingsEnabled ? <SystemSettingsPage /> : <Navigate to="/" replace />} />
            <Route path="/data" element={<Outlet />}>
              <Route index element={<Navigate to="files/browse" replace />} />
              <Route path="files/edit/*" element={<DataFileEditorPage />} />
              <Route path="files/browse/*" element={<FilesBrowserPage />} />
              <Route path="files/recent" element={<Navigate to="/data/files" replace />} />
              <Route path="files/*" element={<FilesBrowserPage />} />
              <Route path="projects" element={<ProjectsPage />} />
              <Route path="projects/:projectName" element={<ProjectsPage />} />
              <Route path="skills" element={<DataSkillsPage />} />
              <Route path="skills/:bundleKey" element={<DataSkillsPage />} />
              <Route path="memory" element={<DataMemoryPage />} />
              <Route path="profile" element={<InfoPage title={tx('我的资料', 'My Profile')} />} />
              <Route path="roles" element={<Navigate to="/data/files" replace />} />
              <Route path="inbox" element={<Navigate to="/data/files" replace />} />
              <Route path="settings" element={<Navigate to={systemSettingsEnabled ? '/settings' : '/'} replace />} />
              <Route path="sync" element={<Navigate to="/git-mirror" replace />} />
            </Route>
            <Route path="/connections" element={<ConnectionsPage />} />
            <Route path="/info" element={<Navigate to="/data/profile" replace />} />
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
