import { useState, useEffect } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, Plus, Trash2, Save } from 'lucide-react'
import { API } from '../api'

export default function TeamForm() {
  const { t } = useTranslation()
  const { id } = useParams()
  const navigate = useNavigate()
  const isEdit = !!id

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [coordinatorId, setCoordinatorId] = useState('')
  const [members, setMembers] = useState([])
  const [agents, setAgents] = useState([])
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    API.listAgents().then(data => {
      const list = data.agents || []
      setAgents(list)
      if (!isEdit && list.length > 0 && !coordinatorId) {
        setCoordinatorId(list[0].id)
      }
    }).catch(err => console.warn('Failed to load agents:', err))
    if (isEdit) {
      API.getTeam(id).then(data => {
        const team = data.team
        setName(team.name)
        setDescription(team.description || '')
        setCoordinatorId(team.coordinator_id)
        setMembers(team.members || [])
      }).catch(err => console.warn('Failed to load team:', err))
    }
  }, [id, isEdit])

  const addMember = () => {
    setMembers([...members, { agent_id: agents[0]?.id || '', name: '', role: '' }])
  }

  const updateMember = (idx, field, value) => {
    setMembers(members.map((m, i) => i === idx ? { ...m, [field]: value } : m))
  }

  const removeMember = (idx) => {
    setMembers(members.filter((_, i) => i !== idx))
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!name.trim() || !coordinatorId) return
    setSaving(true)
    try {
      const data = {
        name: name.trim(),
        description: description.trim(),
        coordinator_id: coordinatorId,
        members: members.filter(m => m.name.trim()),
      }
      if (isEdit) {
        await API.updateTeam(id, data)
      } else {
        await API.createTeam(data)
      }
      navigate('/teams')
    } catch (err) {
      alert(err.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="page">
      <header className="page-header">
        <Link to="/teams" className="btn-icon"><ArrowLeft size={18} /></Link>
        <h1>{isEdit ? t('edit_team') : t('new_team')}</h1>
      </header>

      <div className="page-content">
        <form className="form" onSubmit={handleSubmit}>
          <div className="form-group">
            <label>{t('team_name')} *</label>
            <input value={name} onChange={e => setName(e.target.value)} required />
          </div>

          <div className="form-group">
            <label>{t('team_desc')}</label>
            <textarea value={description} onChange={e => setDescription(e.target.value)} rows={2} />
          </div>

          <div className="form-group">
            <label>{t('coordinator')} *</label>
            <select value={coordinatorId} onChange={e => setCoordinatorId(e.target.value)} required>
              <option value="">{t('select_agent')}</option>
              {agents.map(a => (
                <option key={a.id} value={a.id}>{a.name}</option>
              ))}
            </select>
          </div>

          <div className="form-group">
            <label>{t('members')}</label>
            {members.map((member, idx) => (
              <div key={idx} className="member-row">
                <select value={member.agent_id} onChange={e => updateMember(idx, 'agent_id', e.target.value)}>
                  <option value="">{t('select_agent')}</option>
                  {agents.map(a => (
                    <option key={a.id} value={a.id}>{a.name}</option>
                  ))}
                </select>
                <input
                  placeholder={t('member_name')}
                  value={member.name}
                  onChange={e => updateMember(idx, 'name', e.target.value)}
                />
                <input
                  placeholder={t('member_role')}
                  value={member.role}
                  onChange={e => updateMember(idx, 'role', e.target.value)}
                />
                <button type="button" className="btn-icon danger" onClick={() => removeMember(idx)}>
                  <Trash2 size={16} />
                </button>
              </div>
            ))}
            <button type="button" className="btn btn-sm" onClick={addMember}>
              <Plus size={14} /> {t('add_member')}
            </button>
          </div>

          <div className="form-actions">
            <button type="submit" className="btn btn-primary" disabled={saving}>
              <Save size={16} /> {saving ? t('loading') : t('save')}
            </button>
            <Link to="/teams" className="btn">{t('cancel')}</Link>
          </div>
        </form>
      </div>
    </div>
  )
}
