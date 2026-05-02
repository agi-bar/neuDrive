import { useNavigate } from 'react-router-dom'
import { useI18n } from '../../i18n'
import { useSetup } from '../SetupPage'
import { SetupCodeBlock, SetupSection } from './SetupShared'

export default function SetupGptActionsPage() {
  const { tx } = useI18n()
  const navigate = useNavigate()
  const { baseUrl, copied, copyToClipboard, gptTokenText, newToken } = useSetup()

  return (
    <SetupSection
      icon={<>&#129302;</>}
      title="ChatGPT GPT Actions"
      description={tx('在自定义 GPT 中通过 Actions 连接 neuDrive。', 'Connect neuDrive to a custom GPT through Actions.')}
      badge="GPT"
    >
      <SetupCodeBlock
        label={tx('1. OpenAPI Schema URL（粘贴到 Actions 配置中）', '1. OpenAPI schema URL (paste into Actions config)')}
        content={`${baseUrl}/gpt/openapi.json`}
        copied={copied}
        copyKey="gpt-schema"
        onCopy={copyToClipboard}
      />

      <SetupCodeBlock
        label={tx('2. Authentication 配置', '2. Authentication settings')}
        content={`Type: API Key\nAuth Type: Bearer\nToken: ${gptTokenText}`}
        action={newToken ? (
          <button
            className="copy-btn"
            onClick={() => copyToClipboard(newToken, 'gpt-token')}
          >
            {copied === 'gpt-token' ? tx('已复制 Token', 'Token copied') : tx('复制 Token', 'Copy token')}
          </button>
        ) : (
          <button
            className="copy-btn"
            onClick={() => navigate('/settings/developer-access')}
          >
            {tx('前往 Developer Access', 'Open Developer Access')}
          </button>
        )}
      />

      <p className="setup-note">
        {tx('本页不会自动为 GPT Actions 生成 token。需要新的 Bearer Token 时，请前往 Developer Access 创建，再把它填到 Actions 的认证配置里。', 'This page does not generate a token for GPT Actions automatically. If you need a new Bearer token, create one in Developer Access and paste it into the Actions auth settings.')}
      </p>
    </SetupSection>
  )
}
