import { useState } from 'react'
import { useSetup } from '../SetupPage'
import { SetupCodeBlock, SetupScreenshotPlaceholder, SetupSection } from './SetupShared'

type WebAppTab = 'claude' | 'chatgpt' | 'cursor' | 'windsurf'

export default function SetupWebAppsPage() {
  const { baseUrl, cloudModeNeedsPublicUrl, copied, copyToClipboard } = useSetup()
  const [platform, setPlatform] = useState<WebAppTab>('claude')
  const mcpUrl = `${baseUrl}/mcp`

  return (
    <SetupSection
      icon={<>&#127760;</>}
      title="Web / Desktop Apps"
      description="在网页应用或桌面图形应用里，把 Agent Hub 添加成远程 MCP Server。"
      highlight
    >
      {cloudModeNeedsPublicUrl && (
        <div className="alert alert-warn">
          当前地址是 <code>{baseUrl}</code>。Web / Desktop Apps 需要一个可公开访问的 HTTPS MCP 地址；如果你现在在本地开发，请先切到公网域名或隧道地址。
        </div>
      )}

      <p className="setup-note setup-note-first">
        这一页面面向通过图形界面完成连接的场景，包括浏览器里的 Apps / Connectors，以及像 Cursor、Windsurf 这样的桌面应用。
      </p>

      <div className="setup-tabs" role="tablist" aria-label="Web / Desktop Apps 平台">
        <button
          type="button"
          role="tab"
          className={`setup-tab ${platform === 'claude' ? 'setup-tab-active' : ''}`}
          aria-selected={platform === 'claude'}
          onClick={() => setPlatform('claude')}
        >
          Claude
        </button>
        <button
          type="button"
          role="tab"
          className={`setup-tab ${platform === 'chatgpt' ? 'setup-tab-active' : ''}`}
          aria-selected={platform === 'chatgpt'}
          onClick={() => setPlatform('chatgpt')}
        >
          ChatGPT
        </button>
        <button
          type="button"
          role="tab"
          className={`setup-tab ${platform === 'cursor' ? 'setup-tab-active' : ''}`}
          aria-selected={platform === 'cursor'}
          onClick={() => setPlatform('cursor')}
        >
          Cursor
        </button>
        <button
          type="button"
          role="tab"
          className={`setup-tab ${platform === 'windsurf' ? 'setup-tab-active' : ''}`}
          aria-selected={platform === 'windsurf'}
          onClick={() => setPlatform('windsurf')}
        >
          Windsurf
        </button>
      </div>

      <div className="setup-tab-panel">
        {platform === 'claude' ? (
          <>
            <h4 className="setup-platform-title">Claude Connectors</h4>
            <p className="setup-note setup-note-first">
              登录 Claude 网页应用后，在 Connectors 里创建一个自定义 connector，再完成 Agent Hub 的网页登录与授权。
            </p>

            <SetupCodeBlock
              label="Remote MCP Server URL"
              content={mcpUrl}
              copied={copied}
              copyKey="webapp-claude-url"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>登录 Claude 网页应用，进入 <code>Settings -&gt; Connectors</code>，点击 <code>Go to Customize</code>。</li>
              <li>在 Customize 页的 Connectors 区域点击 <code>+</code>，再点击 <code>Add custom connector</code>。</li>
              <li>名称可以自定义，例如 <code>AgentHub</code>；把 <code>Remote MCP server URL</code> 填写为 <code>{mcpUrl}</code>，然后点击 <code>Add</code>。</li>
              <li>回到 connector 列表后，打开刚创建的 <code>AgentHub</code> connector，点击 <code>Connect</code>。</li>
              <li>浏览器会跳转到 Agent Hub 的登录与授权页；登录后点击授权，完成后回到 Claude，会显示成功连接。</li>
              <li>可选：在 <code>AgentHub</code> configuration 的 <code>Tools Permissions</code> 里选择 <code>Always allow</code>，减少每次工具调用前的确认。</li>
              <li>回到 Claude chat 后，就可以直接发起工具调用，例如“从 Agent Hub 中读取我的 profile”。</li>
            </ol>

            <div className="setup-screenshot-grid">
              <SetupScreenshotPlaceholder
                title="Claude Connectors 列表"
                caption="占位图：Settings -> Connectors -> Go to Customize -> Add custom connector"
              />
              <SetupScreenshotPlaceholder
                title="Agent Hub 授权完成"
                caption="占位图：Claude 中的 AgentHub connector 已显示 Connected"
              />
            </div>

            <p className="setup-note">
              如果你使用的是团队版或企业版 Claude，Connectors 的入口位置可能由管理员策略决定；看不到自定义 connector 入口时，请先确认当前账号支持 Remote MCP Custom Connectors。
            </p>
          </>
        ) : platform === 'chatgpt' ? (
          <>
            <h4 className="setup-platform-title">ChatGPT Apps</h4>
            <p className="setup-note setup-note-first">
              登录 ChatGPT 后，在 Apps 设置里创建一个指向 Agent Hub 的 MCP app，再按提示完成连接。
            </p>

            <div className="alert alert-warn">
              ChatGPT 的 Apps / MCP 入口取决于你的账号计划和灰度范围。如果设置里看不到 <code>Apps</code>，通常意味着当前账号还没有这个入口。
            </div>

            <SetupCodeBlock
              label="MCP Server URL"
              content={mcpUrl}
              copied={copied}
              copyKey="webapp-chatgpt-url"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>登录 ChatGPT，进入 <code>Settings -&gt; Apps</code>。</li>
              <li>在 <code>Advanced settings</code> 区域点击 <code>Create app</code>。</li>
              <li>把 <code>MCP Server URL</code> 填写为 <code>{mcpUrl}</code>，然后点击 <code>Create</code>。</li>
              <li>如果随后出现 <code>Connect</code>、<code>Sign in</code> 或授权提示，按提示跳转到 Agent Hub 登录并完成授权。</li>
              <li>返回 ChatGPT 后，确认这个 app 已处于可用状态，再回到对话里使用对应工具。</li>
            </ol>

            <div className="setup-screenshot-grid">
              <SetupScreenshotPlaceholder
                title="ChatGPT Apps 设置"
                caption="占位图：Settings -> Apps -> Advanced settings -> Create app"
              />
              <SetupScreenshotPlaceholder
                title="ChatGPT App 已创建"
                caption="占位图：新建的 AgentHub app 已出现在 Apps 列表中"
              />
            </div>

            <p className="setup-note">
              创建完成后，你可以回到 ChatGPT 对话中直接要求它使用 Agent Hub，例如“从 Agent Hub 中读取我的 profile”。
            </p>
          </>
        ) : platform === 'cursor' ? (
          <>
            <h4 className="setup-platform-title">Cursor Desktop</h4>
            <p className="setup-note setup-note-first">
              在 Cursor Desktop 里添加一个自定义 Remote MCP Server，然后通过浏览器 OAuth 完成授权。
            </p>

            <SetupCodeBlock
              label="Remote MCP Server URL"
              content={mcpUrl}
              copied={copied}
              copyKey="webapp-cursor-url"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label="可选：~/.cursor/mcp.json"
              content={JSON.stringify({
                mcpServers: {
                  agenthub: {
                    url: mcpUrl,
                  },
                },
              }, null, 2)}
              copied={copied}
              copyKey="webapp-cursor-json"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>打开 Cursor，进入 <code>Settings -&gt; Tools &amp; MCPs</code>，点击 <code>Add Custom MCP</code>。</li>
              <li>如果界面要求填写 URL，就把 <code>Remote MCP Server URL</code> 设为 <code>{mcpUrl}</code>；如果要求粘贴配置，也可以直接使用上面的 <code>~/.cursor/mcp.json</code> 片段。</li>
              <li>保存后点击 <code>Connect</code> 或 <code>Authenticate</code>；Cursor 会自动发现 Agent Hub 的 OAuth metadata。</li>
              <li>浏览器会跳转到 Agent Hub 的登录与授权页；完成登录和批准后，Cursor 会回到已连接状态。</li>
              <li>接通后，Cursor 会立即拉取工具和资源列表；你可以直接在对话里让它读取 profile、Memory、项目或技能。</li>
            </ol>

            <p className="setup-note">
              Cursor Desktop 当前已验证 Remote MCP + OAuth 可用，真实请求形态是 dynamic registration + <code>client_secret_post</code> token exchange。
            </p>
          </>
        ) : (
          <>
            <h4 className="setup-platform-title">Windsurf Desktop</h4>
            <p className="setup-note setup-note-first">
              Windsurf 当前通过 <code>~/.codeium/windsurf/mcp_config.json</code> 添加远程 MCP；保存配置后会自动弹出 OAuth 授权。
            </p>

            <SetupCodeBlock
              label="Remote MCP Server URL"
              content={mcpUrl}
              copied={copied}
              copyKey="webapp-windsurf-url"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label="~/.codeium/windsurf/mcp_config.json"
              content={JSON.stringify({
                mcpServers: {
                  agenthub: {
                    serverUrl: mcpUrl,
                  },
                },
              }, null, 2)}
              copied={copied}
              copyKey="webapp-windsurf-json"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>打开 <code>Windsurf Settings</code>，点击 <code>Cascade</code>。</li>
              <li>在 <code>MCP Servers</code> 区域点击 <code>Open MCP Marketplace</code>。</li>
              <li>进入 MCP Marketplace 后，点击右上角的 config 图标；Windsurf 会打开 <code>~/.codeium/windsurf/mcp_config.json</code>。</li>
              <li>把上面的 <code>agenthub</code> 配置写进去并保存。</li>
              <li>保存后会弹出授权提示框；点击 <code>Open</code>，浏览器会跳转到 Agent Hub 的登录与授权页。</li>
              <li>完成登录和批准后，回到 Windsurf 的 MCP Marketplace，可以看到 <code>agenthub</code> 出现在 <code>Installed MCPs</code> 中，并且状态为 <code>Enabled</code>。</li>
            </ol>

            <p className="setup-note">
              Windsurf Desktop 当前已验证 Remote MCP + OAuth 可用，真实请求形态是 dynamic registration + <code>client_secret_post</code> token exchange。
            </p>
          </>
        )}
      </div>
    </SetupSection>
  )
}
