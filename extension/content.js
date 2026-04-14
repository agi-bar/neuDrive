/**
 * neuDrive - Content Script
 * Injects floating Hub button and context panel into AI chat interfaces.
 */

(function () {
  'use strict';

  // Prevent double injection
  if (window.__agentHubInjected) return;
  window.__agentHubInjected = true;

  // --- Platform Detection ---

  const PLATFORMS = {
    'claude.ai': {
      name: 'Claude',
      inputSelector: 'div.ProseMirror[contenteditable="true"]',
      conversationSelector: '[data-testid="conversation-turn"]',
      newConversationUrl: /^https:\/\/claude\.ai\/?$/,
    },
    'chat.openai.com': {
      name: 'ChatGPT',
      inputSelector: '#prompt-textarea',
      conversationSelector: '[data-message-id]',
      newConversationUrl: /^https:\/\/chat\.openai\.com\/?$/,
    },
    'gemini.google.com': {
      name: 'Gemini',
      inputSelector: 'div.ql-editor[contenteditable="true"], rich-textarea .textarea',
      conversationSelector: 'message-content',
      newConversationUrl: /^https:\/\/gemini\.google\.com\/app\/?$/,
    },
    'kimi.moonshot.cn': {
      name: 'Kimi',
      inputSelector: 'div[contenteditable="true"].editor',
      conversationSelector: '.chat-message',
      newConversationUrl: /^https:\/\/kimi\.moonshot\.cn\/?$/,
    },
  };

  const hostname = window.location.hostname;
  const platform = PLATFORMS[hostname];

  if (!platform) {
    console.log('[NeuDrive] Unsupported platform:', hostname);
    return;
  }

  console.log(`[NeuDrive] Detected platform: ${platform.name}`);

  // --- State ---

  let panelVisible = false;
  let profileData = null;
  let isConnected = false;

  // --- Send message to background ---

  function sendMessage(action, payload) {
    return new Promise((resolve, reject) => {
      chrome.runtime.sendMessage({ action, payload }, response => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message));
          return;
        }
        if (!response) {
          reject(new Error('No response from background'));
          return;
        }
        if (response.ok) {
          resolve(response.data);
        } else {
          reject(new Error(response.error));
        }
      });
    });
  }

  // --- UI Creation ---

  function createFloatingButton() {
    const btn = document.createElement('div');
    btn.id = 'neudrive-fab';
    btn.innerHTML = `
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
        <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="2"/>
        <text x="12" y="16" text-anchor="middle" font-size="10" font-weight="bold" fill="currentColor">H</text>
      </svg>
    `;
    btn.title = 'neuDrive';
    btn.addEventListener('click', togglePanel);
    document.body.appendChild(btn);
    return btn;
  }

  function createPanel() {
    const panel = document.createElement('div');
    panel.id = 'neudrive-panel';
    panel.innerHTML = `
      <div class="neudrive-panel-header">
        <span class="neudrive-panel-title">neuDrive</span>
        <button class="neudrive-panel-close" title="关闭">&times;</button>
      </div>
      <div class="neudrive-panel-body">
        <div id="neudrive-status" class="neudrive-status">检查连接中...</div>
        <div id="neudrive-profile" class="neudrive-profile" style="display:none;"></div>
        <div id="neudrive-actions" class="neudrive-actions" style="display:none;">
          <button class="neudrive-btn" data-action="inject-preferences">
            <span class="neudrive-btn-icon">&#9881;</span>
            注入偏好
          </button>
          <button class="neudrive-btn" data-action="inject-project">
            <span class="neudrive-btn-icon">&#128193;</span>
            注入项目上下文
          </button>
          <button class="neudrive-btn" data-action="inject-skills">
            <span class="neudrive-btn-icon">&#9889;</span>
            注入技能
          </button>
        </div>
        <div id="neudrive-not-connected" style="display:none;">
          <p class="neudrive-hint">请先在扩展弹窗中配置 neuDrive 连接。</p>
        </div>
      </div>
    `;

    // Event listeners
    panel.querySelector('.neudrive-panel-close').addEventListener('click', togglePanel);
    panel.querySelectorAll('.neudrive-btn[data-action]').forEach(btn => {
      btn.addEventListener('click', () => handleInjectAction(btn.dataset.action));
    });

    document.body.appendChild(panel);
    return panel;
  }

  function togglePanel() {
    panelVisible = !panelVisible;
    const panel = document.getElementById('neudrive-panel');
    const fab = document.getElementById('neudrive-fab');
    if (panel) {
      panel.classList.toggle('neudrive-panel-visible', panelVisible);
    }
    if (fab) {
      fab.classList.toggle('neudrive-fab-active', panelVisible);
    }
    if (panelVisible) {
      refreshStatus();
    }
  }

  // --- Status & Profile ---

  async function refreshStatus() {
    const statusEl = document.getElementById('neudrive-status');
    const profileEl = document.getElementById('neudrive-profile');
    const actionsEl = document.getElementById('neudrive-actions');
    const notConnectedEl = document.getElementById('neudrive-not-connected');

    if (!statusEl) return;

    try {
      const status = await sendMessage('getStatus');
      isConnected = status.connected;
      profileData = status.profile;

      if (status.connected && status.profile) {
        const p = status.profile;
        statusEl.innerHTML = '<span class="neudrive-dot neudrive-dot-ok"></span> 已连接';
        profileEl.style.display = 'block';
        profileEl.innerHTML = `
          <div class="neudrive-profile-name">${escapeHtml(p.name || p.username || 'User')}</div>
          ${p.bio ? `<div class="neudrive-profile-bio">${escapeHtml(p.bio)}</div>` : ''}
        `;
        actionsEl.style.display = 'flex';
        notConnectedEl.style.display = 'none';
      } else if (status.configured && !status.connected) {
        statusEl.innerHTML = '<span class="neudrive-dot neudrive-dot-err"></span> 连接失败';
        profileEl.style.display = 'none';
        actionsEl.style.display = 'none';
        notConnectedEl.style.display = 'block';
        notConnectedEl.querySelector('.neudrive-hint').textContent = status.error || '无法连接到 neuDrive 服务器。';
      } else {
        statusEl.innerHTML = '<span class="neudrive-dot neudrive-dot-off"></span> 未配置';
        profileEl.style.display = 'none';
        actionsEl.style.display = 'none';
        notConnectedEl.style.display = 'block';
      }
    } catch (err) {
      statusEl.innerHTML = '<span class="neudrive-dot neudrive-dot-err"></span> 错误';
      console.error('[NeuDrive] Status check failed:', err);
    }
  }

  // --- Inject Actions ---

  async function handleInjectAction(action) {
    try {
      let contextText = '';

      switch (action) {
        case 'inject-preferences': {
          const prefs = await sendMessage('getPreferences');
          contextText = await sendMessage('buildContext', { type: 'preferences', data: prefs });
          break;
        }
        case 'inject-project': {
          const projects = await sendMessage('listProjects');
          if (projects && projects.length > 0) {
            // Inject the first / active project
            contextText = await sendMessage('buildContext', { type: 'project', data: projects[0] });
          } else {
            showToast('没有找到项目数据');
            return;
          }
          break;
        }
        case 'inject-skills': {
          const skills = await sendMessage('listSkills', { limit: 20 });
          const list = skills?.items || skills || [];
          if (list.length === 0) {
            showToast('没有找到技能数据');
            return;
          }
          contextText = await sendMessage('buildContext', { type: 'skills', data: list });
          break;
        }
        default:
          return;
      }

      if (contextText) {
        insertTextIntoChat(contextText);
        showToast('上下文已注入');
      }
    } catch (err) {
      console.error('[NeuDrive] Inject failed:', err);
      showToast('注入失败: ' + err.message);
    }
  }

  // --- Chat Input Interaction ---

  function findChatInput() {
    return document.querySelector(platform.inputSelector);
  }

  function insertTextIntoChat(text) {
    const input = findChatInput();
    if (!input) {
      // Fallback: copy to clipboard
      navigator.clipboard.writeText(text).then(() => {
        showToast('已复制到剪贴板 (未找到输入框)');
      });
      return;
    }

    // Handle contenteditable divs (Claude, Gemini, Kimi)
    if (input.getAttribute('contenteditable') === 'true') {
      input.focus();
      // Insert as a text block wrapped in a code-like format
      const wrappedText = text;
      // Use execCommand for maximum compatibility with contenteditable
      document.execCommand('insertText', false, wrappedText);
      // Trigger input event so the platform registers the change
      input.dispatchEvent(new Event('input', { bubbles: true }));
    }
    // Handle textarea (ChatGPT)
    else if (input.tagName === 'TEXTAREA') {
      input.focus();
      const start = input.selectionStart;
      const end = input.selectionEnd;
      const value = input.value;
      input.value = value.substring(0, start) + text + value.substring(end);
      input.selectionStart = input.selectionEnd = start + text.length;
      // React needs a native input event setter trick
      const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
        window.HTMLTextAreaElement.prototype, 'value'
      ).set;
      nativeInputValueSetter.call(input, input.value);
      input.dispatchEvent(new Event('input', { bubbles: true }));
    }
  }

  // --- New Conversation Detection ---

  function observeNewConversation() {
    let lastUrl = window.location.href;

    // Poll for URL changes (SPA navigation)
    setInterval(async () => {
      const currentUrl = window.location.href;
      if (currentUrl !== lastUrl) {
        lastUrl = currentUrl;
        if (platform.newConversationUrl.test(currentUrl)) {
          await handleNewConversation();
        }
      }
    }, 1500);
  }

  async function handleNewConversation() {
    if (!isConnected) return;

    try {
      const settings = await sendMessage('getSettings');
      if (!settings.autoInject) return;

      const platformKey = hostname;
      if (settings.platforms && settings.platforms[platformKey] === false) return;

      // Wait a moment for the chat interface to render
      setTimeout(() => {
        showAutoInjectBanner();
      }, 1000);
    } catch (err) {
      console.error('[NeuDrive] Auto-inject check failed:', err);
    }
  }

  function showAutoInjectBanner() {
    // Don't show if already present
    if (document.getElementById('neudrive-auto-banner')) return;

    const banner = document.createElement('div');
    banner.id = 'neudrive-auto-banner';
    banner.innerHTML = `
      <span>neuDrive: 检测到新对话，是否注入用户上下文？</span>
      <button id="neudrive-auto-yes" class="neudrive-banner-btn neudrive-banner-btn-yes">注入</button>
      <button id="neudrive-auto-no" class="neudrive-banner-btn neudrive-banner-btn-no">跳过</button>
    `;
    document.body.appendChild(banner);

    // Auto-dismiss after 10 seconds
    const timer = setTimeout(() => removeBanner(), 10000);

    banner.querySelector('#neudrive-auto-yes').addEventListener('click', async () => {
      clearTimeout(timer);
      removeBanner();
      await handleInjectAction('inject-preferences');
    });

    banner.querySelector('#neudrive-auto-no').addEventListener('click', () => {
      clearTimeout(timer);
      removeBanner();
    });

    function removeBanner() {
      banner.remove();
    }
  }

  // --- Toast Notification ---

  function showToast(message) {
    const existing = document.getElementById('neudrive-toast');
    if (existing) existing.remove();

    const toast = document.createElement('div');
    toast.id = 'neudrive-toast';
    toast.textContent = message;
    document.body.appendChild(toast);

    // Trigger animation
    requestAnimationFrame(() => {
      toast.classList.add('neudrive-toast-visible');
    });

    setTimeout(() => {
      toast.classList.remove('neudrive-toast-visible');
      setTimeout(() => toast.remove(), 300);
    }, 2500);
  }

  // --- Utilities ---

  function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  // --- Listen for messages from background ---

  chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (message.action === 'tabUpdated') {
      // Page navigated, check if new conversation
      if (platform.newConversationUrl.test(message.payload.url)) {
        handleNewConversation();
      }
    }
    sendResponse({ ok: true });
    return false;
  });

  // --- Initialize ---

  function init() {
    createFloatingButton();
    createPanel();
    observeNewConversation();

    // Pre-check connection status
    sendMessage('getStatus').then(status => {
      isConnected = status.connected;
      profileData = status.profile;
    }).catch(() => {
      // Not configured yet, that's fine
    });

    console.log(`[NeuDrive] Content script initialized on ${platform.name}`);
  }

  // Wait for DOM to be ready
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
