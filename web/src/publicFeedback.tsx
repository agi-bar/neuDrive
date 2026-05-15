import { createContext, useContext, type ReactNode } from 'react'

const PublicFeedbackContext = createContext(false)

export function PublicFeedbackProvider({ children, enabled }: { children: ReactNode; enabled: boolean }) {
  return <PublicFeedbackContext.Provider value={enabled}>{children}</PublicFeedbackContext.Provider>
}

export function usePublicFeedbackEnabled() {
  return useContext(PublicFeedbackContext)
}
