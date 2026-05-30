import { useState, useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Plus, Edit, Trash2, Users, ArrowLeft, Globe, LogOut, ChevronRight } from 'lucide-react'
import { API } from '../api'
import { useAuth } from '../AuthContext'
import i18n from '../i18n'

export default function Teams() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { user, logout } = useAuth()
  const [teams, setTeams] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    API.listTeams().then(data => {
      setTeams(data.teams || [])
      setLoading(false)
    }).catch(() => setLoading(false))
  }, [])

  const deleteTeam = async (id, name) => {
    if (!window.confirm(t('confirm_delete_team', { name }))) return
    try {
      await API.deleteTeam(id)
      setTeams(prev => prev.filter(t => t.id !== id))
    } catch (err) {
      alert(t('error') + ': ' + err.message)
    }
  }

  const toggleLang = () => i18n.changeLanguage(i18n.language === 'zh' ? 'en' : 'zh')

  if (loading) return <div className="page"><div className="empty-state"><div className="spin" /></div></div>

  return (
    <div className="page">
      <header className="page-header">
        <Link to="/" className="btn-icon" title={t('back')}><ArrowLeft size={18} /></Link>
        <h1>{t('teams')}</h1>
        <div className="header-right">
          <button className="btn-icon" onClick={toggleLang} title={t('language')}>
            <Globe size={16} />
          </button>
          <span className="user-name">{user?.nickname || user?.username}</span>
          <button className="btn-icon" onClick={logout} title={t('logout')}>
            <LogOut size={16} />
          </button>
        </div>
      </header>

      <div className="page-content">
        <div className="action-bar">
          <Link to="/teams/new" className="btn btn-primary">
            <Plus size={16} /> {t('new_team')}
          </Link>
        </div>

        {teams.length === 0 ? (
          <div className="empty-state">
            <Users size={48} />
            <p>{t('no_teams')}</p>
          </div>
        ) : (
          <div className="card-list">
            {teams.map(team => (
              <div key={team.id} className="card">
                <div className="card-header">
                  <div className="card-title">
                    <Users size={18} />
                    <h3>{team.name}</h3>
                  </div>
                  <div className="card-actions">
                    <Link to={`/teams/${team.id}/edit`} className="btn-icon" title={t('edit')}>
                      <Edit size={16} />
                    </Link>
                    <button className="btn-icon danger" onClick={() => deleteTeam(team.id, team.name)} title={t('delete')}>
                      <Trash2 size={16} />
                    </button>
                  </div>
                </div>
                {team.description && <p className="card-desc">{team.description}</p>}
                <div className="card-meta">
                  <span className="badge">{t('coordinator')}: {team.coordinator_id}</span>
                  <span className="badge">{team.members?.length || 0} {t('members')}</span>
                </div>
                {team.members?.length > 0 && (
                  <div className="card-members">
                    {team.members.map((m, i) => (
                      <span key={i} className="member-chip">
                        <span className="member-name">{m.name}</span>
                        <span className="member-role">{m.role}</span>
                      </span>
                    ))}
                  </div>
                )}
                <button className="btn btn-sm" onClick={() => navigate(`/chat?team_id=${team.id}`)}>
                  <ChevronRight size={14} /> {t('chat')}
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
