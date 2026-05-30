import { useState, useRef, useEffect, useCallback } from 'react'
 import { useParams, useNavigate, Link, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Markdown from 'react-markdown'
import { useDropzone } from 'react-dropzone'
import {
  Send, Image as ImageIcon, X, Loader2, Bot, User, Trash2,
  ArrowLeft, Settings, Plus, MessageSquare, ChevronLeft, ChevronRight,
  Globe, LogOut, Wrench, Zap, Brain, ImageIcon as ImgIcon,
  ChevronDown, ChevronUp, Clock,
} from 'lucide-react'
import { API, sseConnect } from '../api'
import { useAuth } from '../AuthContext'
import i18n from '../i18n'

const API_BASE = import.meta.env.VITE_API_BASE || ''

// ─── Sub-components for enhanced message rendering ───

function ToolCallBlock({ toolCall }) {
  const [expanded, setExpanded] = useState(false)
  const [showArgs, setShowArgs] = useState(false)
  const [showResult, setShowResult] = useState(false)

  const truncate = (str, len = 200) => {
    if (!str) return ''
    const s = typeof str === 'string' ? str : JSON.stringify(str, null, 2)
    return s.length > len ? s.slice(0, len) + '...' : s
  }

  return (
    <div className={`tool-call-block ${expanded ? 'expanded' : ''}`}>
      <div className="tool-call-header" role="button" tabIndex={0} onClick={() => setExpanded(!expanded)} onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setExpanded(!expanded) } }}>
        <div className="tool-call-title">
          <Wrench size={14} />
          <span className="tool-call-name">{toolCall.name}</span>
          {toolCall.duration && (
            <span className="tool-call-duration">
              <Clock size={10} /> {toolCall.duration}
            </span>
          )}
        </div>
        <div className="tool-call-toggle">
          {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
        </div>
      </div>
      {expanded && (
        <div className="tool-call-body">
          {toolCall.args && (
            <div className="tool-call-section">
              <div className="tool-call-section-header" role="button" tabIndex={0} onClick={() => setShowArgs(!showArgs)} onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setShowArgs(!showArgs) } }}>
                <span>Arguments</span>
                {showArgs ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
              </div>
              {showArgs && (
                <pre className="tool-call-code">{truncate(toolCall.args, 500)}</pre>
              )}
            </div>
          )}
          {toolCall.result && (
            <div className="tool-call-section">
              <div className="tool-call-section-header" role="button" tabIndex={0} onClick={() => setShowResult(!showResult)} onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setShowResult(!showResult) } }}>
                <span>Result</span>
                {showResult ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
              </div>
              {showResult && (
                <pre className={`tool-call-code ${toolCall.error ? 'error' : 'success'}`}>
                  {truncate(toolCall.result, 500)}
                </pre>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function SkillBadge({ skill }) {
  return (
    <span className="skill-badge">
      <span className="skill-badge-icon">{skill.icon || '🔧'}</span>
      <span className="skill-badge-name">{skill.name}</span>
    </span>
  )
}

function ThinkingBlock({ thinking }) {
  const [expanded, setExpanded] = useState(false)

  if (!thinking) return null

  return (
    <div className="thinking-block">
      <div className="thinking-header" role="button" tabIndex={0} onClick={() => setExpanded(!expanded)} onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setExpanded(!expanded) } }}>
        <Brain size={14} />
        <span>Thinking...</span>
        {expanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
      </div>
      {expanded && (
        <div className="thinking-body">
          <p>{thinking}</p>
        </div>
      )}
    </div>
  )
}

function ImageBlock({ image }) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className={`image-block ${expanded ? 'expanded' : ''}`} role="button" tabIndex={0} onClick={() => setExpanded(!expanded)} onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setExpanded(!expanded) } }}>
      <img src={image.url} alt={image.alt || 'Image'} />
      {expanded && (
        <div className="image-block-overlay" onClick={(e) => { e.stopPropagation(); setExpanded(false) }}>
          <img src={image.url} alt={image.alt || 'Image'} />
        </div>
      )}
    </div>
  )
}

// ─── Main Chat Component ───

export default function Chat() {
  const { t } = useTranslation()
  const { agentId } = useParams()
  const navigate = useNavigate()
  const { user, logout } = useAuth()

  const [agent, setAgent] = useState(null)
  const [allAgents, setAllAgents] = useState([])
  const [selectedAgents, setSelectedAgents] = useState([])
  const [teams, setTeams] = useState([])
  const [selectedTeam, setSelectedTeam] = useState(null)
  const [sessions, setSessions] = useState([])
  const [currentSession, setCurrentSession] = useState(null)
  const [messages, setMessages] = useState([])
  const [input, setInput] = useState('')
  const [images, setImages] = useState([])
  const [loading, setLoading] = useState(false)
  const [streaming, setStreaming] = useState(false)
  const [activeAgentName, setActiveAgentName] = useState('')
  const [compacting, setCompacting] = useState(false)
  const [sidebarOpen, setSidebarOpen] = useState(() => window.innerWidth > 768)

  const bottomRef = useRef(null)
  const inputRef = useRef(null)
  const currentSessionRef = useRef(null)
  const imagesRef = useRef([])
  const flashTimerRef = useRef(null) // not used in Chat but placeholder

  // Fix 1: Sync currentSessionRef with currentSession
  useEffect(() => {
    currentSessionRef.current = currentSession
  }, [currentSession])

  // Fix 3: Revoke blob URLs on unmount
  useEffect(() => {
    return () => {
      imagesRef.current.forEach(img => URL.revokeObjectURL(img.preview || img.url))
    }
  }, [])

  // imagesRef kept in sync with images state
  useEffect(() => {
    imagesRef.current = images
  }, [images])

  const [searchParams] = useSearchParams()

  useEffect(() => {
    const id = agentId || 'default'
    const teamIdFromUrl = searchParams.get('team_id')
    Promise.all([
      API.getAgent(id),
      API.listAgents(),
      API.listSessions(),
      API.listTeams(),
    ]).then(([agentData, agentsData, sessData, teamsData]) => {
      setAgent(agentData)
      setAllAgents(agentsData.agents || [])
      setSessions(sessData.sessions || [])
      const loadedTeams = teamsData.teams || []
      setTeams(loadedTeams)
      setSelectedAgents([id])
      // Fix 5: Team chat navigation from URL
      if (teamIdFromUrl) {
        const found = loadedTeams.find(t => t.id === teamIdFromUrl)
        if (found) setSelectedTeam(found.id)
      }
    }).catch(() => navigate('/'))
  }, [agentId, navigate, searchParams])

  // Fix 2: Race condition protection with AbortController
  // Fix 15: Loading indicator
  const [messagesLoading, setMessagesLoading] = useState(false)

  useEffect(() => {
    if (!currentSession) { setMessages([]); return }
    // Cancel previous request
    if (abortRef.current) abortRef.current()
    const controller = new AbortController()
    const { signal } = controller
    setMessagesLoading(true)

    API.getSessionMessages(currentSession.id, { signal }).then(data => {
      if (signal.aborted) return
      const raw = data.messages || []
      // Build a map of tool_call_id → result content from tool messages
      const toolResults = {}
      for (const m of raw) {
        if (m.role === 'tool' && m.tool_call_id) {
          toolResults[m.tool_call_id] = m.content
        }
      }
      const msgs = raw
        .filter(m => m.role !== 'system' && m.role !== 'tool')
        .map(m => {
          let tc = []
          if (m.tool_calls) {
            try { tc = typeof m.tool_calls === 'string' ? JSON.parse(m.tool_calls) : m.tool_calls } catch { tc = [] }
          }
          // Attach results from tool messages to tool calls
          tc = tc.map(t => ({
            ...t,
            name: t.function?.name || t.name || '',
            args: t.function?.arguments ? (typeof t.function.arguments === 'string' ? JSON.parse(t.function.arguments) : t.function.arguments) : (t.args || {}),
            result: toolResults[t.id] || t.result || null,
          }))
          return {
            role: m.role,
            content: m.content,
            name: m.name,
            id: m.id || `msg_${Date.now()}_${Math.random().toString(36).slice(2)}`,
            toolCalls: tc,
            skills: m.skills || [],
            thinking: m.thinking || '',
            images: m.images || [],
          }
        })
      setMessages(msgs)
    }).catch(err => {
      if (err.name !== 'AbortError') console.warn('Failed to load messages:', err)
    }).finally(() => {
      if (!signal.aborted) setMessagesLoading(false)
    })

    return () => controller.abort()
  }, [currentSession])
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streaming])

  const onDrop = useCallback(async (files) => {
    for (const file of files) {
      if (!file.type.startsWith('image/')) continue
      const preview = URL.createObjectURL(file)
      setImages(prev => [...prev, { file, preview, uploaded: null, uploading: true }])
      try {
        const result = await API.upload(file)
        setImages(prev => prev.map(img =>
          img.preview === preview ? { ...img, uploaded: result, uploading: false } : img
        ))
      } catch {
        setImages(prev => prev.map(img =>
          img.preview === preview ? { ...img, uploading: false, error: true } : img
        ))
      }
    }
  }, [])

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop, accept: { 'image/*': [] }, noClick: true, noKeyboard: true,
  })

  const removeImage = (idx) => {
    setImages(prev => {
      const img = prev[idx]
      if (img.preview) URL.revokeObjectURL(img.preview)
      return prev.filter((_, i) => i !== idx)
    })
  }

  const newChat = () => { setCurrentSession(null); setMessages([]) }

  // Fix 8: deleteSession error handling
  const deleteSession = async (id, e) => {
    e.stopPropagation()
    try {
      await API.deleteSession(id)
      setSessions(prev => prev.filter(s => s.id !== id))
      if (currentSession?.id === id) { setCurrentSession(null); setMessages([]) }
    } catch (err) {
      alert(t('error') + ': ' + err.message)
    }
  }

  const sendMessage = () => {
    if ((!input.trim() && images.length === 0) || loading || !agent) return

    const userMsg = {
      role: 'user', content: input.trim(),
      images: images.filter(i => i.uploaded).map(i => ({ filename: i.uploaded.filename, url: i.uploaded.url })),
      displayImages: images.filter(i => i.uploaded).map(i => i.uploaded.url || i.preview),
      _id: 'user_' + Date.now() + '_' + Math.random().toString(36).slice(2),
    }

    const newMessages = [...messages, userMsg]
    setMessages(newMessages)
    setInput('')
    // Fix 3: Defer blob URL revocation until after React renders
    const urlsToRevoke = images.map(i => i.preview)
    setImages([])
    setTimeout(() => urlsToRevoke.forEach(u => { if (u) URL.revokeObjectURL(u) }), 1000)
    setLoading(true)
    setStreaming(true)

    const assistantIdx = newMessages.length
    setMessages(prev => [...prev, {
      role: 'assistant', content: '', streaming: true, name: '',
      toolCalls: [], skills: [], thinking: '', images: [],
      _id: 'asst_' + Date.now() + '_' + Math.random().toString(36).slice(2),
    }])

    // Use a ref to track the current assistant message index
    // (it can shift when multi-agent mode adds new assistant messages)
    const currentAssistantIdx = { value: assistantIdx }

    abortRef.current = sseConnect('/api/chat', {
      messages: newMessages.map(m => ({ role: m.role, content: m.content, images: m.images || [] })),
      agent_ids: selectedTeam ? [] : selectedAgents,
      team_id: selectedTeam || undefined,
      session: currentSession?.id || undefined,
    }, {
      onSession: (sid) => {
        const newSession = { id: sid, title: userMsg.content.slice(0, 40), agent_ids: selectedAgents }
        setCurrentSession(newSession)
        setSessions(prev => [newSession, ...prev.filter(s => s.id !== sid)])
      },
      onDelta: (text) => {
        setMessages(prev => prev.map((m, i) => i === currentAssistantIdx.value ? { ...m, content: m.content + text } : m))
      },
      onAgentStart: (name) => {
        setActiveAgentName(name)
        // Fix 16: Move idx mutation outside state updater to avoid side effects
        let nextIdx = currentAssistantIdx.value
        setMessages(prev => {
          const last = prev[prev.length - 1]
          if (last?.role === 'assistant' && last.content) {
            nextIdx = prev.length
            return [...prev, { role: 'assistant', content: '', streaming: true, name, toolCalls: [], skills: [], thinking: '', images: [], _id: 'asst_' + Date.now() + '_' + Math.random().toString(36).slice(2) }]
          }
          return prev.map((m, i) => i === currentAssistantIdx.value ? { ...m, name } : m)
        })
        currentAssistantIdx.value = nextIdx
      },
      onAgentEnd: () => {
        setActiveAgentName('')
        setMessages(prev => prev.map((m, i) =>
          i === currentAssistantIdx.value ? { ...m, streaming: false } : m
        ))
      },
      onToolCall: (tc) => {
        setActiveAgentName(tc.agent || '')
        setMessages(prev => prev.map((m, i) => {
          if (i !== currentAssistantIdx.value) return m
          return { ...m, toolCalls: [...(m.toolCalls || []), { id: tc.id, name: tc.name, args: tc.args, result: null, duration: tc.duration || '' }] }
        }))
      },
      onToolResult: (tr) => {
        setMessages(prev => prev.map((m, i) => {
          if (i !== currentAssistantIdx.value) return m
          const toolCalls = [...(m.toolCalls || [])]
          const idx = toolCalls.findIndex(tc => tc.id === tr.id || (tc.name === tr.name && !tc.result))
          if (idx >= 0) {
            toolCalls[idx] = { ...toolCalls[idx], result: tr.content, duration: tr.duration_ms ? `${tr.duration_ms}ms` : toolCalls[idx].duration }
          }
          return { ...m, toolCalls }
        }))
      },
      onSkillActivate: (data) => {
        setMessages(prev => prev.map((m, i) => {
          if (i !== currentAssistantIdx.value) return m
          const skills = Array.isArray(data) ? data : [data]
          const existing = m.skills || []
          const newSkills = skills.filter(s => !existing.find(e => e.name === s.name))
          return { ...m, skills: [...existing, ...newSkills] }
        }))
      },
      onTitleUpdate: (title) => {
        // Fix 1: Use ref to avoid stale closure for new sessions
        const sid = currentSessionRef.current?.id || currentSession?.id
        if (sid) {
          setSessions(prev => prev.map(s => s.id === sid ? { ...s, title } : s))
        }
      },
      onCompact: () => {
        setCompacting(true)
      },
      onCompactDone: (msg) => {
        setCompacting(false)
      },
      onTeamProgress: (data) => {
        setMessages(prev => {
          const updated = [...prev]
          const last = updated[updated.length - 1]
          if (last?.role === 'assistant') {
            const progress = [...(last.teamProgress || [])]
            const existing = progress.findIndex(p => p.name === data.name && p.status === 'start' && data.status === 'done')
            if (existing >= 0) {
              progress[existing] = { ...progress[existing], status: 'done' }
            } else {
              progress.push(data)
            }
            updated[updated.length - 1] = { ...last, teamProgress: [...progress] }
          }
          return updated
        })
      },
      onImage: (data) => {
        setMessages(prev => prev.map((m, i) => {
          if (i !== currentAssistantIdx.value) return m
          return { ...m, images: [...(m.images || []), { url: data.url, alt: data.alt || '' }] }
        }))
      },
      onThinking: (text) => {
        setMessages(prev => prev.map((m, i) => {
          if (i !== currentAssistantIdx.value) return m
          return { ...m, thinking: (m.thinking || '') + text }
        }))
      },
      onDone: () => {
        setMessages(prev => prev.map(m => m.streaming ? { ...m, streaming: false } : m))
        setLoading(false); setStreaming(false); setActiveAgentName('')
        inputRef.current?.focus()
      },
      onError: (err) => {
        setMessages(prev => {
          const last = prev[prev.length - 1]
          if (last?.streaming) {
            return prev.map((m, i) => i === prev.length - 1 ? { ...m, content: m.content || `Error: ${err}`, streaming: false, error: true } : m)
          }
          return [...prev, { role: 'assistant', content: `Error: ${err}`, error: true }]
        })
        setLoading(false); setStreaming(false); setActiveAgentName('')
      },
    })
  }

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage() }
  }

  const toggleAgent = (id) => {
    setSelectedAgents(prev => prev.includes(id) ? prev.filter(a => a !== id) : [...prev, id])
  }

  const toggleLang = () => i18n.changeLanguage(i18n.language === 'zh' ? 'en' : 'zh')

  const abortRef = useRef(null)

  // Cleanup SSE connection on unmount
  useEffect(() => {
    return () => {
      abortRef.current?.()
    }
  }, [])

  if (!agent) return <div className="page"><div className="empty-state"><Loader2 size={32} className="spin" /></div></div>

  return (
    <div className="chat-layout" {...getRootProps()}>
      <input {...getInputProps()} />
      {isDragActive && <div className="drag-overlay"><p>{t('drop_image')}</p></div>}

      {/* Floating toggle when sidebar collapsed */}
      {!sidebarOpen && (
        <button className="sidebar-float-toggle" onClick={() => setSidebarOpen(true)} title={t('chats')}>
          <ChevronRight size={18} />
        </button>
      )}

      {sidebarOpen && <div className="sidebar-backdrop" onClick={() => setSidebarOpen(false)} />}
      <aside className={`sidebar ${sidebarOpen ? 'open' : 'collapsed'}`}>
        <div className="sidebar-header">
          <Link to="/" className="btn-icon" title={t('back')}><ArrowLeft size={18} /></Link>
          <span className="sidebar-title">{t('chats')}</span>
          <button className="btn-icon" onClick={() => setSidebarOpen(false)} title={t('close')}><ChevronLeft size={18} /></button>
          <button className="btn-icon" onClick={newChat} title={t('new_chat')}><Plus size={18} /></button>
        </div>

        <div className="sidebar-agents">
          <div className="sidebar-label">{t('agents_in_chat')}</div>
          <div className="agent-chips">
            {allAgents.map(a => (
              <button key={a.id}
                className={`agent-chip ${selectedAgents.includes(a.id) ? 'active' : ''} ${selectedTeam ? 'disabled' : ''}`}
                onClick={() => !selectedTeam && toggleAgent(a.id)}>
                {a.name}
              </button>
            ))}
          </div>
          {teams.length > 0 && (
            <>
              <div className="sidebar-label" style={{marginTop: '8px'}}>{t('teams')}</div>
              <div className="agent-chips">
                {teams.map(team => (
                  <button key={team.id}
                    className={`agent-chip team-chip ${selectedTeam === team.id ? 'active' : ''}`}
                    onClick={() => setSelectedTeam(selectedTeam === team.id ? null : team.id)}>
                    {team.name}
                  </button>
                ))}
              </div>
            </>
          )}
        </div>

        <div className="session-list">
          {sessions.map(sess => (
            <div key={sess.id}
              className={`session-item ${currentSession?.id === sess.id ? 'active' : ''}`}
              onClick={() => setCurrentSession(sess)}>
              <MessageSquare size={14} />
              <span className="session-title">{sess.title || t('new_chat')}</span>
              <button className="session-delete" onClick={(e) => deleteSession(sess.id, e)} title={t('delete')}>
                <Trash2 size={12} />
              </button>
            </div>
          ))}
          {sessions.length === 0 && <div className="session-empty">{t('no_conversations')}</div>}
        </div>

        <div className="sidebar-footer">
          <button className="btn-icon" onClick={toggleLang} title={t('language')}>
            <Globe size={16} />
          </button>
          <span className="sidebar-user">{user?.nickname || user?.username}</span>
          <button className="btn-icon" onClick={logout} title={t('logout')}>
            <LogOut size={16} />
          </button>
        </div>
      </aside>

      <div className="chat-main">
        <header className="chat-header">
          <button className="btn-icon sidebar-toggle" onClick={() => setSidebarOpen(!sidebarOpen)}>
            <ChevronLeft size={20} className={sidebarOpen ? 'rotated' : ''} />
          </button>
          <div className="header-center">
            <h1>{agent.name}</h1>
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
            <Link to={`/agents/${agent.id}/edit`} className="btn-icon" title={t('settings')}>
              <Settings size={18} />
            </Link>
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

                {/* Skills badges */}
                {msg.skills?.length > 0 && (
                  <div className="skill-badges">
                    {msg.skills.map((skill, j) => <SkillBadge key={j} skill={skill} />)}
                  </div>
                )}

                {/* Thinking block */}
                {msg.thinking && <ThinkingBlock thinking={msg.thinking} />}

                {/* Team progress */}
                {msg.teamProgress?.length > 0 && (
                  <div className="team-progress">
                    {msg.teamProgress.map((tp, j) => (
                      <div key={j} className={`team-progress-item ${tp.status}`}>
                        <span className="tp-status">{tp.status === 'done' ? '✅' : '⏳'}</span>
                        <span className="tp-name">{tp.name}</span>
                        {tp.role && <span className="tp-role">({tp.role})</span>}
                        {tp.task && <span className="tp-task">: {tp.task.slice(0, 60)}{tp.task.length > 60 ? '...' : ''}</span>}
                      </div>
                    ))}
                  </div>
                )}

                {/* Tool calls */}
                {msg.toolCalls?.length > 0 && (
                  <div className="tool-calls">
                    {msg.toolCalls.map((tc, j) => <ToolCallBlock key={j} toolCall={tc} />)}
                  </div>
                )}

                {/* Images */}
                {msg.images?.length > 0 && (
                  <div className="msg-inline-images">
                    {msg.images.map((img, j) => <ImageBlock key={j} image={img} />)}
                  </div>
                )}

                {/* User images */}
                {msg.displayImages?.length > 0 && (
                  <div className="msg-images">
                    {msg.displayImages.map((src, j) => (
                      <img key={j} src={src.startsWith('blob:') ? src : `${API_BASE}${src}`} alt="" />
                    ))}
                  </div>
                )}

                {msg.role === 'assistant' ? (
                  <div className="markdown-body">
                    <Markdown>{msg.content || (msg.streaming ? '...' : '')}</Markdown>
                    {msg.streaming && <span className="cursor">▊</span>}
                  </div>
                ) : <p>{msg.content}</p>}
                {msg.error && <p className="error-tag">{t('error')}</p>}
              </div>
            </div>
          ))}
          <div ref={bottomRef} />
        </main>

        <footer className="input-area">
          {images.length > 0 && (
            <div className="image-previews">
              {images.map((img, i) => (
                <div key={i} className={`preview-thumb ${img.error ? 'error' : ''}`}>
                  <img src={img.preview} alt="" />
                  {img.uploading && <div className="uploading"><Loader2 size={16} className="spin" /></div>}
                  <button className="remove-btn" onClick={() => removeImage(i)}><X size={12} /></button>
                </div>
              ))}
            </div>
          )}
          <div className="input-row">
            <label className="btn-icon upload-btn" title={t('upload_image')}>
              <ImageIcon size={20} />
              <input type="file" accept="image/*" multiple style={{ display: 'none' }}
                onChange={(e) => onDrop(Array.from(e.target.files))} />
            </label>
            <textarea ref={inputRef} value={input} onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={selectedAgents.length > 1 ? t('group_chat') + '...' : `${t('type_message')} ${agent.name}...`}
              rows={1} disabled={loading} />
            <button className="btn-icon send-btn" onClick={sendMessage}
              disabled={loading || (!input.trim() && images.length === 0)}>
              {loading ? <Loader2 size={20} className="spin" /> : <Send size={20} />}
            </button>
          </div>
        </footer>
      </div>
    </div>
  )
}
