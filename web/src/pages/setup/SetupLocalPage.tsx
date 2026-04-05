import { TOKEN_ENV_NAME, TOKEN_PLACEHOLDER, useSetup } from '../SetupPage'
import { SetupCodeBlock, SetupSection } from './SetupShared'

export default function SetupLocalPage() {
  const {
    copied,
    copyToClipboard,
    localPlatform,
    setLocalPlatform,
    openModes,
    modeTokens,
    provisioningMode,
    localSessionToken,
    localEnvCommand,
    localClaudeCommand,
    localCodexCommand,
    localConfig,
    toggleMode,
    provisionModeToken,
  } = useSetup()

  return (
    <SetupSection
      icon={<>&#128187;</>}
      title="本地模式（stdio + Token）"
      description="通过本地 agenthub-mcp binary + scoped token 连接，适合本地开发或内网环境。"
    >
      <p className="setup-note setup-note-first">
        说明默认直接可看，不会自动创建 token。推荐把 token 放进环境变量 <code>{TOKEN_ENV_NAME}</code>，再让 Claude Code 或 Codex CLI 在启动本地 MCP binary 时读取它。
      </p>

      <div className="setup-tabs" role="tablist" aria-label="本地模式平台">
        <button
          type="button"
          role="tab"
          className={`setup-tab ${localPlatform === 'claude' ? 'setup-tab-active' : ''}`}
          aria-selected={localPlatform === 'claude'}
          onClick={() => setLocalPlatform('claude')}
        >
          Claude
        </button>
        <button
          type="button"
          role="tab"
          className={`setup-tab ${localPlatform === 'codex' ? 'setup-tab-active' : ''}`}
          aria-selected={localPlatform === 'codex'}
          onClick={() => setLocalPlatform('codex')}
        >
          Codex
        </button>
      </div>

      <div className="setup-mode-actions">
        <button
          className="btn btn-primary"
          onClick={() => toggleMode('local')}
        >
          {openModes.local ? '隐藏本地模式配置' : '查看本地模式配置'}
        </button>
        {openModes.local && (
          <button
            className="btn btn-outline"
            onClick={() => provisionModeToken('local', !!modeTokens.local)}
            disabled={provisioningMode === 'local'}
          >
            {provisioningMode === 'local'
              ? '生成中...'
              : modeTokens.local
                ? '重新生成 Token'
                : '创建本模式 Token'}
          </button>
        )}
      </div>

      {openModes.local && (
        <div className="setup-tab-panel">
          {modeTokens.local ? (
            <>
              <div className="alert alert-success">
                已为本地模式创建一个新的 token。推荐下一步把它保存到环境变量 <code>{TOKEN_ENV_NAME}</code>；完整值只会在当前页面会话里显示一次，丢失后需要重新生成。
              </div>
              <SetupCodeBlock
                label="刚创建的 Token（仅当前会话可见）"
                content={localSessionToken}
                copied={copied}
                copyKey="local-token"
                onCopy={copyToClipboard}
                copyLabel="复制 Token"
              />
            </>
          ) : (
            <div className="alert alert-warn">
              当前显示的是环境变量和配置模板，里面的 <code>{TOKEN_PLACEHOLDER}</code> 只是占位符。查看接法不需要新建 token；如果你要立即接入，再点上面的“创建本模式 Token”即可。
            </div>
          )}

          {localPlatform === 'claude' ? (
            <>
              <h4 className="setup-platform-title">Claude Code</h4>
              <p className="setup-note setup-note-first">
                先在启动 Claude Code 的同一 shell、shell profile 或 launcher 里设置 <code>{TOKEN_ENV_NAME}</code>，再把 Agent Hub 注册为全局 stdio MCP server。
              </p>

              <SetupCodeBlock
                label="步骤 1：设置环境变量"
                content={localEnvCommand}
                copied={copied}
                copyKey="local-env"
                onCopy={copyToClipboard}
              />

              <SetupCodeBlock
                label="步骤 2：注册本地 MCP Server（全局）"
                content={localClaudeCommand}
                copied={copied}
                copyKey="local-claude-cmd"
                onCopy={copyToClipboard}
              />

              <p className="setup-or">或者手动写入 Claude Code 的 MCP 配置：</p>

              <SetupCodeBlock
                label="Claude Code MCP JSON"
                content={localConfig}
                copied={copied}
                copyKey="local-claude-json"
                onCopy={copyToClipboard}
              />
            </>
          ) : (
            <>
              <h4 className="setup-platform-title">Codex CLI</h4>
              <p className="setup-note setup-note-first">
                先在启动 Codex CLI 的同一 shell、shell profile 或 launcher 里设置 <code>{TOKEN_ENV_NAME}</code>，再把 Agent Hub 添加到 Codex 的 stdio MCP 配置中。
              </p>

              <SetupCodeBlock
                label="步骤 1：设置环境变量"
                content={localEnvCommand}
                copied={copied}
                copyKey="local-env-codex"
                onCopy={copyToClipboard}
              />

              <SetupCodeBlock
                label="步骤 2：注册本地 MCP Server"
                content={localCodexCommand}
                copied={copied}
                copyKey="local-codex-cmd"
                onCopy={copyToClipboard}
              />

              <p className="setup-note">
                Codex 推荐直接使用上面的 <code>codex mcp add ...</code> 命令完成配置，无需手动编辑配置文件。
              </p>
            </>
          )}
        </div>
      )}
    </SetupSection>
  )
}
