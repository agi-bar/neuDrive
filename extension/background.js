/**
 * neuDrive - Background Service Worker
 * Manages API connection, token storage, profile caching,
 * and message handling for content scripts and popup.
 */

importScripts('lib/neudrive-client.js');

const client = new NeuDriveClient();
const OFFICIAL_HUB_URL = 'https://www.neudrive.ai';
const OFFICIAL_LOGIN_URL = `${OFFICIAL_HUB_URL}/setup/tokens?source=browser-extension`;
const OFFICIAL_LOGIN_PENDING_KEY = 'officialLoginPending';
const OFFICIAL_LOGIN_PENDING_TTL_MS = 15 * 60 * 1000;
const OFFICIAL_EXTENSION_TOKEN_REQUEST = {
  name: 'Browser Extension',
  scopes: [
    'read:profile',
    'read:memory',
    'read:skills',
    'read:projects',
    'read:tree',
    'write:tree',
    'search',
  ],
  max_trust_level: 3,
  expires_in_days: 30,
};
const SUPPORTED_CHAT_HOSTS = ['claude.ai', 'chat.openai.com', 'chatgpt.com', 'gemini.google.com', 'kimi.moonshot.cn'];
const SUPPORTED_CHAT_URL_PATTERNS = SUPPORTED_CHAT_HOSTS.map(host => `https://${host}/*`);
let officialLoginInFlight = false;

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

    case 'startOfficialLogin': {
      await chrome.storage.local.set({
        [OFFICIAL_LOGIN_PENDING_KEY]: {
          startedAt: Date.now(),
          sourceTabId: sender?.tab?.id || null,
        },
      });
      const tab = await chrome.tabs.create({ url: OFFICIAL_LOGIN_URL, active: true });
      return { tabId: tab.id, url: OFFICIAL_LOGIN_URL };
    }

    case 'completeOfficialLogin': {
      return handleOfficialLoginCompletion(payload, sender);
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

    case 'importCurrentConversation': {
      return importCurrentConversationArchive(payload);
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
          'chatgpt.com': true,
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

async function handleOfficialLoginCompletion(payload, sender) {
  const pending = await getPendingOfficialLogin();
  if (!pending) {
    return { ignored: true };
  }
  if (officialLoginInFlight) {
    return { ignored: true };
  }

  const authToken = payload?.authToken || '';
  if (!authToken) {
    throw new Error('Missing official auth token.');
  }

  officialLoginInFlight = true;
  try {
    const created = await createOfficialExtensionToken(authToken);
    await client.configure(OFFICIAL_HUB_URL, created.token);

    let profile = null;
    try {
      profile = await client.getProfile(true);
    } catch (err) {
      await client.disconnect();
      throw err;
    }

    await clearPendingOfficialLogin();
    await notifyOfficialLoginComplete(profile);

    if (sender?.tab?.id) {
      await chrome.tabs.remove(sender.tab.id).catch(() => {});
    }

    return { configured: true, profile };
  } catch (err) {
    await notifyOfficialLoginError(err.message || 'Official login failed.');
    throw err;
  } finally {
    officialLoginInFlight = false;
  }
}

async function createOfficialExtensionToken(authToken) {
  const response = await fetch(`${OFFICIAL_HUB_URL}/api/tokens`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${authToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(OFFICIAL_EXTENSION_TOKEN_REQUEST),
  });

  const body = await response.json().catch(() => null);
  if (!response.ok) {
    const message = body?.message || body?.error || response.statusText || 'Failed to create extension token.';
    throw new Error(message);
  }

  const data = body && body.ok === true && body.data ? body.data : body;
  if (!data?.token) {
    throw new Error('Official login succeeded, but no extension token was returned.');
  }
  return data;
}

async function getPendingOfficialLogin() {
  const result = await chrome.storage.local.get([OFFICIAL_LOGIN_PENDING_KEY]);
  const pending = result[OFFICIAL_LOGIN_PENDING_KEY];
  if (!pending) return null;
  if (!pending.startedAt || (Date.now() - pending.startedAt) > OFFICIAL_LOGIN_PENDING_TTL_MS) {
    await clearPendingOfficialLogin();
    return null;
  }
  return pending;
}

async function clearPendingOfficialLogin() {
  await chrome.storage.local.remove([OFFICIAL_LOGIN_PENDING_KEY]);
}

async function notifyOfficialLoginComplete(profile) {
  await notifyOpenChatTabs({
    action: 'officialLoginComplete',
    payload: { hubUrl: OFFICIAL_HUB_URL, profile: profile || null },
  });
  await chrome.runtime.sendMessage({
    action: 'officialLoginComplete',
    payload: { hubUrl: OFFICIAL_HUB_URL, profile: profile || null },
  }).catch(() => {});
}

async function notifyOfficialLoginError(message) {
  await notifyOpenChatTabs({
    action: 'officialLoginError',
    payload: { message },
  });
  await chrome.runtime.sendMessage({
    action: 'officialLoginError',
    payload: { message },
  }).catch(() => {});
}

async function notifyOpenChatTabs(message) {
  const tabs = await chrome.tabs.query({ url: SUPPORTED_CHAT_URL_PATTERNS });
  await Promise.allSettled(
    tabs
      .filter(tab => typeof tab.id === 'number')
      .map(tab => chrome.tabs.sendMessage(tab.id, message).catch(() => {}))
  );
}

async function importCurrentConversationArchive(payload) {
  const sourcePlatform = payload?.sourcePlatform || 'claude-web';
  const importedAt = new Date().toISOString();
  const title = sanitizeLine(payload?.title || 'Untitled conversation');
  const conversationId = sanitizeLine(payload?.conversationId || '');
  const url = sanitizeLine(payload?.url || '');
  const importStrategy = sanitizeLine(payload?.importStrategy || 'unknown');
  const extraMetadata = payload?.extraMetadata && typeof payload.extraMetadata === 'object'
    ? payload.extraMetadata
    : {};
  try {
    const normalizedConversation = buildNormalizedConversationFromPayload(payload, {
      sourcePlatform,
      title,
      url,
      conversationId,
      importedAt,
      importStrategy,
      extraMetadata,
    });
    if (normalizedConversation.turns.length === 0) {
      throw new Error('No normalized conversation turns were captured from the page.');
    }
    const baseName = buildConversationArchiveBaseName({
      sourcePlatform,
      title,
      conversationId,
      importedAt,
    });

    const commonMetadata = {
      conversation_id: conversationId,
      imported_at: importedAt,
      import_strategy: importStrategy,
      turn_count: normalizedConversation.turns.length,
      url,
      title,
      ...extraMetadata,
    };

    return writeConversationArchiveSplit({
      sourcePlatform,
      baseName,
      title,
      url,
      conversationId,
      importedAt,
      normalizedConversation,
      importStrategy,
      extraMetadata: commonMetadata,
    });
  } catch (err) {
    throw normalizeImportError(err);
  }
}

async function writeConversationArchiveSplit({
  sourcePlatform,
  baseName,
  title,
  url,
  conversationId,
  importedAt,
  normalizedConversation,
  importStrategy,
  extraMetadata,
}) {
  const rootPath = buildConversationSplitRoot(sourcePlatform, baseName);
  await deleteConversationArchiveRoot(rootPath);
  const transcriptPath = `${rootPath}/conversation.md`;
  const conversationPath = `${rootPath}/conversation.json`;
  const exportPaths = buildConversationExportPaths(rootPath);
  await ensureConversationArchiveDirectory({
    rootPath,
    conversation: normalizedConversation,
    transcriptPath,
    conversationPath,
    exportPaths,
    sourcePlatform,
    metadata: extraMetadata,
  });
  const transcriptEntry = await writeReadableTranscriptArtifact({
    basePath: transcriptPath,
    conversation: normalizedConversation,
    sourcePlatform,
    metadata: {
      ...extraMetadata,
      import_kind: 'conversation_archive_transcript',
      storage_mode: 'compact',
    },
  });

  for (const target of CONVERSATION_EXPORT_TARGETS) {
    const exportPath = exportPaths[target];
    if (!exportPath) continue;
    await writePortableArtifact({
      basePath: exportPath,
      content: renderConversationContinuationMarkdown(normalizedConversation, target),
      mimeType: 'text/markdown',
      sourcePlatform,
      metadata: {
        ...extraMetadata,
        import_kind: 'conversation_archive_export',
        storage_mode: 'compact',
        target_platform: target,
        transcript_path: transcriptEntry.path,
        conversation_path: conversationPath,
      },
    });
  }

  const conversationEntry = await writePortableArtifact({
    basePath: conversationPath,
    content: JSON.stringify({
      ...normalizedConversation,
      transcript_path: transcriptEntry.path,
      exports: exportPaths,
    }) + '\n',
    mimeType: 'application/json',
    sourcePlatform,
    metadata: {
      ...extraMetadata,
      import_kind: 'conversation_archive_normalized',
      storage_mode: 'compact',
    },
  });

  return {
    path: transcriptEntry.path,
    conversationPath: conversationEntry.path,
    exportPaths,
    turnCount: normalizedConversation.turns.length,
    title,
    storageMode: 'compact',
  };
}

async function writeReadableTranscriptArtifact({
  basePath,
  conversation,
  sourcePlatform,
  metadata,
}) {
  const primaryContent = normalizedConversationToMarkdown(conversation, { compact: false });
  try {
    await client.writeFile(basePath, primaryContent, {
      mimeType: 'text/markdown',
      minTrustLevel: 3,
      source: 'browser-extension',
      sourcePlatform,
      metadata,
    });
    return {
      path: basePath,
      encoding: 'text',
      contentType: 'text/markdown',
    };
  } catch (err) {
    if (!isCloudflareBlock(err)) {
      throw err;
    }
  }

  const fallbackContent = normalizedConversationToMarkdown(conversation, { compact: true });
  return writePortableArtifact({
    basePath,
    content: fallbackContent,
    mimeType: 'text/markdown',
    sourcePlatform,
    metadata,
  });
}

async function writePortableArtifact({
  basePath,
  content,
  mimeType,
  sourcePlatform,
  metadata,
  index,
  role,
}) {
  try {
    await client.writeFile(basePath, content, {
      mimeType,
      minTrustLevel: 3,
      source: 'browser-extension',
      sourcePlatform,
      metadata,
    });
    const entry = {
      path: basePath,
      encoding: 'text',
      contentType: mimeType,
    };
    if (typeof index === 'number') entry.index = index;
    if (role) entry.role = role;
    return entry;
  } catch (err) {
    if (!isCloudflareBlock(err)) {
      throw err;
    }
  }

  return writeBase64ChunkedFile({
    basePath,
    content,
    sourcePlatform,
    metadata: {
      ...metadata,
      encoding: 'base64',
    },
    contentType: mimeType,
    index,
    role,
  });
}

async function writeBase64ChunkedFile({
  basePath,
  content,
  sourcePlatform,
  metadata,
  contentType,
  index,
  role,
}) {
  const encoded = utf8ToBase64(content);
  const chunkSize = 12000;
  const partPaths = [];
  const totalBytes = utf8ByteLength(content);
  for (let offset = 0, part = 0; offset < encoded.length; offset += chunkSize, part += 1) {
    const partPath = `${basePath}.b64.part-${String(part + 1).padStart(4, '0')}.txt`;
    const chunk = encoded.slice(offset, offset + chunkSize);
    await client.writeFile(partPath, chunk, {
      mimeType: 'text/plain',
      minTrustLevel: 3,
      source: 'browser-extension',
      sourcePlatform,
      metadata: {
        ...metadata,
        chunk_index: part + 1,
        chunk_size: chunk.length,
      },
    });
    partPaths.push(partPath);
  }

  const manifestPath = `${basePath}.b64.json`;
  const manifest = {
    encoding: 'base64',
    original_content_type: contentType || 'text/plain',
    original_size_bytes: totalBytes,
    part_count: partPaths.length,
    part_paths: partPaths,
  };
  if (typeof index === 'number') {
    manifest.turn_index = index;
  }
  if (role) {
    manifest.turn_role = role;
  }

  await client.writeFile(manifestPath, JSON.stringify(manifest, null, 2) + '\n', {
    mimeType: 'application/json',
    minTrustLevel: 3,
    source: 'browser-extension',
    sourcePlatform,
    metadata: {
      ...metadata,
      part_count: partPaths.length,
      original_content_type: contentType || 'text/plain',
    },
  });

  const entry = {
    path: manifestPath,
    manifestPath,
    partPaths,
    encoding: 'base64',
    partCount: partPaths.length,
    originalContentType: contentType || 'text/plain',
  };
  if (typeof index === 'number') {
    entry.index = index;
  }
  if (role) {
    entry.role = role;
  }
  return entry;
}

function buildConversationSplitRoot(sourcePlatform, baseName) {
  return `/conversations/${sourcePlatform}/${baseName}-compact`;
}

function buildConversationExportPaths(rootPath) {
  return {
    claude: `${rootPath}/resume-claude.md`,
    chatgpt: `${rootPath}/resume-chatgpt.md`,
  };
}

async function ensureConversationArchiveDirectory({
  rootPath,
  conversation,
  transcriptPath,
  conversationPath,
  exportPaths,
  sourcePlatform,
  metadata,
}) {
  await client.writeDirectory(rootPath, {
    minTrustLevel: 3,
    source: 'browser-extension',
    sourcePlatform,
    metadata: {
      ...metadata,
      ...buildConversationBundleMetadata(conversation, transcriptPath, conversationPath, exportPaths),
    },
  });
}

async function deleteConversationArchiveRoot(rootPath) {
  try {
    await client.deletePath(rootPath);
  } catch (err) {
    const message = String(err?.message || err || '');
    if (
      message.includes('API error 404') ||
      message.includes('API error 405') ||
      /not found/i.test(message)
    ) {
      return;
    }
    throw err;
  }
}

function buildNormalizedConversationFromPayload(payload, defaults) {
  const normalized = payload?.normalizedConversation && typeof payload.normalizedConversation === 'object'
    ? payload.normalizedConversation
    : {};
  const turns = Array.isArray(normalized.turns) && normalized.turns.length > 0
    ? normalized.turns.map((turn, index) => sanitizeNormalizedTurn(turn, index))
    : buildLegacyNormalizedTurns(payload?.turns);

  return {
    version: 'neudrive.conversation/v1',
    source_platform: sanitizeLine(normalized.source_platform || defaults.sourcePlatform || ''),
    source_url: sanitizeLine(normalized.source_url || defaults.url || ''),
    source_conversation_id: sanitizeLine(normalized.source_conversation_id || defaults.conversationId || ''),
    title: sanitizeLine(normalized.title || defaults.title || 'Untitled conversation'),
    imported_at: sanitizeLine(normalized.imported_at || defaults.importedAt || new Date().toISOString()),
    import_strategy: sanitizeLine(normalized.import_strategy || defaults.importStrategy || 'unknown'),
    model: sanitizeLine(normalized.model || defaults.extraMetadata?.model || ''),
    created_at: sanitizeLine(normalized.created_at || defaults.extraMetadata?.created_at || ''),
    updated_at: sanitizeLine(normalized.updated_at || defaults.extraMetadata?.updated_at || ''),
    provenance: safeStructuredData(normalized.provenance || {
      organization_id: defaults.extraMetadata?.organization_id || '',
      branch_message_count: defaults.extraMetadata?.branch_message_count || turns.length,
      message_count: defaults.extraMetadata?.message_count || turns.length,
    }),
    turn_count: turns.length,
    turns,
  };
}

function buildLegacyNormalizedTurns(turns) {
  if (!Array.isArray(turns)) {
    return [];
  }
  return turns
    .map((turn, index) => sanitizeNormalizedTurn({
      id: `turn_${String(index + 1).padStart(4, '0')}`,
      role: turn?.role || '',
      at: turn?.createdAt || '',
      source_message_id: turn?.uuid || '',
      parts: [{ type: 'text', text: turn?.content || '' }],
    }, index))
    .filter(turn => turn.parts.length > 0);
}

function sanitizeNormalizedTurn(turn, index) {
  const parts = sanitizeNormalizedParts(turn?.parts, turn?.content);
  return {
    id: sanitizeLine(turn?.id || `turn_${String(index + 1).padStart(4, '0')}`),
    role: normalizeRole(turn?.role),
    at: sanitizeLine(turn?.at || turn?.createdAt || ''),
    source_message_id: sanitizeLine(turn?.source_message_id || turn?.sourceMessageId || turn?.uuid || ''),
    parent_source_message_id: sanitizeLine(turn?.parent_source_message_id || turn?.parentSourceMessageId || ''),
    source_message_kind: sanitizeLine(turn?.source_message_kind || turn?.sourceMessageKind || ''),
    parts,
  };
}

function sanitizeNormalizedParts(parts, fallbackContent) {
  const normalizedParts = Array.isArray(parts)
    ? parts.map(part => sanitizeNormalizedPart(part)).filter(Boolean)
    : [];
  if (normalizedParts.length > 0) {
    return normalizedParts;
  }

  const text = sanitizeBlockText(fallbackContent || '');
  return text ? [{ type: 'text', text }] : [];
}

function sanitizeNormalizedPart(part) {
  if (!part || typeof part !== 'object') {
    return null;
  }

  const type = sanitizeLine(part.type || 'text') || 'text';
  if (type === 'text' || type === 'thinking') {
    const maxLength = type === 'thinking' ? 4000 : 32000;
    const text = truncateText(sanitizeBlockText(part.text || ''), maxLength);
    return text ? { type, text, truncated: text.includes('[truncated') } : null;
  }

  if (type === 'tool_call') {
    const argsPreview = serializeStructuredPreview(part.args, 1600);
    return {
      type,
      name: sanitizeLine(part.name || ''),
      args_text: argsPreview.text,
      args_truncated: argsPreview.truncated,
    };
  }

  if (type === 'tool_result') {
    const textSource = sanitizeBlockText(part.text || '') || serializeStructuredPreview(part.data, 2400).text;
    const text = truncateText(textSource, 2400);
    return {
      type,
      text,
      truncated: text.includes('[truncated'),
    };
  }

  if (type === 'attachment') {
    return {
      type,
      file_name: sanitizeLine(part.file_name || part.name || ''),
      mime_type: sanitizeLine(part.mime_type || ''),
    };
  }

  const preview = serializeStructuredPreview(part.data != null ? part.data : part, 1200);
  return {
    type,
    text: preview.text,
    truncated: preview.truncated,
  };
}

function normalizeRole(role) {
  const value = sanitizeLine(role || '').toLowerCase();
  if (['user', 'assistant', 'system', 'tool'].includes(value)) {
    return value;
  }
  return value || 'assistant';
}

function sanitizeBlockText(value) {
  return String(value || '')
    .replace(/\r/g, '')
    .replace(/\u0000/g, '')
    .replace(/\u00a0/g, ' ')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

function safeStructuredData(value) {
  if (value == null) return null;
  try {
    return JSON.parse(JSON.stringify(value));
  } catch {
    return { value: String(value) };
  }
}

function normalizedConversationToMarkdown(conversation, options = {}) {
  const lines = [
    '---',
    `title: "${escapeYaml(conversation.title || 'Untitled conversation')}"`,
    `source_platform: "${escapeYaml(conversation.source_platform || '')}"`,
    `imported_at: "${escapeYaml(conversation.imported_at || '')}"`,
    `import_strategy: "${escapeYaml(conversation.import_strategy || '')}"`,
    `turn_count: ${Array.isArray(conversation.turns) ? conversation.turns.length : 0}`,
  ];

  if (conversation.source_conversation_id) {
    lines.push(`source_conversation_id: "${escapeYaml(conversation.source_conversation_id)}"`);
  }
  if (conversation.source_url) {
    lines.push(`source_url: "${escapeYaml(conversation.source_url)}"`);
  }
  if (conversation.model) {
    lines.push(`model: "${escapeYaml(conversation.model)}"`);
  }

  lines.push('---', '', `# ${conversation.title || 'Conversation'}`, '');
  lines.push('This is the primary readable archive for the conversation. Tool payloads are condensed for readability, while `conversation.json` keeps a compact normalized sidecar.');
  lines.push('');

  (conversation.turns || []).forEach((turn, index) => {
    lines.push(`## ${capitalize(turn.role || 'turn')} ${index + 1}`);
    lines.push('');
    if (turn.at) {
      lines.push(`_at: ${turn.at}_`);
      lines.push('');
    }
    lines.push(renderNormalizedTurnText(turn, options));
    lines.push('');
  });

  return lines.join('\n').trim() + '\n';
}

const CONVERSATION_EXPORT_TARGETS = ['claude', 'chatgpt'];

function buildConversationBundleMetadata(conversation, transcriptPath, conversationPath, exportPaths) {
  return {
    bundle_kind: 'conversation',
    bundle_name: sanitizeLine(conversation.title || 'Untitled conversation'),
    name: sanitizeLine(conversation.title || 'Untitled conversation'),
    source: sanitizeLine(conversation.source_platform || ''),
    source_platform: sanitizeLine(conversation.source_platform || ''),
    source_conversation_id: sanitizeLine(conversation.source_conversation_id || ''),
    import_strategy: sanitizeLine(conversation.import_strategy || ''),
    imported_at: sanitizeLine(conversation.imported_at || ''),
    turn_count: Array.isArray(conversation.turns) ? conversation.turns.length : 0,
    description: conversationBundleDescription(conversation),
    status: 'archived',
    bundle_primary_path: transcriptPath,
    bundle_capabilities: ['transcript', 'normalized', 'exports'],
    conversation_transcript_path: transcriptPath,
    conversation_path: conversationPath,
    conversation_exports: exportPaths,
  };
}

function conversationBundleDescription(conversation) {
  const source = sanitizeLine(conversation.source_platform || 'unknown-platform') || 'unknown-platform';
  const turnCount = Array.isArray(conversation.turns) ? conversation.turns.length : 0;
  if (turnCount > 0) {
    return `Imported from ${source} with ${turnCount} turns.`;
  }
  return `Imported from ${source}.`;
}

function renderConversationContinuationMarkdown(conversation, target) {
  const normalizedTarget = normalizeConversationExportTarget(target);
  const targetLabel = conversationExportTargetLabel(normalizedTarget);
  const title = conversation.title || 'Conversation';
  const lines = [
    '---',
    `title: "${escapeYaml(`Resume ${title} in ${targetLabel}`)}"`,
    `target_platform: "${escapeYaml(normalizedTarget)}"`,
    `source_platform: "${escapeYaml(conversation.source_platform || '')}"`,
    `generated_from: "${escapeYaml(conversation.version || 'neudrive.conversation/v1')}"`,
    `turn_count: ${Array.isArray(conversation.turns) ? conversation.turns.length : 0}`,
  ];

  if (conversation.source_conversation_id) {
    lines.push(`source_conversation_id: "${escapeYaml(conversation.source_conversation_id)}"`);
  }
  if (conversation.project_name) {
    lines.push(`project_name: "${escapeYaml(conversation.project_name)}"`);
  }

  lines.push('---', '', `# Resume ${title} in ${targetLabel}`, '');
  lines.push(`Paste the prompt below into a new ${targetLabel} conversation or project chat when you want to continue from this archived context.`);
  lines.push('');
  lines.push('## Resume Prompt', '');
  lines.push(`Continue the conversation below. It was imported into neuDrive from \`${conversation.source_platform || 'unknown-platform'}\`.`);
  lines.push('- Preserve speaker order and chronology.');
  lines.push('- Treat tool calls and tool results as historical context unless the user explicitly asks to rerun them.');
  lines.push('- Do not restate the full transcript unless asked.');
  lines.push('- Continue naturally from the latest turn or respond to the next user message.');
  lines.push('');
  lines.push('## Transcript', '', '```text', normalizedConversationToPortableTranscript(conversation), '```');
  return lines.join('\n').trim() + '\n';
}

function normalizeConversationExportTarget(target) {
  return String(target || '').toLowerCase() === 'chatgpt' ? 'chatgpt' : 'claude';
}

function conversationExportTargetLabel(target) {
  return normalizeConversationExportTarget(target) === 'chatgpt' ? 'ChatGPT' : 'Claude';
}

function normalizedConversationToPortableTranscript(conversation) {
  const turns = Array.isArray(conversation.turns) ? conversation.turns : [];
  const blocks = turns.map(turn => {
    const headerParts = [capitalize(turn.role || 'turn')];
    if (turn.at) headerParts.push(turn.at);
    if (turn.source_message_kind) headerParts.push(turn.source_message_kind);
    const body = (turn.parts || [])
      .map(renderPortableConversationPart)
      .filter(Boolean)
      .join('\n\n')
      .trim();
    if (!body) return '';
    return `[${headerParts.join(' | ')}]\n${body}`;
  }).filter(Boolean);

  return blocks.length > 0 ? blocks.join('\n\n') : '[Conversation]\n(no turns captured)';
}

function renderPortableConversationPart(part) {
  if (!part || typeof part !== 'object') {
    return '';
  }

  switch (part.type) {
    case 'text':
    case '':
      return sanitizeBlockText(part.text || '');
    case 'thinking':
      return part.text ? `Thinking:\n${sanitizeBlockText(part.text)}` : '';
    case 'tool_call': {
      const lines = [`Tool call: ${part.name || 'tool'}`];
      if (part.args_text) {
        lines.push('Args:');
        lines.push(part.args_text);
      }
      return lines.join('\n');
    }
    case 'tool_result':
      return part.text ? `Tool result:\n${sanitizeBlockText(part.text)}` : 'Tool result captured.';
    case 'attachment': {
      const meta = [];
      if (part.file_name) meta.push(`name=${part.file_name}`);
      if (part.mime_type) meta.push(`mime=${part.mime_type}`);
      return meta.length ? `Attachment: ${meta.join(', ')}` : 'Attachment captured.';
    }
    default:
      return part.text ? `${capitalize(part.type || 'content')}:\n${sanitizeBlockText(part.text)}` : '';
  }
}

function renderNormalizedTurnText(turn, options = {}) {
  const rendered = (turn.parts || [])
    .map(part => renderNormalizedPart(part, options))
    .filter(Boolean);
  return rendered.join('\n\n').trim();
}

function renderNormalizedPart(part, options = {}) {
  if (!part || typeof part !== 'object') {
    return '';
  }

  switch (part.type) {
    case 'text':
      return sanitizeBlockText(part.text || '');
    case 'thinking':
      return options.compact ? '' : `> Thinking (condensed)\n>\n> ${sanitizeBlockText(part.text || '').replace(/\n/g, '\n> ')}`;
    case 'tool_call': {
      const lines = [options.compact ? `Tool call: \`${part.name || 'tool'}\`` : `### Tool Call: \`${part.name || 'tool'}\``];
      if (part.args_text && !options.compact) {
        lines.push('');
        lines.push('```json');
        lines.push(part.args_text);
        lines.push('```');
      } else if (part.args_text && options.compact) {
        lines.push(`Args: ${truncateInline(part.args_text, 180)}`);
      }
      return lines.join('\n');
    }
    case 'tool_result': {
      const lines = [options.compact ? 'Tool result captured.' : '### Tool Result'];
      if (part.text && !options.compact) {
        lines.push('');
        lines.push('```text');
        lines.push(sanitizeBlockText(part.text));
        lines.push('```');
      } else if (part.text && options.compact) {
        lines[0] = `Tool result: ${truncateInline(part.text, 280)}`;
      }
      return lines.join('\n');
    }
    case 'attachment': {
      const lines = ['Attachment'];
      if (part.file_name) lines.push(`name: ${part.file_name}`);
      if (part.mime_type) lines.push(`mime: ${part.mime_type}`);
      return lines.join('\n');
    }
    default: {
      const lines = [options.compact ? `Additional ${part.type || 'content'} metadata preserved.` : `### ${capitalize(part.type || 'content')}`];
      if (part.text && !options.compact) {
        lines.push('');
        lines.push('```text');
        lines.push(sanitizeBlockText(part.text));
        lines.push('```');
      }
      return lines.join('\n');
    }
  }
}

function serializeStructuredPreview(value, maxLength) {
  if (value == null) {
    return { text: '', truncated: false };
  }
  let text = '';
  try {
    text = typeof value === 'string' ? sanitizeBlockText(value) : JSON.stringify(value, null, 2);
  } catch {
    text = String(value);
  }
  const normalized = sanitizeBlockText(text);
  const truncatedText = truncateText(normalized, maxLength);
  return {
    text: truncatedText,
    truncated: truncatedText.includes('[truncated'),
  };
}

function truncateText(text, maxLength) {
  const normalized = sanitizeBlockText(text || '');
  if (!normalized || normalized.length <= maxLength) {
    return normalized;
  }
  return `${normalized.slice(0, maxLength).trimEnd()}\n\n[truncated ${normalized.length - maxLength} chars]`;
}

function truncateInline(text, maxLength) {
  const normalized = sanitizeBlockText(text || '').replace(/\s+/g, ' ');
  if (normalized.length <= maxLength) {
    return normalized;
  }
  return `${normalized.slice(0, maxLength).trimEnd()}...`;
}

function isCloudflareBlock(err) {
  const message = String(err?.message || err || '');
  return message.includes('API error 403') && (
    message.includes('Cloudflare') ||
    message.includes('Attention Required') ||
    message.includes('Sorry, you have been blocked')
  );
}

function normalizeImportError(err) {
  const message = String(err?.message || err || '');
  if (message.includes('token missing required scope: write:tree')) {
    return new Error('当前 neuDrive Token 缺少 write:tree，不能导入对话。请在扩展弹窗里重新登录官方版，或改用带 write:tree 的 Token。');
  }
  return err instanceof Error ? err : new Error(message);
}

function utf8ToBase64(text) {
  const bytes = new TextEncoder().encode(text);
  let binary = '';
  const step = 0x8000;
  for (let i = 0; i < bytes.length; i += step) {
    const slice = bytes.subarray(i, i + step);
    binary += String.fromCharCode(...slice);
  }
  return btoa(binary);
}

function utf8ByteLength(text) {
  return new TextEncoder().encode(text).length;
}

function buildConversationArchiveBaseName({ sourcePlatform, title, conversationId, importedAt }) {
  const stamp = importedAt.slice(0, 10);
  const idPart = slugifySegment(conversationId || '');
  const titlePart = slugifySegment(title || '');
  return idPart || titlePart || `conversation-${stamp}`;
}

function sanitizeLine(value) {
  return String(value || '').replace(/\r/g, '').trim();
}

function slugifySegment(value) {
  return String(value || '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 80);
}

function escapeYaml(value) {
  return String(value || '').replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}

function capitalize(value) {
  const text = String(value || '');
  return text ? text[0].toUpperCase() + text.slice(1) : 'Turn';
}

// Listen for tab updates to notify content scripts about navigation
chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
  const changedUrl = changeInfo.url || tab.url || '';
  if (changedUrl.startsWith(OFFICIAL_HUB_URL)) {
    tryNotifyOfficialSiteSession(changedUrl, tabId).catch(err => {
      console.error('[NeuDrive] Official login bridge failed:', err);
    });
  }

  if (changeInfo.status === 'complete' && tab.url) {
    try {
      const url = new URL(tab.url);
      if (SUPPORTED_CHAT_HOSTS.includes(url.hostname)) {
        chrome.tabs.sendMessage(tabId, { action: 'tabUpdated', payload: { url: tab.url } }).catch(() => {
          // Content script may not be ready yet, that's fine
        });
      }
    } catch (e) {
      // Invalid URL, ignore
    }
  }
});

async function tryNotifyOfficialSiteSession(urlString, tabId) {
  const pending = await getPendingOfficialLogin();
  if (!pending || typeof tabId !== 'number') return;
  const url = new URL(urlString);
  const source = url.searchParams.get('source') || '';
  if (source !== 'browser-extension' && !url.searchParams.get('auth_token')) return;
  chrome.tabs.sendMessage(tabId, {
    action: 'officialLoginRequested',
    payload: { hubUrl: OFFICIAL_HUB_URL },
  }).catch(() => {});
}
