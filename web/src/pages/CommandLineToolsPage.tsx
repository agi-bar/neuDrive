import { useState } from 'react'
import { useI18n } from '../i18n'

type CopyKey = 'install' | 'login' | 'daily' | 'import' | 'backup'

const installCommand = `git clone https://github.com/agi-bar/neuDrive.git
cd neuDrive
./tools/install-neudrive.sh`

const loginCommand = `neudrive login`

const dailyCommands = `neudrive status
neudrive platform ls
neudrive connect claude
neudrive browse`

const importCommands = `neudrive import claude --dry-run
neudrive import codex --dry-run
neudrive import skill ./demo-skill
neudrive import memory ./notes`

const backupCommands = `neudrive git init --output ./neudrive-export/git-mirror
neudrive git pull
neudrive sync pull --format archive --output neudrive-backup.ndrvz`

function CommandBlock({
  title,
  description,
  command,
  copyKey,
  copied,
  onCopy,
}: {
  title: string
  description: string
  command: string
  copyKey: CopyKey
  copied: CopyKey | ''
  onCopy: (key: CopyKey, command: string) => void
}) {
  const { tx } = useI18n()
  return (
    <article className="cli-command-item">
      <div className="cli-command-item-head">
        <div>
          <h3 className="card-title">{title}</h3>
          <p className="materials-section-copy">{description}</p>
        </div>
        <button className="btn btn-sm" type="button" onClick={() => onCopy(copyKey, command)}>
          {copied === copyKey ? tx('已复制', 'Copied') : tx('复制', 'Copy')}
        </button>
      </div>
      <pre className="cli-command-code"><code>{command}</code></pre>
    </article>
  )
}

export default function CommandLineToolsPage() {
  const { tx } = useI18n()
  const [copied, setCopied] = useState<CopyKey | ''>('')

  const copyCommand = async (key: CopyKey, command: string) => {
    try {
      await navigator.clipboard?.writeText(command)
      setCopied(key)
      window.setTimeout(() => setCopied((current) => current === key ? '' : current), 1500)
    } catch {
      setCopied('')
    }
  }

  return (
    <div className="page materials-page cli-tools-page">
      <section className="materials-hero cli-tools-hero">
        <div className="materials-hero-copy">
          <div className="materials-kicker">neuDrive CLI</div>
          <h2 className="materials-title">{tx('命令行工具', 'Command Line Tools')}</h2>
          <p className="materials-subtitle">
            {tx(
              '使用 neudrive 在终端里登录官方云服务、检查本地运行状态、连接 AI 工具、导入本地数据，并把 neuDrive 内容备份出来。',
              'Use neudrive from your terminal to sign in to the hosted service, check local status, connect AI tools, import local data, and back up neuDrive content.',
            )}
          </p>
        </div>
        <div className="materials-actions">
          <a className="btn" href="https://github.com/agi-bar/neuDrive/blob/main/docs/cli.md" target="_blank" rel="noreferrer">
            {tx('打开 CLI 手册', 'Open CLI manual')}
          </a>
        </div>
      </section>

      <section className="materials-panel cli-command-panel">
        <div className="cli-command-panel-head">
          <div>
            <h3 className="card-title">{tx('常用命令', 'Common commands')}</h3>
            <p className="materials-section-copy">
              {tx('从安装、登录到导入和备份，按需要复制对应命令。', 'Copy the command you need, from install and login to import and backup.')}
            </p>
          </div>
        </div>
        <div className="cli-command-list">
        <CommandBlock
          title={tx('安装 neuDrive CLI', 'Install neuDrive CLI')}
          description={tx('从当前开源仓库安装 CLI。安装后直接使用 neu 命令。', 'Install the CLI from the open-source repository. After install, use the neu command directly.')}
          command={installCommand}
          copyKey="install"
          copied={copied}
          onCopy={copyCommand}
        />
        <CommandBlock
          title={tx('登录官方云服务', 'Sign in to hosted neuDrive')}
          description={tx('运行这条命令即可登录 neuDrive 官网账号。', 'Run this command to sign in to your neuDrive website account.')}
          command={loginCommand}
          copyKey="login"
          copied={copied}
          onCopy={copyCommand}
        />
        <CommandBlock
          title={tx('日常检查和连接', 'Daily checks and connection')}
          description={tx('检查本地运行状态，查看可连接的平台，并快速配置 Claude 等工具。', 'Check local status, list available platforms, and quickly configure tools such as Claude.')}
          command={dailyCommands}
          copyKey="daily"
          copied={copied}
          onCopy={copyCommand}
        />
        <CommandBlock
          title={tx('导入本地数据', 'Import local data')}
          description={tx('在本地模式下扫描 Claude、Codex、skills、memory 等内容；正式导入前推荐先 dry-run。', 'In local mode, scan Claude, Codex, skills, memory, and related content. Run dry-run before writing data.')}
          command={importCommands}
          copyKey="import"
          copied={copied}
          onCopy={copyCommand}
        />
        <CommandBlock
          title={tx('备份和同步', 'Backup and sync')}
          description={tx('初始化 GitHub 备份、刷新 Git mirror，或拉取 archive 备份包。', 'Initialize GitHub backup, refresh the Git mirror, or pull an archive backup bundle.')}
          command={backupCommands}
          copyKey="backup"
          copied={copied}
          onCopy={copyCommand}
        />
        </div>
      </section>
    </div>
  )
}
