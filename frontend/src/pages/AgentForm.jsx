import { useState, useEffect } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Save, Bot } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { API } from '../api'

const DEFAULT_AGENT = {
  name: '',
  description: '',
  model: '',
  system_prompt: 'You are a helpful AI assistant.',
  temperature: 0.7,
  max_tokens: 4096,
  max_rounds: 10,
  tools: [],
}

export default function AgentForm() {
  const { t } = useTranslation()
  const { id } = useParams()
  const navigate = useNavigate()
  const isEdit = !!id

  const [form, setForm] = useState({ ...DEFAULT_AGENT })
  const [allTools, setAllTools] = useState([])
  const [availableModels, setAvailableModels] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    API.listTools().then(r => setAllTools(r.tools || [])).catch(err => console.warn('Failed to load tools:', err))
    API.listAvailableModels().then(r => setAvailableModels(r.models || [])).catch(err => console.warn('Failed to load models:', err))
   if (isEdit) {
      API.getAgent(id).then(agent => {
        setForm({
          name: agent.name || '',
          description: agent.description || '',
          model: agent.model || '',
          system_prompt: agent.system_prompt || '',
          temperature: agent.temperature ?? 0.7,
          max_tokens: agent.max_tokens ?? 4096,
          max_rounds: agent.max_rounds ?? 10,
          tools: agent.tools || [],
        })
      }).catch(err => setError(err.message))
    }
  }, [id, isEdit])

  const update = (key, value) => setForm(prev => ({ ...prev, [key]: value }))

  const toggleTool = (name) => {
    setForm(prev => ({
      ...prev,
      tools: prev.tools.includes(name)
        ? prev.tools.filter(t => t !== name)
        : [...prev.tools, name],
    }))
  }

  const selectAll = () => update('tools', allTools.map(t => t.name))
  const selectNone = () => update('tools', [])

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!form.name.trim()) {
      setError('Name is required')
      return
    }
    setLoading(true)
    setError('')
    try {
      if (isEdit) {
        await API.updateAgent(id, form)
      } else {
        await API.createAgent(form)
      }
      navigate('/')
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  // Group tools by prefix
  const toolGroups = {}
  allTools.forEach(t => {
    const group = t.name.split('_')[0] || 'other'
    if (!toolGroups[group]) toolGroups[group] = []
    toolGroups[group].push(t)
  })

  return (
    <div className="page">
      <header className="page-header">
        <button className="btn-icon" onClick={() => navigate('/')}>
          <ArrowLeft size={20} />
        </button>
        <h1>{isEdit ? t('edit_agent') : t('new_agent')}</h1>
      </header>

      <main className="form-page">
        <form onSubmit={handleSubmit} className="agent-form">
          {error && <div className="form-error">{error}</div>}

          <div className="form-section">
            <h2><Bot size={18} /> {t('basic_info')}</h2>

            <label>
              <span>{t('agent_name')} *</span>
              <input
                type="text"
                value={form.name}
                onChange={e => update('name', e.target.value)}
                placeholder="e.g. Code Assistant"
                required
              />
            </label>

            <label>
              <span>{t('agent_desc')}</span>
              <textarea
                value={form.description}
                onChange={e => update('description', e.target.value)}
                placeholder={t('agent_desc_placeholder')}
                rows={2}
              />
            </label>

            <label>
              <span>{t('agent_model')}</span>
              <select value={form.model} onChange={e => update('model', e.target.value)}>
                <option value="">{t('default')}</option>
                {Object.entries(
                  availableModels.reduce((acc, m) => {
                    if (!acc[m.provider_name]) acc[m.provider_name] = []
                    acc[m.provider_name].push(m)
                    return acc
                  }, {})
                ).map(([provider, models]) => (
                  <optgroup key={provider} label={provider}>
                    {models.map(m => (
                      <option key={m.full_id} value={m.full_id}>{m.model}</option>
                    ))}
                  </optgroup>
                ))}
              </select>
            </label>
          </div>

          <div className="form-section">
            <h2>{t('system_prompt')}</h2>
            <textarea
              value={form.system_prompt}
              onChange={e => update('system_prompt', e.target.value)}
              rows={6}
              className="mono"
              placeholder={t('system_prompt_placeholder')}
            />
          </div>

          <div className="form-section">
            <h2>{t('parameters')}</h2>
            <div className="form-row">
              <label>
                <span>{t('temperature')} ({form.temperature})</span>
                <input
                  type="range"
                  min="0"
                  max="2"
                  step="0.1"
                  value={form.temperature}
                  onChange={e => update('temperature', parseFloat(e.target.value))}
                />
              </label>
              <label>
                <span>{t('max_tokens')}</span>
                <input
                  type="number"
                  value={form.max_tokens}
                  onChange={e => update('max_tokens', parseInt(e.target.value) || 4096)}
                  min={256}
                  max={128000}
                />
              </label>
              <label>
                <span>{t('max_rounds')}</span>
                <input
                  type="number"
                  value={form.max_rounds}
                  onChange={e => update('max_rounds', parseInt(e.target.value) || 10)}
                  min={1}
                  max={50}
                />
              </label>
            </div>
          </div>

          <div className="form-section">
            <div className="section-header">
              <h2>{t('tools')} ({form.tools.length}/{allTools.length})</h2>
              <div className="section-actions">
                <button type="button" className="btn-link" onClick={selectAll}>{t('select_all')}</button>
                <button type="button" className="btn-link" onClick={selectNone}>{t('select_none')}</button>
              </div>
            </div>
            <div className="tool-grid">
              {Object.entries(toolGroups).map(([group, tools]) => (
                <div key={group} className="tool-group">
                  <h3>{group}</h3>
                  {tools.map(tool => (
                    <label key={tool.name} className={`tool-item ${form.tools.includes(tool.name) ? 'active' : ''}`}>
                      <input
                        type="checkbox"
                        checked={form.tools.includes(tool.name)}
                        onChange={() => toggleTool(tool.name)}
                      />
                      <span className="tool-name">{tool.name}</span>
                      <span className="tool-desc">{tool.description}</span>
                    </label>
                  ))}
                </div>
              ))}
            </div>
          </div>

          <div className="form-actions">
            <button type="button" className="btn" onClick={() => navigate('/')}>{t('cancel')}</button>
            <button type="submit" className="btn btn-primary" disabled={loading}>
              <Save size={16} />
              {loading ? t('saving') : isEdit ? t('save_changes') : t('new_agent')}
            </button>
          </div>
        </form>
      </main>
    </div>
  )
}
