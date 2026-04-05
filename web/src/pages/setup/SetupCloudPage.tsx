import { useSetup } from '../SetupPage'
import { SetupCodeBlock, SetupSection } from './SetupShared'

export default function SetupCloudPage() {
  const {
    baseUrl,
    cloudModeNeedsPublicUrl,
    cloudPlatform,
    setCloudPlatform,
    copied,
    copyToClipboard,
    claudeCloudCommand,
    codexCloudCommand,
    codexLoginCommand,
    codexStatusCommand,
  } = useSetup()

  return (
    <SetupSection
      icon={<>&#9729;</>}
      title="云端模式（浏览器授权）"
      description="通过远程 HTTP MCP Server 连接 Agent Hub。默认添加到全局配置，设置一次，可在多个项目中复用。"
      badge="推荐"
      highlight
    >
      {cloudModeNeedsPublicUrl && (
        <div className="alert alert-warn">
          当前地址是 <code>{baseUrl}</code>。云端模式需要可公开访问的 HTTPS Hub URL；如果你现在在本地开发，建议先用本地模式，或通过公网域名 / 隧道暴露这个 Hub。
        </div>
      )}

      <div className="setup-tabs" role="tablist" aria-label="云端模式平台">
        <button
          type="button"
          role="tab"
          className={`setup-tab ${cloudPlatform === 'claude' ? 'setup-tab-active' : ''}`}
          aria-selected={cloudPlatform === 'claude'}
          onClick={() => setCloudPlatform('claude')}
        >
          Claude
        </button>
        <button
          type="button"
          role="tab"
          className={`setup-tab ${cloudPlatform === 'codex' ? 'setup-tab-active' : ''}`}
          aria-selected={cloudPlatform === 'codex'}
          onClick={() => setCloudPlatform('codex')}
        >
          Codex
        </button>
      </div>

      <div className="setup-tab-panel">
        {cloudPlatform === 'claude' ? (
          <>
            <h4 className="setup-platform-title">Claude Code</h4>
            <p className="setup-note setup-note-first">
              把 Agent Hub 添加到 Claude Code 的全局 MCP 配置中，然后在 Claude Code 里发起浏览器授权。
            </p>

            <SetupCodeBlock
              label="步骤 1：添加远程 MCP Server（全局）"
              content={claudeCloudCommand}
              copied={copied}
              copyKey="cloud-claude-cmd"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label="步骤 2：在 Claude Code 中发起授权"
              content="/mcp"
              copied={copied}
              copyKey="cloud-claude-auth"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>运行上面的命令后，Agent Hub 会作为全局 MCP Server 出现在 Claude Code 中。</li>
              <li>打开 Claude Code，执行 <code>/mcp</code>，选择 <code>agenthub</code>，然后开始认证。</li>
              <li>浏览器会打开授权页面；完成登录和批准后，Claude Code 会自动保存并刷新凭证。</li>
              <li>如果浏览器没有自动打开，就手动复制 Claude Code 提供的授权链接；如果网页授权完成后 CLI 仍提示等待，把浏览器地址栏里的完整 callback URL 粘回 Claude Code。</li>
            </ol>

            <p className="setup-note">
              授权完成后，你可以在 Claude Code 的 <code>/mcp</code> 菜单里重新认证或清除认证；Agent Hub 侧也会在“连接管理”中显示这条平台连接。
            </p>
          </>
        ) : (
          <>
            <h4 className="setup-platform-title">Codex CLI</h4>
            <p className="setup-note setup-note-first">
              把 Agent Hub 添加到 Codex 的全局 MCP 配置中，然后用 Codex CLI 发起浏览器授权。
            </p>

            <SetupCodeBlock
              label="步骤 1：添加远程 MCP Server（全局）"
              content={codexCloudCommand}
              copied={copied}
              copyKey="cloud-codex-add"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label="步骤 2：发起授权"
              content={codexLoginCommand}
              copied={copied}
              copyKey="cloud-codex-login"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label="步骤 3：确认连接状态"
              content={codexStatusCommand}
              copied={copied}
              copyKey="cloud-codex-list"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>运行 add 命令后，Agent Hub 会写入 Codex 的用户级 MCP 配置，可在多个工作区复用。</li>
              <li>运行 <code>codex mcp login agenthub</code> 后，浏览器会打开授权页面。</li>
              <li>完成登录和批准后，Codex 会保存 OAuth 凭证；再次运行 <code>codex mcp list</code> 可以查看连接状态。</li>
              <li>如果浏览器没有自动打开，就手动复制终端里提供的授权链接继续完成授权。</li>
            </ol>

            <p className="setup-note">
              授权完成后，Agent Hub 侧会在“连接管理”中显示这条平台连接；需要重新认证时，可再次运行 <code>codex mcp login agenthub</code>。
            </p>
          </>
        )}
      </div>

      <p className="setup-note">
        如果你本机已经有一个同名的本地 MCP 配置，例如旧的 <code>agenthub</code> stdio 配置，建议先删除或改名，避免在平台列表中和云端连接混淆。
      </p>
    </SetupSection>
  )
}
