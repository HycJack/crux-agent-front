import { useState, useEffect } from 'react'
import { X, Plus, Save, Trash2, Users, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { API } from '../api'

export default function TeamModal({ onClose }) {
  const { t } = useTranslation()
  const [teams, setTeams] = useState([])
  const [agents, setAgents] = useState([])
  const [editing, setEditing] = useState(null)
  const [form, setForm] = useState({ name: '', description: '', coordinator_id: '', members: [] })
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    loadTeams()
    API.listAgents().then(d => setAgents(d.agents || [])).catch(() => {})
  }, [])

  const loadTeams = () => API.listTeams().then(d => setTeams(d.teams || [])).catch(() => {})

  const startEdit = (team) => {
    if (team) {
      setForm({
        name: team.name || '', description: team.description || '',
        coordinator_id: team.coordinator_id || '', members: team.members || [],
      })
      setEditing(team)
    } else {
      setForm({ name: '', description: '', coordinator_id: agents[0]?.id || '', members: [] })
      setEditing({})
    }
  }

  const startEditId = async (id) => {
    try {
      const data = await API.getTeam(id)
      const team = data.team
      setForm({ name: team.name, description: team.description || '', coordinator_id: team.coordinator_id, members: team.members || [] })
      setEditing(team)
    } catch (err) { alert(err.message) }
  }

  const save = async () => {
    if (!form.name.trim() || !form.coordinator_id) return
    setSaving(true)
    try {
      const data = { ...form, members: form.members.filter(m => m.name?.trim()) }
      if (editing?.id) { await API.updateTeam(editing.id, data) }
      else { await API.createTeam(data) }
      await loadTeams()
      setEditing(null)
    } catch (err) { alert(err.message) }
    finally { setSaving(false) }
  }

  const deleteTeam = async (id, e) => {
    e.stopPropagation()
    if (!confirm(t('confirm_delete') || 'Delete?')) return
    try { await API.deleteTeam(id); await loadTeams() } catch (err) { alert(err.message) }
  }

  const addMember = () => setForm(f => ({ ...f, members: [...f.members, { agent_id: agents[0]?.id || '', name: '', role: '' }] }))
  const updateMember = (idx, field, value) => setForm(f => ({ ...f, members: f.members.map((m, i) => i === idx ? { ...m, [field]: value } : m) }))
  const removeMember = (idx) => setForm(f => ({ ...f, members: f.members.filter((_, i) => i !== idx) }))

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-container modal-lg" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <h2>{editing ? (editing.id ? t('edit_team') : t('new_team')) : t('teams')}</h2>
          <button className="btn-icon" onClick={onClose}><X size={18} /></button>
        </div>

        <div className="modal-body">
          {!editing ? (
            <>
              <div className="modal-list-header">
                <button className="btn btn-sm btn-primary" onClick={() => startEdit(null)}>
                  <Plus size={14} /> {t('new_team')}
                </button>
              </div>
              <div className="modal-list">
                {teams.map(team => (
                  <div key={team.id} className="modal-list-item">
                    <div className="modal-list-item-main" onClick={() => startEditId(team.id)}>
                      <Users size={16} />
                      <div>
                        <div className="item-name">{team.name}</div>
                        <div className="item-desc">{team.description || `${team.members?.length || 0} members`}</div>
                      </div>
                    </div>
                    <div className="modal-list-item-actions">
                      <button className="btn-icon" onClick={() => startEditId(team.id)} title={t('edit')}>
                        <Settings size={14} />
                      </button>
                      <button className="btn-icon danger" onClick={(e) => deleteTeam(team.id, e)} title={t('delete')}>
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </div>
                ))}
                {teams.length === 0 && <div className="modal-empty">No teams</div>}
              </div>
            </>
          ) : (
            <div className="modal-form">
              <label>
                <span>{t('team_name')} *</span>
                <input value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} required />
              </label>
              <label>
                <span>{t('team_desc')}</span>
                <textarea value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))} rows={2} />
              </label>
              <label>
                <span>{t('coordinator')} *</span>
                <select value={form.coordinator_id} onChange={e => setForm(f => ({ ...f, coordinator_id: e.target.value }))} required>
                  <option value="">{t('select_agent')}</option>
                  {agents.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
                </select>
              </label>

              <div className="form-section">
                <h3>{t('members')}</h3>
                {form.members.map((member, idx) => (
                  <div key={idx} className="member-row">
                    <select value={member.agent_id} onChange={e => updateMember(idx, 'agent_id', e.target.value)}>
                      <option value="">{t('select_agent')}</option>
                      {agents.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
                    </select>
                    <input placeholder={t('member_name')} value={member.name} onChange={e => updateMember(idx, 'name', e.target.value)} />
                    <input placeholder={t('member_role')} value={member.role} onChange={e => updateMember(idx, 'role', e.target.value)} />
                    <button type="button" className="btn-icon danger" onClick={() => removeMember(idx)}><Trash2 size={14} /></button>
                  </div>
                ))}
                <button type="button" className="btn btn-sm" onClick={addMember}>
                  <Plus size={14} /> {t('add_member')}
                </button>
              </div>
            </div>
          )}
        </div>

        {editing && (
          <div className="modal-footer">
            <button className="btn" onClick={() => setEditing(null)}>{t('cancel')}</button>
            <button className="btn btn-primary" onClick={save} disabled={saving}>
              <Save size={14} /> {saving ? t('saving') : t('save')}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
