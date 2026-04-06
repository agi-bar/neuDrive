import { useEffect, useMemo, useState } from 'react'
import { strFromU8, unzipSync } from 'fflate'
import {
  api,
  type BundleFilters,
  type BundlePreviewResult,
  type SyncJob,
  type SyncTokenResponse,
} from '../../api'
import { formatDateTime, summarizeText } from './DataShared'

function parseCommaList(value: string) {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

async function readJSONBundle(file: File) {
  return JSON.parse(await file.text())
}

async function readArchiveManifest(file: File) {
  const bytes = new Uint8Array(await file.arrayBuffer())
  const archive = unzipSync(bytes)
  const manifestBytes = archive['manifest.json']
  if (!manifestBytes) {
    throw new Error('archive is missing manifest.json')
  }
  return {
    bytes,
    manifest: JSON.parse(strFromU8(manifestBytes)),
  }
}

function triggerDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  URL.revokeObjectURL(url)
}

export default function DataSyncPage() {
  const [syncToken, setSyncToken] = useState<SyncTokenResponse | null>(null)
  const [ttlMinutes, setTTLMinutes] = useState(30)
  const [tokenBusy, setTokenBusy] = useState(false)
  const [tokenError, setTokenError] = useState('')

  const [importFile, setImportFile] = useState<File | null>(null)
  const [importMode, setImportMode] = useState<'merge' | 'mirror'>('merge')
  const [preview, setPreview] = useState<BundlePreviewResult | null>(null)
  const [importBusy, setImportBusy] = useState(false)
  const [importMessage, setImportMessage] = useState('')
  const [importError, setImportError] = useState('')
  const [resumeSessionId, setResumeSessionId] = useState<string>('')

  const [exportFormat, setExportFormat] = useState<'json' | 'archive'>('json')
  const [domainProfile, setDomainProfile] = useState(true)
  const [domainMemory, setDomainMemory] = useState(true)
  const [domainSkills, setDomainSkills] = useState(true)
  const [includeSkillsText, setIncludeSkillsText] = useState('')
  const [excludeSkillsText, setExcludeSkillsText] = useState('')
  const [exportBusy, setExportBusy] = useState(false)
  const [exportError, setExportError] = useState('')

  const [jobs, setJobs] = useState<SyncJob[]>([])
  const [jobsBusy, setJobsBusy] = useState(false)
  const [jobsError, setJobsError] = useState('')

  const exportFilters = useMemo<BundleFilters>(() => ({
    include_domains: [
      ...(domainProfile ? ['profile'] : []),
      ...(domainMemory ? ['memory'] : []),
      ...(domainSkills ? ['skills'] : []),
    ],
    include_skills: parseCommaList(includeSkillsText),
    exclude_skills: parseCommaList(excludeSkillsText),
  }), [domainMemory, domainProfile, domainSkills, excludeSkillsText, includeSkillsText])

  const loadJobs = async (tokenValue: string) => {
    setJobsBusy(true)
    setJobsError('')
    try {
      const nextJobs = await api.listSyncJobs(tokenValue)
      setJobs(nextJobs)
    } catch (err: any) {
      setJobsError(err.message || '加载历史失败')
    } finally {
      setJobsBusy(false)
    }
  }

  useEffect(() => {
    if (syncToken?.token) {
      void loadJobs(syncToken.token)
    }
  }, [syncToken?.token])

  const handleCreateSyncToken = async () => {
    setTokenBusy(true)
    setTokenError('')
    try {
      const created = await api.createSyncToken({ access: 'both', ttl_minutes: ttlMinutes })
      setSyncToken(created)
      setImportMessage('已生成临时同步 token，可用于当前页面的导入、导出和历史查询。')
    } catch (err: any) {
      setTokenError(err.message || '生成同步 token 失败')
    } finally {
      setTokenBusy(false)
    }
  }

  const handlePreview = async () => {
    if (!syncToken?.token || !importFile) return
    setImportBusy(true)
    setImportError('')
    setImportMessage('')
    try {
      if (importFile.name.endsWith('.ahubz')) {
        const { manifest } = await readArchiveManifest(importFile)
        manifest.mode = importMode
        const nextPreview = await api.previewBundle(syncToken.token, { manifest })
        setPreview(nextPreview)
      } else {
        const bundle = await readJSONBundle(importFile)
        bundle.mode = importMode
        const nextPreview = await api.previewBundle(syncToken.token, bundle)
        setPreview(nextPreview)
      }
    } catch (err: any) {
      setImportError(err.message || '预览失败')
    } finally {
      setImportBusy(false)
    }
  }

  const handleImport = async () => {
    if (!syncToken?.token || !importFile) return
    setImportBusy(true)
    setImportError('')
    setImportMessage('')
    try {
      if (importFile.name.endsWith('.ahubz')) {
        const { bytes, manifest } = await readArchiveManifest(importFile)
        manifest.mode = importMode
        let sessionId = resumeSessionId
        if (!sessionId) {
          const started = await api.startSyncSession(syncToken.token, {
            transport_version: 'ahub.sync/v1',
            format: 'archive',
            mode: importMode,
            manifest,
            archive_size_bytes: bytes.length,
            archive_sha256: manifest.archive_sha256,
          })
          sessionId = started.session_id
        }
        const current = await api.getSyncSession(syncToken.token, sessionId)
        const chunkSize = Math.max(current.chunk_size_bytes || 1, 1)
        const missing = current.missing_parts || []
        let uploaded = 0
        for (const index of missing) {
          const start = index * chunkSize
          const end = Math.min(bytes.length, start + chunkSize)
          await api.uploadSyncPart(syncToken.token, sessionId, index, bytes.slice(start, end))
          uploaded += 1
          setImportMessage(`正在上传分片 ${uploaded}/${missing.length}...`)
        }
        const result = await api.commitSyncSession(syncToken.token, sessionId, preview?.fingerprint)
        setImportMessage(`导入完成：${JSON.stringify(result)}`)
        setResumeSessionId('')
      } else {
        const bundle = await readJSONBundle(importFile)
        bundle.mode = importMode
        const result = await api.importBundle(syncToken.token, bundle)
        setImportMessage(`导入完成：${JSON.stringify(result)}`)
      }
      await loadJobs(syncToken.token)
    } catch (err: any) {
      setImportError(err.message || '导入失败')
    } finally {
      setImportBusy(false)
    }
  }

  const handleExport = async () => {
    if (!syncToken?.token) return
    setExportBusy(true)
    setExportError('')
    try {
      const exported = await api.exportBundle(syncToken.token, exportFormat, exportFilters)
      const date = new Date().toISOString().slice(0, 10)
      if (exportFormat === 'archive') {
        triggerDownload(exported as Blob, `agenthub-sync-${date}.ahubz`)
      } else {
        triggerDownload(new Blob([JSON.stringify(exported, null, 2)], { type: 'application/json' }), `agenthub-sync-${date}.ahub`)
      }
      await loadJobs(syncToken.token)
    } catch (err: any) {
      setExportError(err.message || '导出失败')
    } finally {
      setExportBusy(false)
    }
  }

  return (
    <div className="page">
      <div className="page-header">
        <h2>数据同步</h2>
      </div>

      <div className="card data-sync-card">
        <div className="card-header">
          <h3 className="card-title">临时同步 Token</h3>
        </div>
        <p className="data-record-secondary">生成一个 30 分钟到 2 小时内有效的短命 token，供本页面或本地 CLI 调用 `/agent/*` 同步接口。</p>
        <div className="data-sync-row">
          <select value={ttlMinutes} onChange={(e) => setTTLMinutes(Number(e.target.value))}>
            <option value={30}>30 分钟</option>
            <option value={60}>1 小时</option>
            <option value={120}>2 小时</option>
          </select>
          <button className="btn btn-primary" onClick={() => { void handleCreateSyncToken() }} disabled={tokenBusy}>
            {tokenBusy ? '生成中...' : '生成 Sync Token'}
          </button>
        </div>
        {syncToken && (
          <div className="data-sync-token-box">
            <div className="data-record-title">当前 Token</div>
            <code className="data-sync-token">{syncToken.token}</code>
            <div className="data-record-meta">过期时间：{formatDateTime(syncToken.expires_at)}</div>
            <div className="data-record-secondary">{syncToken.usage}</div>
          </div>
        )}
        {tokenError && <div className="alert alert-error">{tokenError}</div>}
      </div>

      <div className="card data-sync-card">
        <div className="card-header">
          <h3 className="card-title">导入上传</h3>
        </div>
        <p className="data-record-secondary">上传 `.ahub` 或 `.ahubz` 文件。JSON bundle 支持先 preview 再导入；archive bundle 会走 resumable session 上传。</p>
        <div className="data-sync-row">
          <input
            type="file"
            accept=".ahub,.ahubz,application/json,application/zip"
            onChange={(e) => {
              setImportFile(e.target.files?.[0] || null)
              setPreview(null)
              setImportMessage('')
              setImportError('')
            }}
          />
          <select value={importMode} onChange={(e) => setImportMode(e.target.value as 'merge' | 'mirror')}>
            <option value="merge">merge</option>
            <option value="mirror">mirror</option>
          </select>
          <button className="btn" onClick={() => { void handlePreview() }} disabled={!syncToken?.token || !importFile || importBusy}>
            Preview
          </button>
          <button className="btn btn-primary" onClick={() => { void handleImport() }} disabled={!syncToken?.token || !importFile || importBusy}>
            {importBusy ? '处理中...' : '开始导入'}
          </button>
        </div>
        {resumeSessionId && <div className="alert alert-warn">将继续未完成 session：{resumeSessionId}</div>}
        {importMode === 'mirror' && (
          <div className="alert alert-warn">`mirror` 会删除 bundle 中声明的 skill 里未出现的额外文件，执行前请确认 preview 结果。</div>
        )}
        {preview && (
          <div className="data-sync-preview">
            <div className="data-record-title">Preview</div>
            <div className="data-inline-list">
              <span className="token-list-prefix">create {preview.summary.create || 0}</span>
              <span className="token-list-prefix">update {preview.summary.update || 0}</span>
              <span className="token-list-prefix">delete {preview.summary.delete || 0}</span>
              <span className="token-list-prefix">skip {preview.summary.skip || 0}</span>
            </div>
            {(preview.summary.delete || 0) > 0 && (
              <div className="alert alert-warn" style={{ marginTop: 12 }}>
                本次 preview 包含删除操作，请确认 `mirror` 目标正确。
              </div>
            )}
          </div>
        )}
        {importMessage && <div className="alert alert-success">{summarizeText(importMessage, 220)}</div>}
        {importError && <div className="alert alert-error">{importError}</div>}
      </div>

      <div className="card data-sync-card">
        <div className="card-header">
          <h3 className="card-title">导出下载</h3>
        </div>
        <div className="data-sync-grid">
          <label className="data-sync-checkbox">
            <input type="checkbox" checked={domainProfile} onChange={(e) => setDomainProfile(e.target.checked)} />
            <span>Profile</span>
          </label>
          <label className="data-sync-checkbox">
            <input type="checkbox" checked={domainMemory} onChange={(e) => setDomainMemory(e.target.checked)} />
            <span>Memory</span>
          </label>
          <label className="data-sync-checkbox">
            <input type="checkbox" checked={domainSkills} onChange={(e) => setDomainSkills(e.target.checked)} />
            <span>Skills</span>
          </label>
        </div>
        <div className="data-sync-grid">
          <input
            value={includeSkillsText}
            onChange={(e) => setIncludeSkillsText(e.target.value)}
            placeholder="仅包含这些 skills，逗号分隔"
          />
          <input
            value={excludeSkillsText}
            onChange={(e) => setExcludeSkillsText(e.target.value)}
            placeholder="排除这些 skills，逗号分隔"
          />
        </div>
        <div className="data-sync-row">
          <select value={exportFormat} onChange={(e) => setExportFormat(e.target.value as 'json' | 'archive')}>
            <option value="json">JSON (.ahub)</option>
            <option value="archive">Archive (.ahubz)</option>
          </select>
          <button className="btn btn-primary" onClick={() => { void handleExport() }} disabled={!syncToken?.token || exportBusy}>
            {exportBusy ? '导出中...' : '下载 Bundle'}
          </button>
        </div>
        {exportError && <div className="alert alert-error">{exportError}</div>}
      </div>

      <div className="card data-sync-card">
        <div className="card-header">
          <h3 className="card-title">最近同步历史</h3>
        </div>
        {jobsBusy && <div className="page-loading">加载中...</div>}
        {jobsError && <div className="alert alert-error">{jobsError}</div>}
        {!jobsBusy && jobs.length === 0 && <div className="empty-state"><p>还没有同步记录。</p></div>}
        {jobs.length > 0 && (
          <div className="data-record-list">
            {jobs.map((job) => (
              <div key={job.id} className="card data-record-item">
                <div className="data-record-head">
                  <div>
                    <div className="data-record-title">{job.direction} / {job.transport}</div>
                    <div className="data-record-secondary">{job.source || 'agenthub'} · {job.mode || 'merge'}</div>
                  </div>
                  <div className="data-record-meta">{formatDateTime(job.created_at)}</div>
                </div>
                <div className="data-inline-list">
                  <span className="token-list-prefix">status {job.status}</span>
                  {job.session_id && <span className="token-list-prefix">session {job.session_id.slice(0, 8)}</span>}
                </div>
                {job.summary && <div className="data-record-secondary">{JSON.stringify(job.summary)}</div>}
                {job.error && <div className="alert alert-error" style={{ marginTop: 12 }}>{job.error}</div>}
                {job.session_id && job.status !== 'succeeded' && (
                  <button
                    className="btn"
                    style={{ marginTop: 12 }}
                    onClick={() => setResumeSessionId(job.session_id || '')}
                  >
                    继续这个 Session
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
