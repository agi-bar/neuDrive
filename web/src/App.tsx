import { useState, useEffect, useCallback } from 'react'
import { Routes, Route, NavLink, Navigate, Outlet, useNavigate, useLocation } from 'react-router-dom'
import { api } from './api'
import LoginPage from './pages/LoginPage'
import DashboardPage from './pages/DashboardPage'
import ConnectionsPage from './pages/ConnectionsPage'
import InfoPage from './pages/InfoPage'
import ProjectsPage from './pages/ProjectsPage'
import SetupPage from './pages/SetupPage'
import CollaborationsPage from './pages/CollaborationsPage'
import OAuthAuthorizePage from './pages/OAuthAuthorizePage'
import SetupWebAppsPage from './pages/setup/SetupWebAppsPage'
import SetupCloudPage from './pages/setup/SetupCloudPage'
import SetupLocalPage from './pages/setup/SetupLocalPage'
import SetupAdvancedPage from './pages/setup/SetupAdvancedPage'
import SetupAdaptersPage from './pages/setup/SetupAdaptersPage'
import SetupGptActionsPage from './pages/setup/SetupGptActionsPage'
import SetupTokensPage from './pages/setup/SetupTokensPage'
import DataFilesPage from './pages/data/DataFilesPage'
import FilesBrowserPage from './pages/data/FilesBrowserPage'
import DataFileEditorPage from './pages/data/DataFileEditorPage'
import DataSkillsPage from './pages/data/DataSkillsPage'
import DataMemoryPage from './pages/data/DataMemoryPage'
import DataDevicesPage from './pages/data/DataDevicesPage'
import DataRolesPage from './pages/data/DataRolesPage'
import DataInboxPage from './pages/data/DataInboxPage'
import DataSyncPage from './pages/data/DataSyncPage'

function App() {
  const [user, setUser] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()
  const location = useLocation()
  const [isSetupNavOpen, setIsSetupNavOpen] = useState(false)
  const [isConnectionsNavOpen, setIsConnectionsNavOpen] = useState(false)
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

  const isTokenManagementRoute = location.pathname === '/setup/tokens'
  const isSetupRoute = location.pathname.startsWith('/setup') && !isTokenManagementRoute
  const isConnectionsRoute = location.pathname.startsWith('/connections') || isTokenManagementRoute
  const isProfileRoute = location.pathname === '/data/profile'
  const isDataRoute = location.pathname.startsWith('/data') && !isProfileRoute

  if (loading) {
    return (
      <div className="loading-screen">
        <div className="loading-spinner" />
        <p>加载中...</p>
      </div>
    )
  }

  // OAuth authorize is a standalone page (no sidebar), regardless of login state
  if (location.pathname === '/oauth/authorize') {
    return <OAuthAuthorizePage />
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

  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <h1>Agent Hub</h1>
          <span className="sidebar-version">v0.0.1</span>
        </div>

        <nav className="sidebar-nav">
          <NavLink to="/" end className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">&#9632;</span>
            <span>概览</span>
          </NavLink>
          <div className="nav-group">
            <button
              type="button"
              className={isSetupRoute ? 'nav-item nav-item-parent nav-item-button active' : 'nav-item nav-item-parent nav-item-button'}
              aria-expanded={isSetupNavOpen}
              aria-controls="setup-nav-submenu"
              onClick={() => setIsSetupNavOpen((current) => !current)}
            >
              <span className="nav-icon">&#9889;</span>
              <span>连接设置</span>
              <span className={`nav-group-caret ${isSetupNavOpen ? 'nav-group-caret-open' : ''}`} aria-hidden="true">
                &#9654;
              </span>
            </button>
            {isSetupNavOpen && (
              <div
                id="setup-nav-submenu"
                className="nav-submenu"
                aria-label="连接设置子菜单"
              >
                <NavLink to="/setup/web-apps" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  网页应用
                </NavLink>
                <NavLink to="/setup/cloud" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  云端模式
                </NavLink>
                <NavLink to="/setup/adapters" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  Adapters
                </NavLink>
                <NavLink to="/setup/local" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  本地模式
                </NavLink>
                <NavLink to="/setup/advanced" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  高级模式
                </NavLink>
                <NavLink to="/setup/gpt-actions" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  ChatGPT Actions
                </NavLink>
              </div>
            )}
          </div>
          <div className="nav-group">
            <button
              type="button"
              className={isConnectionsRoute ? 'nav-item nav-item-parent nav-item-button active' : 'nav-item nav-item-parent nav-item-button'}
              aria-expanded={isConnectionsNavOpen}
              aria-controls="connections-nav-submenu"
              onClick={() => setIsConnectionsNavOpen((current) => !current)}
            >
              <span className="nav-icon">&#9670;</span>
              <span>连接管理</span>
              <span className={`nav-group-caret ${isConnectionsNavOpen ? 'nav-group-caret-open' : ''}`} aria-hidden="true">
                &#9654;
              </span>
            </button>
            {isConnectionsNavOpen && (
              <div
                id="connections-nav-submenu"
                className="nav-submenu"
                aria-label="连接管理子菜单"
              >
                <NavLink to="/connections" end className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  平台连接
                </NavLink>
                <NavLink to="/setup/tokens" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  Token 管理
                </NavLink>
              </div>
            )}
          </div>
          <NavLink to="/data/profile" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">&#9786;</span>
            <span>我的资料</span>
          </NavLink>
          <div className="nav-group">
            <button
              type="button"
              className={isDataRoute ? 'nav-item nav-item-parent nav-item-button active' : 'nav-item nav-item-parent nav-item-button'}
              aria-expanded={isDataNavOpen}
              aria-controls="data-nav-submenu"
              onClick={() => setIsDataNavOpen((current) => !current)}
            >
              <span className="nav-icon">&#9776;</span>
              <span>数据文件</span>
              <span className={`nav-group-caret ${isDataNavOpen ? 'nav-group-caret-open' : ''}`} aria-hidden="true">
                &#9654;
              </span>
            </button>
            {isDataNavOpen && (
              <div
                id="data-nav-submenu"
                className="nav-submenu"
                aria-label="数据文件子菜单"
              >
                <NavLink to="/data/files" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  文件管理器
                </NavLink>
                <NavLink to="/data/files/recent" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  最近更新
                </NavLink>
                <NavLink to="/data/projects" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  项目
                </NavLink>
                <NavLink to="/data/skills" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  技能
                </NavLink>
                <NavLink to="/data/memory" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  Memory
                </NavLink>
                <NavLink to="/data/devices" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  设备
                </NavLink>
                <NavLink to="/data/roles" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  Roles
                </NavLink>
                <NavLink to="/data/inbox" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  Inbox
                </NavLink>
                <NavLink to="/data/sync" className={({ isActive }) => isActive ? 'nav-subitem active' : 'nav-subitem'}>
                  Sync
                </NavLink>
              </div>
            )}
          </div>
          <NavLink to="/collaborations" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">&#9830;</span>
            <span>协作</span>
          </NavLink>
        </nav>

        <div className="sidebar-footer">
          <div className="user-info">
            <span className="user-name">{user.name || user.slug || 'User'}</span>
          </div>
          <button className="btn-text" onClick={handleLogout}>退出</button>
        </div>
      </aside>

      <main className="main-content">
        <Routes>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/setup" element={<SetupPage />}>
            <Route index element={<Navigate to="web-apps" replace />} />
            <Route path="web-apps" element={<SetupWebAppsPage />} />
            <Route path="cloud" element={<SetupCloudPage />} />
            <Route path="adapters" element={<SetupAdaptersPage />} />
            <Route path="local" element={<SetupLocalPage />} />
            <Route path="advanced" element={<SetupAdvancedPage />} />
            <Route path="gpt-actions" element={<SetupGptActionsPage />} />
            <Route path="tokens" element={<SetupTokensPage />} />
          </Route>
          <Route path="/data" element={<Outlet />}>
            <Route index element={<Navigate to="files" replace />} />
            <Route path="files/edit/*" element={<DataFileEditorPage />} />
            <Route path="files/browse/*" element={<FilesBrowserPage />} />
            <Route path="files/recent" element={<DataFilesPage />} />
            <Route path="files/*" element={<FilesBrowserPage />} />
            <Route path="projects" element={<ProjectsPage />} />
            <Route path="skills" element={<DataSkillsPage />} />
            <Route path="memory" element={<DataMemoryPage />} />
            <Route path="profile" element={<InfoPage title="我的资料" />} />
            <Route path="devices" element={<DataDevicesPage />} />
            <Route path="roles" element={<DataRolesPage />} />
            <Route path="inbox" element={<DataInboxPage />} />
            <Route path="sync" element={<DataSyncPage />} />
          </Route>
          <Route path="/connections" element={<ConnectionsPage />} />
          <Route path="/info" element={<Navigate to="/data/profile" replace />} />
          <Route path="/projects" element={<Navigate to="/data/projects" replace />} />
          <Route path="/collaborations" element={<CollaborationsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  )
}

export default App
