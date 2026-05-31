import { useState, useEffect } from 'react'
import { X, Plus, Save, Trash2, Bot, ChevronRight, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { API } from '../api'

const DEFAULT_AGENT = {
  name: '', description: '', model: '', system_prompt: 'You are a helpful AI assistant.',
  temperature: 0.7, max_tokens: 4096, max_rounds: 10, tools: [],
}

export default function AgentModal({ onClose, onSelect, currentAgentId }) {
  const { t } = useTranslation()
  const [agents, setAgents] = useState([])
  const [editing, setEditing] = useState(null) // null = list, object = edit/new form
  const [form, setForm] = useState({ ...DEFAULT_AGENT })
  const [allTools, setAllTools] = useState([])
  const [availableModels, setAvailableModels] = useState([])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    loadAgents()
    API.listTools().then(r => setAllTools(r.tools || [])).catch(() => {})
    API.listAvailableModels().then(r => setAvailableModels(r.models || [])).catch(() => {})
  }, [])

  const loadAgents = () => API.listAgents().then(d => setAgents(d.agents || [])).catch(() => {})

  const startEdit = (agent) => {
    if (agent) {
      setForm({ ...DEFAULT_AGENT, ...agent })
      setEditing(agent)
    } else {
      setForm({ ...DEFAULT_AGENT })
      setEditing({})
    }
    setError('')
  }

  const startEditId = async (id) => {
    try {
      const agent = await API.getAgent(id)
      setForm({ ...DEFAULT_AGENT, ...agent })
      setEditing(agent)
      setError('')
    } catch (err) { setError(err.message) }
  }

  const save = async () => {
    if (!form.name.trim()) { setError('Name is required'); return }
    setSaving(true); setError('')
    try {
      if (editing?.id) {
        await API.updateAgent(editing.id, form)
      } else {
        await API.createAgent(form)
      }
      await loadAgents()
      setEditing(null)
    } catch (err) { setError(err.message) }
    finally { setSaving(false) }
  }

  const deleteAgent = async (id, e) => {
    e.stopPropagation()
    if (!confirm(t('confirm_delete') || 'Delete?')) return
    try { await API.deleteAgent(id); await loadAgents() } catch (err) { alert(err.message) }
  }

  const update = (key, value) => setForm(prev => ({ ...prev, [key]: value }))
  const toggleTool = (name) => setForm(prev => ({
    ...prev, tools: prev.tools.includes(name) ? prev.tools.filter(t => t !== name) : [...prev.tools, name],
  }))

  // Tool groups
  const toolGroups = {}
  allTools.forEach(t => {
    const group = t.name.split('_')[0] || 'other'
    if (!toolGroups[group]) toolGroups[group] = []
    toolGroups[group].push(t)
  })

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-container modal-lg" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <h2>{editing ? (editing.id ? t('edit_agent') : t('new_agent')) : t('agents')}</h2>
          <button className="btn-icon" onClick={onClose}><X size={18} /></button>
        </div>

        <div className="modal-body">
          {!editing ? (
            /* ── Agent List ── */
            <>
              <div className="modal-list-header">
                <button className="btn btn-sm btn-primary" onClick={() => startEdit(null)}>
                  <Plus size={14} /> {t('new_agent')}
                </button>
              </div>
              <div className="modal-list">
                {agents.map(a => (
                  <div key={a.id} className={`modal-list-item ${a.id === currentAgentId ? 'active' : ''}`}>
                    <div className="modal-list-item-main" onClick={() => { onSelect(a.id); onClose() }}>
                      <Bot size={16} />
                      <div>
                        <div className="item-name">{a.name}</div>
                        <div className="item-desc">{a.description || a.model || 'default'}</div>
                      </div>
                    </div>
                    <div className="modal-list-item-actions">
                      <button className="btn-icon" onClick={() => startEditId(a.id)} title={t('edit')}>
                        <Settings size={14} />
                      </button>
                      <button className="btn-icon danger" onClick={(e) => deleteAgent(a.id, e)} title={t('delete')}>
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </div>
                ))}
                {agents.length === 0 && <div className="modal-empty">No agents</div>}
              </div>
            </>
          ) : (
            /* ── Agent Edit Form ── */
            <div className="modal-form">
              {error && <div className="form-error">{error}</div>}

              <div className="form-section">
                <h3><Bot size={16} /> {t('basic_info')}</h3>
                <label>
                  <span>{t('agent_name')} *</span>
                  <input type="text" value={form.name} onChange={e => update('name', e.target.value)} required />
                </label>
                <label>
                  <span>{t('agent_desc')}</span>
                  <textarea value={form.description} onChange={e => update('description', e.target.value)} rows={2} />
                </label>
                <label>
                  <span>{t('agent_model')}</span>
                  <select value={form.model} onChange={e => update('model', e.target.value)}>
                    <option value="">{t('default')}</option>
                    {Object.entries(
                      availableModels.reduce((acc, m) => { (acc[m.provider_name] ??= []).push(m); return acc }, {})
                    ).map(([provider, models]) => (
                      <optgroup key={provider} label={provider}>
                        {models.map(m => <option key={m.full_id} value={m.full_id}>{m.model}</option>)}
                      </optgroup>
                    ))}
                  </select>
                </label>
              </div>

              <div className="form-section">
                <h3>{t('system_prompt')}</h3>
                <textarea value={form.system_prompt} onChange={e => update('system_prompt', e.target.value)}
                  rows={4} className="mono" />
              </div>

              <div className="form-section">
                <h3>{t('parameters')}</h3>
                <div className="form-row">
                  <label><span>{t('temperature')} ({form.temperature})</span>
                    <input type="range" min="0" max="2" step="0.1" value={form.temperature}
                      onChange={e => update('temperature', parseFloat(e.target.value))} />
                  </label>
                  <label><span>{t('max_tokens')}</span>
                    <input type="number" value={form.max_tokens} min={256} max={128000}
                      onChange={e => update('max_tokens', parseInt(e.target.value) || 4096)} />
                  </label>
                  <label><span>{t('max_rounds')}</span>
                    <input type="number" value={form.max_rounds} min={1} max={50}
                      onChange={e => update('max_rounds', parseInt(e.target.value) || 10)} />
                  </label>
                </div>
              </div>

              <div className="form-section">
                <div className="section-header">
                  <h3>{t('tools')} ({form.tools.length}/{allTools.length})</h3>
                  <div className="section-actions">
                    <button type="button" className="btn-link" onClick={() => update('tools', allTools.map(t => t.name))}>{t('select_all')}</button>
                    <button type="button" className="btn-link" onClick={() => update('tools', [])}>{t('select_none')}</button>
                  </div>
                </div>
                <div className="tool-grid compact">
                  {Object.entries(toolGroups).map(([group, tools]) => (
                    <div key={group} className="tool-group">
                      <h4>{group}</h4>
                      {tools.map(tool => (
                        <label key={tool.name} className={`tool-item ${form.tools.includes(tool.name) ? 'active' : ''}`}>
                          <input type="checkbox" checked={form.tools.includes(tool.name)} onChange={() => toggleTool(tool.name)} />
                          <span className="tool-name">{tool.name}</span>
                        </label>
                      ))}
                    </div>
                  ))}
                </div>
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
