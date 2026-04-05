import { TOKEN_ENV_NAME, TOKEN_PLACEHOLDER, useSetup } from '../SetupPage'
import { SetupCodeBlock, SetupSection } from './SetupShared'

export default function SetupAdvancedPage() {
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
      title="高级模式（HTTP + 手动 Bearer Token）"
      description="面向支持 HTTP MCP 的通用客户端，使用静态 Bearer Token 直连 /mcp。"
    >
      <p className="setup-note setup-note-first">
        说明默认直接可看，不会自动创建 token。推荐优先把 Bearer Token 放进环境变量 <code>{TOKEN_ENV_NAME}</code>；只有客户端不支持 env 方式时，再退回静态 Bearer header。
      </p>

      <div className="setup-mode-actions">
        <button
          className="btn btn-primary"
          onClick={() => toggleMode('advanced')}
        >
          {openModes.advanced ? '隐藏高级模式配置' : '查看高级模式配置'}
        </button>
        {openModes.advanced && (
          <button
            className="btn btn-outline"
            onClick={() => provisionModeToken('advanced', !!modeTokens.advanced)}
            disabled={provisioningMode === 'advanced'}
          >
            {provisioningMode === 'advanced'
              ? '生成中...'
              : modeTokens.advanced
                ? '重新生成 Token'
                : '创建本模式 Token'}
          </button>
        )}
      </div>

      {openModes.advanced && (
        <>
          {modeTokens.advanced ? (
            <>
              <div className="alert alert-success">
                已为高级模式创建一个新的 Bearer Token。推荐下一步把它保存到环境变量 <code>{TOKEN_ENV_NAME}</code>；完整值只会在当前页面会话里显示一次。
              </div>
              <SetupCodeBlock
                label="刚创建的 Token（仅当前会话可见）"
                content={advancedSessionToken}
                copied={copied}
                copyKey="advanced-token"
                onCopy={copyToClipboard}
                copyLabel="复制 Token"
              />
            </>
          ) : (
            <div className="alert alert-warn">
              当前显示的是环境变量和配置模板，里面的 <code>{TOKEN_PLACEHOLDER}</code> 只是占位符。只有在你明确点击“创建本模式 Token”时，才会生成新的 Bearer Token。
            </div>
          )}

          <SetupCodeBlock
            label="步骤 1：设置环境变量"
            content={advancedEnvCommand}
            copied={copied}
            copyKey="advanced-env"
            onCopy={copyToClipboard}
          />

          <SetupCodeBlock
            label="步骤 2：Codex CLI 直接接入（推荐）"
            content={advancedCodexCommand}
            copied={copied}
            copyKey="advanced-codex-cmd"
            onCopy={copyToClipboard}
          />

          <SetupCodeBlock
            label="步骤 3：通用 MCP HTTP 配置（静态 Bearer，兜底方案）"
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
