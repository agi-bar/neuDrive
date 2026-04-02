import { useState, useEffect, useCallback } from 'react'
import { Routes, Route, NavLink, Navigate, useNavigate } from 'react-router-dom'
import { api } from './api'
import LoginPage from './pages/LoginPage'
import DashboardPage from './pages/DashboardPage'
import ConnectionsPage from './pages/ConnectionsPage'
import InfoPage from './pages/InfoPage'
import ProjectsPage from './pages/ProjectsPage'
import SetupPage from './pages/SetupPage'
import CollaborationsPage from './pages/CollaborationsPage'

function App() {
  const [user, setUser] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  const checkAuth = useCallback(async () => {
    // Check for GitHub OAuth redirect tokens in URL
    const params = new URLSearchParams(window.location.search)
    const ghToken = params.get('github_token')
    const ghRefresh = params.get('github_refresh')
    if (ghToken) {
      localStorage.setItem('token', ghToken)
      if (ghRefresh) localStorage.setItem('refresh_token', ghRefresh)
      // Clean URL
      window.history.replaceState({}, '', '/')
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
    setUser(userData)

    // Check if there's a redirect URL (from OAuth authorize flow)
    const params = new URLSearchParams(window.location.search)
    const redirect = params.get('redirect')
    if (redirect) {
      // Redirect back to OAuth authorize page (now with token in localStorage)
      window.location.href = redirect
      return
    }
    navigate('/')
  }

  const handleLogout = async () => {
    await api.logout()
    setUser(null)
    navigate('/login')
  }

  if (loading) {
    return (
      <div className="loading-screen">
        <div className="loading-spinner" />
        <p>加载中...</p>
      </div>
    )
  }

  if (!user) {
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
          <NavLink to="/setup" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">&#9889;</span>
            <span>连接设置</span>
          </NavLink>
          <NavLink to="/connections" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">&#9670;</span>
            <span>连接管理</span>
          </NavLink>
          <NavLink to="/info" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">&#9733;</span>
            <span>信息配置</span>
          </NavLink>
          <NavLink to="/projects" className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}>
            <span className="nav-icon">&#9654;</span>
            <span>项目</span>
          </NavLink>
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
          <Route path="/setup" element={<SetupPage />} />
          <Route path="/connections" element={<ConnectionsPage />} />
          <Route path="/info" element={<InfoPage />} />
          <Route path="/projects" element={<ProjectsPage />} />
          <Route path="/collaborations" element={<CollaborationsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  )
}

export default App
