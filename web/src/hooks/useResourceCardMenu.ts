import { useCallback, useEffect, useState } from 'react'

export default function useResourceCardMenu() {
  const [activeMenuId, setActiveMenuId] = useState<string | null>(null)

  const closeMenu = useCallback(() => {
    setActiveMenuId(null)
  }, [])

  const toggleMenu = useCallback((id: string) => {
    setActiveMenuId((value) => value === id ? null : id)
  }, [])

  const isMenuOpen = useCallback((id: string) => activeMenuId === id, [activeMenuId])

  useEffect(() => {
    if (!activeMenuId) return

    const onPointerDown = (event: MouseEvent) => {
      const target = event.target as HTMLElement | null
      if (target?.closest('[data-resource-menu-root]')) return
      setActiveMenuId(null)
    }

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      setActiveMenuId(null)
    }

    window.addEventListener('mousedown', onPointerDown)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('mousedown', onPointerDown)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [activeMenuId])

  return {
    activeMenuId,
    closeMenu,
    isMenuOpen,
    setActiveMenuId,
    toggleMenu,
  }
}
