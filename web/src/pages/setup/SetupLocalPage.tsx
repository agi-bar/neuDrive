import { TOKEN_ENV_NAME, TOKEN_PLACEHOLDER, useSetup } from '../SetupPage'
import { useI18n } from '../../i18n'
import { SetupCodeBlock, SetupSection } from './SetupShared'

export default function SetupLocalPage() {
  const { tx } = useI18n()
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
      title={tx('本地模式（stdio + Token）', 'Local mode (stdio + token)')}
      description={tx('通过本地 agenthub-mcp binary + scoped token 连接，适合本地开发或内网环境。', 'Connect through a local agenthub-mcp binary and scoped token for local development or internal networks.')}
    >
      <p className="setup-note setup-note-first">
        {tx('说明默认直接可看，不会自动创建 token。推荐把 token 放进环境变量 ', 'The guide is view-only by default and will not create a token automatically. Prefer storing the token in ')}<code>{TOKEN_ENV_NAME}</code>{tx('，再让 Claude Code 或 Codex CLI 在启动本地 MCP binary 时读取它。', ' so Claude Code or Codex CLI can read it when starting the local MCP binary.')}
      </p>

      <div className="setup-tabs" role="tablist" aria-label={tx('本地模式平台', 'Local mode platforms')}>
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
          {openModes.local ? tx('隐藏本地模式配置', 'Hide local configuration') : tx('查看本地模式配置', 'View local configuration')}
        </button>
        {openModes.local && (
          <button
            className="btn btn-outline"
            onClick={() => provisionModeToken('local', !!modeTokens.local)}
            disabled={provisioningMode === 'local'}
          >
            {provisioningMode === 'local'
              ? tx('生成中...', 'Creating...')
              : modeTokens.local
                ? tx('重新生成 Token', 'Regenerate token')
                : tx('创建本模式 Token', 'Create token for this mode')}
          </button>
        )}
      </div>

      {openModes.local && (
        <div className="setup-tab-panel">
          {modeTokens.local ? (
            <>
              <div className="alert alert-success">
                {tx('已为本地模式创建一个新的 token。推荐下一步把它保存到环境变量 ', 'A new token was created for local mode. The recommended next step is to save it to the environment variable ')}<code>{TOKEN_ENV_NAME}</code>{tx('；完整值只会在当前页面会话里显示一次，丢失后需要重新生成。', '. The full value is shown only once in this page session, and you will need to regenerate it if you lose it.')}
              </div>
              <SetupCodeBlock
                label={tx('刚创建的 Token（仅当前会话可见）', 'Newly created token (visible in this session only)')}
                content={localSessionToken}
                copied={copied}
                copyKey="local-token"
                onCopy={copyToClipboard}
                copyLabel={tx('复制 Token', 'Copy token')}
              />
            </>
          ) : (
            <div className="alert alert-warn">
              {tx('当前显示的是环境变量和配置模板，里面的 ', 'The current content only shows environment-variable and config templates. ')}<code>{TOKEN_PLACEHOLDER}</code>{tx(' 只是占位符。查看接法不需要新建 token；如果你要立即接入，再点上面的“创建本模式 Token”即可。', ' is only a placeholder. You do not need to create a token just to review the setup. If you want to connect right now, click "Create token for this mode" above.')}
            </div>
          )}

          {localPlatform === 'claude' ? (
            <>
              <h4 className="setup-platform-title">Claude Code</h4>
              <p className="setup-note setup-note-first">
                {tx('先在启动 Claude Code 的同一 shell、shell profile 或 launcher 里设置 ', 'Set ')}<code>{TOKEN_ENV_NAME}</code>{tx('，再把 Agent Hub 注册为全局 stdio MCP server。', ' in the same shell, shell profile, or launcher that starts Claude Code, then register Agent Hub as a global stdio MCP server.')}
              </p>

              <SetupCodeBlock
                label={tx('步骤 1：设置环境变量', 'Step 1: set the environment variable')}
                content={localEnvCommand}
                copied={copied}
                copyKey="local-env"
                onCopy={copyToClipboard}
              />

              <SetupCodeBlock
                label={tx('步骤 2：注册本地 MCP server（全局）', 'Step 2: register the local MCP server (global)')}
                content={localClaudeCommand}
                copied={copied}
                copyKey="local-claude-cmd"
                onCopy={copyToClipboard}
              />

              <p className="setup-or">{tx('或者手动写入 Claude Code 的 MCP 配置：', 'Or edit the Claude Code MCP config manually:')}</p>

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
                {tx('先在启动 Codex CLI 的同一 shell、shell profile 或 launcher 里设置 ', 'Set ')}<code>{TOKEN_ENV_NAME}</code>{tx('，再把 Agent Hub 添加到 Codex 的 stdio MCP 配置中。', ' in the same shell, shell profile, or launcher that starts Codex CLI, then add Agent Hub to Codex stdio MCP config.')}
              </p>

              <SetupCodeBlock
                label={tx('步骤 1：设置环境变量', 'Step 1: set the environment variable')}
                content={localEnvCommand}
                copied={copied}
                copyKey="local-env-codex"
                onCopy={copyToClipboard}
              />

              <SetupCodeBlock
                label={tx('步骤 2：注册本地 MCP server', 'Step 2: register the local MCP server')}
                content={localCodexCommand}
                copied={copied}
                copyKey="local-codex-cmd"
                onCopy={copyToClipboard}
              />

              <p className="setup-note">
                {tx('Codex 推荐直接使用上面的 ', 'Codex recommends using the ')}<code>codex mcp add ...</code>{tx(' 命令完成配置，无需手动编辑配置文件。', ' command above instead of editing config files manually.')}
              </p>
            </>
          )}
        </div>
      )}
    </SetupSection>
  )
}
