"""OAuth / authentication helper for third-party apps integrating with neuDrive."""

from __future__ import annotations

import secrets
from typing import Any, Optional
from urllib.parse import urlencode

import httpx


class NeuDriveAuth:
    """OAuth 2.0 helper for applications that need to authenticate users
    against an neuDrive instance.

    Typical flow::

        auth = NeuDriveAuth(
            base_url="https://neudrive.ai",
            client_id="my-app",
            client_secret="secret",
        )

        # 1. Redirect user to the authorization URL.
        url = auth.get_authorization_url(
            redirect_uri="https://myapp.example.com/callback",
            scopes=["read:profile", "read:inbox"],
        )

        # 2. After the user authorises, exchange the code for tokens.
        tokens = auth.exchange_code(code="abc123", redirect_uri="https://myapp.example.com/callback")
        access_token = tokens["access_token"]

        # 3. Fetch user information.
        user = auth.get_user_info(access_token)
    """

    def __init__(
        self,
        base_url: str,
        client_id: str,
        client_secret: str,
        timeout: float = 30.0,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.client_id = client_id
        self.client_secret = client_secret
        self._client = httpx.Client(
            base_url=self.base_url,
            timeout=timeout,
        )

    # ------------------------------------------------------------------
    # OAuth flow
    # ------------------------------------------------------------------

    def get_authorization_url(
        self,
        redirect_uri: str,
        scopes: Optional[list[str]] = None,
        state: Optional[str] = None,
    ) -> str:
        """Build the URL to which the user should be redirected to begin
        the OAuth authorization-code flow.

        If *state* is not provided a random value is generated (recommended
        for CSRF protection).
        """
        params: dict[str, str] = {
            "client_id": self.client_id,
            "redirect_uri": redirect_uri,
            "response_type": "code",
            "state": state or secrets.token_urlsafe(32),
        }
        if scopes:
            params["scope"] = " ".join(scopes)
        return f"{self.base_url}/oauth/authorize?{urlencode(params)}"

    def exchange_code(self, code: str, redirect_uri: str) -> dict[str, Any]:
        """Exchange an authorization *code* for an access token (and optional
        refresh token).

        Returns the full token response dict, typically containing
        ``access_token``, ``token_type``, ``expires_in``, and
        ``refresh_token``.
        """
        resp = self._client.post(
            "/oauth/token",
            json={
                "grant_type": "authorization_code",
                "code": code,
                "redirect_uri": redirect_uri,
                "client_id": self.client_id,
                "client_secret": self.client_secret,
            },
        )
        resp.raise_for_status()
        return resp.json()

    def refresh_token(self, refresh_token: str) -> dict[str, Any]:
        """Use a *refresh_token* to obtain a new access token."""
        raise NotImplementedError(
            "neuDrive OAuth currently supports authorization_code exchange only."
        )

    def get_user_info(self, access_token: str) -> dict[str, Any]:
        """Retrieve the authenticated user's profile from neuDrive.

        Returns a dict with keys like ``slug``, ``display_name``,
        ``timezone``, and ``language``.
        """
        resp = self._client.get(
            "/oauth/userinfo",
            headers={"Authorization": f"Bearer {access_token}"},
        )
        resp.raise_for_status()
        return resp.json()

    # ------------------------------------------------------------------
    # GitHub OAuth shortcut
    # ------------------------------------------------------------------

    def github_callback(self, code: str) -> dict[str, Any]:
        """Complete a GitHub OAuth flow by forwarding the GitHub *code* to
        the neuDrive backend which handles the token exchange.

        Returns the Hub session tokens.
        """
        resp = self._client.post(
            "/api/auth/github/callback",
            json={"code": code},
        )
        resp.raise_for_status()
        return resp.json()

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    def close(self) -> None:
        """Close the underlying HTTP client."""
        self._client.close()

    def __enter__(self) -> "NeuDriveAuth":
        return self

    def __exit__(self, *args: Any) -> None:
        self._client.close()
