import type { ReactNode } from 'react'
import type { FileNode } from '../api'
import { useI18n } from '../i18n'
import MaterialsTile, { type MaterialsTileSelectOptions } from './MaterialsTile'

type FileLikeNode = Pick<FileNode, 'path' | 'name' | 'is_dir' | 'kind'>

type FileMaterialsTileProps = {
  node: FileLikeNode
  subtitle?: ReactNode
  description?: ReactNode
  path?: ReactNode
  extraPills?: ReactNode
  actions?: ReactNode
  footerStart?: ReactNode
  footerEnd?: ReactNode
  selected?: boolean
  emphasized?: boolean
  className?: string
  menu?: ReactNode
  onSelect?: (options: MaterialsTileSelectOptions) => void
  onOpen?: () => void
}

function meaningfulKind(kind?: string) {
  const normalized = (kind || '').trim().toLowerCase()
  if (!normalized) return ''
  if (normalized === 'file' || normalized === 'folder' || normalized === 'dir' || normalized === 'directory') {
    return ''
  }
  return kind || ''
}

export default function FileMaterialsTile({
  node,
  subtitle,
  description,
  path,
  extraPills,
  actions,
  footerStart,
  footerEnd,
  selected = false,
  emphasized = false,
  className = '',
  menu,
  onSelect,
  onOpen,
}: FileMaterialsTileProps) {
  const { tx } = useI18n()
  const kind = meaningfulKind(node.kind)
  const hasPills = Boolean(kind) || Boolean(extraPills)
  const tileClassName = ['materials-tile-file', className].filter(Boolean).join(' ')

  return (
    <MaterialsTile
      iconClassName={node.is_dir ? 'icon-folder' : 'icon-file'}
      title={node.name}
      titleActionAriaLabel={tx(`打开 ${node.name}`, `Open ${node.name}`)}
      subtitle={subtitle}
      description={description}
      path={path}
      pills={hasPills ? (
        <>
          {kind ? <span className="materials-tile-pill">{kind}</span> : null}
          {extraPills}
        </>
      ) : undefined}
      actions={actions}
      footerStart={footerStart}
      footerEnd={footerEnd}
      selected={selected}
      emphasized={emphasized}
      className={tileClassName}
      menu={menu}
      onSelect={onSelect}
      onOpen={onOpen}
    />
  )
}
