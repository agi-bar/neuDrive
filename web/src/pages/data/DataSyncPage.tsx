import { useEffect, useMemo, useState } from 'react'
import { strFromU8, unzipSync } from 'fflate'
import { useI18n } from '../../i18n'
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

const PREVIEW_ACTION_ORDER: Record<string, number> = {
  delete: 0,
  update: 1,
  create: 2,
  conflict: 3,
  skip: 4,
}

const PREVIEW_ACTION_LABEL: Record<string, string> = {
  delete: 'delete',
  update: 'update',
  create: 'create',
  conflict: 'conflict',
  skip: 'skip',
}

function sortPreviewEntries(entries: Array<{ path: string; action: string; kind?: string }>) {
  return [...entries].sort((left, right) => {
    const actionDiff = (PREVIEW_ACTION_ORDER[left.action] ?? 99) - (PREVIEW_ACTION_ORDER[right.action] ?? 99)
    if (actionDiff !== 0) return actionDiff
    return left.path.localeCompare(right.path)
  })
}

function previewSummaryBadges(summary: Record<string, number>) {
  return ['delete', 'update', 'create', 'skip']
    .filter((action) => (summary[action] || 0) > 0)
    .map((action) => (
      <span key={action} className={`token-list-prefix preview-action preview-action-${action}`}>
        {PREVIEW_ACTION_LABEL[action]} {summary[action] || 0}
      </span>
    ))
}

function previewActionLabel(action: string) {
  return PREVIEW_ACTION_LABEL[action] || action
}

function previewActionClass(action: string) {
  return `preview-action preview-action-${action}`
}

function jobStatusLabel(job: SyncJob, locale: 'zh-CN' | 'en') {
  switch (job.status) {
    case 'succeeded':
      return locale === 'zh-CN' ? '已完成' : 'Succeeded'
    case 'failed':
      return locale === 'zh-CN' ? '失败' : 'Failed'
    case 'aborted':
      return locale === 'zh-CN' ? '已中止' : 'Aborted'
    case 'running':
      return locale === 'zh-CN' ? '进行中' : 'Running'
    case 'pending':
      return locale === 'zh-CN' ? '排队中' : 'Pending'
    default:
      return job.status
  }
}

export default function DataSyncPage() {
  const { locale, tx } = useI18n()
  const [syncToken, setSyncToken] = useState<SyncTokenResponse | null>(null)
  const [ttlMinutes, setTTLMinutes] = useState(30)
  const [tokenBusy, setTokenBusy] = useState(false)
  const [tokenError, setTokenError] = useState('')
  const [tokenMessage, setTokenMessage] = useState('')

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
      setJobsError(err.message || tx('加载历史失败', 'Failed to load history'))
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
    setTokenMessage('')
    try {
      const created = await api.createSyncToken({ access: 'both', ttl_minutes: ttlMinutes })
      setSyncToken(created)
      setTokenMessage(tx('已生成临时同步 token，可用于当前页面的导入、导出和历史查询，也可复制到本地 CLI 登录。', 'A temporary sync token was created. You can use it on this page for import, export, and history queries, or copy it to a local CLI login.'))
    } catch (err: any) {
      setTokenError(err.message || tx('生成同步 token 失败', 'Failed to create sync token'))
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
      if (importFile.name.endsWith('.ndrvz')) {
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
      setImportError(err.message || tx('预览失败', 'Preview failed'))
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
      if (resumeSessionId && !importFile.name.endsWith('.ndrvz')) {
        throw new Error(tx('继续未完成 session 时，请重新选择原始 .ndrvz 文件。', 'To continue an unfinished session, reselect the original .ndrvz file.'))
      }
      if (importFile.name.endsWith('.ndrvz')) {
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
          setImportMessage(tx(`正在上传分片 ${uploaded}/${missing.length}...`, `Uploading chunk ${uploaded}/${missing.length}...`))
        }
        const result = await api.commitSyncSession(syncToken.token, sessionId, preview?.fingerprint)
        setImportMessage(tx(`导入完成：${JSON.stringify(result)}`, `Import completed: ${JSON.stringify(result)}`))
        setResumeSessionId('')
      } else {
        const bundle = await readJSONBundle(importFile)
        bundle.mode = importMode
        const result = await api.importBundle(syncToken.token, bundle)
        setImportMessage(tx(`导入完成：${JSON.stringify(result)}`, `Import completed: ${JSON.stringify(result)}`))
      }
      await loadJobs(syncToken.token)
    } catch (err: any) {
      setImportError(err.message || tx('导入失败', 'Import failed'))
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
        triggerDownload(exported as Blob, `neudrive-sync-${date}.ndrvz`)
      } else {
        triggerDownload(new Blob([JSON.stringify(exported, null, 2)], { type: 'application/json' }), `neudrive-sync-${date}.ndrv`)
      }
      await loadJobs(syncToken.token)
    } catch (err: any) {
      setExportError(err.message || tx('导出失败', 'Export failed'))
    } finally {
      setExportBusy(false)
    }
  }

  return (
    <div className="page materials-page">
      <section className="materials-hero">
        <div className="materials-hero-copy">
          <div className="materials-kicker">neuDrive Data</div>
          <h2 className="materials-title">Sync</h2>
          <p className="materials-subtitle">{tx('把同步也收进同一套卡片语言里。这里集中处理 token、bundle 导入导出，以及最近的同步历史。', 'Bring sync into the same card-based language. This page handles tokens, bundle import/export, and recent sync history.')}</p>
        </div>
      </section>

      <div className="materials-panel data-sync-card">
        <div className="card-header">
          <h3 className="card-title">{tx('临时同步 Token', 'Temporary sync token')}</h3>
        </div>
        <p className="data-record-secondary">{tx('生成一个 30 分钟到 2 小时内有效的短命 token，供本页面或本地 CLI 调用 `/agent/*` 同步接口。', 'Create a short-lived token valid for 30 minutes to 2 hours for this page or a local CLI to call `/agent/*` sync endpoints.')}</p>
        <div className="data-sync-cli-box">
          <div className="data-record-title">{tx('推荐 CLI 流程', 'Recommended CLI flow')}</div>
          <p className="data-record-secondary">{tx('先登录一次保存默认 profile。CLI 会自动打开独立的网页登录页，不再跳进完整管理后台。', 'Sign in once to save the default profile. The CLI opens a dedicated login page instead of the full dashboard.')}</p>
          <div className="data-sync-cli-steps">
            <code>neudrive sync login --api-base {window.location.origin}</code>
            <code>neudrive sync push --bundle backup.ndrvz</code>
          </div>
          <div className="data-record-secondary">{tx('如果你已经生成了当前 token，也可以手工执行：', 'If you already generated the current token, you can also run:')}<code>neudrive sync login --api-base {window.location.origin} --token &lt;PASTE_TOKEN&gt;</code></div>
        </div>
        <div className="data-sync-row">
          <select aria-label="Sync token TTL" value={ttlMinutes} onChange={(e) => setTTLMinutes(Number(e.target.value))}>
            <option value={30}>{tx('30 分钟', '30 minutes')}</option>
            <option value={60}>{tx('1 小时', '1 hour')}</option>
            <option value={120}>{tx('2 小时', '2 hours')}</option>
          </select>
          <button className="btn btn-primary" onClick={() => { void handleCreateSyncToken() }} disabled={tokenBusy}>
            {tokenBusy ? tx('生成中...', 'Creating...') : tx('生成 Sync Token', 'Create Sync token')}
          </button>
        </div>
        {syncToken && (
          <div className="data-sync-token-box">
            <div className="data-record-title">{tx('当前 Token', 'Current token')}</div>
            <code className="data-sync-token">{syncToken.token}</code>
            <div className="data-record-meta">{tx('过期时间：', 'Expires at: ')}{formatDateTime(syncToken.expires_at, locale)}</div>
            <div className="data-record-secondary">{syncToken.usage}</div>
          </div>
        )}
        {tokenMessage && <div className="alert alert-success" style={{ marginTop: 12 }}>{tokenMessage}</div>}
        {tokenError && <div className="alert alert-error">{tokenError}</div>}
      </div>

      <div className="materials-panel data-sync-card">
        <div className="card-header">
          <h3 className="card-title">{tx('导入上传', 'Import')}</h3>
        </div>
        <p className="data-record-secondary">{tx('上传 `.ndrv` 或 `.ndrvz` 文件。JSON bundle 支持先 preview 再导入；archive bundle 会走 resumable session 上传。', 'Upload `.ndrv` or `.ndrvz` files. JSON bundles can be previewed before import, while archive bundles use resumable session uploads.')}</p>
        <div className="data-sync-row">
          <input
            type="file"
            aria-label="Bundle file"
            accept=".ndrv,.ndrvz,application/json,application/zip"
            onChange={(e) => {
              setImportFile(e.target.files?.[0] || null)
              setPreview(null)
              setImportMessage('')
              setImportError('')
            }}
          />
          <select aria-label="Import mode" value={importMode} onChange={(e) => setImportMode(e.target.value as 'merge' | 'mirror')}>
            <option value="merge">merge</option>
            <option value="mirror">mirror</option>
          </select>
          <button className="btn" onClick={() => { void handlePreview() }} disabled={!syncToken?.token || !importFile || importBusy}>
            Preview
          </button>
          <button className="btn btn-primary" onClick={() => { void handleImport() }} disabled={!syncToken?.token || !importFile || importBusy}>
            {importBusy ? tx('处理中...', 'Working...') : tx('开始导入', 'Start import')}
          </button>
        </div>
        {resumeSessionId && <div className="alert alert-warn">{tx('将继续未完成 session：', 'Continuing unfinished session: ')}{resumeSessionId}</div>}
        {importMode === 'mirror' && (
          <div className="alert alert-warn">{tx('`mirror` 会删除 bundle 中声明的 skill 里未出现的额外文件，执行前请确认 preview 结果。', '`mirror` removes extra files not present in the bundle for declared skills. Review the preview carefully before continuing.')}</div>
        )}
        {resumeSessionId && (
          <div className="alert alert-warn">
            {tx('已选择继续未完成 session：', 'Selected unfinished session: ')}{resumeSessionId}{tx('。请重新选择原始 `.ndrvz` 文件，再点击“开始导入”。', '. Reselect the original `.ndrvz` file, then click "Start import".')}
          </div>
        )}
        {preview && (
          <div className="data-sync-preview">
            <div className="data-record-title">Preview</div>
            <div className="data-inline-list">{previewSummaryBadges(preview.summary)}</div>
            {(preview.summary.delete || 0) > 0 && (
              <div className="alert alert-warn" style={{ marginTop: 12 }}>
                {tx('本次 preview 包含删除操作。`mirror` 只会影响 bundle 中声明的 skill，不会全局删除其他 skill。', 'This preview includes deletions. `mirror` only affects skills declared by the bundle and does not delete other skills globally.')}
              </div>
            )}
            <div className="data-sync-preview-sections">
              {(preview.profile?.length || 0) > 0 && (
                <div className="data-sync-preview-section">
                  <div className="data-record-title">Profile</div>
                  <div className="data-sync-preview-list">
                    {sortPreviewEntries(preview.profile || []).map((entry) => (
                      <div key={entry.path} className={`data-sync-preview-entry ${entry.action === 'delete' ? 'is-danger' : ''}`}>
                        <span className={previewActionClass(entry.action)}>{previewActionLabel(entry.action)}</span>
                        <code>{entry.path}</code>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              {(preview.memory?.length || 0) > 0 && (
                <div className="data-sync-preview-section">
                  <div className="data-record-title">Memory</div>
                  <div className="data-sync-preview-list">
                    {sortPreviewEntries(preview.memory || []).map((entry) => (
                      <div key={entry.path} className={`data-sync-preview-entry ${entry.action === 'delete' ? 'is-danger' : ''}`}>
                        <span className={previewActionClass(entry.action)}>{previewActionLabel(entry.action)}</span>
                        <code>{entry.path}</code>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              {Object.entries(preview.skills || {})
                .sort(([left], [right]) => left.localeCompare(right))
                .map(([skillName, skillPreview]) => (
                  <details key={skillName} className="data-sync-preview-section" open>
                    <summary className="data-sync-preview-summary">
                      <div>
                        <div className="data-record-title">{skillName}</div>
                        <div className="data-record-secondary">{tx('仅展示将作用于该 skill 的文件变更', 'Only file changes affecting this skill are shown')}</div>
                      </div>
                      <div className="data-inline-list">{previewSummaryBadges(skillPreview.summary || {})}</div>
                    </summary>
                    <div className="data-sync-preview-list">
                      {sortPreviewEntries(skillPreview.files || []).map((entry) => (
                        <div key={entry.path} className={`data-sync-preview-entry ${entry.action === 'delete' ? 'is-danger' : ''}`}>
                          <span className={previewActionClass(entry.action)}>{previewActionLabel(entry.action)}</span>
                          <code>{entry.path}</code>
                          {entry.kind && <span className="data-record-secondary">{entry.kind}</span>}
                        </div>
                      ))}
                    </div>
                  </details>
                ))}
            </div>
          </div>
        )}
        {importMessage && <div className="alert alert-success">{summarizeText(importMessage, 220, locale)}</div>}
        {importError && <div className="alert alert-error">{importError}</div>}
      </div>

      <div className="materials-panel data-sync-card">
        <div className="card-header">
          <h3 className="card-title">{tx('导出下载', 'Export')}</h3>
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
            placeholder={tx('仅包含这些 skills，逗号分隔', 'Only include these skills, comma-separated')}
          />
          <input
            value={excludeSkillsText}
            onChange={(e) => setExcludeSkillsText(e.target.value)}
            placeholder={tx('排除这些 skills，逗号分隔', 'Exclude these skills, comma-separated')}
          />
        </div>
        <div className="data-sync-row">
          <select aria-label="Export format" value={exportFormat} onChange={(e) => setExportFormat(e.target.value as 'json' | 'archive')}>
            <option value="json">JSON (.ndrv)</option>
            <option value="archive">Archive (.ndrvz)</option>
          </select>
          <button className="btn btn-primary" onClick={() => { void handleExport() }} disabled={!syncToken?.token || exportBusy}>
            {exportBusy ? tx('导出中...', 'Exporting...') : tx('下载 Bundle', 'Download bundle')}
          </button>
        </div>
        {exportError && <div className="alert alert-error">{exportError}</div>}
      </div>

      <div className="materials-panel data-sync-card">
        <div className="card-header">
          <h3 className="card-title">{tx('最近同步历史', 'Recent sync history')}</h3>
        </div>
        {jobsBusy && <div className="page-loading">{tx('加载中...', 'Loading...')}</div>}
        {jobsError && <div className="alert alert-error">{jobsError}</div>}
        {!jobsBusy && jobs.length === 0 && <div className="empty-state"><p>{tx('还没有同步记录。', 'No sync history yet.')}</p></div>}
        {jobs.length > 0 && (
          <div className="data-record-list">
            {jobs.map((job) => (
              <div key={job.id} className="card data-record-item">
                <div className="data-record-head">
                  <div>
                    <div className="data-record-title">{job.direction} / {job.transport}</div>
                    <div className="data-record-secondary">{job.source || 'neudrive'} · {job.mode || 'merge'}</div>
                  </div>
                  <div className="data-record-meta">{formatDateTime(job.created_at, locale)}</div>
                </div>
                <div className="data-inline-list">
                  <span className="token-list-prefix">{tx('状态', 'Status')} {jobStatusLabel(job, locale)}</span>
                  {job.session_id && <span className="token-list-prefix">{tx('会话', 'Session')} {job.session_id.slice(0, 8)}</span>}
                </div>
                {job.summary && <div className="data-record-secondary">{JSON.stringify(job.summary)}</div>}
                {job.error && <div className="alert alert-error" style={{ marginTop: 12 }}>{job.error}</div>}
                {job.session_id && job.status !== 'succeeded' && (
                  <>
                    <div className="data-record-secondary" style={{ marginTop: 12 }}>
                      {tx('重新选择原始 `.ndrvz` 文件后，可以继续这个未完成的 session。', 'After reselecting the original `.ndrvz` file, you can continue this unfinished session.')}
                    </div>
                    <button
                      className="btn"
                      style={{ marginTop: 12 }}
                      onClick={() => setResumeSessionId(job.session_id || '')}
                    >
                      {tx('选择并继续这个 Session', 'Select and continue this session')}
                    </button>
                  </>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
