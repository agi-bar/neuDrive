/**
 * neuDrive - Content Script
 * Injects floating Hub button and context panel into AI chat interfaces.
 */

(function () {
  'use strict';

  // Prevent double injection
  if (window.__agentHubInjected) return;
  window.__agentHubInjected = true;

  const OFFICIAL_HUB_URL = 'https://www.neudrive.ai';
  const OFFICIAL_HUB_HOSTS = ['www.neudrive.ai', 'neudrive.ai'];
  const hostname = window.location.hostname;

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

  if (OFFICIAL_HUB_HOSTS.includes(hostname)) {
    initOfficialBridge();
    return;
  }

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
      newConversationUrl: /^https:\/\/chat\.openai\.com\/?(?:\?.*)?$/,
    },
    'chatgpt.com': {
      name: 'ChatGPT',
      inputSelector: '#prompt-textarea',
      conversationSelector: '[data-message-id]',
      newConversationUrl: /^https:\/\/chatgpt\.com\/?(?:\?.*)?$/,
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
  let manualConfigVisible = false;
  let importInFlight = false;
  const supportsConversationImport = ['claude.ai', 'chat.openai.com', 'chatgpt.com'].includes(hostname);

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
    const importAction = supportsConversationImport ? `
          <button class="neudrive-btn neudrive-btn-primary-lite" data-action="import-current-conversation">
            <span class="neudrive-btn-icon">&#8681;</span>
            导入当前对话
          </button>
    ` : '';
    panel.innerHTML = `
      <div class="neudrive-panel-header">
        <span class="neudrive-panel-title">neuDrive</span>
        <button class="neudrive-panel-close" title="关闭">&times;</button>
      </div>
      <div class="neudrive-panel-body">
        <div id="neudrive-status" class="neudrive-status">检查连接中...</div>
        <div id="neudrive-profile" class="neudrive-profile" style="display:none;"></div>
        <div id="neudrive-actions" class="neudrive-actions" style="display:none;">
          ${importAction}
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
          <p id="neudrive-action-status" class="neudrive-inline-message" style="display:none;"></p>
        </div>
        <div id="neudrive-not-connected" style="display:none;">
          <p id="neudrive-hint" class="neudrive-hint">首次使用可以直接登录 neuDrive 官方版，或手动填写 Hub URL 和 Token。</p>
          <div class="neudrive-empty-actions">
            <button id="neudrive-btn-official-login" class="neudrive-btn neudrive-btn-primary-lite" type="button">
              <span class="neudrive-btn-icon">&#128274;</span>
              登录 neuDrive 官方版
            </button>
            <button id="neudrive-btn-manual-toggle" class="neudrive-btn" type="button">
              <span class="neudrive-btn-icon">&#9881;</span>
              手动配置 URL + Token
            </button>
          </div>
          <div id="neudrive-manual-form" class="neudrive-manual-form" style="display:none;">
            <label class="neudrive-field">
              <span class="neudrive-field-label">Hub URL</span>
              <input id="neudrive-input-url" class="neudrive-input" type="url" placeholder="https://www.neudrive.ai" />
            </label>
            <label class="neudrive-field">
              <span class="neudrive-field-label">Scoped Token</span>
              <input id="neudrive-input-token" class="neudrive-input" type="password" placeholder="ndt_xxx" />
            </label>
            <button id="neudrive-btn-manual-connect" class="neudrive-btn neudrive-btn-primary-lite" type="button">
              <span class="neudrive-btn-icon">&#10132;</span>
              连接
            </button>
            <p id="neudrive-manual-message" class="neudrive-inline-message" style="display:none;"></p>
          </div>
        </div>
      </div>
    `;

    // Event listeners
    panel.querySelector('.neudrive-panel-close').addEventListener('click', togglePanel);
    panel.querySelectorAll('.neudrive-btn[data-action]').forEach(btn => {
      btn.addEventListener('click', () => handleInjectAction(btn.dataset.action));
    });
    panel.querySelector('#neudrive-btn-official-login').addEventListener('click', handleOfficialLogin);
    panel.querySelector('#neudrive-btn-manual-toggle').addEventListener('click', () => toggleManualConfig());
    panel.querySelector('#neudrive-btn-manual-connect').addEventListener('click', handleManualConnect);
    panel.querySelector('#neudrive-input-url').addEventListener('keydown', (event) => {
      if (event.key === 'Enter') {
        event.preventDefault();
        panel.querySelector('#neudrive-input-token').focus();
      }
    });
    panel.querySelector('#neudrive-input-token').addEventListener('keydown', (event) => {
      if (event.key === 'Enter') {
        event.preventDefault();
        handleManualConnect();
      }
    });

    preloadManualConfig();

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
    const hintEl = document.getElementById('neudrive-hint');

    if (!statusEl) return;

    try {
      await preloadManualConfig();
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
        toggleManualConfig(false);
        showManualMessage('', false);
      } else if (status.configured && !status.connected) {
        statusEl.innerHTML = '<span class="neudrive-dot neudrive-dot-err"></span> 连接失败';
        profileEl.style.display = 'none';
        actionsEl.style.display = 'none';
        notConnectedEl.style.display = 'block';
        hintEl.textContent = status.error || '当前保存的连接不可用。你可以重新登录官方版，或改用手动配置。';
      } else {
        statusEl.innerHTML = '<span class="neudrive-dot neudrive-dot-off"></span> 未配置';
        profileEl.style.display = 'none';
        actionsEl.style.display = 'none';
        notConnectedEl.style.display = 'block';
        hintEl.textContent = '首次使用可以直接登录 neuDrive 官方版，或手动填写 Hub URL 和 Token。';
      }
    } catch (err) {
      statusEl.innerHTML = '<span class="neudrive-dot neudrive-dot-err"></span> 错误';
      console.error('[NeuDrive] Status check failed:', err);
    }
  }

  function showActionStatus(text, isError) {
    const messageEl = document.getElementById('neudrive-action-status');
    if (!messageEl) return;
    if (!text) {
      messageEl.style.display = 'none';
      messageEl.textContent = '';
      messageEl.className = 'neudrive-inline-message';
      return;
    }
    messageEl.style.display = 'block';
    messageEl.textContent = text;
    messageEl.className = `neudrive-inline-message ${isError ? 'neudrive-inline-message-error' : 'neudrive-inline-message-success'}`;
  }

  function setActionBusy(action, busy, busyLabel) {
    const button = document.querySelector(`.neudrive-btn[data-action="${action}"]`);
    if (!button) return;
    if (!button.dataset.defaultHtml) {
      button.dataset.defaultHtml = button.innerHTML;
    }
    button.disabled = busy;
    button.classList.toggle('neudrive-btn-busy', busy);
    if (busy) {
      button.innerHTML = `<span class="neudrive-btn-icon">&#8987;</span>${busyLabel || '处理中...'}`;
    } else {
      button.innerHTML = button.dataset.defaultHtml;
    }
  }

  async function preloadManualConfig() {
    const inputUrl = document.getElementById('neudrive-input-url');
    if (!inputUrl) return;
    const data = await chrome.storage.local.get(['hubUrl']);
    if (!inputUrl.value) {
      inputUrl.value = data.hubUrl || OFFICIAL_HUB_URL;
    }
  }

  function toggleManualConfig(nextVisible) {
    const manualForm = document.getElementById('neudrive-manual-form');
    if (!manualForm) return;
    manualConfigVisible = typeof nextVisible === 'boolean' ? nextVisible : !manualConfigVisible;
    manualForm.style.display = manualConfigVisible ? 'block' : 'none';
    if (manualConfigVisible) {
      preloadManualConfig();
    }
  }

  function showManualMessage(text, isError) {
    const messageEl = document.getElementById('neudrive-manual-message');
    if (!messageEl) return;
    if (!text) {
      messageEl.style.display = 'none';
      messageEl.textContent = '';
      messageEl.className = 'neudrive-inline-message';
      return;
    }
    messageEl.style.display = 'block';
    messageEl.textContent = text;
    messageEl.className = `neudrive-inline-message ${isError ? 'neudrive-inline-message-error' : 'neudrive-inline-message-success'}`;
  }

  async function handleOfficialLogin() {
    try {
      await sendMessage('startOfficialLogin');
      showToast('已打开 neuDrive 官方登录页，完成授权后扩展会自动连接');
    } catch (err) {
      console.error('[NeuDrive] Failed to start official login:', err);
      showToast('打开官方登录失败: ' + err.message);
    }
  }

  async function handleManualConnect() {
    const inputUrl = document.getElementById('neudrive-input-url');
    const inputToken = document.getElementById('neudrive-input-token');
    if (!inputUrl || !inputToken) return;

    const hubUrl = inputUrl.value.trim();
    const token = inputToken.value.trim();

    if (!hubUrl) {
      showManualMessage('请输入 Hub 服务地址', true);
      return;
    }
    if (!token) {
      showManualMessage('请输入 Scoped Token', true);
      return;
    }

    try {
      new URL(hubUrl);
    } catch {
      showManualMessage('Hub URL 格式不正确', true);
      return;
    }

    showManualMessage('连接中...', false);

    try {
      await sendMessage('configure', { hubUrl, token });
      inputToken.value = '';
      toggleManualConfig(false);
      await refreshStatus();
      showToast('neuDrive 已连接');
    } catch (err) {
      console.error('[NeuDrive] Manual connect failed:', err);
      showManualMessage(err.message, true);
    }
  }

  // --- Inject Actions ---

  async function handleInjectAction(action) {
    try {
      let contextText = '';

      switch (action) {
        case 'import-current-conversation': {
          if (importInFlight) {
            showActionStatus('正在导入当前对话，请稍候…', false);
            showToast('正在导入当前对话，请稍候…', 3200);
            return;
          }
          importInFlight = true;
          setActionBusy(action, true, '导入中...');
          showActionStatus('正在导入当前对话到 neuDrive…', false);
          showToast('正在导入当前对话到 neuDrive…', 3200);
          const result = await importCurrentConversation();
          showActionStatus(`已导入 ${result.turnCount} 条消息，主文件已整理成可读 transcript。`, false);
          showToast(`已导入 ${result.turnCount} 条消息`, 4200);
          return;
        }
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
      if (action === 'import-current-conversation') {
        showActionStatus(`导入失败：${err.message}`, true);
      }
      showToast('注入失败: ' + err.message);
    } finally {
      if (action === 'import-current-conversation') {
        importInFlight = false;
        setActionBusy(action, false);
      }
    }
  }

  async function importCurrentConversation() {
    let payload = null;

    if (hostname === 'claude.ai') {
      try {
        payload = await buildClaudeConversationImportPayload();
      } catch (err) {
        console.warn('[NeuDrive] Claude API import failed, falling back to DOM capture:', err);
      }
    } else if (hostname === 'chat.openai.com' || hostname === 'chatgpt.com') {
      payload = buildChatGPTConversationImportPayload();
    }

    if (!payload) {
      const turns = collectConversationTurns();
      if (turns.length === 0) {
        throw new Error(`当前页面没有可导入内容，${platform.name} 页面抓取没有拿到消息。`);
      }

      payload = {
        sourcePlatform: currentConversationSourcePlatform(),
        title: getConversationTitle(),
        url: window.location.href,
        conversationId: getConversationId(),
        importStrategy: 'dom',
        normalizedConversation: buildNormalizedConversation({
          sourcePlatform: currentConversationSourcePlatform(),
          title: getConversationTitle(),
          url: window.location.href,
          conversationId: getConversationId(),
          importStrategy: 'dom',
          turns: turns.map((turn, index) => buildNormalizedTurn({
            id: `turn_${String(index + 1).padStart(4, '0')}`,
            role: turn.role,
            at: turn.createdAt || '',
            sourceMessageId: turn.uuid || '',
            parts: [{ type: 'text', text: turn.content }],
          })),
        }),
      };
    }

    return sendMessage('importCurrentConversation', payload);
  }

  function buildChatGPTConversationImportPayload() {
    const turns = collectConversationTurns();
    if (turns.length === 0) {
      throw new Error('当前页面没有可导入的 ChatGPT 消息。');
    }

    return {
      sourcePlatform: 'chatgpt-web',
      title: getConversationTitle(),
      url: window.location.href,
      conversationId: getConversationId(),
      importStrategy: 'dom',
      normalizedConversation: buildNormalizedConversation({
        sourcePlatform: 'chatgpt-web',
        title: getConversationTitle(),
        url: window.location.href,
        conversationId: getConversationId(),
        importStrategy: 'dom',
        provenance: {
          message_count: turns.length,
          host: hostname,
        },
        turns: turns.map((turn, index) => buildNormalizedTurn({
          id: `turn_${String(index + 1).padStart(4, '0')}`,
          role: turn.role,
          at: turn.createdAt || '',
          sourceMessageId: turn.uuid || '',
          parts: [{ type: 'text', text: turn.content }],
        })),
      }),
      extraMetadata: {
        host: hostname,
        message_count: turns.length,
      },
    };
  }

  async function buildClaudeConversationImportPayload() {
    const conversationId = getConversationId();
    if (!conversationId) {
      throw new Error('当前不是 Claude 具体会话页面');
    }

    const organizations = await fetchClaudeOrganizations();
    if (organizations.length === 0) {
      throw new Error('未找到 Claude organization');
    }

    let lastError = null;
    for (const organization of organizations) {
      const orgId = organization?.uuid || organization?.id || '';
      if (!orgId) continue;

      try {
        const conversation = await fetchClaudeConversation(orgId, conversationId);
        const branchMessages = getCurrentClaudeBranch(conversation);
        const turns = branchMessages
          .map(message => ({
            role: normalizeClaudeSenderRole(message?.sender),
            content: extractClaudeMessageText(message),
            createdAt: message?.created_at || '',
            uuid: message?.uuid || '',
          }))
          .filter(turn => turn.content);

        if (turns.length === 0) {
          throw new Error('Claude API 返回了会话，但没有可归档的消息内容');
        }

        return {
          sourcePlatform: 'claude-web',
          title: sanitizeImportText(conversation?.name) || getConversationTitle(),
          url: window.location.href,
          conversationId,
          importStrategy: 'claude-api',
          normalizedConversation: buildNormalizedConversation({
            sourcePlatform: 'claude-web',
            title: sanitizeImportText(conversation?.name) || getConversationTitle(),
            url: window.location.href,
            conversationId,
            importStrategy: 'claude-api',
            model: conversation?.model || '',
            createdAt: conversation?.created_at || '',
            updatedAt: conversation?.updated_at || '',
            provenance: {
              organization_id: orgId,
              branch_message_count: branchMessages.length,
              message_count: Array.isArray(conversation?.chat_messages) ? conversation.chat_messages.length : turns.length,
            },
            turns: branchMessages
              .map((message, index) => buildNormalizedTurnFromClaudeMessage(message, index))
              .filter(turn => turn.parts.length > 0),
          }),
          extraMetadata: {
            organization_id: orgId,
            branch_message_count: branchMessages.length,
            message_count: Array.isArray(conversation?.chat_messages) ? conversation.chat_messages.length : turns.length,
            created_at: conversation?.created_at || '',
            updated_at: conversation?.updated_at || '',
            model: conversation?.model || '',
          },
        };
      } catch (err) {
        lastError = err;
      }
    }

    throw lastError || new Error('无法通过 Claude API 获取当前会话');
  }

  async function fetchClaudeOrganizations() {
    const response = await fetch('https://claude.ai/api/organizations', {
      credentials: 'include',
      headers: {
        'Accept': 'application/json',
      },
    });

    if (!response.ok) {
      throw new Error(`读取 Claude organizations 失败 (${response.status})`);
    }

    const data = await response.json();
    if (Array.isArray(data)) return data;
    if (Array.isArray(data?.organizations)) return data.organizations;
    if (Array.isArray(data?.data)) return data.data;
    return [];
  }

  async function fetchClaudeConversation(orgId, conversationId) {
    const url = `https://claude.ai/api/organizations/${encodeURIComponent(orgId)}/chat_conversations/${encodeURIComponent(conversationId)}?tree=True&rendering_mode=messages&render_all_tools=true`;
    const response = await fetch(url, {
      credentials: 'include',
      headers: {
        'Accept': 'application/json',
      },
    });

    if (!response.ok) {
      throw new Error(`读取 Claude 会话失败 (${response.status})`);
    }

    return response.json();
  }

  function getCurrentClaudeBranch(conversation) {
    const messages = Array.isArray(conversation?.chat_messages) ? conversation.chat_messages : [];
    const leafId = conversation?.current_leaf_message_uuid || '';
    if (!messages.length || !leafId) {
      return [];
    }

    const messageMap = new Map(messages.map(message => [message?.uuid, message]));
    const branch = [];
    let currentId = leafId;

    while (currentId && messageMap.has(currentId)) {
      const message = messageMap.get(currentId);
      branch.unshift(message);
      currentId = message?.parent_message_uuid || '';
      if (!currentId || !messageMap.has(currentId)) {
        break;
      }
    }

    return branch;
  }

  function normalizeClaudeSenderRole(sender) {
    if (sender === 'human') return 'user';
    if (sender === 'assistant') return 'assistant';
    return sender || 'assistant';
  }

  function buildNormalizedConversation({
    sourcePlatform,
    title,
    url,
    conversationId,
    importStrategy,
    model,
    createdAt,
    updatedAt,
    provenance,
    turns,
  }) {
    const normalizedTurns = Array.isArray(turns) ? turns.filter(turn => turn && Array.isArray(turn.parts) && turn.parts.length > 0) : [];
    return {
      version: 'neudrive.conversation/v1',
      source_platform: sourcePlatform || '',
      source_url: url || '',
      source_conversation_id: conversationId || '',
      title: title || 'Untitled conversation',
      imported_at: new Date().toISOString(),
      import_strategy: importStrategy || 'unknown',
      model: model || '',
      created_at: createdAt || '',
      updated_at: updatedAt || '',
      provenance: provenance || {},
      turns: normalizedTurns,
      turn_count: normalizedTurns.length,
    };
  }

  function buildNormalizedTurnFromClaudeMessage(message, index) {
    return buildNormalizedTurn({
      id: `turn_${String(index + 1).padStart(4, '0')}`,
      role: normalizeClaudeSenderRole(message?.sender),
      at: message?.created_at || '',
      sourceMessageId: message?.uuid || '',
      parentSourceMessageId: message?.parent_message_uuid || '',
      parts: normalizeClaudeContentParts(message?.content, message?.text),
    });
  }

  function buildNormalizedTurn({
    id,
    role,
    at,
    sourceMessageId,
    parentSourceMessageId,
    parts,
  }) {
    const normalizedParts = Array.isArray(parts) ? parts.filter(Boolean) : [];
    return {
      id: id || '',
      role: role || 'assistant',
      at: at || '',
      source_message_id: sourceMessageId || '',
      parent_source_message_id: parentSourceMessageId || '',
      parts: normalizedParts,
    };
  }

  function normalizeClaudeContentParts(contentBlocks, fallbackText) {
    const blocks = Array.isArray(contentBlocks) ? contentBlocks : [];
    const parts = blocks
      .map(block => normalizeClaudeContentBlock(block))
      .filter(Boolean);

    if (parts.length > 0) {
      return parts;
    }

    const text = sanitizeImportText(fallbackText || '');
    return text ? [{ type: 'text', text }] : [];
  }

  function normalizeClaudeContentBlock(block) {
    if (!block || typeof block !== 'object') {
      return null;
    }

    if (typeof block.text === 'string' && block.text.trim()) {
      return {
        type: 'text',
        text: sanitizeImportText(block.text),
      };
    }

    if (typeof block.thinking === 'string' && block.thinking.trim()) {
      return {
        type: 'thinking',
        text: sanitizeImportText(block.thinking),
      };
    }

    const type = block.type || 'content';
    if (type === 'tool_use') {
      return {
        type: 'tool_call',
        name: sanitizeImportText(block.name || ''),
        args: toSafeJson(block.input),
      };
    }

    if (type === 'tool_result') {
      return {
        type: 'tool_result',
        text: renderToolResultText(block.content),
        data: renderToolResultData(block.content),
      };
    }

    if (block.file_name || block.mime_type) {
      return {
        type: 'attachment',
        file_name: sanitizeImportText(block.file_name || ''),
        mime_type: sanitizeImportText(block.mime_type || ''),
      };
    }

    return {
      type: sanitizeImportText(type || 'unknown'),
      data: toSafeJson(stripTransientFields(block)),
    };
  }

  function renderToolResultText(content) {
    if (typeof content === 'string') {
      return sanitizeImportText(content);
    }
    if (Array.isArray(content)) {
      return content
        .map(item => {
          if (typeof item === 'string') return sanitizeImportText(item);
          if (item && typeof item.text === 'string') return sanitizeImportText(item.text);
          return '';
        })
        .filter(Boolean)
        .join('\n\n');
    }
    return '';
  }

  function renderToolResultData(content) {
    if (typeof content === 'string') {
      return null;
    }
    return toSafeJson(content);
  }

  function stripTransientFields(block) {
    const safe = { ...block };
    delete safe.signature;
    return safe;
  }

  function toSafeJson(value) {
    if (value == null) return null;
    try {
      return JSON.parse(JSON.stringify(value));
    } catch {
      return { value: String(value) };
    }
  }

  function extractClaudeMessageText(message) {
    const parts = normalizeClaudeContentParts(message?.content, message?.text);
    return parts
      .map(part => renderNormalizedPartToText(part))
      .filter(Boolean)
      .join('\n\n')
      .trim();
  }

  function renderNormalizedPartToText(part) {
    if (!part || typeof part !== 'object') {
      return '';
    }

    switch (part.type) {
      case 'text':
        return sanitizeImportText(part.text || '');
      case 'thinking':
        return `[thinking]\n${sanitizeImportText(part.text || '')}`;
      case 'tool_call': {
        const lines = ['[tool_call]'];
        if (part.name) lines.push(`name: ${part.name}`);
        if (part.args != null) lines.push(JSON.stringify(part.args, null, 2));
        return lines.join('\n');
      }
      case 'tool_result': {
        const lines = ['[tool_result]'];
        if (part.text) lines.push(sanitizeImportText(part.text));
        else if (part.data != null) lines.push(JSON.stringify(part.data, null, 2));
        return lines.join('\n');
      }
      case 'attachment': {
        const lines = ['[attachment]'];
        if (part.file_name) lines.push(`name: ${part.file_name}`);
        if (part.mime_type) lines.push(`mime: ${part.mime_type}`);
        return lines.join('\n');
      }
      default: {
        const lines = [`[${part.type || 'content'}]`];
        if (part.data != null) lines.push(JSON.stringify(part.data, null, 2));
        return lines.join('\n');
      }
    }
  }

  function sanitizeImportText(text) {
    return String(text || '')
      .replace(/\r/g, '')
      .replace(/\u00a0/g, ' ')
      .replace(/\n{3,}/g, '\n\n')
      .trim();
  }

  function collectConversationTurns() {
    const turnNodes = Array.from(document.querySelectorAll(platform.conversationSelector));
    return turnNodes
      .map((node, index) => ({
        role: detectConversationRole(node, index),
        content: extractTurnText(node),
        uuid: extractTurnId(node, index),
        createdAt: extractTurnTimestamp(node),
      }))
      .filter(turn => turn.content);
  }

  function detectConversationRole(node, index) {
    const authorRole = node.getAttribute('data-message-author-role')
      || node.closest('[data-message-author-role]')?.getAttribute('data-message-author-role')
      || node.querySelector('[data-message-author-role]')?.getAttribute('data-message-author-role')
      || '';
    if (authorRole) {
      return normalizeMessageRole(authorRole);
    }
    const labeled = node.querySelector('[data-testid*="author"], [data-testid*="sender"], h3, h4, header');
    const labeledText = normalizeWhitespace(labeled?.textContent || '');
    if (/(you|user|human|me|我)/i.test(labeledText)) {
      return 'user';
    }
    if (/(claude|assistant|model)/i.test(labeledText)) {
      return 'assistant';
    }
    return index % 2 === 0 ? 'user' : 'assistant';
  }

  function normalizeMessageRole(role) {
    const normalized = String(role || '').trim().toLowerCase();
    if (normalized === 'user' || normalized === 'human') return 'user';
    if (normalized === 'assistant') return 'assistant';
    if (normalized === 'tool') return 'tool';
    if (normalized === 'system') return 'system';
    return normalized || 'assistant';
  }

  function extractTurnText(node) {
    const clone = node.cloneNode(true);
    clone.querySelectorAll('button, svg, textarea, input, nav, footer, [aria-hidden="true"]').forEach(el => el.remove());
    const lines = normalizeWhitespace(clone.innerText || '')
      .split('\n')
      .map(line => line.trim())
      .filter(Boolean)
      .filter(line => !isChromeLineNoise(line));
    return lines.join('\n').trim();
  }

  function isChromeLineNoise(line) {
    return /^(copy|edit|retry|thumbs up|thumbs down|good response|bad response|copy code|share|read aloud|regenerate|saved memory updated)$/i.test(line);
  }

  function extractTurnId(node, index) {
    return node.getAttribute('data-message-id')
      || node.dataset?.messageId
      || node.closest('[data-message-id]')?.getAttribute('data-message-id')
      || `message_${index + 1}`;
  }

  function extractTurnTimestamp(node) {
    const timeEl = node.querySelector('time');
    return timeEl?.getAttribute('datetime') || '';
  }

  function getConversationTitle() {
    const fallback = `${platform.name} conversation`;
    const raw = document.title || fallback;
    return raw
      .replace(/\s*[-|]\s*(Claude|ChatGPT).*$/i, '')
      .replace(/^(Claude|ChatGPT)\s*[-|]\s*/i, '')
      .trim() || fallback;
  }

  function getConversationId() {
    const parts = window.location.pathname.split('/').filter(Boolean);
    const candidate = parts[parts.length - 1] || '';
    if (/^[a-z0-9_-]{8,}$/i.test(candidate)) {
      return candidate;
    }
    return '';
  }

  function currentConversationSourcePlatform() {
    if (hostname === 'claude.ai') return 'claude-web';
    if (hostname === 'chat.openai.com' || hostname === 'chatgpt.com') return 'chatgpt-web';
    return hostname;
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

  function showToast(message, duration = 2500) {
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
    }, duration);
  }

  // --- Utilities ---

  function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  function normalizeWhitespace(text) {
    return String(text || '')
      .replace(/\r/g, '')
      .replace(/\n{3,}/g, '\n\n')
      .trim();
  }

  // --- Listen for messages from background ---

  chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (message.action === 'tabUpdated') {
      // Page navigated, check if new conversation
      if (platform.newConversationUrl.test(message.payload.url)) {
        handleNewConversation();
      }
    } else if (message.action === 'officialLoginComplete') {
      refreshStatus();
      showToast('neuDrive 官方账号已连接');
    } else if (message.action === 'officialLoginError') {
      showToast('官方登录失败: ' + (message.payload?.message || '请重试'));
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

  function shouldBridgeOfficialLogin() {
    const url = new URL(window.location.href);
    if (url.searchParams.get('auth_token')) return true;
    if (url.searchParams.get('source') === 'browser-extension') return true;
    const redirect = url.searchParams.get('redirect') || '';
    return redirect.includes('source=browser-extension');
  }

  function readOfficialAuthToken() {
    const url = new URL(window.location.href);
    return url.searchParams.get('auth_token') || localStorage.getItem('token') || '';
  }

  function readOfficialRefreshToken() {
    const url = new URL(window.location.href);
    return url.searchParams.get('auth_refresh') || localStorage.getItem('refresh_token') || '';
  }

  function initOfficialBridge() {
    if (!shouldBridgeOfficialLogin()) return;

    let finished = false;
    const startedAt = Date.now();

    const tryBridge = async () => {
      if (finished) return;
      const authToken = readOfficialAuthToken();
      if (!authToken) return;
      try {
        const result = await sendMessage('completeOfficialLogin', {
          authToken,
          refreshToken: readOfficialRefreshToken(),
          pageUrl: window.location.href,
        });
        if (result?.configured || result?.ignored) {
          finished = true;
        }
      } catch (err) {
        console.error('[NeuDrive] Official auth bridge failed:', err);
        finished = true;
      }
    };

    tryBridge();

    const timer = setInterval(() => {
      if (finished || (Date.now() - startedAt) > 5 * 60 * 1000) {
        clearInterval(timer);
        return;
      }
      tryBridge();
    }, 1000);
  }
})();
