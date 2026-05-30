const API_BASE = import.meta.env.VITE_API_BASE || ''
let authToken = localStorage.getItem('token')

export function setToken(token) {
  authToken = token
  if (token) {
    localStorage.setItem('token', token)
  } else {
    localStorage.removeItem('token')
  }
}

export async function fetchJSON(path, options = {}) {
  const headers = { 'Content-Type': 'application/json', ...options.headers }
  if (authToken) {
    headers['Authorization'] = `Bearer ${authToken}`
  }
  const res = await fetch(`${API_BASE}${path}`, { ...options, headers })
  if (res.status === 401) {
    // Token expired
    setToken(null)
    window.location.href = '/login'
    throw new Error('unauthorized')
  }
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}

export function sseConnect(path, body, handlers) {
  const controller = new AbortController()
  const headers = { 'Content-Type': 'application/json' }
  if (authToken) {
    headers['Authorization'] = `Bearer ${authToken}`
  }

  fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
    signal: controller.signal,
  }).then(async (res) => {
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }))
      handlers.onError?.(err.error || res.statusText)
      return
    }

    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let doneReceived = false

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue
        try {
          const event = JSON.parse(line.slice(6))
          switch (event.event) {
            case 'delta':
              handlers.onDelta?.(event.data)
              break
            case 'tool_call':
              handlers.onToolCall?.(JSON.parse(event.data))
              break
            case 'tool_result':
              handlers.onToolResult?.(JSON.parse(event.data))
              break
            case 'skill_activate':
              handlers.onSkillActivate?.(JSON.parse(event.data))
              break
            case 'title_update':
              handlers.onTitleUpdate?.(event.data)
              break
            case 'image':
              handlers.onImage?.(JSON.parse(event.data))
              break
            case 'thinking':
              try {
                const thinkObj = JSON.parse(event.data)
                handlers.onThinking?.(thinkObj.content || event.data)
              } catch {
                handlers.onThinking?.(event.data)
              }
              break
            case 'session':
              handlers.onSession?.(event.data)
              break
            case 'agent_start':
              handlers.onAgentStart?.(event.data)
              break
            case 'agent_end':
              handlers.onAgentEnd?.(event.data)
              break
            case 'compact':
              handlers.onCompact?.(event.data)
              break
            case 'compact_done':
              handlers.onCompactDone?.(event.data)
              break
            case 'team_progress':
              handlers.onTeamProgress?.(JSON.parse(event.data))
              break
            case 'done':
              doneReceived = true
              handlers.onDone?.()
              return
            case 'error':
              handlers.onError?.(event.data)
              return
          }
        } catch (e) { console.warn('SSE parse error:', e) }
      }
    }
    if (!doneReceived) handlers.onError?.('Connection closed unexpectedly')
  }).catch((err) => {
    if (err.name !== 'AbortError') {
      handlers.onError?.(err.message)
    }
  })

  return () => controller.abort()
}

export const API = {
  setToken,

  // Auth
  login: (username, password) => fetchJSON('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  }),
  register: (username, password, nickname, invite_code) => fetchJSON('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, password, nickname, ...(invite_code ? { invite_code } : {}) }),
  }),
  getMe: () => fetchJSON('/api/auth/me'),
  updateProfile: (data) => fetchJSON('/api/auth/profile', {
    method: 'PUT',
    body: JSON.stringify(data),
  }),

  // Invite Code
  getInviteCode: () => fetchJSON('/api/invite-code'),
  listInviteCodes: () => fetchJSON('/api/invite-codes'),

  // Sessions
  listSessions: () => fetchJSON('/api/sessions'),
  getSessionMessages: (id, opts = {}) => fetchJSON(`/api/sessions/${id}/messages`, opts),
  updateSession: (id, data) => fetchJSON(`/api/sessions/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  deleteSession: (id) => fetchJSON(`/api/sessions/${id}`, { method: 'DELETE' }),

  // Agents
  listAgents: () => fetchJSON('/api/agents'),
  getAgent: (id) => fetchJSON(`/api/agents/${id}`),
  createAgent: (data) => fetchJSON('/api/agents', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  updateAgent: (id, data) => fetchJSON(`/api/agents/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  deleteAgent: (id) => fetchJSON(`/api/agents/${id}`, { method: 'DELETE' }),

  // Teams
  listTeams: () => fetchJSON('/api/teams'),
  getTeam: (id) => fetchJSON(`/api/teams/${id}`),
  createTeam: (data) => fetchJSON('/api/teams', {
    method: 'POST',
    body: JSON.stringify(data),
  }),
  updateTeam: (id, data) => fetchJSON(`/api/teams/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  }),
  deleteTeam: (id) => fetchJSON(`/api/teams/${id}`, { method: 'DELETE' }),

  // Tools
  listTools: () => fetchJSON('/api/tools'),

  // Upload
  upload: async (file) => {
    const fd = new FormData()
    fd.append('file', file)
    const headers = {}
    if (authToken) {
      headers['Authorization'] = `Bearer ${authToken}`
    }
    const res = await fetch(`${API_BASE}/api/upload`, {
      method: 'POST',
      headers,
      body: fd,
    })
    if (res.status === 401) {
      setToken(null)
      window.location.href = '/login'
      throw new Error('unauthorized')
    }
    if (!res.ok) throw new Error('Upload failed')
    return res.json()
  },

  health: () => fetchJSON('/api/health'),

  // Model Providers
  listProviders: () => fetchJSON('/api/user/providers'),
  createProvider: (data) => fetchJSON('/api/user/providers', { method: 'POST', body: JSON.stringify(data) }),
  updateProvider: (id, data) => fetchJSON(`/api/user/providers/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteProvider: (id) => fetchJSON(`/api/user/providers/${id}`, { method: 'DELETE' }),
  listAvailableModels: () => fetchJSON('/api/models/available'),

  // Skills
  listSkills: () => fetchJSON('/api/skills'),
  createSkill: (data) => fetchJSON('/api/skills', { method: 'POST', body: JSON.stringify(data) }),
  updateSkill: (id, data) => fetchJSON(`/api/skills/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteSkill: (id) => fetchJSON(`/api/skills/${id}`, { method: 'DELETE' }),
  createBuiltinSkill: (data) => fetchJSON('/api/admin/skills', { method: 'POST', body: JSON.stringify(data) }),
  listBuiltinSkills: () => fetchJSON('/api/admin/skills'),

  // Gateway
  listGatewayConfig: () => fetchJSON('/api/gateway/config'),
  getGatewayConfig: (platform) => fetchJSON(`/api/gateway/config/${platform}`),
  setGatewayConfig: (platform, data) => fetchJSON(`/api/gateway/config/${platform}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteGatewayConfig: (platform) => fetchJSON(`/api/gateway/config/${platform}`, { method: 'DELETE' }),
  startGateway: (platform) => fetchJSON(`/api/gateway/${platform}/start`, { method: 'POST' }),
  stopGateway: (platform) => fetchJSON(`/api/gateway/${platform}/stop`, { method: 'POST' }),
  gatewayStatus: () => fetchJSON('/api/gateway/status'),
  getWechatQR: () => fetchJSON('/api/gateway/wechat/qr'),
  getWechatQRStatus: () => fetchJSON('/api/gateway/wechat/qr-status'),
}
