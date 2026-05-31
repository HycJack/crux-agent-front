import { useState, useRef, useEffect, useCallback, useMemo } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Markdown from 'react-markdown'
import remarkMath from 'remark-math'
import remarkGfm from 'remark-gfm'
import rehypeKatex from 'rehype-katex'
import rehypeHighlight from 'rehype-highlight'
import mermaid from 'mermaid'
import 'katex/dist/katex.min.css'
import { useDropzone } from 'react-dropzone'
import {
  Send, Image as ImageIcon, X, Loader2, Bot, User, Trash2,
  Settings, Plus, MessageSquare,
  Globe, LogOut, Wrench, Zap, Brain, ImageIcon as ImgIcon,
  ChevronDown, ChevronUp, Clock, Square, Copy, RefreshCw, Maximize2, Download,
  Users, BotMessageSquare,
  PanelLeftClose, PanelLeftOpen,
} from 'lucide-react'
import { API } from '../api'
import { useAuth } from '../AuthContext'
import AgentModal from '../components/AgentModal'
import TeamModal from '../components/TeamModal'
import SettingsPage from './Settings'
import i18n from '../i18n'

const API_BASE = import.meta.env.VITE_API_BASE || ''

// Initialize mermaid with dark theme
mermaid.initialize({
  startOnLoad: false,
  theme: 'dark',
  themeVariables: {
    primaryColor: '#6c63ff',
    primaryTextColor: '#e0e0e0',
    primaryBorderColor: '#6c63ff',
    lineColor: '#888',
    secondaryColor: '#1e1e2e',
    tertiaryColor: '#2a2a3e',
    fontFamily: 'inherit',
  },
})

// Mermaid component
function MermaidBlock({ code, streaming, onZoom }) {
  const ref = useRef(null)
  const [error, setError] = useState(null)
  const isComplete = useMemo(() => {
    if (!streaming) return true
    if (!code) return false
    const lines = code.trim().split('\n')
    if (lines.length < 2) return false
    const firstLine = lines[0].trim().toLowerCase()
    const validStarters = ['graph', 'flowchart', 'sequencediagram', 'classDiagram', 'stateDiagram',
      'erDiagram', 'gantt', 'pie', 'mindmap', 'timeline', 'block-beta', 'sankey-beta']
    return validStarters.some(s => firstLine.startsWith(s))
  }, [code, streaming])

  useEffect(() => {
    if (!ref.current || !code || !isComplete) return
    const id = 'mermaid-' + Math.random().toString(36).slice(2, 10)
    const render = async () => {
      try {
        const { svg } = await mermaid.render(id, code)
        if (ref.current) { ref.current.innerHTML = svg; setError(null) }
      } catch (err) { setError(err.message || 'Failed to render diagram') }
    }
    render()
  }, [code, isComplete])

  if (!isComplete) return <pre className="mermaid-pending"><code>{code}</code></pre>
  if (error) return <pre className="mermaid-error">Mermaid error: {error}</pre>
  return (
    <div className="mermaid-wrapper">
      <div ref={ref} className="mermaid" />
      {onZoom && (
        <button className="mermaid-zoom-btn" onClick={() => onZoom(code)} title="放大查看">
          <Maximize2 size={14} />
        </button>
      )}
    </div>
  )
}

// Mermaid zoom modal
function MermaidModal({ code, onClose }) {
  const ref = useRef(null)
  const [error, setError] = useState(null)

  useEffect(() => {
    if (!ref.current || !code) return
    const id = 'mermaid-modal-' + Math.random().toString(36).slice(2, 10)
    const render = async () => {
      try {
        const { svg } = await mermaid.render(id, code)
        if (ref.current) ref.current.innerHTML = svg
      } catch (err) { setError(err.message || 'Failed to render') }
    }
    render()
  }, [code])

  const handleDownload = () => {
    if (!ref.current) return
    const svgEl = ref.current.querySelector('svg')
    if (!svgEl) return
    const clone = svgEl.cloneNode(true)
    const bbox = svgEl.getBoundingClientRect()
    clone.setAttribute('width', bbox.width)
    clone.setAttribute('height', bbox.height)
    const svgData = new XMLSerializer().serializeToString(clone)
    const canvas = document.createElement('canvas')
    const scale = 2
    canvas.width = bbox.width * scale
    canvas.height = bbox.height * scale
    const ctx = canvas.getContext('2d')
    ctx.scale(scale, scale)
    const img = new Image()
    img.onload = () => {
      ctx.fillStyle = '#1a1a2e'
      ctx.fillRect(0, 0, bbox.width, bbox.height)
      ctx.drawImage(img, 0, 0)
      const link = document.createElement('a')
      link.download = 'mermaid-diagram.png'
      link.href = canvas.toDataURL('image/png')
      link.click()
    }
    img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svgData)
  }

  return (
    <div className="mermaid-modal-overlay" onClick={onClose}>
      <div className="mermaid-modal" onClick={e => e.stopPropagation()}>
        <div className="mermaid-modal-header">
          <button className="mermaid-modal-btn" onClick={handleDownload} title="下载 PNG">
            <Download size={14} />
          </button>
          <button className="mermaid-modal-close" onClick={onClose}><X size={20} /></button>
        </div>
        {error ? <pre className="mermaid-error">{error}</pre> : <div ref={ref} className="mermaid-modal-content" />}
      </div>
    </div>
  )
}

// ─── Sub-components ───

function ToolCallBlock({ toolCall }) {
  const [expanded, setExpanded] = useState(false)
  const [showArgs, setShowArgs] = useState(false)
  const [showResult, setShowResult] = useState(false)
  const truncate = (str, len = 200) => {
    if (!str) return ''
    const s = typeof str === 'string' ? str : JSON.stringify(str, null, 2)
    return s.length > len ? s.slice(0, len) + '...' : s
  }
  // 适配后端格式: {id, type, function: {name, arguments}} 或直接 {name, args}
  const name = toolCall.function?.name || toolCall.name || 'unknown'
  const args = toolCall.function?.arguments || toolCall.args
  const result = toolCall.result
  const duration = toolCall.duration
  return (
    <div className={`tool-call-block ${expanded ? 'expanded' : ''}`}>
      <div className="tool-call-header" role="button" tabIndex={0} onClick={() => setExpanded(!expanded)}>
        <div className="tool-call-title">
          <Wrench size={14} />
          <span className="tool-call-name">{name}</span>
          {duration && <span className="tool-call-duration"><Clock size={10} /> {duration}</span>}
        </div>
        <div className="tool-call-toggle">{expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}</div>
      </div>
      {expanded && (
        <div className="tool-call-body">
          {args && (
            <div className="tool-call-section">
              <div className="tool-call-section-header" role="button" tabIndex={0} onClick={() => setShowArgs(!showArgs)}>
                <span>Arguments</span>{showArgs ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
              </div>
              {showArgs && <pre className="tool-call-code">{truncate(args, 500)}</pre>}
            </div>
          )}
          {result && (
            <div className="tool-call-section">
              <div className="tool-call-section-header" role="button" tabIndex={0} onClick={() => setShowResult(!showResult)}>
                <span>Result</span>{showResult ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
              </div>
              {showResult && <pre className="tool-call-code">{truncate(result, 500)}</pre>}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function SkillBadge({ skill }) {
  return <span className="skill-badge"><Zap size={10} /> {skill}</span>
}

// ─── Markdown renderer ───

function MarkdownContent({ content, streaming, onMermaidZoom }) {
  return (
    <Markdown
      remarkPlugins={[remarkMath, remarkGfm]}
      rehypePlugins={[rehypeKatex, rehypeHighlight]}
      components={{
        code({ node, inline, className, children, ...props }) {
          const match = /language-(\w+)/.exec(className || '')
          const codeStr = String(children).replace(/\n$/, '')
          if (!inline && match && match[1] === 'mermaid') {
            return <MermaidBlock code={codeStr} streaming={streaming} onZoom={onMermaidZoom} />
          }
          if (!inline && match) {
            return <pre className={className}><code {...props}>{children}</code></pre>
          }
          return <code className={className} {...props}>{children}</code>
        }
      }}
    >
      {content}
    </Markdown>
  )
}

// ─── Message actions ───

function MessageActions({ msg, onResend }) {
  const [copied, setCopied] = useState(false)
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(msg.content || '')
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch { /* ignore */ }
  }
  return (
    <div className="msg-actions">
      <button className="msg-action-btn" onClick={handleCopy} title="复制">
        {copied ? '✓' : <Copy size={13} />}
      </button>
      {msg.role === 'user' && onResend && (
        <button className="msg-action-btn" onClick={() => onResend(msg)} title="重新发送">
          <RefreshCw size={13} />
        </button>
      )}
    </div>
  )
}

// ─── Main Chat Component ───

export default function Chat() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { user, logout } = useAuth()
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const toggleLang = () => {
    const next = i18n.language === 'zh' ? 'en' : 'zh'
    i18n.changeLanguage(next)
    localStorage.setItem('crux-lang', next)
  }

  const [agent, setAgent] = useState(null)
  const [allAgents, setAllAgents] = useState([])
  const [selectedAgents, setSelectedAgents] = useState([])
  const [teams, setTeams] = useState([])
  const [selectedTeam, setSelectedTeam] = useState(null)
  const [sessions, setSessions] = useState([])
  const [currentSession, setCurrentSession] = useState(null)
  const [messages, setMessages] = useState([])
  const [messagesLoading, setMessagesLoading] = useState(false)
  const [input, setInput] = useState('')
  const [streaming, setStreaming] = useState(false)
  const [loading, setLoading] = useState(false)
  const [activeAgentName, setActiveAgentName] = useState('')
  const [compacting, setCompacting] = useState(false)
  const [sidebarOpen, setSidebarOpen] = useState(() => window.innerWidth > 768)
  const [agentModal, setAgentModal] = useState(false)
  const [teamModal, setTeamModal] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [mermaidCode, setMermaidCode] = useState(null)
  const [imageFile, setImageFile] = useState(null)
  const [imagePreview, setImagePreview] = useState(null)

  const streamingRef = useRef(false)
  const abortRef = useRef(null)
  const messagesEndRef = useRef(null)
  const textareaRef = useRef(null)

  // Load agents and teams
  const initialLoadDone = useRef(false)
  const loadAgents = useCallback(async () => {
    try {
      const resp = await API.get('/api/agents')
      const data = resp.agents || resp
      setAllAgents(data)
      if (!initialLoadDone.current && data.length > 0) {
        initialLoadDone.current = true
        const first = data[0]
        setAgent(first)
        setSelectedAgents([first.id])
      }
    } catch { /* ignore */ }
  }, [])

  const loadTeams = useCallback(async () => {
    try {
      const resp = await API.get('/api/teams')
      const data = resp.teams || resp
      setTeams(data)
    } catch { /* ignore */ }
  }, [])

  useEffect(() => { loadAgents(); loadTeams() }, [])

  // Reload agents when modal closes (in case user created/edited)
  useEffect(() => {
    if (!agentModal) loadAgents()
  }, [agentModal])

  useEffect(() => {
    if (!teamModal) loadTeams()
  }, [teamModal])

  // Load sessions
  const loadSessions = useCallback(async () => {
    try {
      const resp = await API.get('/api/sessions')
      const data = resp.sessions || resp
      setSessions(data)
      setCurrentSession(prev => {
        if (prev) return prev
        return data.length > 0 ? data[0] : null
      })
    } catch { /* ignore */ }
  }, [])

  useEffect(() => { loadSessions() }, [loadSessions])

  // Load messages when session changes
  useEffect(() => {
    if (!currentSession) { setMessages([]); return }
    setMessagesLoading(true)
    API.get(`/api/sessions/${currentSession.id}/messages`)
      .then(resp => {
        const data = resp.messages || resp
        // Parse tool_calls from JSON strings
        data.forEach(m => {
          if (typeof m.tool_calls === 'string') {
            try { m.tool_calls = JSON.parse(m.tool_calls || '[]') } catch { m.tool_calls = [] }
          }
          if (!Array.isArray(m.tool_calls)) m.tool_calls = []
        })
        // 构建 tool_call_id → result 映射
        const toolResults = {}
        data.filter(m => m.role === 'tool' && m.tool_call_id).forEach(m => {
          toolResults[m.tool_call_id] = m.content
        })
        // 过滤掉 tool 消息，合并 result 到 tool_call
        const filtered = data.filter(m => m.role !== 'tool')
        filtered.forEach(m => {
          if (m.tool_calls?.length) {
            m.tool_calls = m.tool_calls.map(tc => ({
              ...tc,
              name: tc.function?.name || tc.name || '',
              args: tc.function?.arguments || tc.args,
              result: toolResults[tc.id] || tc.result || null,
            }))
          }
        })
        // Merge consecutive assistant messages into one bubble
        const merged = []
        let lastAssistant = null
        for (const msg of filtered) {
          if (msg.role === 'assistant') {
            if (lastAssistant) {
              // Merge content
              if (msg.content) lastAssistant.content += msg.content
              // Merge tool_calls
              if (msg.tool_calls?.length) {
                lastAssistant.tool_calls = [...(lastAssistant.tool_calls || []), ...msg.tool_calls]
              }
            } else {
              lastAssistant = { ...msg }
              merged.push(lastAssistant)
            }
          } else {
            lastAssistant = null
            merged.push(msg)
          }
        }
        setMessages(merged)
      })
      .catch(() => setMessages([]))
      .finally(() => setMessagesLoading(false))
  }, [currentSession])

  // Auto-scroll
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  // New chat
  const newChat = async () => {
    let a = agent
    if (!a) {
      // Try loading agents first
      try {
        const resp = await API.get('/api/agents')
        const data = resp.agents || resp
        if (data.length > 0) {
          a = data[0]
          setAgent(a)
          setSelectedAgents([a.id])
          setAllAgents(data)
        }
      } catch { /* ignore */ }
    }
    if (!a) return
    try {
      const resp = await API.post('/api/sessions', { agent_id: a.id, title: t('new_chat') })
      const sess = resp.session || resp
      setSessions(prev => [sess, ...prev])
      setCurrentSession(sess)
      setMessages([])
    } catch { /* ignore */ }
  }

  // Delete session
  const deleteSession = async (id, e) => {
    e.stopPropagation()
    try {
      await API.delete(`/api/sessions/${id}`)
      setSessions(prev => prev.filter(s => s.id !== id))
      if (currentSession?.id === id) {
        setCurrentSession(null)
        setMessages([])
      }
    } catch { /* ignore */ }
  }

  // Switch agent
  const switchAgent = (a) => {
    setAgent(a)
    setSelectedAgents([a.id])
    setCurrentSession(null)
    setMessages([])
    setAgentModal(false)
  }

  const toggleAgent = (id) => {
    setSelectedAgents(prev => prev.includes(id) ? prev.filter(a => a !== id) : [...prev, id])
  }

  // Cleanup SSE on unmount
  useEffect(() => {
    return () => { abortRef.current?.() }
  }, [])

  // Image dropzone
  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    noClick: true,
    noKeyboard: true,
    accept: { 'image/*': ['.png', '.jpg', '.jpeg', '.gif', '.webp'] },
    onDrop: (files) => {
      if (files[0]) {
        if (imagePreview) URL.revokeObjectURL(imagePreview)
        setImageFile(files[0])
        setImagePreview(URL.createObjectURL(files[0]))
      }
    }
  })

  // Auto-resize textarea
  useEffect(() => {
    const el = textareaRef.current
    if (el) {
      el.style.height = 'auto'
      el.style.height = Math.min(el.scrollHeight, 200) + 'px'
    }
  }, [input])

  // Send message
  const sendMessage = async (textOverride) => {
    const text = (textOverride ?? input).trim()
    if ((!text && !imageFile) || loading || streaming || !agent) return

    setInput('')
    const userMsg = { role: 'user', content: text, _id: Date.now().toString() }

    // Handle image upload
    if (imageFile) {
      const reader = new FileReader()
      const base64 = await new Promise((resolve) => {
        reader.onload = () => resolve(reader.result)
        reader.readAsDataURL(imageFile)
      })
      userMsg.image_url = base64
      setImageFile(null)
      setImagePreview(null)
    }

    setMessages(prev => [...prev, userMsg])
    setLoading(true)
    setStreaming(true)
    streamingRef.current = true

    // Create session if needed
    let session = currentSession
    if (!session) {
      try {
        const resp = await API.post('/api/sessions', { agent_id: agent.id, title: text.slice(0, 30) })
        session = resp.session || resp
        setSessions(prev => [session, ...prev])
        setCurrentSession(session)
      } catch { setLoading(false); setStreaming(false); streamingRef.current = false; return }
    }

    // Build messages for API
    const apiMessages = [...messages, userMsg].map(m => ({
      role: m.role,
      content: m.content,
      ...(m.image_url ? { image_url: m.image_url } : {}),
      ...(m.tool_calls?.length ? { tool_calls: m.tool_calls } : {}),
    }))

    // Start streaming
    const controller = new AbortController()
    abortRef.current = () => controller.abort()

    let assistantMsg = { role: 'assistant', content: '', tool_calls: [], _id: 'stream-' + Date.now() }
    setMessages(prev => [...prev, assistantMsg])

    try {
      const res = await fetch(`${API_BASE}/api/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${localStorage.getItem('token')}` },
        body: JSON.stringify({
          session: session.id,
          messages: apiMessages,
          agent_ids: selectedAgents,
        }),
        signal: controller.signal,
      })

      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop()

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const data = line.slice(6)
          if (data === '[DONE]') continue

          try {
            const parsed = JSON.parse(data)
            const evt = parsed.event
            const evtData = parsed.data

            if (evt === 'delta' && evtData) {
              assistantMsg.content += evtData
              if (parsed.agent_name) setActiveAgentName(parsed.agent_name)
              setMessages(prev => {
                const updated = [...prev]
                updated[updated.length - 1] = { ...assistantMsg }
                return updated
              })
            } else if (evt === 'tool_call') {
              const tc = typeof evtData === 'string' ? JSON.parse(evtData) : evtData
              assistantMsg.tool_calls.push({ id: tc.id, name: tc.name, args: tc.args, result: null })
              setMessages(prev => {
                const updated = [...prev]
                updated[updated.length - 1] = { ...assistantMsg }
                return updated
              })
            } else if (evt === 'tool_result') {
              const tr = typeof evtData === 'string' ? JSON.parse(evtData) : evtData
              const tc = assistantMsg.tool_calls.find(t => t.id === tr.id)
              if (tc) tc.result = tr.content
              setMessages(prev => {
                const updated = [...prev]
                updated[updated.length - 1] = { ...assistantMsg }
                return updated
              })
            } else if (evt === 'title_update') {
              setSessions(prev => prev.map(s => s.id === session.id ? { ...s, title: evtData } : s))
            } else if (evt === 'session') {
              // Backend confirms/returns the actual session ID
              if (evtData && evtData !== session.id) {
                session = { ...session, id: evtData }
                setCurrentSession(session)
                setSessions(prev => {
                  if (prev.some(s => s.id === evtData)) return prev
                  return [session, ...prev]
                })
              }
            } else if (evt === 'compact') {
              setCompacting(true)
            } else if (evt === 'compact_done') {
              setCompacting(false)
            } else if (evt === 'error') {
              assistantMsg.content += `\n\n❌ ${evtData}`
              setMessages(prev => {
                const updated = [...prev]
                updated[updated.length - 1] = { ...assistantMsg }
                return updated
              })
            }
          } catch { /* skip parse errors */ }
        }
      }
    } catch (err) {
      if (err.name !== 'AbortError') {
        assistantMsg.content += `\n\n❌ Connection error`
        setMessages(prev => {
          const updated = [...prev]
          updated[updated.length - 1] = { ...assistantMsg }
          return updated
        })
      }
    } finally {
      setLoading(false)
      setStreaming(false)
      streamingRef.current = false
      setActiveAgentName('')
      setCompacting(false)
    }
  }

  // Resend message
  const resendMessage = (msg) => {
    const idx = messages.findIndex(m => m._id === msg._id)
    if (idx === -1) return
    setMessages(messages.slice(0, idx))
    sendMessage(msg.content)
  }

  // Stop streaming
  const stopStreaming = () => {
    abortRef.current?.()
    streamingRef.current = false
    setStreaming(false)
    setLoading(false)
    // Notify backend to stop processing
    if (currentSession?.id) {
      API.cancelChat(currentSession.id).catch(() => {})
    }
  }

  // Enter to send
  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      sendMessage()
    }
  }

  if (!agent) return <div className="page"><div className="empty-state"><Loader2 size={32} className="spin" /></div></div>

  return (
    <div className="chat-layout" {...getRootProps()}>
      <input {...getInputProps()} />
      {isDragActive && <div className="drag-overlay"><p>{t('drop_image')}</p></div>}

      {sidebarOpen && <div className="sidebar-backdrop" onClick={() => setSidebarOpen(false)} />}
      <aside className={`sidebar ${sidebarOpen ? 'open' : 'collapsed'}`}>
        <div className="sidebar-header">
          <button className="btn-icon sidebar-collapse-btn" onClick={() => setSidebarOpen(false)} title="收起">
            <PanelLeftClose size={18} />
          </button>
          <span className="sidebar-title">Crux</span>
          <button className="btn-icon sidebar-new-chat" onClick={newChat} title={t('new_chat')}>
            <Plus size={18} />
          </button>
        </div>

        {sidebarOpen && (
          <div className="sidebar-agents">
            <button className="sidebar-config-btn" onClick={() => setAgentModal(true)}>
              <BotMessageSquare size={16} />
              <span>{agent.name || t('agents')}</span>
            </button>
            <button className="sidebar-config-btn" onClick={() => setTeamModal(true)}>
              <Users size={16} />
              <span>{selectedTeam ? teams.find(t => t.id === selectedTeam)?.name || t('teams') : t('teams')}</span>
            </button>
          </div>
        )}

        <div className="session-list">
          {sidebarOpen && sessions.map(sess => (
            <div key={sess.id}
              className={`session-item ${currentSession?.id === sess.id ? 'active' : ''}`}
              onClick={() => {
                if (streamingRef.current) {
                  abortRef.current?.()
                  streamingRef.current = false
                  setStreaming(false)
                  setLoading(false)
                }
                setCurrentSession(sess)
              }}>
              <MessageSquare size={14} />
              <span className="session-title">{sess.title || t('new_chat')}</span>
              <button className="session-delete" onClick={(e) => deleteSession(sess.id, e)} title={t('delete')}>
                <Trash2 size={12} />
              </button>
            </div>
          ))}
          {sidebarOpen && sessions.length === 0 && <div className="session-empty">{t('no_conversations')}</div>}
        </div>

        <div className="sidebar-footer">
          <button className="sidebar-config-btn" onClick={() => setSettingsOpen(true)}>
            <Settings size={16} />
            {sidebarOpen && <span>{t('settings')}</span>}
          </button>
        </div>
      </aside>

      <div className="chat-main">
        <header className="chat-header">
          {!sidebarOpen && (
            <button className="btn-icon header-expand-btn" onClick={() => setSidebarOpen(true)} title="展开">
              <PanelLeftOpen size={18} />
            </button>
          )}
          <div className="header-center">
            {selectedAgents.length > 1 && (
              <span className="subtitle">
                {t('group_chat')}: {selectedAgents.map(id => allAgents.find(a => a.id === id)?.name || id).join(' + ')}
              </span>
            )}
            {streaming && activeAgentName && (
              <span className="subtitle streaming-indicator">
                <Loader2 size={12} className="spin" /> {t('thinking', { name: activeAgentName })}
              </span>
            )}
            {compacting && (
              <span className="subtitle compacting-indicator">
                <Loader2 size={12} className="spin" /> {t('compacting') || 'Compacting context...'}
              </span>
            )}
          </div>
          <div className="header-right">
            <button className="btn-icon" onClick={toggleLang} title={t('language')}>
              <Globe size={18} />
            </button>
            <div className="user-menu-wrapper">
              <button className="btn-icon user-menu-trigger" onClick={() => setUserMenuOpen(!userMenuOpen)}>
                <User size={18} />
              </button>
              {userMenuOpen && (
                <div className="user-dropdown" onClick={() => setUserMenuOpen(false)}>
                  <div className="user-dropdown-info">
                    <span className="user-dropdown-name">{user?.nickname || user?.username}</span>
                    <span className="user-dropdown-role">{user?.role}</span>
                  </div>
                  <button className="user-dropdown-item" onClick={logout}>
                    <LogOut size={14} /> {t('logout')}
                  </button>
                </div>
              )}
            </div>
          </div>
        </header>

        <main className="messages">
          {messagesLoading && (
            <div className="empty-state small" style={{ padding: '16px' }}>
              <Loader2 size={24} className="spin" />
            </div>
          )}
          {!messagesLoading && messages.length === 0 && (
            <div className="empty-state">
              <Bot size={48} />
              <h2>{agent.name}</h2>
              <p>{agent.description || t('type_message')}</p>
              {selectedAgents.length > 1 && (
                <p className="hint">{t('group_chat')}: {selectedAgents.length} {t('agents')}</p>
              )}
            </div>
          )}

          {messages.map((msg, i) => (
            <div key={msg._id || msg.id || i} className={`message ${msg.role}`}>
              <div className="avatar">
                {msg.role === 'user' ? <User size={18} /> : <Bot size={18} />}
              </div>
              <div className="bubble">
                {msg.role === 'assistant' && msg.name && <div className="agent-name-tag">{msg.name}</div>}
                {msg.skills?.length > 0 && (
                  <div className="skill-badges">
                    {msg.skills.map((skill, j) => <SkillBadge key={j} skill={skill} />)}
                  </div>
                )}
                {msg.image_url && (
                  <div className="message-image">
                    <img src={msg.image_url} alt="uploaded" />
                  </div>
                )}
                {msg.tool_calls?.length > 0 && (
                  <div className="tool-calls">
                    {msg.tool_calls.map((tc, j) => <ToolCallBlock key={j} toolCall={tc} />)}
                  </div>
                )}
                {msg.content && (
                  <div className="message-content">
                    <MarkdownContent content={msg.content} streaming={streaming && i === messages.length - 1} onMermaidZoom={setMermaidCode} />
                  </div>
                )}
                {msg.role === 'assistant' && !msg.content && streaming && i === messages.length - 1 && (
                  <div className="typing-indicator">
                    <span></span><span></span><span></span>
                  </div>
                )}
                <div className="bubble-footer">
                  {msg.created_at && <span className="msg-timestamp">{new Date(msg.created_at).toLocaleString()}</span>}
                  <MessageActions msg={msg} onResend={resendMessage} />
                </div>
              </div>
            </div>
          ))}
          <div ref={messagesEndRef} />
        </main>

        <footer className="input-area">
          {imagePreview && (
            <div className="image-preview">
              <img src={imagePreview} alt="preview" />
              <button className="image-remove" onClick={() => { if (imagePreview) URL.revokeObjectURL(imagePreview); setImageFile(null); setImagePreview(null) }}>
                <X size={14} />
              </button>
            </div>
          )}
          <div className="input-row">
            <button className="btn-icon" onClick={() => document.querySelector('.dropzone input').click()} title={t('upload_image')}>
              <ImgIcon size={20} />
            </button>
            <textarea
              ref={textareaRef}
              value={input}
              onChange={e => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={t('type_message')}
              rows={1}
              disabled={loading}
            />
            {streaming ? (
              <button className="btn-icon stop-btn" onClick={stopStreaming} title={t('stop')}>
                <Square size={20} />
              </button>
            ) : (
              <button className="btn-icon send-btn" onClick={sendMessage} disabled={!input.trim() && !imageFile} title={t('send')}>
                <Send size={20} />
              </button>
            )}
          </div>
        </footer>
      </div>

      {/* Modals */}
      {agentModal && (
        <AgentModal
          agents={allAgents}
          selectedAgents={selectedAgents}
          currentAgent={agent}
          onSelect={switchAgent}
          onToggle={toggleAgent}
          onClose={() => setAgentModal(false)}
        />
      )}
      {teamModal && (
        <TeamModal
          teams={teams}
          selectedTeam={selectedTeam}
          onSelect={(id) => { setSelectedTeam(id); setTeamModal(false) }}
          onClose={() => setTeamModal(false)}
        />
      )}
      {mermaidCode && <MermaidModal code={mermaidCode} onClose={() => setMermaidCode(null)} />}
      {settingsOpen && (
        <div className="settings-overlay">
          <div className="settings-overlay-header">
            <button className="btn-icon" onClick={() => setSettingsOpen(false)}>
              <X size={20} />
            </button>
          </div>
          <div className="settings-overlay-body">
            <SettingsPage />
          </div>
        </div>
      )}
    </div>
  )
}
