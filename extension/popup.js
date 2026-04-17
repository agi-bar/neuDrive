/**
 * neuDrive - Popup Script
 * Handles login, status display, settings, and quick links.
 */

(function () {
  'use strict';

  const OFFICIAL_HUB_URL = 'https://www.neudrive.ai';

  // --- DOM References ---
  const viewLogin = document.getElementById('view-login');
  const viewConnected = document.getElementById('view-connected');

  const inputUrl = document.getElementById('input-url');
  const inputToken = document.getElementById('input-token');
  const btnOfficialLogin = document.getElementById('btn-official-login');
  const btnConnect = document.getElementById('btn-connect');
  const loginMessage = document.getElementById('login-message');

  const connectionStatus = document.getElementById('connection-status');
  const profileName = document.getElementById('profile-name');
  const profileBio = document.getElementById('profile-bio');

  const linkDashboard = document.getElementById('link-dashboard');
  const linkNewToken = document.getElementById('link-new-token');

  const toggleAutoInject = document.getElementById('toggle-auto-inject');
  const platformCheckboxes = document.querySelectorAll('#platform-list input[type="checkbox"]');
  const btnDisconnect = document.getElementById('btn-disconnect');

  // --- Helper: send message to background ---
  function sendMessage(action, payload) {
    return new Promise((resolve, reject) => {
      chrome.runtime.sendMessage({ action, payload }, response => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message));
          return;
        }
        if (!response) {
          reject(new Error('No response'));
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

  // --- Show/Hide Views ---
  function showLogin() {
    viewLogin.classList.remove('hidden');
    viewConnected.classList.add('hidden');
  }

  function showConnected(profile) {
    viewLogin.classList.add('hidden');
    viewConnected.classList.remove('hidden');
    connectionStatus.innerHTML = '<span class="dot dot-ok"></span><span>已连接</span>';

    if (profile) {
      profileName.textContent = profile.name || profile.username || 'User';
      profileBio.textContent = profile.bio || profile.email || '';
    }
  }

  function showLoginMessage(text, isError) {
    loginMessage.textContent = text;
    loginMessage.className = 'message ' + (isError ? 'message-error' : 'message-success');
  }

  function clearLoginMessage() {
    loginMessage.className = 'message';
    loginMessage.textContent = '';
  }

  function prefillHubUrl(savedHubUrl) {
    if (!inputUrl.value) {
      inputUrl.value = savedHubUrl || OFFICIAL_HUB_URL;
    }
  }

  // --- Initialize ---
  async function init() {
    try {
      const status = await sendMessage('getStatus');

      if (status.connected && status.profile) {
        showConnected(status.profile);
        await loadSettings();
        updateQuickLinks();
      } else if (status.configured && !status.connected) {
        showLogin();
        // Pre-fill URL from storage
        const data = await chrome.storage.local.get(['hubUrl']);
        prefillHubUrl(data.hubUrl);
        showLoginMessage(status.error || '连接失败，请检查配置', true);
      } else {
        showLogin();
        prefillHubUrl('');
      }
    } catch (err) {
      showLogin();
      prefillHubUrl('');
    }
  }

  btnOfficialLogin.addEventListener('click', async () => {
    clearLoginMessage();
    btnOfficialLogin.disabled = true;
    try {
      await sendMessage('startOfficialLogin');
      showLoginMessage('已打开 neuDrive 官方登录页，完成授权后扩展会自动连接。', false);
    } catch (err) {
      showLoginMessage(err.message, true);
    } finally {
      btnOfficialLogin.disabled = false;
    }
  });

  // --- Connect ---
  btnConnect.addEventListener('click', async () => {
    const hubUrl = inputUrl.value.trim();
    const token = inputToken.value.trim();

    if (!hubUrl) {
      showLoginMessage('请输入 Hub 服务地址', true);
      return;
    }
    if (!token) {
      showLoginMessage('请输入 Token', true);
      return;
    }

    // Validate URL format
    try {
      new URL(hubUrl);
    } catch {
      showLoginMessage('服务地址格式不正确', true);
      return;
    }

    clearLoginMessage();
    btnConnect.disabled = true;
    btnConnect.textContent = '连接中...';

    try {
      const profile = await sendMessage('configure', { hubUrl, token });
      showConnected(profile);
      await loadSettings();
      updateQuickLinks();
    } catch (err) {
      showLoginMessage(err.message, true);
    } finally {
      btnConnect.disabled = false;
      btnConnect.textContent = '连接';
    }
  });

  // --- Disconnect ---
  btnDisconnect.addEventListener('click', async () => {
    try {
      await sendMessage('disconnect');
      showLogin();
      inputToken.value = '';
      clearLoginMessage();
    } catch (err) {
      console.error('Disconnect failed:', err);
    }
  });

  // --- Quick Links ---
  function updateQuickLinks() {
    chrome.storage.local.get(['hubUrl'], data => {
      const baseUrl = data.hubUrl || OFFICIAL_HUB_URL;
      linkDashboard.href = baseUrl + '/';
      linkDashboard.addEventListener('click', (e) => {
        e.preventDefault();
        chrome.tabs.create({ url: baseUrl + '/' });
      });
      linkNewToken.href = baseUrl + '/setup/tokens';
      linkNewToken.addEventListener('click', (e) => {
        e.preventDefault();
        chrome.tabs.create({ url: baseUrl + '/setup/tokens' });
      });
    });
  }

  // --- Settings ---
  async function loadSettings() {
    try {
      const settings = await sendMessage('getSettings');
      toggleAutoInject.checked = settings.autoInject || false;
      platformCheckboxes.forEach(cb => {
        const platform = cb.dataset.platform;
        if (!settings.platforms) return;
        if (platform === 'chat.openai.com') {
          const chatgptEnabled = settings.platforms['chatgpt.com'];
          if (platform in settings.platforms) {
            cb.checked = settings.platforms[platform];
          } else if (typeof chatgptEnabled === 'boolean') {
            cb.checked = chatgptEnabled;
          }
          return;
        }
        if (platform in settings.platforms) {
          cb.checked = settings.platforms[platform];
        }
      });
    } catch (err) {
      console.error('Failed to load settings:', err);
    }
  }

  async function saveSettings() {
    const platforms = {};
    platformCheckboxes.forEach(cb => {
      if (cb.dataset.platform === 'chat.openai.com') {
        platforms['chat.openai.com'] = cb.checked;
        platforms['chatgpt.com'] = cb.checked;
        return;
      }
      platforms[cb.dataset.platform] = cb.checked;
    });

    const settings = {
      autoInject: toggleAutoInject.checked,
      platforms,
    };

    try {
      await sendMessage('saveSettings', settings);
    } catch (err) {
      console.error('Failed to save settings:', err);
    }
  }

  toggleAutoInject.addEventListener('change', saveSettings);
  platformCheckboxes.forEach(cb => {
    cb.addEventListener('change', saveSettings);
  });

  // --- Allow Enter key to submit login ---
  inputToken.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') btnConnect.click();
  });
  inputUrl.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') inputToken.focus();
  });

  chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (message.action === 'officialLoginComplete') {
      clearLoginMessage();
      init();
    } else if (message.action === 'officialLoginError') {
      showLoginMessage(message.payload?.message || '官方登录失败，请重试。', true);
    }
    sendResponse({ ok: true });
    return false;
  });

  // --- Start ---
  init();
})();
