/**
 * Agent Hub API Client
 * Lightweight fetch wrapper for the Agent Hub API.
 * Uses a scoped token stored in chrome.storage.local.
 */
class AgentHubClient {
  constructor() {
    this._baseUrl = null;
    this._token = null;
    this._profileCache = null;
    this._profileCacheTime = 0;
    this._cacheTTL = 5 * 60 * 1000; // 5 minutes
  }

  /**
   * Initialize client from stored config.
   * Must be called before any API calls.
   */
  async init() {
    const data = await chrome.storage.local.get(['hubUrl', 'hubToken']);
    this._baseUrl = data.hubUrl || null;
    this._token = data.hubToken || null;
    return this.isConfigured();
  }

  /** Check if both URL and token are set */
  isConfigured() {
    return !!(this._baseUrl && this._token);
  }

  /** Update connection config and persist it */
  async configure(hubUrl, token) {
    // Normalize: strip trailing slash
    this._baseUrl = hubUrl.replace(/\/+$/, '');
    this._token = token;
    this._profileCache = null;
    await chrome.storage.local.set({ hubUrl: this._baseUrl, hubToken: this._token });
  }

  /** Clear stored credentials */
  async disconnect() {
    this._baseUrl = null;
    this._token = null;
    this._profileCache = null;
    await chrome.storage.local.remove(['hubUrl', 'hubToken']);
  }

  /**
   * Core fetch wrapper. Adds auth header, handles errors.
   * @param {string} path - API path (e.g. "/api/v1/profile")
   * @param {object} options - fetch options
   * @returns {Promise<object>} parsed JSON response
   */
  async _request(path, options = {}) {
    if (!this.isConfigured()) {
      throw new Error('Agent Hub not configured. Please set Hub URL and token.');
    }

    const url = `${this._baseUrl}${path}`;
    const headers = {
      'Authorization': `Bearer ${this._token}`,
      'Content-Type': 'application/json',
      ...options.headers,
    };

    const response = await fetch(url, {
      ...options,
      headers,
    });

    if (!response.ok) {
      if (response.status === 401) {
        throw new Error('Authentication failed. Please check your token.');
      }
      const text = await response.text().catch(() => '');
      throw new Error(`API error ${response.status}: ${text || response.statusText}`);
    }

    // Handle 204 No Content
    if (response.status === 204) {
      return null;
    }

    return response.json();
  }

  /**
   * Get user profile with caching.
   * @param {boolean} forceRefresh - bypass cache
   */
  async getProfile(forceRefresh = false) {
    const now = Date.now();
    if (!forceRefresh && this._profileCache && (now - this._profileCacheTime) < this._cacheTTL) {
      return this._profileCache;
    }

    const profile = await this._request('/api/v1/profile');
    this._profileCache = profile;
    this._profileCacheTime = now;
    return profile;
  }

  /**
   * List user skills.
   * @param {object} params - query params { limit, offset, tag }
   */
  async listSkills(params = {}) {
    const query = new URLSearchParams();
    if (params.limit) query.set('limit', params.limit);
    if (params.offset) query.set('offset', params.offset);
    if (params.tag) query.set('tag', params.tag);
    const qs = query.toString();
    return this._request(`/api/v1/skills${qs ? '?' + qs : ''}`);
  }

  /**
   * Get a specific project by ID.
   * @param {string} projectId
   */
  async getProject(projectId) {
    return this._request(`/api/v1/projects/${encodeURIComponent(projectId)}`);
  }

  /**
   * List all projects.
   */
  async listProjects() {
    return this._request('/api/v1/projects');
  }

  /**
   * Search user memory / knowledge base.
   * @param {string} query - search query
   * @param {object} params - { limit, type }
   */
  async searchMemory(query, params = {}) {
    const body = { query, ...params };
    return this._request('/api/v1/memory/search', {
      method: 'POST',
      body: JSON.stringify(body),
    });
  }

  /**
   * Get user preferences.
   */
  async getPreferences() {
    return this._request('/api/v1/preferences');
  }

  /**
   * Build a context injection string from profile data.
   * @param {string} type - "preferences" | "project" | "skills"
   * @param {object} data - relevant data payload
   */
  buildContextBlock(type, data) {
    const blocks = {
      preferences: () => {
        const p = data;
        const lines = ['[Agent Hub - 用户偏好]'];
        if (p.name) lines.push(`姓名: ${p.name}`);
        if (p.language) lines.push(`首选语言: ${p.language}`);
        if (p.tone) lines.push(`回复风格: ${p.tone}`);
        if (p.expertise) lines.push(`专业领域: ${Array.isArray(p.expertise) ? p.expertise.join(', ') : p.expertise}`);
        if (p.instructions) lines.push(`自定义指令:\n${p.instructions}`);
        return lines.join('\n');
      },
      project: () => {
        const proj = data;
        const lines = ['[Agent Hub - 项目上下文]'];
        if (proj.name) lines.push(`项目: ${proj.name}`);
        if (proj.description) lines.push(`描述: ${proj.description}`);
        if (proj.stack) lines.push(`技术栈: ${Array.isArray(proj.stack) ? proj.stack.join(', ') : proj.stack}`);
        if (proj.conventions) lines.push(`编码规范:\n${proj.conventions}`);
        return lines.join('\n');
      },
      skills: () => {
        const skills = Array.isArray(data) ? data : [data];
        const lines = ['[Agent Hub - 技能清单]'];
        skills.forEach(s => {
          lines.push(`- ${s.name}: ${s.description || ''}`);
        });
        return lines.join('\n');
      },
    };

    const builder = blocks[type];
    if (!builder) return JSON.stringify(data, null, 2);
    return builder();
  }
}

// Export for use in different contexts (background, content script, popup)
if (typeof globalThis !== 'undefined') {
  globalThis.AgentHubClient = AgentHubClient;
}
