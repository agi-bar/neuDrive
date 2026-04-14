/**
 * neuDrive - Background Service Worker
 * Manages API connection, token storage, profile caching,
 * and message handling for content scripts and popup.
 */

importScripts('lib/neudrive-client.js');

const client = new NeuDriveClient();

// Initialize client on startup
client.init().then(configured => {
  console.log('[NeuDrive] Background initialized, configured:', configured);
});

/**
 * Message handler for popup and content scripts.
 * Protocol: { action: string, payload?: any }
 * Response: { ok: boolean, data?: any, error?: string }
 */
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  handleMessage(message, sender)
    .then(result => sendResponse({ ok: true, data: result }))
    .catch(err => sendResponse({ ok: false, error: err.message }));
  // Return true to indicate async response
  return true;
});

async function handleMessage(message, sender) {
  const { action, payload } = message;

  switch (action) {
    // --- Connection management ---
    case 'configure': {
      const { hubUrl, token } = payload;
      await client.configure(hubUrl, token);
      // Validate by fetching profile
      const profile = await client.getProfile(true);
      return profile;
    }

    case 'disconnect': {
      await client.disconnect();
      return null;
    }

    case 'getStatus': {
      await client.init();
      const configured = client.isConfigured();
      let profile = null;
      if (configured) {
        try {
          profile = await client.getProfile();
        } catch (e) {
          return { configured, connected: false, error: e.message };
        }
      }
      return { configured, connected: !!profile, profile };
    }

    // --- Data fetching ---
    case 'getProfile': {
      const forceRefresh = payload?.forceRefresh || false;
      return client.getProfile(forceRefresh);
    }

    case 'listSkills': {
      return client.listSkills(payload || {});
    }

    case 'getProject': {
      return client.getProject(payload.projectId);
    }

    case 'listProjects': {
      return client.listProjects();
    }

    case 'searchMemory': {
      return client.searchMemory(payload.query, payload.params || {});
    }

    case 'getPreferences': {
      return client.getPreferences();
    }

    // --- Context building ---
    case 'buildContext': {
      const { type, data } = payload;
      return client.buildContextBlock(type, data);
    }

    // --- Settings ---
    case 'getSettings': {
      const result = await chrome.storage.local.get(['settings']);
      return result.settings || {
        autoInject: false,
        platforms: {
          'claude.ai': true,
          'chat.openai.com': true,
          'gemini.google.com': true,
          'kimi.moonshot.cn': true,
        },
      };
    }

    case 'saveSettings': {
      await chrome.storage.local.set({ settings: payload });
      return payload;
    }

    default:
      throw new Error(`Unknown action: ${action}`);
  }
}

// Listen for tab updates to notify content scripts about navigation
chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
  if (changeInfo.status === 'complete' && tab.url) {
    const supportedHosts = ['claude.ai', 'chat.openai.com', 'gemini.google.com', 'kimi.moonshot.cn'];
    try {
      const url = new URL(tab.url);
      if (supportedHosts.includes(url.hostname)) {
        chrome.tabs.sendMessage(tabId, { action: 'tabUpdated', payload: { url: tab.url } }).catch(() => {
          // Content script may not be ready yet, that's fine
        });
      }
    } catch (e) {
      // Invalid URL, ignore
    }
  }
});
