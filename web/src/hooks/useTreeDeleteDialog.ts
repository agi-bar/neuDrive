import { useCallback, useState } from 'react'
import { api } from '../api'

export type TreeDeleteDialogState = {
  paths: string[]
  nonEmptyDirectories: string[]
}

type UseTreeDeleteDialogOptions = {
  tx: (zh: string, en: string) => string
  onDeleted?: () => Promise<void> | void
}

function normalizeDeletePath(path: string) {
  const value = (path || '').trim()
  if (!value || value === '/') return '/'
  return `/${value.replace(/^\/+/, '').replace(/\/+$/, '')}`
}

function isAncestorPath(ancestor: string, candidate: string) {
  if (ancestor === '/') return true
  return candidate === ancestor || candidate.startsWith(`${ancestor}/`)
}

function collapseDeletePaths(paths: string[]) {
  const normalized = Array.from(new Set(paths.map(normalizeDeletePath).filter(Boolean)))
  return normalized.filter((path, index) => {
    return !normalized.some((candidate, candidateIndex) => {
      if (candidateIndex === index) return false
      if (candidate.length > path.length) return false
      return isAncestorPath(candidate, path)
    })
  })
}

async function inspectTreeDeleteTargets(paths: string[]): Promise<TreeDeleteDialogState> {
  const collapsedPaths = collapseDeletePaths(paths)
  const nonEmptyDirectories = (await Promise.all(collapsedPaths.map(async (pathValue) => {
    try {
      const node = await api.getTree(pathValue)
      const isDirectory = Boolean(node.is_dir || node.children)
      return isDirectory && (node.children?.length ?? 0) > 0 ? pathValue : null
    } catch {
      return pathValue
    }
  }))).filter((pathValue): pathValue is string => Boolean(pathValue))

  return {
    paths: collapsedPaths,
    nonEmptyDirectories,
  }
}

export default function useTreeDeleteDialog({ tx, onDeleted }: UseTreeDeleteDialogOptions) {
  const [dialog, setDialog] = useState<TreeDeleteDialogState | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const closeDialog = useCallback(() => {
    if (submitting) return
    setDialog(null)
  }, [submitting])

  const requestDelete = useCallback(async (paths: string[]) => {
    if (paths.length === 0 || submitting) return
    setDialog(await inspectTreeDeleteTargets(paths))
  }, [submitting])

  const confirmDelete = useCallback(async () => {
    if (!dialog || submitting) return
    setSubmitting(true)
    for (const pathValue of dialog.paths) {
      try {
        await api.deleteTree(pathValue)
      } catch (err: any) {
        alert(tx(`删除失败：${pathValue}\n${err.message || err}`, `Failed to delete: ${pathValue}\n${err.message || err}`))
      }
    }
    setSubmitting(false)
    setDialog(null)
    await onDeleted?.()
  }, [dialog, onDeleted, submitting, tx])

  return {
    closeDialog,
    confirmDelete,
    dialog,
    requestDelete,
    submitting,
  }
}
