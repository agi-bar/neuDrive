import { useSetup } from '../SetupPage'
import { SetupCodeBlock, SetupSection } from './SetupShared'

export default function SetupAdaptersPage() {
  const { baseUrl, cloudModeNeedsPublicUrl, copied, copyToClipboard } = useSetup()
  const callbackUrl = `${baseUrl}/api/adapters/feishu/<your-slug>/events`

  return (
    <SetupSection
      icon={<>&#128279;</>}
      title="Adapters"
      description="通过 webhook / bot / 事件回调接入飞书、钉钉、Slack 等工作区平台。"
      highlight
    >
      {cloudModeNeedsPublicUrl && (
        <div className="alert alert-warn">
          当前地址是 <code>{baseUrl}</code>。飞书回调必须使用可公开访问的 HTTPS 地址；如果你现在在本地开发，请先切到公网域名或隧道地址。
        </div>
      )}

      <h4 className="setup-platform-title">Feishu Bot Adapter</h4>
      <p className="setup-note setup-note-first">
        当前飞书先提供一版 Bot Webhook MVP：完成请求网址校验后，飞书发来的事件会进入 neuDrive 的结构化事件记录。
      </p>

      <SetupCodeBlock
        label="Feishu Callback URL"
        content={callbackUrl}
        copied={copied}
        copyKey="adapter-feishu-callback"
        onCopy={copyToClipboard}
      />

      <SetupCodeBlock
        label="Server Environment Variables"
        content={[
          'FEISHU_APP_ID=replace-with-your-app-id',
          'FEISHU_APP_SECRET=replace-with-your-app-secret',
          'FEISHU_VERIFICATION_TOKEN=replace-with-your-verification-token',
          'FEISHU_ENCRYPT_KEY=replace-with-your-encrypt-key',
        ].join('\n')}
        copied={copied}
        copyKey="adapter-feishu-env"
        onCopy={copyToClipboard}
      />

      <ol className="setup-steps">
        <li>在飞书开放平台创建一个自建应用，并启用 <code>机器人能力</code>。</li>
        <li>进入 <code>事件与回调</code>，订阅 <code>消息与群组 - 接收消息 v2.0</code>。</li>
        <li>订阅方式选择 <code>将事件发送至开发者服务器</code>。</li>
        <li>请求网址填写 <code>{callbackUrl}</code>，把其中的 <code>&lt;your-slug&gt;</code> 换成 neuDrive 用户 slug。</li>
        <li>把飞书应用里的 <code>App ID</code>、<code>App Secret</code>、<code>Verification Token</code> 配到服务端环境变量 <code>FEISHU_APP_ID</code>、<code>FEISHU_APP_SECRET</code>、<code>FEISHU_VERIFICATION_TOKEN</code>。</li>
        <li>推荐同时开启加密推送，并把 <code>Encrypt Key</code> 配到 <code>FEISHU_ENCRYPT_KEY</code>；neuDrive 已支持飞书要求的签名校验和事件解密。</li>
        <li>保存配置后，飞书会先发起 <code>challenge</code> 验证；验证通过后，后续消息事件就会被 neuDrive 接收。</li>
      </ol>

      <p className="setup-note">
        当前 MVP 会把飞书消息写入 neuDrive 的事件记录，并在配置了 <code>FEISHU_APP_ID</code> / <code>FEISHU_APP_SECRET</code> 后自动回一条确认消息。文本消息会提取正文；非文本消息会以原始内容的形式保存在结构化 payload 中。更深的对话桥接会在后续版本补上。
      </p>
    </SetupSection>
  )
}
