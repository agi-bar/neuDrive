import { useState, useEffect, useCallback } from 'react'
import { Routes, Route, NavLink, Navigate, Outlet, useNavigate, useLocation } from 'react-router-dom'
import { api } from './api'
import LanguageToggle from './components/LanguageToggle'
import { useI18n } from './i18n'
import LoginPage from './pages/LoginPage'
import DashboardPage from './pages/DashboardPage'
import ConnectionsPage from './pages/ConnectionsPage'
import InfoPage from './pages/InfoPage'
import ProjectsPage from './pages/ProjectsPage'
import SetupPage from './pages/SetupPage'
import OAuthAuthorizePage from './pages/OAuthAuthorizePage'
import SetupWebAppsPage from './pages/setup/SetupWebAppsPage'
import SetupCloudPage from './pages/setup/SetupCloudPage'
import SetupLocalPage from './pages/setup/SetupLocalPage'
import SetupAdvancedPage from './pages/setup/SetupAdvancedPage'
import SetupGptActionsPage from './pages/setup/SetupGptActionsPage'
import SetupTokensPage from './pages/setup/SetupTokensPage'
import FilesBrowserPage from './pages/data/FilesBrowserPage'
import DataFileEditorPage from './pages/data/DataFileEditorPage'
import DataSkillsPage from './pages/data/DataSkillsPage'
import DataMemoryPage from './pages/data/DataMemoryPage'
import DataSyncPage from './pages/data/DataSyncPage'
import SyncLoginPage from './pages/SyncLoginPage'
import SkillsImportPage from './pages/SkillsImportPage'

function App() {
  const [user, setUser] = useState<any>(null)
  const [publicConfig, setPublicConfig] = useState<any>({})
  const [loading, setLoading] = useState(true)
  const { tx } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const [isDataNavOpen, setIsDataNavOpen] = useState(false)

  const checkAuth = useCallback(async () => {
    // Check for GitHub OAuth redirect tokens in URL
    const params = new URLSearchParams(window.location.search)
    const ghToken = params.get('github_token')
    const ghRefresh = params.get('github_refresh')
    const localToken = params.get('local_token')
    if (ghToken) {
      localStorage.setItem('token', ghToken)
      if (ghRefresh) localStorage.setItem('refresh_token', ghRefresh)
      // Clean URL
      window.history.replaceState({}, '', window.location.pathname)
    }
    if (localToken) {
      localStorage.setItem('token', localToken)
      localStorage.removeItem('refresh_token')
      window.history.replaceState({}, '', window.location.pathname)
    }

    try {
      const cfg = await api.getPublicConfig()
      setPublicConfig(cfg || {})
    } catch {
      setPublicConfig({})
    }

    const token = localStorage.getItem('token')
    if (!token) {
      setLoading(false)
      return
    }
    try {
      const me = await api.getMe()
      setUser(me)
    } catch {
      localStorage.removeItem('token')
      localStorage.removeItem('refresh_token')
    }
    setLoading(false)
  }, [])

  useEffect(() => {
    checkAuth()
  }, [checkAuth])

  const handleLogin = (token: string, userData: any) => {
    localStorage.setItem('token', token)

    // Check if there's a redirect URL (from OAuth authorize flow)
    // Do this BEFORE setUser to avoid flashing dashboard
    const params = new URLSearchParams(window.location.search)
    const redirect = params.get('redirect')
    if (redirect) {
      // Redirect back to OAuth authorize page (now with token in localStorage)
      window.location.href = redirect
      return
    }

    setUser(userData)
    navigate('/')
  }

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
    return <OAuthAuthorizePage />
  }

  if (isSkillsImportRoute) {
    return <SkillsImportPage />
  }

  if (!user) {
    if (location.pathname !== '/login') {
      return <Navigate to={`/login?redirect=${encodeURIComponent(window.location.href)}`} replace />
    }
    return (
      <Routes>
        <Route path="/login" element={<LoginPage onLogin={handleLogin} />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    )
  }

  if (isLegacySyncLoginRoute) {
    return <Navigate to={`/sync/login${location.search}`} replace />
  }

  if (isSyncLoginRoute) {
    return <SyncLoginPage systemSettingsEnabled={systemSettingsEnabled} />
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
        <Routes>
          <Route path="/" element={<DashboardPage systemSettingsEnabled={systemSettingsEnabled} />} />
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
          <Route path="/settings" element={systemSettingsEnabled ? <DataSyncPage /> : <Navigate to="/" replace />} />
          <Route path="/data" element={<Outlet />}>
            <Route index element={<Navigate to="files/browse" replace />} />
            <Route path="files/edit/*" element={<DataFileEditorPage />} />
            <Route path="files/browse/*" element={<FilesBrowserPage />} />
            <Route path="files/recent" element={<Navigate to="/data/files" replace />} />
            <Route path="files/*" element={<FilesBrowserPage />} />
            <Route path="projects" element={<ProjectsPage />} />
            <Route path="skills/*" element={<DataSkillsPage />} />
            <Route path="memory" element={<DataMemoryPage />} />
            <Route path="profile" element={<InfoPage title={tx('我的资料', 'My Profile')} />} />
            <Route path="devices" element={<Navigate to="/data/files" replace />} />
            <Route path="roles" element={<Navigate to="/data/files" replace />} />
            <Route path="inbox" element={<Navigate to="/data/files" replace />} />
            <Route path="settings" element={<Navigate to={systemSettingsEnabled ? '/settings' : '/'} replace />} />
            <Route path="sync" element={<Navigate to={systemSettingsEnabled ? '/settings' : '/'} replace />} />
          </Route>
          <Route path="/connections" element={<ConnectionsPage />} />
          <Route path="/info" element={<Navigate to="/data/profile" replace />} />
          <Route path="/projects" element={<Navigate to="/data/projects" replace />} />
          <Route path="/collaborations" element={<Navigate to="/" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  )
}

export default App
