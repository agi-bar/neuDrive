import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  api,
  type LocalGitSyncInfo,
  type LocalPlatformImportPreview,
  type LocalPlatformImportPreviewTaskStatus,
  type LocalPlatformImportSummary,
} from "../api";
import { useI18n } from "../i18n";

type MigrationMode = "agent" | "files" | "all";

interface ClaudeMigrationPageProps {
  localMode?: boolean;
}

const modeOptions: MigrationMode[] = ["agent", "all", "files"];

function formatBytes(bytes: number | undefined, locale: "zh-CN" | "en") {
  if (!Number.isFinite(bytes) || !bytes || bytes <= 0)
    return locale === "zh-CN" ? "0 字节" : "0 bytes";
  const units =
    locale === "zh-CN"
      ? ["字节", "KB", "MB", "GB"]
      : ["bytes", "KB", "MB", "GB"];
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value.toFixed(value >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

function formatDurationMs(ms: number, locale: "zh-CN" | "en") {
  if (!Number.isFinite(ms) || ms <= 0)
    return locale === "zh-CN" ? "不到 1 秒" : "under 1 second";
  const totalSeconds = Math.max(1, Math.round(ms / 1000));
  if (totalSeconds < 60) {
    return locale === "zh-CN" ? `${totalSeconds} 秒` : `${totalSeconds} sec`;
  }
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (locale === "zh-CN") {
    return seconds > 0 ? `${minutes} 分 ${seconds} 秒` : `${minutes} 分`;
  }
  return seconds > 0 ? `${minutes} min ${seconds} sec` : `${minutes} min`;
}

function formatExactDateTime(
  value: string | undefined,
  locale: "zh-CN" | "en",
) {
  if (!value) return "";
  try {
    return new Date(value).toLocaleString(
      locale === "zh-CN" ? "zh-CN" : "en-US",
    );
  } catch {
    return value;
  }
}

function formatRelativeTime(value: string | undefined, locale: "zh-CN" | "en") {
  if (!value) return "";
  const timestamp = new Date(value).getTime();
  if (!Number.isFinite(timestamp)) return "";
  const diffMs = Date.now() - timestamp;
  if (diffMs < 0) {
    return locale === "zh-CN" ? "刚刚" : "just now";
  }
  const minuteMs = 60 * 1000;
  const hourMs = 60 * minuteMs;
  const dayMs = 24 * hourMs;
  if (diffMs < minuteMs) {
    return locale === "zh-CN" ? "刚刚" : "just now";
  }
  if (diffMs < hourMs) {
    const minutes = Math.max(1, Math.floor(diffMs / minuteMs));
    return locale === "zh-CN" ? `${minutes} 分钟前` : `${minutes} minutes ago`;
  }
  if (diffMs < dayMs) {
    const hours = Math.max(1, Math.floor(diffMs / hourMs));
    return locale === "zh-CN" ? `${hours} 小时前` : `${hours} hours ago`;
  }
  const days = Math.max(1, Math.floor(diffMs / dayMs));
  return locale === "zh-CN" ? `${days} 天前` : `${days} days ago`;
}

function categoryLabel(name: string, tx: (zh: string, en: string) => string) {
  switch (name) {
    case "raw_platform_snapshot":
      return tx("原始平台快照", "Raw platform snapshot");
    case "profile_rules":
      return tx("Profile 规则", "Profile rules");
    case "memory_items":
      return tx("Memory 条目", "Memory items");
    case "projects":
      return tx("项目", "Projects");
    case "claude_projects":
      return tx("Claude 项目上下文", "Claude project context");
    case "bundles":
      return tx("Skills / Bundles", "Skills / Bundles");
    case "conversations":
      return tx("聊天会话", "Conversations");
    case "structured_archives":
      return tx("结构化归档", "Structured archives");
    case "agent_artifacts":
      return tx("Agent 归档项", "Agent artifacts");
    default:
      return name.split("_").join(" ");
  }
}

export default function ClaudeMigrationPage({
  localMode = false,
}: ClaudeMigrationPageProps) {
  const { locale, tx } = useI18n();
  const [mode, setMode] = useState<MigrationMode>("agent");
  const [preview, setPreview] = useState<LocalPlatformImportPreview | null>(
    null,
  );
  const [taskStatus, setTaskStatus] =
    useState<LocalPlatformImportPreviewTaskStatus | null>(null);
  const [result, setResult] = useState<LocalPlatformImportSummary | null>(null);
  const [syncInfo, setSyncInfo] = useState<LocalGitSyncInfo | null>(null);
  const [loadingPreviewTask, setLoadingPreviewTask] = useState(false);
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [elapsedMs, setElapsedMs] = useState(0);

  const previewing = taskStatus?.state === "running";

  useEffect(() => {
    if (!localMode) return;
    let cancelled = false;
    setLoadingPreviewTask(true);
    setTaskStatus(null);
    setPreview(null);
    void api
      .getLocalPlatformImportPreviewTask({ platform: "claude", mode })
      .then((data) => {
        if (cancelled) return;
        setTaskStatus(data.status || null);
        setPreview(data.preview || null);
      })
      .catch(() => {
        if (cancelled) return;
        setTaskStatus(null);
        setPreview(null);
      })
      .finally(() => {
        if (!cancelled) {
          setLoadingPreviewTask(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [localMode, mode]);

  useEffect(() => {
    if (!previewing || !taskStatus?.started_at) return;
    const startedAt = new Date(taskStatus.started_at).getTime();
    if (!Number.isFinite(startedAt)) return;
    setElapsedMs(Math.max(0, Date.now() - startedAt));
    const interval = window.setInterval(() => {
      setElapsedMs(Math.max(0, Date.now() - startedAt));
    }, 500);
    return () => window.clearInterval(interval);
  }, [previewing, taskStatus?.started_at]);

  useEffect(() => {
    if (!localMode || !previewing) return;
    let cancelled = false;
    const interval = window.setInterval(() => {
      void api
        .getLocalPlatformImportPreviewTask({ platform: "claude", mode })
        .then((data) => {
          if (cancelled) return;
          setTaskStatus(data.status || null);
          setPreview(data.preview || null);
        })
        .catch(() => {});
    }, 1500);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, [localMode, mode, previewing]);

  const handleRefresh = async () => {
    setError("");
    setSuccess("");
    setLoadingPreviewTask(true);
    try {
      const data = await api.startLocalPlatformImportPreviewTask({
        platform: "claude",
        mode,
      });
      setTaskStatus(data.status || null);
      setPreview(data.preview || null);
      if (data.status?.started_at) {
        const startedAt = new Date(data.status.started_at).getTime();
        setElapsedMs(
          Number.isFinite(startedAt) ? Math.max(0, Date.now() - startedAt) : 0,
        );
      } else {
        setElapsedMs(0);
      }
    } catch (err: any) {
      setError(err.message || tx("扫描失败", "Preview failed"));
    } finally {
      setLoadingPreviewTask(false);
    }
  };

  const handleImport = async () => {
    setImporting(true);
    setError("");
    setSuccess("");
    try {
      const response = await api.importLocalPlatform({
        platform: "claude",
        mode,
      });
      setResult(response.data);
      setSyncInfo(response.localGitSync || null);
      setSuccess(
        tx(
          "Claude Code 数据已导入到 neuDrive。",
          "Claude Code data has been imported into neuDrive.",
        ),
      );
    } catch (err: any) {
      setError(err.message || tx("导入失败", "Import failed"));
    } finally {
      setImporting(false);
    }
  };

  const totalDiscovered =
    preview?.categories.reduce(
      (sum, category) => sum + (category.discovered || 0),
      0,
    ) || 0;
  const totalImportable =
    preview?.categories.reduce(
      (sum, category) => sum + (category.importable || 0),
      0,
    ) || 0;
  const totalArchived =
    preview?.categories.reduce(
      (sum, category) => sum + (category.archived || 0),
      0,
    ) || 0;
  const totalBlocked =
    preview?.categories.reduce(
      (sum, category) => sum + (category.blocked || 0),
      0,
    ) || 0;
  const importPaths = result?.agent?.paths || result?.files?.paths || [];
  const lastScanAt = preview?.completed_at || preview?.started_at || "";
  const statusCompletedAt =
    taskStatus?.completed_at || taskStatus?.updated_at || "";
  const durationEstimateMs =
    preview?.duration_ms && preview.duration_ms > 0 ? preview.duration_ms : 0;
  const lastScanMetaLabel = lastScanAt
    ? tx(
        `上次扫描：${formatRelativeTime(lastScanAt, locale)}（${formatExactDateTime(lastScanAt, locale)}）${preview?.duration_ms ? ` · 耗时 ${formatDurationMs(preview.duration_ms, locale)}` : ""}`,
        `Last scan: ${formatRelativeTime(lastScanAt, locale)} (${formatExactDateTime(lastScanAt, locale)})${preview?.duration_ms ? ` · Took ${formatDurationMs(preview.duration_ms, locale)}` : ""}`,
      )
    : "";
  const remainingEstimateMs =
    durationEstimateMs > elapsedMs ? durationEstimateMs - elapsedMs : 0;
  const scanTimingLabel = previewing
    ? durationEstimateMs > 0
      ? elapsedMs <= durationEstimateMs
        ? tx(
            `${lastScanMetaLabel ? `当前显示的是上次快照。` : ""} 已扫描 ${formatDurationMs(elapsedMs, locale)}，预计总耗时约 ${formatDurationMs(durationEstimateMs, locale)}，剩余约 ${formatDurationMs(remainingEstimateMs, locale)}。`,
            `${lastScanMetaLabel ? "Showing the previous snapshot. " : ""}Scanned for ${formatDurationMs(elapsedMs, locale)}. Estimated total is about ${formatDurationMs(durationEstimateMs, locale)}, with roughly ${formatDurationMs(remainingEstimateMs, locale)} remaining.`,
          )
        : tx(
            `${lastScanMetaLabel ? `当前显示的是上次快照。` : ""} 已扫描 ${formatDurationMs(elapsedMs, locale)}，已经超过通常的 ${formatDurationMs(durationEstimateMs, locale)}。`,
            `${lastScanMetaLabel ? "Showing the previous snapshot. " : ""}Scanned for ${formatDurationMs(elapsedMs, locale)} and already exceeded the usual ${formatDurationMs(durationEstimateMs, locale)}.`,
          )
      : tx(
          `${lastScanMetaLabel ? `当前显示的是上次快照。` : ""} 已扫描 ${formatDurationMs(elapsedMs, locale)}。首次扫描还没有可用的预估时间。`,
          `${lastScanMetaLabel ? "Showing the previous snapshot. " : ""}Scanned for ${formatDurationMs(elapsedMs, locale)}. There is no usable estimate yet for the first run.`,
        )
    : taskStatus?.state === "failed" && taskStatus.error_message
      ? tx(
          `最近一次扫描失败：${taskStatus.error_message}${statusCompletedAt ? `（${formatExactDateTime(statusCompletedAt, locale)}）` : ""}`,
          `The latest scan failed: ${taskStatus.error_message}${statusCompletedAt ? ` (${formatExactDateTime(statusCompletedAt, locale)})` : ""}`,
        )
      : preview?.duration_ms
        ? tx(
            `最近一次扫描耗时 ${formatDurationMs(preview.duration_ms, locale)}。下次扫描会基于这个结果给出粗略预估。`,
            `The last scan took ${formatDurationMs(preview.duration_ms, locale)}. Future scans will use this as a rough estimate.`,
          )
        : "";

  return (
    <div className="page materials-page">
      <div className="page-header">
        <div>
          <h2>Claude</h2>
          <p className="page-subtitle">
            {tx(
              "本地扫描并迁移 Claude Code 数据，再决定只做语义迁移，还是顺带保留完整原始快照。",
              "Scan and migrate local Claude Code data, then decide whether to do semantic migration only or keep the full raw snapshot as well.",
            )}
          </p>
        </div>
      </div>

      {!localMode ? (
        <div className="card">
          <div className="alert alert-warn">
            {tx(
              "这个页面只在本地模式下可用，因为它需要直接扫描当前机器上的 Claude Code 文件。",
              "This page is only available in local mode because it needs to scan Claude Code files on this machine directly.",
            )}
          </div>
          <div
            style={{
              marginTop: "1rem",
              display: "flex",
              gap: "0.75rem",
              flexWrap: "wrap",
            }}
          >
            <Link to="/" className="btn btn-primary">
              {tx("返回概览", "Back to overview")}
            </Link>
            <Link to="/imports/claude-export" className="btn">
              {tx("Claude 导出 ZIP", "Claude Export ZIP")}
            </Link>
            <Link to="/connections" className="btn">
              {tx("查看连接", "View connections")}
            </Link>
          </div>
        </div>
      ) : (
        <>
          <section className="materials-hero migration-hero">
            <div className="materials-hero-copy">
              <div className="materials-kicker">
                {tx("Scan First", "Scan First")}
              </div>
              <h3 className="materials-title">
                {tx("先看清楚再迁移", "See the shape before you migrate")}
              </h3>
              <p className="materials-subtitle">
                {tx(
                  "推荐先用语义迁移模式检查 projects、memory、skills、会话和敏感项，再决定要不要把原始平台快照一起带进来。",
                  "Start with semantic migration mode to inspect projects, memory, skills, conversations, and sensitive findings, then decide whether to include the raw platform snapshot too.",
                )}
              </p>
              <div className="migration-mode-grid">
                {modeOptions.map((option) => (
                  <button
                    key={option}
                    type="button"
                    className={`migration-mode-card ${mode === option ? "is-active" : ""}`}
                    onClick={() => setMode(option)}
                  >
                    <div className="migration-mode-title">
                      {option === "agent" &&
                        tx("语义迁移", "Semantic migration")}
                      {option === "all" &&
                        tx("迁移 + 原始快照", "Migration + raw snapshot")}
                      {option === "files" &&
                        tx("仅原始快照", "Raw snapshot only")}
                    </div>
                    <div className="migration-mode-copy">
                      {option === "agent" &&
                        tx(
                          "推荐。把可提升的数据迁成一等 neuDrive 内容。",
                          "Recommended. Promote durable Claude data into first-class neuDrive content.",
                        )}
                      {option === "all" &&
                        tx(
                          "同时保留 /platforms 下的原始文件证据。",
                          "Keep the raw files under /platforms as well.",
                        )}
                      {option === "files" &&
                        tx(
                          "只做文件级备份，不做语义重建。",
                          "Do a file-level backup only, without semantic reconstruction.",
                        )}
                    </div>
                  </button>
                ))}
              </div>
            </div>
            <div className="page-actions">
              <Link to="/imports/claude-export" className="btn">
                {tx("Claude 导出 ZIP", "Claude Export ZIP")}
              </Link>
              <button
                className="btn"
                type="button"
                disabled={previewing || importing || loadingPreviewTask}
                onClick={() => void handleRefresh()}
              >
                {previewing
                  ? tx("扫描中...", "Scanning...")
                  : taskStatus
                    ? tx("重新扫描", "Scan again")
                    : tx("开始扫描", "Start scan")}
              </button>
              <button
                className="btn btn-primary"
                type="button"
                disabled={previewing || importing}
                onClick={() => void handleImport()}
              >
                {importing
                  ? tx("导入中...", "Importing...")
                  : tx("导入到 neuDrive", "Import into neuDrive")}
              </button>
            </div>
            {scanTimingLabel ? (
              <p className="migration-timing-note">{scanTimingLabel}</p>
            ) : null}
            {!previewing && lastScanMetaLabel ? (
              <p className="migration-timing-note">{lastScanMetaLabel}</p>
            ) : null}
          </section>

          {error && <div className="alert alert-warn">{error}</div>}
          {success && <div className="alert alert-ok">{success}</div>}
          {syncInfo?.message && (
            <div className="alert alert-ok">{syncInfo.message}</div>
          )}

          <div className="stats-grid">
            <div className="stat-card">
              <div className="stat-value">{totalDiscovered}</div>
              <div className="stat-label">{tx("已发现项", "Discovered")}</div>
            </div>
            <div className="stat-card">
              <div className="stat-value">{totalImportable}</div>
              <div className="stat-label">{tx("可导入项", "Importable")}</div>
            </div>
            <div className="stat-card">
              <div className="stat-value">
                {preview?.sensitive_findings.length || 0}
              </div>
              <div className="stat-label">
                {tx("敏感项", "Sensitive findings")}
              </div>
            </div>
            <div className="stat-card">
              <div className="stat-value">
                {preview?.vault_candidates.length || 0}
              </div>
              <div className="stat-label">
                {tx("Vault 候选", "Vault candidates")}
              </div>
            </div>
          </div>

          <div className="dashboard-content-grid">
            <div className="card dashboard-card">
              <div className="card-header">
                <h3 className="card-title">
                  {tx("扫描分类", "Scan categories")}
                </h3>
                <span className="dashboard-card-link-muted">
                  {tx("按迁移口径统计", "Grouped by migration outcome")}
                </span>
              </div>
              {!preview || preview.categories.length === 0 ? (
                <p className="dashboard-empty-copy">
                  {previewing
                    ? tx("扫描中...", "Scanning...")
                    : loadingPreviewTask
                      ? tx("正在读取扫描状态...", "Loading scan status...")
                      : tx("还没有扫描结果。", "No preview data yet.")}
                </p>
              ) : (
                <div className="migration-category-list">
                  {preview.categories.map((category) => (
                    <div
                      key={category.name}
                      className="migration-category-item"
                    >
                      <div className="migration-category-name">
                        {categoryLabel(category.name, tx)}
                      </div>
                      <div className="migration-category-meta">
                        {tx("发现", "Discovered")}: {category.discovered} ·{" "}
                        {tx("可导入", "Importable")}: {category.importable} ·{" "}
                        {tx("归档", "Archived")}: {category.archived} ·{" "}
                        {tx("阻塞", "Blocked")}: {category.blocked}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="card dashboard-card">
              <div className="card-header">
                <h3 className="card-title">
                  {tx("建议命令", "Suggested command")}
                </h3>
                <span className="dashboard-card-link-muted">
                  {tx("CLI 和 dashboard 对齐", "Matches the CLI workflow")}
                </span>
              </div>
              <pre className="migration-command">
                {preview?.next_command ||
                  "neu import platform claude --dry-run --mode agent"}
              </pre>
              <div className="migration-preview-totals">
                <span>
                  {tx("归档项", "Archived")}: {totalArchived}
                </span>
                <span>
                  {tx("阻塞项", "Blocked")}: {totalBlocked}
                </span>
              </div>
            </div>
          </div>

          <div className="dashboard-content-grid">
            <div className="card dashboard-card">
              <div className="card-header">
                <h3 className="card-title">
                  {tx("敏感项", "Sensitive findings")}
                </h3>
                <span className="dashboard-card-link-muted">
                  {tx(
                    "默认只报告，不导入 secret 值",
                    "Reported only; secret values are not imported by default",
                  )}
                </span>
              </div>
              {preview?.sensitive_findings.length ? (
                <div className="migration-finding-list">
                  {preview.sensitive_findings.slice(0, 6).map((finding) => (
                    <div
                      key={`${finding.title}-${finding.redacted_example || ""}`}
                      className="migration-finding-item"
                    >
                      <div className="migration-finding-head">
                        <span
                          className={`migration-severity migration-severity-${finding.severity || "high"}`}
                        >
                          {finding.severity || "high"}
                        </span>
                        <span className="migration-finding-title">
                          {finding.title}
                        </span>
                      </div>
                      <div className="migration-finding-copy">
                        {finding.detail}
                      </div>
                      {finding.redacted_example ? (
                        <code className="migration-inline-code">
                          {finding.redacted_example}
                        </code>
                      ) : null}
                    </div>
                  ))}
                </div>
              ) : (
                <p className="dashboard-empty-copy">
                  {tx(
                    "这次扫描没有发现敏感项。",
                    "This scan did not find any sensitive entries.",
                  )}
                </p>
              )}
            </div>

            <div className="card dashboard-card">
              <div className="card-header">
                <h3 className="card-title">
                  {tx("Vault 候选", "Vault candidates")}
                </h3>
                <span className="dashboard-card-link-muted">
                  {tx("后续可以单独处理", "Follow up separately when needed")}
                </span>
              </div>
              {preview?.vault_candidates.length ? (
                <div className="migration-finding-list">
                  {preview.vault_candidates.slice(0, 6).map((candidate) => (
                    <div
                      key={candidate.scope}
                      className="migration-finding-item"
                    >
                      <div className="migration-finding-title">
                        {candidate.scope}
                      </div>
                      <div className="migration-finding-copy">
                        {candidate.description}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="dashboard-empty-copy">
                  {tx("还没有 Vault 候选项。", "No vault candidates yet.")}
                </p>
              )}
            </div>
          </div>

          {preview?.notes.length ? (
            <div className="card">
              <h3 className="card-title">{tx("扫描备注", "Scan notes")}</h3>
              <div className="migration-note-list">
                {preview.notes.map((note) => (
                  <div key={note} className="migration-note-item">
                    {note}
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          {result ? (
            <div className="card">
              <h3 className="card-title">
                {tx("最近一次导入结果", "Latest import result")}
              </h3>
              <div className="stats-grid migration-result-grid">
                <div className="stat-card">
                  <div className="stat-value">
                    {result.agent?.imported || result.files?.files || 0}
                  </div>
                  <div className="stat-label">{tx("已导入", "Imported")}</div>
                </div>
                <div className="stat-card">
                  <div className="stat-value">
                    {result.agent?.archived || 0}
                  </div>
                  <div className="stat-label">{tx("已归档", "Archived")}</div>
                </div>
                <div className="stat-card">
                  <div className="stat-value">{result.agent?.blocked || 0}</div>
                  <div className="stat-label">{tx("已阻塞", "Blocked")}</div>
                </div>
                <div className="stat-card">
                  <div className="stat-value">
                    {result.files
                      ? formatBytes(result.files.bytes, locale)
                      : String(result.agent?.conversations || 0)}
                  </div>
                  <div className="stat-label">
                    {result.files
                      ? tx("原始快照体积", "Raw snapshot size")
                      : tx("聊天会话", "Conversations")}
                  </div>
                </div>
              </div>
              <div className="migration-import-meta">
                {result.agent ? (
                  <span>
                    {tx("Profile", "Profile")}:{" "}
                    {result.agent.profile_categories} · Memory:{" "}
                    {result.agent.memory_items} · {tx("项目", "Projects")}:{" "}
                    {result.agent.projects} · Skills: {result.agent.bundles} ·{" "}
                    {tx("敏感项", "Sensitive")}:{" "}
                    {result.agent.sensitive_findings}
                  </span>
                ) : null}
                {result.files ? (
                  <span>
                    {tx("原始文件", "Raw files")}: {result.files.files} ·{" "}
                    {tx("体积", "Size")}:{" "}
                    {formatBytes(result.files.bytes, locale)}
                  </span>
                ) : null}
              </div>
              {importPaths.length ? (
                <div className="dashboard-file-list">
                  {importPaths.slice(0, 10).map((path) => (
                    <div key={path} className="dashboard-file-item">
                      <div className="dashboard-file-path">{path}</div>
                    </div>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}
        </>
      )}
    </div>
  );
}
