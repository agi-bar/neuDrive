package auth

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

// ConsentPageData holds the data passed to the consent HTML template.
type ConsentPageData struct {
	AppName     string
	AppLogoURL  string
	Scopes      []string
	ClientID    string
	RedirectURI string
	Scope       string
	State       string
	Error       string
	ShowLogin   bool
}

// consentTemplate is the HTML template for the OAuth consent screen.
var consentTemplate = template.Must(template.New("consent").Funcs(template.FuncMap{
	"scopeLabel": scopeLabel,
}).Parse(consentHTML))

// RenderConsentPage renders the OAuth consent screen.
func RenderConsentPage(w http.ResponseWriter, data ConsentPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if data.Error != "" {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if err := consentTemplate.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("failed to render consent page: %v", err), http.StatusInternalServerError)
	}
}

// scopeLabel returns a human-readable label for a scope string.
func scopeLabel(scope string) string {
	labels := map[string]string{
		"read:profile":    "View your profile information",
		"write:profile":   "Update your profile information",
		"read:memory":     "Read your memory data",
		"write:memory":    "Write to your memory data",
		"read:vault":      "Read your vault secrets",
		"read:vault.auth": "Read authentication vault entries",
		"write:vault":     "Write to your vault",
		"read:skills":     "View your skills",
		"write:skills":    "Manage your skills",
		"read:devices":    "View your registered devices",
		"call:devices":    "Call your devices",
		"read:inbox":      "Read your inbox messages",
		"write:inbox":     "Send inbox messages",
		"read:projects":   "View your projects",
		"write:projects":  "Manage your projects",
		"read:tree":       "Read your file tree",
		"write:tree":      "Write to your file tree",
		"search":          "Search your data",
		"admin":           "Full administrative access",
	}
	if label, ok := labels[scope]; ok {
		return label
	}
	return scope
}

// splitScopes splits a space-separated scope string into a slice.
func SplitScopes(scopeStr string) []string {
	if scopeStr == "" {
		return nil
	}
	// Handle both space and + separated scopes
	scopeStr = strings.ReplaceAll(scopeStr, "+", " ")
	parts := strings.Fields(scopeStr)
	var scopes []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			scopes = append(scopes, p)
		}
	}
	return scopes
}

const consentHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Authorize Application - Agent Hub</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #f5f5f5;
            color: #333;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            padding: 20px;
        }
        .consent-card {
            background: white;
            border-radius: 12px;
            box-shadow: 0 2px 16px rgba(0,0,0,0.1);
            max-width: 480px;
            width: 100%;
            padding: 32px;
        }
        .header {
            text-align: center;
            margin-bottom: 24px;
        }
        .header h1 {
            font-size: 20px;
            font-weight: 600;
            margin-bottom: 4px;
        }
        .header p {
            color: #666;
            font-size: 14px;
        }
        .app-info {
            display: flex;
            align-items: center;
            gap: 16px;
            padding: 16px;
            background: #f8f9fa;
            border-radius: 8px;
            margin-bottom: 24px;
        }
        .app-logo {
            width: 48px;
            height: 48px;
            border-radius: 8px;
            background: #e9ecef;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 24px;
            flex-shrink: 0;
        }
        .app-logo img {
            width: 100%;
            height: 100%;
            border-radius: 8px;
            object-fit: cover;
        }
        .app-name {
            font-weight: 600;
            font-size: 16px;
        }
        .app-subtitle {
            color: #666;
            font-size: 13px;
        }
        .scopes-section {
            margin-bottom: 24px;
        }
        .scopes-section h2 {
            font-size: 14px;
            font-weight: 600;
            margin-bottom: 12px;
            color: #555;
        }
        .scope-item {
            display: flex;
            align-items: center;
            gap: 10px;
            padding: 10px 0;
            border-bottom: 1px solid #eee;
            font-size: 14px;
        }
        .scope-item:last-child {
            border-bottom: none;
        }
        .scope-icon {
            color: #4CAF50;
            font-size: 16px;
            flex-shrink: 0;
        }
        .actions {
            display: flex;
            gap: 12px;
        }
        .btn {
            flex: 1;
            padding: 12px 24px;
            border-radius: 8px;
            border: none;
            font-size: 14px;
            font-weight: 600;
            cursor: pointer;
            transition: background 0.2s;
            text-align: center;
        }
        .btn-approve {
            background: #4CAF50;
            color: white;
        }
        .btn-approve:hover {
            background: #43A047;
        }
        .btn-deny {
            background: #e9ecef;
            color: #333;
        }
        .btn-deny:hover {
            background: #dee2e6;
        }
        .error-box {
            background: #fff3f3;
            border: 1px solid #ffcdd2;
            border-radius: 8px;
            padding: 16px;
            color: #c62828;
            font-size: 14px;
            margin-bottom: 16px;
        }
    </style>
</head>
<body>
    <div class="consent-card">
        <div class="header">
            <h1>Agent Hub</h1>
            <p>An application is requesting access to your account</p>
        </div>

        {{if .Error}}
        <div class="error-box">{{.Error}}</div>
        {{else}}

        <div class="app-info">
            <div class="app-logo">
                {{if .AppLogoURL}}<img src="{{.AppLogoURL}}" alt="{{.AppName}}">{{else}}&#x1f916;{{end}}
            </div>
            <div>
                <div class="app-name">{{.AppName}}</div>
                <div class="app-subtitle">wants to access your Agent Hub account</div>
            </div>
        </div>

        {{if .Scopes}}
        <div class="scopes-section">
            <h2>This application will be able to:</h2>
            {{range .Scopes}}
            <div class="scope-item">
                <span class="scope-icon">&#10003;</span>
                <span>{{scopeLabel .}}</span>
            </div>
            {{end}}
        </div>
        {{end}}

        <form method="POST" action="/oauth/authorize">
            <input type="hidden" name="client_id" value="{{.ClientID}}">
            <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
            <input type="hidden" name="scope" value="{{.Scope}}">
            <input type="hidden" name="state" value="{{.State}}">
            <input type="hidden" name="action" value="approve">

            {{if .ShowLogin}}
            <div style="margin-bottom:16px;">
                <label style="display:block;margin-bottom:4px;font-size:14px;color:#555;">Email</label>
                <input type="email" name="email" placeholder="your@email.com" required
                    style="width:100%;padding:10px 12px;border:1px solid #ddd;border-radius:8px;font-size:14px;box-sizing:border-box;">
            </div>
            <div style="margin-bottom:16px;">
                <label style="display:block;margin-bottom:4px;font-size:14px;color:#555;">Password</label>
                <input type="password" name="password" placeholder="Password" required
                    style="width:100%;padding:10px 12px;border:1px solid #ddd;border-radius:8px;font-size:14px;box-sizing:border-box;">
            </div>
            {{end}}

            <div class="actions">
                <button type="button" class="btn btn-deny" style="flex:1;"
                    onclick="window.location='{{.RedirectURI}}?error=access_denied&state={{.State}}'">Deny</button>
                <button type="submit" class="btn btn-approve" style="flex:1;">
                    {{if .ShowLogin}}Login & Authorize{{else}}Authorize{{end}}
                </button>
            </div>
        </form>

        {{end}}
    </div>

    <script>
    (function() {
        // Hide consent card until we know what to show
        var card = document.querySelector('.consent-card');
        if (card) card.style.display = 'none';

        var token = localStorage.getItem('token');
        if (!token) {
            // Not logged in — redirect to login immediately (no flash)
            window.location.href = '/login?redirect=' + encodeURIComponent(window.location.href);
            return;
        }

        // Verify token then auto-authorize (no form shown)
        fetch('/api/auth/me', {
            headers: { 'Authorization': 'Bearer ' + token }
        }).then(function(resp) {
            if (resp.status !== 200) {
                localStorage.removeItem('token');
                window.location.href = '/login?redirect=' + encodeURIComponent(window.location.href);
                return null;
            }
            return resp.json();
        }).then(function(data) {
            if (!data) return;

            // Auto-submit authorize
            var form = document.querySelector('form');
            if (!form) return;

            var formData = new FormData(form);
            formData.set('action', 'approve');
            formData.delete('email');
            formData.delete('password');

            fetch('/oauth/authorize', {
                method: 'POST',
                headers: { 'Authorization': 'Bearer ' + token },
                body: new URLSearchParams(formData)
            }).then(function(resp) {
                if (resp.redirected) {
                    window.location.href = resp.url;
                } else {
                    // Fallback: show the form
                    if (card) card.style.display = '';
                }
            }).catch(function() {
                if (card) card.style.display = '';
            });
        }).catch(function() {
            if (card) card.style.display = '';
        });
    })();
    </script>
</body>
</html>
`
