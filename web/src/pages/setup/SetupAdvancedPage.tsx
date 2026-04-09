import { TOKEN_ENV_NAME, TOKEN_PLACEHOLDER, useSetup } from '../SetupPage'
import { useI18n } from '../../i18n'
import { SetupCodeBlock, SetupSection } from './SetupShared'

export default function SetupAdvancedPage() {
  const { tx } = useI18n()
  const {
    copied,
    copyToClipboard,
    openModes,
    modeTokens,
    provisioningMode,
    advancedSessionToken,
    advancedEnvCommand,
    advancedCodexCommand,
    advancedConfig,
    toggleMode,
    provisionModeToken,
  } = useSetup()

  return (
    <SetupSection
      icon={<>&#128736;</>}
      title={tx('高级模式（HTTP + 手动 Bearer Token）', 'Advanced mode (HTTP + manual Bearer token)')}
      description={tx('面向支持 HTTP MCP 的通用客户端，使用静态 Bearer Token 直连 /mcp。', 'For generic clients that support HTTP MCP and connect directly to /mcp with a static Bearer token.')}
    >
      <p className="setup-note setup-note-first">
        {tx('说明默认直接可看，不会自动创建 token。推荐优先把 Bearer Token 放进环境变量 ', 'The guide is view-only by default and will not create a token automatically. Prefer storing the Bearer token in the environment variable ')}<code>{TOKEN_ENV_NAME}</code>{tx('；只有客户端不支持 env 方式时，再退回静态 Bearer header。', '; only fall back to a static Bearer header when the client cannot read from env.')}
      </p>

      <div className="setup-mode-actions">
        <button
          className="btn btn-primary"
          onClick={() => toggleMode('advanced')}
        >
          {openModes.advanced ? tx('隐藏高级模式配置', 'Hide advanced configuration') : tx('查看高级模式配置', 'View advanced configuration')}
        </button>
        {openModes.advanced && (
          <button
            className="btn btn-outline"
            onClick={() => provisionModeToken('advanced', !!modeTokens.advanced)}
            disabled={provisioningMode === 'advanced'}
          >
            {provisioningMode === 'advanced'
              ? tx('生成中...', 'Creating...')
              : modeTokens.advanced
                ? tx('重新生成 Token', 'Regenerate token')
                : tx('创建本模式 Token', 'Create token for this mode')}
          </button>
        )}
      </div>

      {openModes.advanced && (
        <>
          {modeTokens.advanced ? (
            <>
              <div className="alert alert-success">
                {tx('已为高级模式创建一个新的 Bearer Token。推荐下一步把它保存到环境变量 ', 'A new Bearer token was created for advanced mode. The recommended next step is to save it to the environment variable ')}<code>{TOKEN_ENV_NAME}</code>{tx('；完整值只会在当前页面会话里显示一次。', '. The full value is shown only once in this page session.')}
              </div>
              <SetupCodeBlock
                label={tx('刚创建的 Token（仅当前会话可见）', 'Newly created token (visible in this session only)')}
                content={advancedSessionToken}
                copied={copied}
                copyKey="advanced-token"
                onCopy={copyToClipboard}
                copyLabel={tx('复制 Token', 'Copy token')}
              />
            </>
          ) : (
            <div className="alert alert-warn">
              {tx('当前显示的是环境变量和配置模板，里面的 ', 'The current content only shows environment-variable and config templates. ')}<code>{TOKEN_PLACEHOLDER}</code>{tx(' 只是占位符。只有在你明确点击“创建本模式 Token”时，才会生成新的 Bearer Token。', ' is only a placeholder. A new Bearer token is created only when you explicitly click "Create token for this mode".')}
            </div>
          )}

          <SetupCodeBlock
            label={tx('步骤 1：设置环境变量', 'Step 1: set the environment variable')}
            content={advancedEnvCommand}
            copied={copied}
            copyKey="advanced-env"
            onCopy={copyToClipboard}
          />

          <SetupCodeBlock
            label={tx('步骤 2：Codex CLI 直接接入（推荐）', 'Step 2: connect from Codex CLI directly (recommended)')}
            content={advancedCodexCommand}
            copied={copied}
            copyKey="advanced-codex-cmd"
            onCopy={copyToClipboard}
          />

          <SetupCodeBlock
            label={tx('步骤 3：通用 MCP HTTP 配置（静态 Bearer，兜底方案）', 'Step 3: generic MCP HTTP config (static Bearer fallback)')}
            content={advancedConfig}
            copied={copied}
            copyKey="advanced-json"
            onCopy={copyToClipboard}
          />
        </>
      )}
    </SetupSection>
  )
}
