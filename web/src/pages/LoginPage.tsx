import { useState } from 'react'
import { api } from '../api'

interface LoginPageProps {
  onLogin: (token: string, user: any) => void
}

export default function LoginPage({ onLogin }: LoginPageProps) {
  const [slug, setSlug] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!slug.trim()) return

    setLoading(true)
    setError('')

    try {
      const result = await api.devLogin(slug.trim())
      onLogin(result.token, result.user)
    } catch (err: any) {
      setError(err.message || '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <h1 className="login-title">Agent Hub</h1>
        <p className="login-desc">
          个人 AI Agent 管理中心
          <br />
          <span className="login-subdesc">配置一次，偶尔回来看看</span>
        </p>

        <form onSubmit={handleSubmit} className="login-form">
          <div className="form-group">
            <label htmlFor="slug">用户标识</label>
            <input
              id="slug"
              type="text"
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
              placeholder="输入你的 slug"
              autoFocus
              disabled={loading}
            />
          </div>

          {error && <div className="form-error">{error}</div>}

          <button type="submit" className="btn btn-primary btn-block" disabled={loading}>
            {loading ? '登录中...' : '登录'}
          </button>
        </form>

        <p className="login-note">
          开发模式 - 仅用于本地开发环境
        </p>
      </div>
    </div>
  )
}
