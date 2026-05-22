import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import {
  api,
  type LocalGitSyncInfo,
  type LocalPlatformImportPreview,
  type LocalPlatformImportPreviewTaskStatus,
  type LocalPlatformImportSummary,
} from "../api";
import { useI18n } from "../i18n";

type LocalImportPlatform = "claude" | "codex";

interface ClaudeMigrationPageProps {
  localMode?: boolean;
  initialPlatform?: LocalImportPlatform;
  officialExportPath?: string;
}

const localImportPlatforms: Array<{
  key: LocalImportPlatform;
  label: string;
  platform: "claude" | "codex";
  displayName: string;
  description: { zh: string; en: string };
}> = [
  {
    key: "claude",
    label: "Claude Code",
    platform: "claude",
    displayName: "Claude Code",
    description: {
      zh: "扫描当前机器上的 Claude Code 项目、记忆、skills、会话和可迁移资料。",
      en: "Scan Claude Code projects, memory, skills, conversations, and portable local data on this machine.",
    },
  },
  {
    key: "codex",
    label: "Codex",
    platform: "codex",
    displayName: "Codex",
    description: {
      zh: "扫描当前机器上的 Codex 配置、规则、skills、会话和可迁移资料。",
      en: "Scan Codex config, rules, skills, sessions, and portable local data on this machine.",
    },
  },
];

const semanticMode = "agent" as const;

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

function displayImportCommand(command: string | undefined, platform: string) {
  const trimmed = (command || "").trim();
  if (!trimmed) return `neu import ${platform}`;
  const legacyPlatformMatch = trimmed.match(/^neudrive\s+import\s+platform\s+([a-z0-9-]+)(?:\s+--mode\s+\S+)?$/i);
  if (legacyPlatformMatch?.[1]) return `neu import ${legacyPlatformMatch[1]}`;
  return trimmed
    .replace(/^neudrive\s+/, "neu ")
    .replace(/\s+--mode\s+\S+/g, "");
}

export default function ClaudeMigrationPage({
  localMode = false,
  initialPlatform = "claude",
  officialExportPath,
}: ClaudeMigrationPageProps) {
  const { locale, tx } = useI18n();
  const [searchParams, setSearchParams] = useSearchParams();
  const urlPlatform = searchParams.get("platform") === "codex" ? "codex" : "claude";
  const [activePlatform, setActivePlatform] = useState<LocalImportPlatform>(
    searchParams.has("platform") ? urlPlatform : initialPlatform,
  );
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

  const selectedPlatform =
    localImportPlatforms.find((item) => item.key === activePlatform) ||
    localImportPlatforms[0];
  const platform = selectedPlatform.platform;
  const displayName = selectedPlatform.displayName;
  const previewing = taskStatus?.state === "running";

  useEffect(() => {
    if (!searchParams.has("platform")) return;
    setActivePlatform(urlPlatform);
  }, [searchParams, urlPlatform]);

  useEffect(() => {
    if (!localMode) return;
    let cancelled = false;
    setLoadingPreviewTask(true);
    setTaskStatus(null);
    setPreview(null);
    void api
      .getLocalPlatformImportPreviewTask({ platform, mode: semanticMode })
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
  }, [localMode, platform]);

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
        .getLocalPlatformImportPreviewTask({ platform, mode: semanticMode })
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
  }, [localMode, platform, previewing]);

  const selectPlatform = (next: LocalImportPlatform) => {
    setActivePlatform(next);
    setSearchParams(next === "claude" ? {} : { platform: next });
    setError("");
    setSuccess("");
    setResult(null);
  };

  const handleRefresh = async () => {
    setError("");
    setSuccess("");
    setLoadingPreviewTask(true);
    try {
      const data = await api.startLocalPlatformImportPreviewTask({
        platform,
        mode: semanticMode,
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
        platform,
        mode: semanticMode,
      });
      setResult(response.data);
      setSyncInfo(response.localGitSync || null);
      setSuccess(
        tx(
          `${displayName} 数据已导入到 neuDrive。`,
          `${displayName} data has been imported into neuDrive.`,
        ),
      );
    } catch (err: any) {
      setError(err.message || tx("导入失败", "Import failed"));
    } finally {
      setImporting(false);
    }
  };

  const totalDiscovered =
    (Array.isArray(preview?.categories) ? preview.categories : []).reduce(
      (sum, category) => sum + (category.discovered || 0),
      0,
    ) || 0;
  const totalImportable =
    (Array.isArray(preview?.categories) ? preview.categories : []).reduce(
      (sum, category) => sum + (category.importable || 0),
      0,
    ) || 0;
  const totalArchived =
    (Array.isArray(preview?.categories) ? preview.categories : []).reduce(
      (sum, category) => sum + (category.archived || 0),
      0,
    ) || 0;
  const totalBlocked =
    (Array.isArray(preview?.categories) ? preview.categories : []).reduce(
      (sum, category) => sum + (category.blocked || 0),
      0,
    ) || 0;
  const previewCategories = Array.isArray(preview?.categories)
    ? preview.categories
    : [];
  const previewSensitiveFindings = Array.isArray(preview?.sensitive_findings)
    ? preview.sensitive_findings
    : [];
  const previewVaultCandidates = Array.isArray(preview?.vault_candidates)
    ? preview.vault_candidates
    : [];
  const previewNotes = Array.isArray(preview?.notes) ? preview.notes : [];
  const importPaths = result?.agent?.paths || result?.files?.paths || [];
  const targetPaths =
    platform === "codex"
      ? ["/skills/<skill-name>/...", "/memory/codex/...", "/conversations/codex/..."]
      : ["/skills/<skill-name>/...", "/memory/claude/...", "/conversations/claude/..."];
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
      {!localMode ? (
        <div className="card">
          <div className="alert alert-warn">
            {tx(
              "这个页面只在本地模式下可用，因为它需要直接扫描当前机器上的 App 文件。",
              "This page is only available in local mode because it needs to scan app files on this machine directly.",
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
            {officialExportPath ? (
              <Link to={officialExportPath} className="btn">
                {tx("Claude 导出 ZIP", "Claude Export ZIP")}
              </Link>
            ) : null}
            <Link to="/connections" className="btn">
              {tx("查看连接", "View connections")}
            </Link>
          </div>
        </div>
      ) : (
        <>
          <section className="local-import-layout">
            <div className="card local-import-action-card">
              <div className="card-header">
                <h3 className="card-title">{tx("本地 App Data 导入", "Local app data import")}</h3>
                <span className="status-pill">{tx("语义迁移", "Semantic import")}</span>
              </div>
              <div className="local-import-tabs" role="tablist" aria-label={tx("选择本地 App", "Choose local app")}>
                {localImportPlatforms.map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    role="tab"
                    aria-selected={activePlatform === item.key}
                    className={activePlatform === item.key ? "is-active" : ""}
                    onClick={() => selectPlatform(item.key)}
                  >
                    {item.label}
                  </button>
                ))}
              </div>
              <p className="local-import-copy">
                {tx(selectedPlatform.description.zh, selectedPlatform.description.en)}
              </p>
              <div className="page-actions">
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
                      : tx("扫描", "Scan")}
                </button>
                <button
                  className="btn btn-primary"
                  type="button"
                  disabled={previewing || importing}
                  onClick={() => void handleImport()}
                >
                  {importing
                    ? tx("导入中...", "Importing...")
                    : tx("导入", "Import")}
                </button>
              </div>
              <div className="local-import-targets" aria-label={tx("目标路径", "Target paths")}>
                <span>{tx("目标路径", "Target paths")}</span>
                {targetPaths.map((target) => (
                  <code key={target}>{target}</code>
                ))}
              </div>
              <div className="local-import-command">
                <div className="local-import-command-label">{tx("也可使用终端命令", "Terminal command")}</div>
                <pre className="migration-command">
                  {displayImportCommand(preview?.next_command, platform)}
                </pre>
              </div>
            </div>
          </section>

          {error && <div className="alert alert-warn">{error}</div>}
          {success && <div className="alert alert-ok">{success}</div>}
          {syncInfo?.message && (
            <div className="alert alert-ok">{syncInfo.message}</div>
          )}

          <div className="local-import-results-grid">
            <div className="card dashboard-card local-import-categories-card">
              <div className="card-header">
                <h3 className="card-title">
                  {tx("扫描分类", "Scan categories")}
                </h3>
              </div>
              {lastScanMetaLabel ? (
                <p className="migration-timing-note">{lastScanMetaLabel}</p>
              ) : null}
              {!preview || previewCategories.length === 0 ? (
                <p className="dashboard-empty-copy">
                  {previewing
                    ? tx("扫描中...", "Scanning...")
                    : loadingPreviewTask
                      ? tx("正在读取扫描状态...", "Loading scan status...")
                      : tx("还没有扫描结果。", "No preview data yet.")}
                </p>
              ) : (
                <div className="migration-category-table">
                  <div className="migration-category-table-head">
                    <span>{tx("分类", "Category")}</span>
                    <span>{tx("发现", "Found")}</span>
                    <span>{tx("可导入", "Importable")}</span>
                    <span>{tx("归档", "Archived")}</span>
                    <span>{tx("阻塞", "Blocked")}</span>
                  </div>
                  {previewCategories.map((category) => (
                    <div
                      key={category.name}
                      className="migration-category-row"
                    >
                      <div className="migration-category-name">
                        {categoryLabel(category.name, tx)}
                      </div>
                      <strong>{category.discovered}</strong>
                      <strong>{category.importable}</strong>
                      <span>{category.archived}</span>
                      <span>{category.blocked}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="local-import-side-stack">
              <div className="card dashboard-card">
                <div className="card-header">
                  <h3 className="card-title">
                    {tx("敏感项", "Sensitive findings")}
                  </h3>
                  <span className="dashboard-card-link-muted">
                    {previewSensitiveFindings.length}
                  </span>
                </div>
                {previewSensitiveFindings.length ? (
                  <>
                    <div className="migration-finding-list local-import-detail-list">
                      {previewSensitiveFindings.slice(0, 2).map((finding) => (
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
                    {previewSensitiveFindings.length > 2 ? (
                      <p className="local-import-more">
                        {tx(
                          `还有 ${previewSensitiveFindings.length - 2} 项未显示`,
                          `${previewSensitiveFindings.length - 2} more hidden`,
                        )}
                      </p>
                    ) : null}
                  </>
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
                    {previewVaultCandidates.length}
                  </span>
                </div>
                {previewVaultCandidates.length ? (
                  <>
                    <div className="migration-finding-list local-import-detail-list">
                      {previewVaultCandidates.slice(0, 3).map((candidate) => (
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
                    {previewVaultCandidates.length > 3 ? (
                      <p className="local-import-more">
                        {tx(
                          `还有 ${previewVaultCandidates.length - 3} 项未显示`,
                          `${previewVaultCandidates.length - 3} more hidden`,
                        )}
                      </p>
                    ) : null}
                  </>
                ) : (
                  <p className="dashboard-empty-copy">
                    {tx("还没有 Vault 候选项。", "No vault candidates yet.")}
                  </p>
                )}
              </div>
            </div>
          </div>

          {previewNotes.length ? (
            <div className="card">
              <h3 className="card-title">{tx("扫描备注", "Scan notes")}</h3>
              <div className="migration-note-list">
                {previewNotes.map((note) => (
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
