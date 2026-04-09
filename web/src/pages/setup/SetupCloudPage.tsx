import { useSetup } from '../SetupPage'
import { useI18n } from '../../i18n'
import { SetupCodeBlock, SetupSection } from './SetupShared'

export default function SetupCloudPage() {
  const { tx } = useI18n()
  const {
    baseUrl,
    cloudModeNeedsPublicUrl,
    cloudPlatform,
    setCloudPlatform,
    copied,
    copyToClipboard,
    claudeCloudCommand,
    codexCloudCommand,
    geminiCloudCommand,
    cursorAgentStatusCommand,
    codexLoginCommand,
    cursorAgentLoginCommand,
    geminiAuthCommand,
    codexStatusCommand,
  } = useSetup()

  return (
    <SetupSection
      icon={<>&#9729;</>}
      title="CLI Apps"
      description={tx('给命令行应用配置远程 HTTP MCP + OAuth。适合 Claude Code、Codex CLI、Gemini CLI 和 Cursor Agent。', 'Configure remote HTTP MCP + OAuth for CLI apps including Claude Code, Codex CLI, Gemini CLI, and Cursor Agent.')}
      badge={tx('推荐', 'Recommended')}
      highlight
    >
      {cloudModeNeedsPublicUrl && (
        <div className="alert alert-warn">
          {tx('当前地址是 ', 'Current address: ')}<code>{baseUrl}</code>{tx('。CLI Apps 需要一个可公开访问的 HTTPS Hub URL；如果你现在在本地开发，建议先用本地模式，或通过公网域名 / 隧道暴露这个 Hub。', '. CLI apps need a publicly reachable HTTPS Hub URL. If you are developing locally, use local mode first or expose this Hub through a public domain / tunnel.')}
        </div>
      )}

      <div className="setup-tabs" role="tablist" aria-label={tx('CLI Apps 平台', 'CLI app platforms')}>
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
        <button
          type="button"
          role="tab"
          className={`setup-tab ${cloudPlatform === 'gemini' ? 'setup-tab-active' : ''}`}
          aria-selected={cloudPlatform === 'gemini'}
          onClick={() => setCloudPlatform('gemini')}
        >
          Gemini
        </button>
        <button
          type="button"
          role="tab"
          className={`setup-tab ${cloudPlatform === 'cursor' ? 'setup-tab-active' : ''}`}
          aria-selected={cloudPlatform === 'cursor'}
          onClick={() => setCloudPlatform('cursor')}
        >
          Cursor
        </button>
      </div>

      <div className="setup-tab-panel">
        {cloudPlatform === 'claude' ? (
          <>
            <h4 className="setup-platform-title">Claude Code</h4>
            <p className="setup-note setup-note-first">
              {tx('把 Agent Hub 添加到 Claude Code 的全局 MCP 配置中，然后在 Claude Code 里发起浏览器授权。', 'Add Agent Hub to Claude Code global MCP config, then start browser authorization from Claude Code.')}
            </p>

            <SetupCodeBlock
              label={tx('步骤 1：添加远程 MCP Server（全局）', 'Step 1: add the remote MCP server (global)')}
              content={claudeCloudCommand}
              copied={copied}
              copyKey="cloud-claude-cmd"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label={tx('步骤 2：在 Claude Code 中发起授权', 'Step 2: start authorization from Claude Code')}
              content="/mcp"
              copied={copied}
              copyKey="cloud-claude-auth"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>{tx('运行上面的命令后，Agent Hub 会作为全局 MCP Server 出现在 Claude Code 中。', 'After running the command above, Agent Hub appears in Claude Code as a global MCP server.')}</li>
              <li>{tx('打开 Claude Code，执行 ', 'Open Claude Code, run ')}<code>/mcp</code>{tx('，选择 ', ', choose ')}<code>agenthub</code>{tx('，然后开始认证。', ', then start authentication.')}</li>
              <li>{tx('浏览器会打开授权页面；完成登录和批准后，Claude Code 会自动保存并刷新凭证。', 'The browser opens the authorization page. After login and approval, Claude Code saves and refreshes the credentials automatically.')}</li>
              <li>{tx('如果浏览器没有自动打开，就手动复制 Claude Code 提供的授权链接；如果网页授权完成后 CLI 仍提示等待，把浏览器地址栏里的完整 callback URL 粘回 Claude Code。', 'If the browser does not open automatically, copy the authorization link shown by Claude Code manually. If the CLI still waits after web auth completes, paste the full callback URL from the browser address bar back into Claude Code.')}</li>
            </ol>

            <p className="setup-note">
              {tx('授权完成后，你可以在 Claude Code 的 ', 'After authorization, you can reauthenticate or clear credentials from the Claude Code ')}<code>/mcp</code>{tx(' 菜单里重新认证或清除认证；Agent Hub 侧也会在“连接管理”中显示这条平台连接。', ' menu. Agent Hub will also show this platform connection under Connections.')}
            </p>
          </>
        ) : cloudPlatform === 'codex' ? (
          <>
            <h4 className="setup-platform-title">Codex CLI</h4>
            <p className="setup-note setup-note-first">
              {tx('把 Agent Hub 添加到 Codex 的全局 MCP 配置中，然后用 Codex CLI 发起浏览器授权。', 'Add Agent Hub to Codex global MCP config, then start browser authorization from Codex CLI.')}
            </p>

            <SetupCodeBlock
              label={tx('步骤 1：添加远程 MCP Server（全局）', 'Step 1: add the remote MCP server (global)')}
              content={codexCloudCommand}
              copied={copied}
              copyKey="cloud-codex-add"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label={tx('步骤 2：发起授权', 'Step 2: start authorization')}
              content={codexLoginCommand}
              copied={copied}
              copyKey="cloud-codex-login"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label={tx('步骤 3：确认连接状态', 'Step 3: confirm connection status')}
              content={codexStatusCommand}
              copied={copied}
              copyKey="cloud-codex-list"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>{tx('运行 add 命令后，Agent Hub 会写入 Codex 的用户级 MCP 配置，可在多个工作区复用。', 'After running the add command, Agent Hub is written into Codex user-level MCP config and can be reused across workspaces.')}</li>
              <li>{tx('运行 ', 'Run ')}<code>codex mcp login agenthub</code>{tx(' 后，浏览器会打开授权页面。', ' and the browser will open the authorization page.')}</li>
              <li>{tx('完成登录和批准后，Codex 会保存 OAuth 凭证；再次运行 ', 'After login and approval, Codex saves the OAuth credentials. Run ')}<code>codex mcp list</code>{tx(' 可以查看连接状态。', ' again to review connection status.')}</li>
              <li>{tx('如果浏览器没有自动打开，就手动复制终端里提供的授权链接继续完成授权。', 'If the browser does not open automatically, manually copy the authorization link shown in the terminal to continue.')}</li>
            </ol>

            <p className="setup-note">
              {tx('授权完成后，Agent Hub 侧会在“连接管理”中显示这条平台连接；需要重新认证时，可再次运行 ', 'After authorization, Agent Hub will show this platform connection under Connections. To reauthenticate, run ')}<code>codex mcp login agenthub</code>{tx('。', '.')}
            </p>
          </>
        ) : cloudPlatform === 'gemini' ? (
          <>
            <h4 className="setup-platform-title">Gemini CLI</h4>
            <p className="setup-note setup-note-first">
              {tx('把 Agent Hub 添加到 Gemini 的远程 MCP 配置中，然后在 Gemini 里发起 OAuth 授权。', 'Add Agent Hub to Gemini remote MCP config, then start OAuth authorization from Gemini.')}</p>

            <SetupCodeBlock
              label={tx('步骤 1：添加远程 MCP Server', 'Step 1: add the remote MCP server')}
              content={geminiCloudCommand}
              copied={copied}
              copyKey="cloud-gemini-add"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label={tx('步骤 2：在 Gemini 中发起授权', 'Step 2: start authorization from Gemini')}
              content={geminiAuthCommand}
              copied={copied}
              copyKey="cloud-gemini-auth"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>{tx('运行 add 命令时一定要带上 ', 'When running the add command, you must include ')}<code>--transport http</code>{tx('；不带这个参数时，Gemini 会把 URL 当成本地 command，而不是远程 MCP server。', '. Without it, Gemini treats the URL as a local command instead of a remote MCP server.')}</li>
              <li>{tx('添加完成后，打开 Gemini，执行 ', 'After adding it, open Gemini and run ')}<code>/mcp auth agenthub</code>{tx('；如果你用了别的 server 名称，把 ', '. If you used a different server name, replace ')}<code>agenthub</code>{tx(' 换成你自己的名称。', ' with your own name.')}</li>
              <li>{tx('浏览器会跳转到 Agent Hub 的登录与授权页；完成登录和批准后，Gemini 会保存 OAuth 凭证。', 'The browser opens the Agent Hub sign-in and authorization page. After login and approval, Gemini saves the OAuth credentials.')}</li>
              <li>{tx('接通后，你可以在 Gemini 里继续使用 ', 'After connecting, you can keep using ')}<code>/mcp</code>{tx(' 查看状态，或直接开始调用 Agent Hub 的工具。', ' in Gemini to inspect status, or immediately start calling Agent Hub tools.')}</li>
            </ol>

            <p className="setup-note">
              {tx('Gemini CLI 当前已验证 Remote MCP + OAuth 可用，真实请求形态是 dynamic registration + ', 'Gemini CLI has been verified with Remote MCP + OAuth. The request pattern is dynamic registration + ')}<code>client_secret_post</code>{tx(' token exchange。', ' token exchange.')}
            </p>
          </>
        ) : (
          <>
            <h4 className="setup-platform-title">Cursor Agent</h4>
            <p className="setup-note setup-note-first">
              {tx('Cursor Agent 会读取 ', 'Cursor Agent reads MCP config from ')}<code>.cursor/mcp.json</code>{tx(' 或 ', ' or ')}<code>~/.cursor/mcp.json</code>{tx(' 里的 MCP 配置，然后通过浏览器 OAuth 完成授权。', ', then finishes authorization through browser OAuth.')}
            </p>

            <SetupCodeBlock
              label={tx('步骤 1：配置 ~/.cursor/mcp.json', 'Step 1: configure ~/.cursor/mcp.json')}
              content={JSON.stringify({
                mcpServers: {
                  agenthub: {
                    url: `${baseUrl}/mcp`,
                  },
                },
              }, null, 2)}
              copied={copied}
              copyKey="cloud-cursor-agent-json"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label={tx('步骤 2：发起授权', 'Step 2: start authorization')}
              content={cursorAgentLoginCommand}
              copied={copied}
              copyKey="cloud-cursor-agent-login"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label={tx('步骤 3：确认连接状态', 'Step 3: confirm connection status')}
              content={cursorAgentStatusCommand}
              copied={copied}
              copyKey="cloud-cursor-agent-list"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>{tx('先在项目目录的 ', 'First add the config above to ')}<code>.cursor/mcp.json</code>{tx('，或用户目录的 ', ' in the project directory, or ')}<code>~/.cursor/mcp.json</code>{tx(' 中加入上面的 ', ', then add the same ')}<code>agenthub</code>{tx(' 配置。', ' config in the user directory.')}</li>
              <li>{tx('运行 ', 'Run ')}<code>cursor-agent mcp login agenthub</code>{tx(' 后，Cursor Agent 会自动读取配置，并在浏览器中发起 Agent Hub 的登录与授权。', '. Cursor Agent reads the config automatically and opens Agent Hub sign-in and authorization in the browser.')}</li>
              <li>{tx('授权完成后，再运行 ', 'After authorization, run ')}<code>cursor-agent mcp list</code>{tx(' 检查状态；需要查看工具时，可以继续运行 ', ' to inspect status. To view tools, you can also run ')}<code>cursor-agent mcp list-tools agenthub</code>{tx('。', '.')}</li>
              <li>{tx('Cursor Agent 当前真实请求形态和 Cursor Desktop 不同：它使用 ', 'Cursor Agent uses a different request pattern than Cursor Desktop. It uses ')}<code>http://localhost:8787/callback</code>{tx(' 作为回调地址，并以 ', ' as the callback URL and identifies itself as ')}<code>Cursor/1.0.0</code>{tx(' 身份完成 dynamic registration。', ' while performing dynamic registration.')}</li>
            </ol>

            <p className="setup-note">
              {tx('Cursor Agent 当前已验证 Remote MCP + OAuth 可用，真实请求形态是 dynamic registration + ', 'Cursor Agent has been verified with Remote MCP + OAuth. The request pattern is dynamic registration + ')}<code>client_secret_post</code>{tx(' token exchange。', ' token exchange.')}
            </p>
          </>
        )}
      </div>

      <p className="setup-note">
        {tx('如果你本机已经有一个同名的本地 MCP 配置，例如旧的 ', 'If you already have a local MCP config with the same name, such as an old ')}<code>agenthub</code>{tx(' stdio 配置，建议先删除或改名，避免在平台列表中和远程 OAuth 连接混淆。', ' stdio config, delete it or rename it first so it does not get confused with the remote OAuth connection in platform lists.')}
      </p>
    </SetupSection>
  )
}
