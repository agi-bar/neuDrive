const PENDING_FEEDBACK_LAUNCH_KEY = 'neudrive.pendingFeedbackLaunch'

export const FEEDBACK_LOGIN_REDIRECT = '/?open_feedback=1'

export function feedbackLoginHref() {
  const params = new URLSearchParams()
  params.set('intent', 'feedback')
  params.set('redirect', FEEDBACK_LOGIN_REDIRECT)
  return `/login?${params.toString()}`
}

export function rememberPendingFeedbackLaunch() {
  try {
    window.sessionStorage.setItem(PENDING_FEEDBACK_LAUNCH_KEY, '1')
  } catch {
    // Session storage is best-effort; the redirect query is still the primary signal.
  }
}

export function consumePendingFeedbackLaunch() {
  try {
    const pending = window.sessionStorage.getItem(PENDING_FEEDBACK_LAUNCH_KEY) === '1'
    if (pending) window.sessionStorage.removeItem(PENDING_FEEDBACK_LAUNCH_KEY)
    return pending
  } catch {
    return false
  }
}

export function isFeedbackLoginIntent(search: string) {
  const params = new URLSearchParams(search)
  return params.get('intent') === 'feedback' || isFeedbackRedirectTarget(params.get('redirect'))
}

export function isFeedbackRedirectTarget(raw: string | null) {
  const redirect = (raw || '').trim()
  if (!redirect) return false
  try {
    const target = redirect.startsWith('/') ? new URL(redirect, window.location.origin) : new URL(redirect)
    if (target.origin !== window.location.origin) return false
    return target.searchParams.get('open_feedback') === '1'
  } catch {
    return false
  }
}
