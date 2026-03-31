import type { User, AuthTokenResponse } from './types'

export interface AgentHubAuthConfig {
  /** Base URL of the Agent Hub instance */
  baseURL: string
  /** OAuth client ID */
  clientId: string
  /** OAuth client secret */
  clientSecret: string
}

/**
 * OAuth helper for third-party applications integrating with Agent Hub.
 *
 * @example
 * ```ts
 * const auth = new AgentHubAuth({
 *   baseURL: 'https://hub.example.com',
 *   clientId: 'your-client-id',
 *   clientSecret: 'your-client-secret',
 * })
 *
 * // Step 1: redirect user
 * const url = auth.getAuthorizationURL('https://yourapp.com/callback', ['read:profile'])
 *
 * // Step 2: exchange code after callback
 * const { access_token, user } = await auth.exchangeCode(code, 'https://yourapp.com/callback')
 * ```
 */
export class AgentHubAuth {
  private readonly baseURL: string
  private readonly clientId: string
  private readonly clientSecret: string

  constructor(config: AgentHubAuthConfig) {
    if (!config.baseURL) throw new Error('AgentHubAuth: baseURL is required')
    if (!config.clientId) throw new Error('AgentHubAuth: clientId is required')
    if (!config.clientSecret)
      throw new Error('AgentHubAuth: clientSecret is required')
    this.baseURL = config.baseURL.replace(/\/+$/, '')
    this.clientId = config.clientId
    this.clientSecret = config.clientSecret
  }

  /**
   * Build the authorization URL that the user should be redirected to.
   *
   * @param redirectURI - Where Agent Hub should redirect after the user authorises.
   * @param scopes      - Requested permission scopes (e.g. ["read:profile", "read:memory"]).
   * @returns A fully-qualified URL string.
   */
  getAuthorizationURL(redirectURI: string, scopes: string[]): string {
    const params = new URLSearchParams({
      response_type: 'code',
      client_id: this.clientId,
      redirect_uri: redirectURI,
      scope: scopes.join(' '),
    })
    return `${this.baseURL}/api/auth/authorize?${params.toString()}`
  }

  /**
   * Exchange an authorisation code for an access token and user info.
   *
   * @param code        - The authorisation code received at the redirect URI.
   * @param redirectURI - Must match the redirect URI used in getAuthorizationURL.
   */
  async exchangeCode(
    code: string,
    redirectURI: string,
  ): Promise<AuthTokenResponse> {
    const res = await fetch(`${this.baseURL}/api/auth/token`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        grant_type: 'authorization_code',
        code,
        redirect_uri: redirectURI,
        client_id: this.clientId,
        client_secret: this.clientSecret,
      }),
    })

    if (!res.ok) {
      let body: unknown
      try {
        body = await res.json()
      } catch {
        body = await res.text()
      }
      const msg =
        typeof body === 'object' && body !== null && 'error' in body
          ? (body as { error: string }).error
          : `HTTP ${res.status}`
      throw new Error(`AgentHubAuth: token exchange failed: ${msg}`)
    }

    return (await res.json()) as AuthTokenResponse
  }

  /**
   * Retrieve user information using an access token.
   *
   * @param accessToken - A valid access token (JWT or scoped token).
   */
  async getUserInfo(accessToken: string): Promise<User> {
    const res = await fetch(`${this.baseURL}/api/auth/me`, {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${accessToken}`,
        'Content-Type': 'application/json',
      },
    })

    if (!res.ok) {
      let body: unknown
      try {
        body = await res.json()
      } catch {
        body = await res.text()
      }
      const msg =
        typeof body === 'object' && body !== null && 'error' in body
          ? (body as { error: string }).error
          : `HTTP ${res.status}`
      throw new Error(`AgentHubAuth: getUserInfo failed: ${msg}`)
    }

    return (await res.json()) as User
  }
}
