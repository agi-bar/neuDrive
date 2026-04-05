import { useNavigate } from 'react-router-dom'
import { useSetup } from '../SetupPage'
import { SetupCodeBlock, SetupSection } from './SetupShared'

export default function SetupGptActionsPage() {
  const navigate = useNavigate()
  const { baseUrl, copied, copyToClipboard, gptTokenText, newToken } = useSetup()

  return (
    <SetupSection
      icon={<>&#129302;</>}
      title="ChatGPT GPT Actions"
      description="在自定义 GPT 中通过 Actions 连接 Agent Hub。"
      badge="GPT"
    >
      <SetupCodeBlock
        label="1. OpenAPI Schema URL（粘贴到 Actions 配置中）"
        content={`${baseUrl}/gpt/openapi.json`}
        copied={copied}
        copyKey="gpt-schema"
        onCopy={copyToClipboard}
      />

      <SetupCodeBlock
        label="2. Authentication 配置"
        content={`Type: API Key\nAuth Type: Bearer\nToken: ${gptTokenText}`}
        action={newToken ? (
          <button
            className="copy-btn"
            onClick={() => copyToClipboard(newToken, 'gpt-token')}
          >
            {copied === 'gpt-token' ? '已复制 Token' : '复制 Token'}
          </button>
        ) : (
          <button
            className="copy-btn"
            onClick={() => navigate('/setup/tokens#token-creator')}
          >
            前往 Token 管理
          </button>
        )}
      />

      <p className="setup-note">
        本页不会自动为 GPT Actions 生成 token。需要新的 Bearer Token 时，请前往“Token 管理”手动创建，再把它填到 Actions 的认证配置里。
      </p>
    </SetupSection>
  )
}
