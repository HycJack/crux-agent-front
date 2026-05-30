import { useState, useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Plus, MessageSquare, Settings, Trash2, Bot, Zap, LogOut, Globe, User, SlidersHorizontal, Users } from 'lucide-react'
import { API } from '../api'
import { useAuth } from '../AuthContext'
import i18n from '../i18n'

export default function AgentList() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { user, logout } = useAuth()
  const [agents, setAgents] = useState([])
  const [health, setHealth] = useState(null)

  useEffect(() => {
    API.listAgents().then(r => setAgents(r.agents || []))
    API.health().then(setHealth).catch(() => {})
  }, [])

  const handleDelete = async (id, name, e) => {
    e.stopPropagation()
    if (!confirm(t('confirm_delete_agent', { name }))) return
    try {
      await API.deleteAgent(id)
      setAgents(prev => prev.filter(a => a.id !== id))
    } catch (err) {
      alert(t('error') + ': ' + err.message)
    }
  }

  const toggleLang = () => {
    i18n.changeLanguage(i18n.language === 'zh' ? 'en' : 'zh')
  }

  return (
    <div className="page">
      <header className="page-header">
        <div className="header-left">
          <Bot size={28} />
          <div>
            <h1>{t('app_name')}</h1>
            {health && (
              <span className="subtitle">
                {health.model} · {health.tools} {t('tools')}
              </span>
            )}
          </div>
        </div>
        <div className="header-right">
          <button className="btn-icon" onClick={toggleLang} title={t('language')}>
            <Globe size={18} />
          </button>
          <div className="user-badge">
            <User size={16} />
            <span>{user?.nickname || user?.username}</span>
          </div>
          <Link to="/settings" className="btn-icon" title={t('config_center')}>
            <SlidersHorizontal size={18} />
          </Link>
          <button className="btn-icon" onClick={logout} title={t('logout')}>
            <LogOut size={18} />
          </button>
          <Link to="/agents/new" className="btn btn-primary">
            <Plus size={16} /> {t('new_agent')}
          </Link>
          <Link to="/teams" className="btn">
            <Users size={16} /> {t('teams')}
          </Link>
        </div>
      </header>

      <main className="agent-grid">
        {agents.map(agent => (
          <div key={agent.id} className="agent-card" onClick={() => navigate(`/chat/${agent.id}`)}>
            <div className="agent-card-header">
              <div className="agent-avatar">
                <Zap size={20} />
              </div>
              <div className="agent-card-actions">
                <Link to={`/agents/${agent.id}/edit`} className="btn-icon"
                  onClick={e => e.stopPropagation()} title={t('edit')}>
                  <Settings size={16} />
                </Link>
                {!agent.builtin && (
                  <button className="btn-icon danger"
                    onClick={(e) => handleDelete(agent.id, agent.name, e)} title={t('delete')}>
                    <Trash2 size={16} />
                  </button>
                )}
              </div>
            </div>
            <h3>{agent.name}</h3>
            <p className="agent-desc">{agent.description || t('agent_desc')}</p>
            <div className="agent-meta">
              <span className="tag">{agent.model || 'default'}</span>
              <span className="tag">{agent.tools?.length || 0} {t('tools')}</span>
              {agent.builtin && <span className="tag builtin">{t('builtin')}</span>}
            </div>
            <button className="btn btn-chat" onClick={() => navigate(`/chat/${agent.id}`)}>
              <MessageSquare size={16} /> {t('chat')}
            </button>
          </div>
        ))}

        {agents.length === 0 && (
          <div className="empty-state">
            <Bot size={48} />
            <h2>{t('no_agents')}</h2>
            <p>{t('create_first')}</p>
            <Link to="/agents/new" className="btn btn-primary">
              <Plus size={16} /> {t('new_agent')}
            </Link>
          </div>
        )}
      </main>
    </div>
  )
}
