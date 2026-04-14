import { useState } from 'react'
import { useI18n } from '../../i18n'
import { useSetup } from '../SetupPage'
import { SetupCodeBlock, SetupScreenshotPlaceholder, SetupSection } from './SetupShared'

type WebAppTab = 'claude' | 'chatgpt' | 'cursor' | 'windsurf'

export default function SetupWebAppsPage() {
  const { tx } = useI18n()
  const { baseUrl, cloudModeNeedsPublicUrl, copied, copyToClipboard } = useSetup()
  const [platform, setPlatform] = useState<WebAppTab>('claude')
  const mcpUrl = `${baseUrl}/mcp`

  return (
    <SetupSection
      icon={<>&#127760;</>}
      title="Web / Desktop Apps"
      description={tx('在网页应用或桌面图形应用里，把 neuDrive 添加成远程 MCP Server。', 'Add neuDrive as a remote MCP server inside web apps or desktop GUI apps.')}
      highlight
    >
      {cloudModeNeedsPublicUrl && (
        <div className="alert alert-warn">
          {tx('当前地址是 ', 'Current address: ')}<code>{baseUrl}</code>{tx('。Web / Desktop Apps 需要一个可公开访问的 HTTPS MCP 地址；如果你现在在本地开发，请先切到公网域名或隧道地址。', '. Web / Desktop apps need a publicly reachable HTTPS MCP address. If you are developing locally, switch to a public domain or tunnel URL first.')}
        </div>
      )}

      <p className="setup-note setup-note-first">
        {tx('这一页面面向通过图形界面完成连接的场景，包括浏览器里的 Apps / Connectors，以及像 Cursor、Windsurf 这样的桌面应用。', 'This page covers GUI-based setup flows, including browser Apps / Connectors and desktop apps such as Cursor and Windsurf.')}
      </p>

      <div className="setup-tabs" role="tablist" aria-label={tx('Web / Desktop Apps 平台', 'Web / Desktop app platforms')}>
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
              {tx('登录 Claude 网页应用后，在 Connectors 里创建一个自定义 connector，再完成 neuDrive 的网页登录与授权。', 'After signing in to Claude web, create a custom connector in Connectors, then complete neuDrive sign-in and authorization.')}
            </p>

            <SetupCodeBlock
              label="Remote MCP Server URL"
              content={mcpUrl}
              copied={copied}
              copyKey="webapp-claude-url"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>{tx('登录 Claude 网页应用，进入 ', 'Sign in to Claude web, open ')}<code>Settings -&gt; Connectors</code>{tx('，点击 ', ', then click ')}<code>Go to Customize</code>{tx('。', '.')}</li>
              <li>{tx('在 Customize 页的 Connectors 区域点击 ', 'In the Connectors area on the Customize page, click ')}<code>+</code>{tx('，再点击 ', ', then click ')}<code>Add custom connector</code>{tx('。', '.')}</li>
              <li>{tx('名称可以自定义，例如 ', 'Choose any name you like, for example ')}<code>NeuDrive</code>{tx('；把 ', '. Set ')}<code>Remote MCP server URL</code>{tx(' 填写为 ', ' to ')}<code>{mcpUrl}</code>{tx('，然后点击 ', ', then click ')}<code>Add</code>{tx('。', '.')}</li>
              <li>{tx('回到 connector 列表后，打开刚创建的 ', 'Return to the connector list, open the newly created ')}<code>NeuDrive</code>{tx(' connector，点击 ', ' connector, then click ')}<code>Connect</code>{tx('。', '.')}</li>
              <li>{tx('浏览器会跳转到 neuDrive 的登录与授权页；登录后点击授权，完成后回到 Claude，会显示成功连接。', 'The browser opens neuDrive sign-in and authorization. After approval, return to Claude and it will show the connector as connected.')}</li>
              <li>{tx('可选：在 ', 'Optional: in ')}<code>NeuDrive</code>{tx(' configuration 的 ', ' configuration, set ')}<code>Tools Permissions</code>{tx(' 里选择 ', ' to ')}<code>Always allow</code>{tx('，减少每次工具调用前的确认。', ' to reduce confirmation prompts before each tool call.')}</li>
              <li>{tx('回到 Claude chat 后，就可以直接发起工具调用，例如“从 neuDrive 中读取我的 profile”。', 'Back in Claude chat, you can immediately call tools, for example asking it to read your profile from neuDrive.')}</li>
            </ol>

            <div className="setup-screenshot-grid">
              <SetupScreenshotPlaceholder
                title={tx('Claude Connectors 列表', 'Claude Connectors list')}
                caption={tx('占位图：Settings -> Connectors -> Go to Customize -> Add custom connector', 'Placeholder: Settings -> Connectors -> Go to Customize -> Add custom connector')}
              />
              <SetupScreenshotPlaceholder
                title={tx('neuDrive 授权完成', 'neuDrive authorization complete')}
                caption={tx('占位图：Claude 中的 NeuDrive connector 已显示 Connected', 'Placeholder: the NeuDrive connector shows Connected inside Claude')}
              />
            </div>

            <p className="setup-note">
              {tx('如果你使用的是团队版或企业版 Claude，Connectors 的入口位置可能由管理员策略决定；看不到自定义 connector 入口时，请先确认当前账号支持 Remote MCP Custom Connectors。', 'If you use Claude Team or Enterprise, the Connectors entry may be controlled by admin policy. If you cannot find custom connectors, confirm that the account supports Remote MCP Custom Connectors first.')}
            </p>
          </>
        ) : platform === 'chatgpt' ? (
          <>
            <h4 className="setup-platform-title">ChatGPT Apps</h4>
            <p className="setup-note setup-note-first">
              {tx('登录 ChatGPT 后，在 Apps 设置里创建一个指向 neuDrive 的 MCP app，再按提示完成连接。', 'After signing in to ChatGPT, create an MCP app pointing to neuDrive from Apps settings, then finish the connection flow.')}
            </p>

            <div className="alert alert-warn">
              {tx('ChatGPT 的 Apps / MCP 入口取决于你的账号计划和灰度范围。如果设置里看不到 ', 'ChatGPT Apps / MCP availability depends on your plan and rollout status. If you do not see ')}<code>Apps</code>{tx('，通常意味着当前账号还没有这个入口。', ' in Settings, this account usually does not have access yet.')}
            </div>

            <SetupCodeBlock
              label="MCP Server URL"
              content={mcpUrl}
              copied={copied}
              copyKey="webapp-chatgpt-url"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>{tx('登录 ChatGPT，进入 ', 'Sign in to ChatGPT and open ')}<code>Settings -&gt; Apps</code>{tx('。', '.')}</li>
              <li>{tx('在 ', 'In the ')}<code>Advanced settings</code>{tx(' 区域点击 ', ' section, click ')}<code>Create app</code>{tx('。', '.')}</li>
              <li>{tx('把 ', 'Set ')}<code>MCP Server URL</code>{tx(' 填写为 ', ' to ')}<code>{mcpUrl}</code>{tx('，然后点击 ', ', then click ')}<code>Create</code>{tx('。', '.')}</li>
              <li>{tx('如果随后出现 ', 'If you then see prompts such as ')}<code>Connect</code>{tx('、', ', ')}<code>Sign in</code>{tx(' 或授权提示，按提示跳转到 neuDrive 登录并完成授权。', ', or authorization, follow them to neuDrive sign-in and finish approval.')}</li>
              <li>{tx('返回 ChatGPT 后，确认这个 app 已处于可用状态，再回到对话里使用对应工具。', 'Back in ChatGPT, confirm the app is available, then return to the conversation and use its tools.')}</li>
            </ol>

            <div className="setup-screenshot-grid">
              <SetupScreenshotPlaceholder
                title={tx('ChatGPT Apps 设置', 'ChatGPT Apps settings')}
                caption={tx('占位图：Settings -> Apps -> Advanced settings -> Create app', 'Placeholder: Settings -> Apps -> Advanced settings -> Create app')}
              />
              <SetupScreenshotPlaceholder
                title={tx('ChatGPT App 已创建', 'ChatGPT app created')}
                caption={tx('占位图：新建的 NeuDrive app 已出现在 Apps 列表中', 'Placeholder: the new NeuDrive app appears in the Apps list')}
              />
            </div>

            <p className="setup-note">
              {tx('创建完成后，你可以回到 ChatGPT 对话中直接要求它使用 neuDrive，例如“从 neuDrive 中读取我的 profile”。', 'After creation, you can return to ChatGPT and directly ask it to use neuDrive, for example to read your profile from neuDrive.')}
            </p>
          </>
        ) : platform === 'cursor' ? (
          <>
            <h4 className="setup-platform-title">Cursor Desktop</h4>
            <p className="setup-note setup-note-first">
              {tx('在 Cursor Desktop 里添加一个自定义 Remote MCP Server，然后通过浏览器 OAuth 完成授权。', 'Add a custom Remote MCP Server in Cursor Desktop, then finish authorization through browser OAuth.')}
            </p>

            <SetupCodeBlock
              label="Remote MCP Server URL"
              content={mcpUrl}
              copied={copied}
              copyKey="webapp-cursor-url"
              onCopy={copyToClipboard}
            />

            <SetupCodeBlock
              label={tx('可选：~/.cursor/mcp.json', 'Optional: ~/.cursor/mcp.json')}
              content={JSON.stringify({
                mcpServers: {
                  neudrive: {
                    url: mcpUrl,
                  },
                },
              }, null, 2)}
              copied={copied}
              copyKey="webapp-cursor-json"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>{tx('打开 Cursor，进入 ', 'Open Cursor, go to ')}<code>Settings -&gt; Tools &amp; MCPs</code>{tx('，点击 ', ', then click ')}<code>Add Custom MCP</code>{tx('。', '.')}</li>
              <li>{tx('如果界面要求填写 URL，就把 ', 'If the UI asks for a URL, set ')}<code>Remote MCP Server URL</code>{tx(' 设为 ', ' to ')}<code>{mcpUrl}</code>{tx('；如果要求粘贴配置，也可以直接使用上面的 ', '. If it asks for pasted config, you can use the ')}<code>~/.cursor/mcp.json</code>{tx(' 片段。', ' snippet above instead.')}</li>
              <li>{tx('保存后点击 ', 'After saving, click ')}<code>Connect</code>{tx(' 或 ', ' or ')}<code>Authenticate</code>{tx('；Cursor 会自动发现 neuDrive 的 OAuth metadata。', '. Cursor will automatically discover neuDrive OAuth metadata.')}</li>
              <li>{tx('浏览器会跳转到 neuDrive 的登录与授权页；完成登录和批准后，Cursor 会回到已连接状态。', 'The browser opens neuDrive sign-in and authorization. After approval, Cursor returns to a connected state.')}</li>
              <li>{tx('接通后，Cursor 会立即拉取工具和资源列表；你可以直接在对话里让它读取 profile、Memory、项目或技能。', 'Once connected, Cursor fetches tools and resources immediately. You can ask it in chat to read profile, Memory, projects, or skills.')}</li>
            </ol>

            <p className="setup-note">
              {tx('Cursor Desktop 当前已验证 Remote MCP + OAuth 可用，真实请求形态是 dynamic registration + ', 'Cursor Desktop has been verified with Remote MCP + OAuth. The request pattern is dynamic registration + ')}<code>client_secret_post</code>{tx(' token exchange。', ' token exchange.')}
            </p>
          </>
        ) : (
          <>
            <h4 className="setup-platform-title">Windsurf Desktop</h4>
            <p className="setup-note setup-note-first">
              {tx('Windsurf 当前通过 ', 'Windsurf currently adds remote MCP through ')}<code>~/.codeium/windsurf/mcp_config.json</code>{tx(' 添加远程 MCP；保存配置后会自动弹出 OAuth 授权。', '. Saving the config opens OAuth authorization automatically.')}
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
                  neudrive: {
                    serverUrl: mcpUrl,
                  },
                },
              }, null, 2)}
              copied={copied}
              copyKey="webapp-windsurf-json"
              onCopy={copyToClipboard}
            />

            <ol className="setup-steps">
              <li>{tx('打开 ', 'Open ')}<code>Windsurf Settings</code>{tx('，点击 ', ', then click ')}<code>Cascade</code>{tx('。', '.')}</li>
              <li>{tx('在 ', 'In the ')}<code>MCP Servers</code>{tx(' 区域点击 ', ' section, click ')}<code>Open MCP Marketplace</code>{tx('。', '.')}</li>
              <li>{tx('进入 MCP Marketplace 后，点击右上角的 config 图标；Windsurf 会打开 ', 'After entering MCP Marketplace, click the config icon in the upper right. Windsurf opens ')}<code>~/.codeium/windsurf/mcp_config.json</code>{tx('。', '.')}</li>
              <li>{tx('把上面的 ', 'Paste the ')}<code>neudrive</code>{tx(' 配置写进去并保存。', ' config above into it and save.')}</li>
              <li>{tx('保存后会弹出授权提示框；点击 ', 'Saving opens an authorization prompt. Click ')}<code>Open</code>{tx('，浏览器会跳转到 neuDrive 的登录与授权页。', ' and the browser will open neuDrive sign-in and authorization.')}</li>
              <li>{tx('完成登录和批准后，回到 Windsurf 的 MCP Marketplace，可以看到 ', 'After login and approval, return to Windsurf MCP Marketplace and you will see ')}<code>neudrive</code>{tx(' 出现在 ', ' under ')}<code>Installed MCPs</code>{tx(' 中，并且状态为 ', ' with status ')}<code>Enabled</code>{tx('。', '.')}</li>
            </ol>

            <p className="setup-note">
              {tx('Windsurf Desktop 当前已验证 Remote MCP + OAuth 可用，真实请求形态是 dynamic registration + ', 'Windsurf Desktop has been verified with Remote MCP + OAuth. The request pattern is dynamic registration + ')}<code>client_secret_post</code>{tx(' token exchange。', ' token exchange.')}
            </p>
          </>
        )}
      </div>
    </SetupSection>
  )
}
