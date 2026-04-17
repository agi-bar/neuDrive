import { useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, type ClaudeDataImportResult } from '../api'
import { useI18n } from '../i18n'

function isZipFile(file: File) {
  const lowerName = file.name.toLowerCase()
  return lowerName.endsWith('.zip') || file.type === 'application/zip' || file.type === 'application/x-zip-compressed'
}

function formatFileSize(bytes: number, locale: 'zh-CN' | 'en') {
  if (!Number.isFinite(bytes) || bytes <= 0) return locale === 'zh-CN' ? '未知大小' : 'Unknown size'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = bytes
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  return `${value.toFixed(value >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`
}

export default function ClaudeImportPage() {
  const { locale, tx } = useI18n()
  const navigate = useNavigate()
  const inputRef = useRef<HTMLInputElement | null>(null)
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [isDragging, setIsDragging] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<ClaudeDataImportResult | null>(null)

  const chooseFile = () => {
    if (busy) return
    inputRef.current?.click()
  }

  const clearSelection = () => {
    if (busy) return
    setSelectedFile(null)
    setResult(null)
    setError('')
    if (inputRef.current) {
      inputRef.current.value = ''
    }
  }

  const applySelectedFile = (file: File | null) => {
    if (!file) return
    if (!isZipFile(file)) {
      setSelectedFile(null)
      setResult(null)
      setError(tx('请上传 Claude 官方导出的 `.zip` 文件。', 'Please upload the official Claude export `.zip` file.'))
      return
    }
    setSelectedFile(file)
    setResult(null)
    setError('')
  }

  const handleImport = async () => {
    if (!selectedFile || busy) return
    setBusy(true)
    setError('')
    setResult(null)
    try {
      const imported = await api.importClaudeData(selectedFile)
      setResult(imported)
    } catch (err: any) {
      setError(err?.message || tx('导入失败', 'Import failed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="page materials-page">
      <section className="materials-hero">
        <div className="materials-hero-copy">
          <nav aria-label={tx('面包屑', 'Breadcrumbs')} className="materials-breadcrumbs">
            <Link to="/imports/claude">{tx('数据导入', 'Data Imports')}</Link>
            <span>/</span>
            <span>{tx('Claude 官方导出', 'Claude Official Export')}</span>
          </nav>
          <div className="materials-kicker">neuDrive Import</div>
          <h2 className="materials-title">{tx('Claude 官方导出', 'Claude Official Export')}</h2>
          <p className="materials-subtitle">
            {tx(
              '上传 Claude Settings 中导出的官方 ZIP。这个页面只处理 official export，不处理 Claude Web skills workspace zip，也不处理单独的 memory 文本导入。',
              'Upload the official ZIP exported from Claude Settings. This page only handles the official export. It does not accept Claude Web skills workspace zips or standalone memory text imports.',
            )}
          </p>
        </div>
        <div className="materials-actions">
          <Link className="btn" to="/imports/claude">
            {tx('Claude Code 扫描迁移', 'Claude Code Migration')}
          </Link>
          <button className="btn" type="button" disabled={busy} onClick={chooseFile}>
            {tx('选择 ZIP', 'Choose ZIP')}
          </button>
          <button className="btn btn-primary" type="button" disabled={!selectedFile || busy} onClick={() => { void handleImport() }}>
            {busy ? tx('导入中...', 'Importing...') : tx('开始导入', 'Start import')}
          </button>
        </div>
      </section>

      <div className="materials-note">
        {tx(
          '当前版本只支持 Claude 官方导出包。预期 ZIP 内包含 `users.json`、`memories.json`、`projects.json`、`conversations.json`。',
          'The current version only supports the official Claude export package. The ZIP is expected to include `users.json`, `memories.json`, `projects.json`, and `conversations.json`.',
        )}
      </div>

      {error ? <div className="alert alert-warn" style={{ marginBottom: 16 }}>{error}</div> : null}

      <section className="materials-section">
        <div className="materials-section-head">
          <div>
            <h3 className="materials-section-title">{tx('如何从 Claude 导出 ZIP', 'How to Export a ZIP from Claude')}</h3>
            <p className="materials-section-copy">
              {tx(
                '先在 Claude 那边拿到官方导出包，再回到这里上传。',
                'Export the official data package from Claude first, then come back here to upload it.',
              )}
            </p>
          </div>
          <a
            className="btn"
            href="https://support.anthropic.com/en/articles/9450526-how-can-i-export-my-claude-ai-data"
            target="_blank"
            rel="noreferrer"
          >
            {tx('查看 Claude 官方说明', 'Open Claude Help')}
          </a>
        </div>
        <div className="data-sync-status-grid">
          <div className="data-sync-status-card">
            <div className="data-record-title">{tx('1. 打开设置', '1. Open Settings')}</div>
            <div className="data-record-secondary">
              {tx(
                '在 Claude Web 或 Claude Desktop 左下角点你的头像/姓名缩写，然后进入 Settings。',
                'In Claude Web or Claude Desktop, click your avatar or initials in the lower-left corner, then open Settings.',
              )}
            </div>
          </div>
          <div className="data-sync-status-card">
            <div className="data-record-title">{tx('2. 发起导出', '2. Start the Export')}</div>
            <div className="data-record-secondary">
              {tx(
                '个人用户和 Team Primary Owner 进入 Privacy，然后点 Export data。Enterprise Primary Owner 进入 Data management，然后点 Export Data。',
                'Individual users and Team Primary Owners go to Privacy and click Export data. Enterprise Primary Owners go to Data management and click Export Data.',
              )}
            </div>
          </div>
          <div className="data-sync-status-card">
            <div className="data-record-title">{tx('3. 查收邮件', '3. Check Your Email')}</div>
            <div className="data-record-secondary">
              {tx(
                'Claude 会异步生成导出包，并把下载链接发到你当前账号绑定的邮箱。生成可能会有一点延迟。',
                'Claude generates the export asynchronously and sends the download link to the email address on your account. There may be a short delay.',
              )}
            </div>
          </div>
          <div className="data-sync-status-card">
            <div className="data-record-title">{tx('4. 下载 ZIP 并上传到这里', '4. Download the ZIP and Upload It Here')}</div>
            <div className="data-record-secondary">
              {tx(
                '打开邮件里的下载链接，下载官方 ZIP，再回到这个页面上传。下载链接通常 24 小时过期，而且需要你登录 Claude 账号后才能下载。',
                'Open the download link from the email, save the official ZIP, then return to this page and upload it. The link usually expires after 24 hours and requires you to be signed in to Claude.',
              )}
            </div>
          </div>
        </div>
        <div className="materials-note" style={{ marginTop: 16 }}>
          {tx(
            '补充说明：Claude 官方帮助当前写明，导出只能从 Claude Web 或 Claude Desktop 发起，不能从移动端 App 直接触发。',
            'Note: Claude’s current official help says exports can be started from Claude Web or Claude Desktop, not directly from the mobile apps.',
          )}
        </div>
      </section>

      <div className="materials-panel">
        <div className="card-header">
          <h3 className="card-title">{tx('上传 ZIP', 'Upload ZIP')}</h3>
        </div>
        <div className="data-record-secondary" style={{ marginTop: 0 }}>
          {tx('支持 `.zip`，后端当前请求上限为 50 MB。', 'Supports `.zip`. The current backend request limit is 50 MB.')}
        </div>

        <input
          ref={inputRef}
          type="file"
          accept=".zip,application/zip,application/x-zip-compressed"
          className="claude-import-input"
          onChange={(event) => {
            const file = event.target.files?.[0] || null
            event.target.value = ''
            applySelectedFile(file)
          }}
          disabled={busy}
        />

        <div
          className={`claude-import-dropzone${isDragging ? ' is-dragging' : ''}${busy ? ' is-disabled' : ''}`}
          role="button"
          tabIndex={busy ? -1 : 0}
          onClick={chooseFile}
          onKeyDown={(event) => {
            if (busy) return
            if (event.key === 'Enter' || event.key === ' ') {
              event.preventDefault()
              chooseFile()
            }
          }}
          onDragOver={(event) => {
            if (busy) return
            event.preventDefault()
            setIsDragging(true)
          }}
          onDragLeave={(event) => {
            event.preventDefault()
            setIsDragging(false)
          }}
          onDrop={(event) => {
            if (busy) return
            event.preventDefault()
            setIsDragging(false)
            applySelectedFile(event.dataTransfer.files?.[0] || null)
          }}
        >
          <div className="claude-import-dropzone-title">
            {tx('拖拽 ZIP 到这里，或点击选择文件', 'Drag a ZIP here, or click to choose a file')}
          </div>
          <div className="claude-import-dropzone-copy">
            {tx(
              '请上传 Claude 官方导出包，不要上传 skills workspace zip。',
              'Upload the official Claude export package, not a skills workspace zip.',
            )}
          </div>
        </div>

        {selectedFile ? (
          <div className="claude-import-file-meta">
            <div className="data-record-title">{selectedFile.name}</div>
            <div className="data-record-secondary">
              {tx('文件大小：', 'File size: ')}{formatFileSize(selectedFile.size, locale)}
            </div>
          </div>
        ) : null}

        <div className="form-actions">
          <button className="btn btn-primary" type="button" disabled={!selectedFile || busy} onClick={() => { void handleImport() }}>
            {busy ? tx('正在上传并解析...', 'Uploading and parsing...') : tx('开始导入', 'Start import')}
          </button>
          <button className="btn" type="button" disabled={busy || (!selectedFile && !result)} onClick={clearSelection}>
            {tx('清空', 'Clear')}
          </button>
        </div>

        {busy ? (
          <div className="alert alert-ok" style={{ marginTop: 16 }}>
            {tx('正在上传并解析 Claude 导出文件，请稍候。', 'Uploading and parsing the Claude export file. Please wait.')}
          </div>
        ) : null}
      </div>

      {result ? (
        <div className="materials-panel">
          <div className="card-header">
            <h3 className="card-title">{tx('导入完成', 'Import Complete')}</h3>
          </div>
          <div className="alert alert-success" style={{ marginBottom: 16 }}>
            {tx('Claude 官方导出 ZIP 已成功导入。', 'The Claude official export ZIP was imported successfully.')}
          </div>
          <div className="data-sync-status-grid">
            <div className="data-sync-status-card">
              <div className="data-record-title">{tx('Memory', 'Memory')}</div>
              <div className="data-record-secondary">{result.memories_imported}</div>
            </div>
            <div className="data-sync-status-card">
              <div className="data-record-title">{tx('Conversations', 'Conversations')}</div>
              <div className="data-record-secondary">{result.conversations_imported}</div>
            </div>
            <div className="data-sync-status-card">
              <div className="data-record-title">{tx('Projects', 'Projects')}</div>
              <div className="data-record-secondary">{result.projects_imported}</div>
            </div>
            <div className="data-sync-status-card">
              <div className="data-record-title">{tx('Files written', 'Files written')}</div>
              <div className="data-record-secondary">{result.files_written}</div>
            </div>
          </div>
          <div className="form-actions">
            <button className="btn btn-primary" type="button" onClick={() => navigate('/data/memory')}>
              {tx('打开 Memory', 'Open Memory')}
            </button>
            <button className="btn" type="button" onClick={() => navigate('/data/skills')}>
              {tx('打开 Skills', 'Open Skills')}
            </button>
            <button className="btn" type="button" onClick={() => navigate('/data/files')}>
              {tx('查看文件', 'Browse Files')}
            </button>
          </div>
        </div>
      ) : null}
    </div>
  )
}
