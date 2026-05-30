import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Settings, Cpu, Puzzle, Shield, Plus, Trash2, Edit3, Save, X,
  Eye, EyeOff, Check, AlertCircle, Key, Globe, Zap, Server,
  Copy, Share2, Users,
} from 'lucide-react'
import { API } from '../api'
import { useAuth } from '../AuthContext'

const PROVIDER_TEMPLATES = [
  { name: 'OpenAI', base_url: 'https://api.openai.com/v1', models: ['gpt-4o', 'gpt-4o-mini', 'gpt-4-turbo', 'o1-mini'] },
  { name: 'DeepSeek', base_url: 'https://api.deepseek.com/v1', models: ['deepseek-chat', 'deepseek-coder', 'deepseek-reasoner'] },
  { name: 'Anthropic', base_url: 'https://api.anthropic.com/v1', models: ['claude-sonnet-4-20250514', 'claude-3.5-haiku-20241022'] },
  { name: 'Moonshot', base_url: 'https://api.moonshot.cn/v1', models: ['moonshot-v1-8k', 'moonshot-v1-32k', 'moonshot-v1-128k'] },
  { name: 'Zhipu', base_url: 'https://open.bigmodel.cn/api/paas/v4', models: ['glm-4-plus', 'glm-4-flash', 'glm-4-long'] },
  { name: 'Xiaomi MiMo', base_url: 'https://api.xiaomi.com/v1', models: ['mimo-v2.5-pro', 'mimo-v2.5-flash'] },
  { name: 'Qwen', base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1', models: ['qwen-max', 'qwen-plus', 'qwen-turbo'] },
]

export default function SettingsPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const [tab, setTab] = useState('providers')
  const [providers, setProviders] = useState([])
  const [skills, setSkills] = useState([])
  const [editingProvider, setEditingProvider] = useState(null)
  const [editingSkill, setEditingSkill] = useState(null)
  const [msg, setMsg] = useState(null)
  const flashTimerRef = useRef(null)

  useEffect(() => {
    return () => clearTimeout(flashTimerRef.current)
  }, [])

  const [inviteCode, setInviteCode] = useState(null)
  const [inviteCopied, setInviteCopied] = useState(false)

  useEffect(() => {
    API.listProviders().then(r => setProviders(r.providers || []))
    API.listSkills().then(r => setSkills(r.skills || []))
    API.getInviteCode().then(r => setInviteCode(r)).catch(() => {})
  }, [])

  const flash = (type, text) => {
    setMsg({ type, text })
    clearTimeout(flashTimerRef.current)
    flashTimerRef.current = setTimeout(() => setMsg(null), 3000)
  }

  const copyInviteLink = () => {
    if (!inviteCode?.code) return
    const link = `${window.location.origin}/login?code=${inviteCode.code}`
    navigator.clipboard.writeText(link).then(() => {
      setInviteCopied(true)
      flash('success', t('invite_link_copied'))
      setTimeout(() => setInviteCopied(false), 2000)
    })
  }

  const tabs = [
    { id: 'providers', icon: Cpu, label: t('model_providers') },
    { id: 'skills', icon: Puzzle, label: t('my_skills') },
    { id: 'gateway', icon: Globe, label: t('gateway') },
  ]
  if (user?.role === 'admin') {
    tabs.push({ id: 'admin', icon: Shield, label: t('admin_panel') })
  }

  return (
    <div className="page">
      <header className="page-header">
        <div className="header-left">
          <Settings size={28} />
          <div>
            <h1>{t('config_center')}</h1>
            <span className="subtitle">{t('config_center_desc')}</span>
          </div>
        </div>
      </header>

      {/* Invite Code Section */}
      {inviteCode && (
        <div className="invite-code-section">
          <div className="invite-code-header">
            <Key size={18} />
            <span className="invite-code-label">{t('my_invite_code')}</span>
          </div>
          <div className="invite-code-body">
            <code className="invite-code-value">{inviteCode.code}</code>
            <button className="btn btn-primary btn-sm" onClick={copyInviteLink}>
              {inviteCopied ? <Check size={14} /> : <Copy size={14} />}
              {inviteCopied ? t('invite_link_copied') : t('share_invite')}
            </button>
          </div>
          {inviteCode.used_count !== undefined && (
            <div className="invite-code-meta">
              <Users size={14} />
              <span>{inviteCode.used_count} {t('invite_code_used') || 'invited'}</span>
            </div>
          )}
        </div>
      )}

      {msg && (
        <div className={`flash-msg ${msg.type}`}>
          {msg.type === 'success' ? <Check size={16} /> : <AlertCircle size={16} />}
          {msg.text}
        </div>
      )}

      <div className="settings-tabs">
        {tabs.map(t2 => (
          <button key={t2.id} className={`tab-btn ${tab === t2.id ? 'active' : ''}`} onClick={() => setTab(t2.id)}>
            <t2.icon size={16} /> {t2.label}
          </button>
        ))}
      </div>

      <main className="settings-content">
        {tab === 'providers' && (
          <ProviderPanel providers={providers} setProviders={setProviders}
            editing={editingProvider} setEditing={setEditingProvider} flash={flash} t={t} />
        )}
        {tab === 'skills' && (
          <SkillPanel skills={skills.filter(s => !s.builtin)} setSkills={setSkills}
            editing={editingSkill} setEditing={setEditingSkill} flash={flash} t={t} />
        )}
        {tab === 'gateway' && (
          <GatewayPanel flash={flash} t={t} />
        )}
        {tab === 'admin' && user?.role === 'admin' && (
          <AdminPanel skills={skills.filter(s => s.builtin)} setSkills={setSkills}
            editing={editingSkill} setEditing={setEditingSkill} flash={flash} t={t} />
        )}
      </main>
    </div>
  )
}

// ─── Provider Panel ───

function ProviderPanel({ providers, setProviders, editing, setEditing, flash, t }) {
  const [form, setForm] = useState(null)

  const startNew = (template) => {
    setForm({
      name: template?.name || '',
      base_url: template?.base_url || '',
      api_key: '',
      models: template?.models?.join(', ') || '',
      default: template?.models?.[0] || '',
    })
    setEditing(null)
  }

  const startEdit = (p) => {
    setForm({
      name: p.name,
      base_url: p.base_url,
      api_key: '',  // don't show existing key
      models: p.models?.join(', ') || '',
      default: p.default || '',
    })
    setEditing(p)
  }

  const handleSave = async () => {
    if (!form.name || !form.base_url) { flash('error', t('fill_required')); return }
    const data = {
      name: form.name,
      base_url: form.base_url,
      models: form.models.split(',').map(m => m.trim()).filter(Boolean),
      default: form.default,
    }
    if (form.api_key) data.api_key = form.api_key

    try {
      if (editing) {
        await API.updateProvider(editing.id, data)
        flash('success', t('provider_updated'))
      } else {
        if (!form.api_key) { flash('error', t('fill_api_key')); return }
        data.api_key = form.api_key
        await API.createProvider(data)
        flash('success', t('provider_created'))
      }
      const r = await API.listProviders()
      setProviders(r.providers || [])
      setForm(null)
      setEditing(null)
    } catch (e) {
      flash('error', e.message)
    }
  }

  const handleDelete = async (id) => {
    if (!confirm(t('confirm_delete_provider'))) return
    try {
      await API.deleteProvider(id)
      setProviders(prev => prev.filter(p => p.id !== id))
      flash('success', t('provider_deleted'))
    } catch (e) {
      flash('error', e.message)
    }
  }

  return (
    <div>
      {!form && (
        <>
          <div className="section-header">
            <h3>{t('your_providers')}</h3>
            <div className="template-buttons">
              <button className="btn btn-primary btn-sm" onClick={() => startNew(null)}>
                <Plus size={14} /> {t('custom_provider')}
              </button>
            </div>
          </div>

          <div className="template-grid">
            {PROVIDER_TEMPLATES.map(tmpl => (
              <button key={tmpl.name} className="template-card" onClick={() => startNew(tmpl)}>
                <Zap size={18} />
                <span>{tmpl.name}</span>
                <small>{tmpl.models.length} {t('models')}</small>
              </button>
            ))}
          </div>

          <div className="item-list">
            {providers.map(p => (
              <div key={p.id} className={`item-card ${p.builtin ? 'builtin' : ''}`}>
                <div className="item-info">
                  <div className="item-title">
                    <Server size={16} />
                    {p.name}
                    {p.builtin && <span className="tag builtin">{t('builtin')}</span>}
                  </div>
                  <div className="item-meta">
                    <span className="tag">{p.base_url}</span>
                    <span className="tag"><Key size={12} /> {p.api_key ? '••••' + p.api_key.slice(-4) : '-'}</span>
                    <span className="tag">{p.models?.length || 0} {t('models')}</span>
                  </div>
                  <div className="item-models">
                    {p.models?.map(m => (
                      <span key={m} className={`model-tag ${m === p.default ? 'default' : ''}`}>{m}</span>
                    ))}
                  </div>
                </div>
                {!p.builtin && (
                  <div className="item-actions">
                    <button className="btn-icon" onClick={() => startEdit(p)}><Edit3 size={16} /></button>
                    <button className="btn-icon danger" onClick={() => handleDelete(p.id)}><Trash2 size={16} /></button>
                  </div>
                )}
              </div>
            ))}
            {providers.length === 0 && (
              <div className="empty-state small">
                <Cpu size={32} />
                <p>{t('no_providers')}</p>
              </div>
            )}
          </div>
        </>
      )}

      {form && (
        <div className="form-panel">
          <h3>{editing ? t('edit_provider') : t('add_provider')}</h3>
          <div className="form-grid">
            <label>
              <span>{t('provider_name')} *</span>
              <input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder="OpenAI" />
            </label>
            <label>
              <span>Base URL *</span>
              <input value={form.base_url} onChange={e => setForm({...form, base_url: e.target.value})} placeholder="https://api.openai.com/v1" />
            </label>
            <label>
              <span>API Key {editing ? '(留空不修改)' : '*'}</span>
              <input type="password" value={form.api_key} onChange={e => setForm({...form, api_key: e.target.value})} placeholder="sk-..." />
            </label>
            <label>
              <span>{t('model_list')} *</span>
              <input value={form.models} onChange={e => setForm({...form, models: e.target.value})} placeholder="gpt-4o, gpt-4o-mini" />
              <small>{t('comma_separated')}</small>
            </label>
            <label>
              <span>{t('default_model')}</span>
              <input value={form.default} onChange={e => setForm({...form, default: e.target.value})} placeholder="gpt-4o" />
            </label>
          </div>
          <div className="form-actions">
            <button className="btn btn-primary" onClick={handleSave}><Save size={14} /> {t('save')}</button>
            <button className="btn" onClick={() => { setForm(null); setEditing(null) }}><X size={14} /> {t('cancel')}</button>
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Skill Panel ───

function SkillPanel({ skills, setSkills, editing, setEditing, flash, t }) {
  return (
    <SkillPanelInner skills={skills} setSkills={setSkills} editing={editing} setEditing={setEditing} flash={flash} t={t} canEditBuiltin={false} />
  )
}

function AdminPanel({ skills, setSkills, editing, setEditing, flash, t }) {
  return (
    <div>
      <div className="section-header">
        <h3>{t('admin_skills')}</h3>
        <p className="hint">{t('admin_skills_desc')}</p>
      </div>
      <SkillPanelInner skills={skills} setSkills={setSkills} editing={editing} setEditing={setEditing} flash={flash} t={t} canEditBuiltin={true} />
    </div>
  )
}

function SkillPanelInner({ skills, setSkills, editing, setEditing, flash, t, canEditBuiltin }) {
  const [form, setForm] = useState(null)
  const [toolsList, setToolsList] = useState([])

  useEffect(() => {
    API.listTools().then(r => setToolsList(r.tools || []))
  }, [])

  const startNew = () => {
    setForm({ name: '', description: '', icon: '🔧', prompt: '', tools: [], env_vars: {}, trigger: '' })
    setEditing(null)
  }

  const startEdit = (s) => {
    setForm({
      name: s.name,
      description: s.description,
      icon: s.icon || '🔧',
      prompt: s.prompt,
      tools: s.tools || [],
      env_vars: s.env_vars || {},
      trigger: s.trigger || '',
    })
    setEditing(s)
  }

  const handleSave = async () => {
    if (!form.name) { flash('error', t('fill_required')); return }
    const data = { ...form }
    try {
      if (editing) {
        await API.updateSkill(editing.id, data)
        flash('success', t('skill_updated'))
      } else {
        if (canEditBuiltin) {
          await API.createBuiltinSkill(data)
        } else {
          await API.createSkill(data)
        }
        flash('success', t('skill_created'))
      }
      const r = await API.listSkills()
      setSkills(r.skills || [])
      setForm(null)
      setEditing(null)
    } catch (e) {
      flash('error', e.message)
    }
  }

  const handleDelete = async (id) => {
    if (!confirm(t('confirm_delete_skill'))) return
    try {
      await API.deleteSkill(id)
      setSkills(prev => prev.filter(s => s.id !== id))
      flash('success', t('skill_deleted'))
    } catch (e) {
      flash('error', e.message)
    }
  }

  const addEnvVar = () => {
    const key = prompt(t('env_var_name'))
    if (!key) return
    setForm({ ...form, env_vars: { ...form.env_vars, [key]: '' } })
  }

  const removeEnvVar = (key) => {
    const { [key]: _, ...rest } = form.env_vars
    setForm({ ...form, env_vars: rest })
  }

  if (form) {
    return (
      <div className="form-panel">
        <h3>{editing ? t('edit_skill') : t('create_skill')}</h3>
        <div className="form-grid">
          <div className="form-row-2">
            <label>
              <span>{t('skill_icon')}</span>
              <input value={form.icon} onChange={e => setForm({...form, icon: e.target.value})} style={{width:60,textAlign:'center',fontSize:24}} />
            </label>
            <label style={{flex:1}}>
              <span>{t('skill_name')} *</span>
              <input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder={t('skill_name_placeholder')} />
            </label>
          </div>
          <label>
            <span>{t('skill_desc')}</span>
            <input value={form.description} onChange={e => setForm({...form, description: e.target.value})} placeholder={t('skill_desc_placeholder')} />
          </label>
          <label>
            <span>{t('skill_prompt')}</span>
            <textarea value={form.prompt} onChange={e => setForm({...form, prompt: e.target.value})} rows={6}
              placeholder={t('skill_prompt_placeholder')} />
          </label>
          <label>
            <span>{t('skill_trigger')}</span>
            <input value={form.trigger} onChange={e => setForm({...form, trigger: e.target.value})} placeholder={t('skill_trigger_placeholder')} />
          </label>

          <div className="form-section">
            <div className="form-section-header">
              <span>{t('required_tools')}</span>
              <div>
                <button className="btn btn-xs" onClick={() => setForm({...form, tools: toolsList.map(t => t.name)})}>{t('select_all')}</button>
                <button className="btn btn-xs" onClick={() => setForm({...form, tools: []})}>{t('select_none')}</button>
              </div>
            </div>
            <div className="tool-checkboxes">
              {toolsList.map(tool => (
                <label key={tool.name} className={`tool-check ${form.tools.includes(tool.name) ? 'active' : ''}`}>
                  <input type="checkbox" checked={form.tools.includes(tool.name)}
                    onChange={e => {
                      if (e.target.checked) setForm({...form, tools: [...form.tools, tool.name]})
                      else setForm({...form, tools: form.tools.filter(t => t !== tool.name)})
                    }} />
                  <span>{tool.name}</span>
                </label>
              ))}
            </div>
            {form.tools.length > 0 && <small className="hint">{t('tools_count', { selected: form.tools.length, total: toolsList.length })}</small>}
          </div>

          <div className="form-section">
            <div className="form-section-header">
              <span>{t('env_vars')}</span>
              <button className="btn btn-xs" onClick={addEnvVar}><Plus size={12} /> {t('add_env_var')}</button>
            </div>
            <p className="hint">{t('env_vars_desc')}</p>
            {Object.entries(form.env_vars).map(([key, val]) => (
              <div key={key} className="env-var-row">
                <span className="env-key">{key}</span>
                <input type="password" value={val} onChange={e => setForm({...form, env_vars: {...form.env_vars, [key]: e.target.value}})} placeholder={t('env_var_value')} />
                <button className="btn-icon danger" onClick={() => removeEnvVar(key)}><Trash2 size={14} /></button>
              </div>
            ))}
          </div>
        </div>
        <div className="form-actions">
          <button className="btn btn-primary" onClick={handleSave}><Save size={14} /> {t('save')}</button>
          <button className="btn" onClick={() => { setForm(null); setEditing(null) }}><X size={14} /> {t('cancel')}</button>
        </div>
      </div>
    )
  }

  return (
    <div>
      <div className="section-header">
        <h3>{canEditBuiltin ? t('builtin_skills') : t('your_skills')}</h3>
        <button className="btn btn-primary btn-sm" onClick={startNew}>
          <Plus size={14} /> {t('create_skill')}
        </button>
      </div>
      <div className="item-list">
        {skills.map(s => (
          <div key={s.id} className="item-card">
            <div className="item-info">
              <div className="item-title">
                <span className="skill-icon">{s.icon || '🔧'}</span>
                {s.name}
                {s.builtin && <span className="tag builtin">{t('builtin')}</span>}
              </div>
              <p className="item-desc">{s.description}</p>
              <div className="item-meta">
                {s.tools?.length > 0 && <span className="tag">{s.tools.length} {t('tools')}</span>}
                {s.env_vars && Object.keys(s.env_vars).length > 0 && (
                  <span className="tag"><Key size={12} /> {Object.keys(s.env_vars).length} {t('env_vars')}</span>
                )}
                {s.prompt && <span className="tag">{t('has_prompt')}</span>}
              </div>
            </div>
            <div className="item-actions">
              <button className="btn-icon" onClick={() => startEdit(s)}><Edit3 size={16} /></button>
              {(!s.builtin || canEditBuiltin) && (
                <button className="btn-icon danger" onClick={() => handleDelete(s.id)}><Trash2 size={16} /></button>
              )}
            </div>
          </div>
        ))}
        {skills.length === 0 && (
          <div className="empty-state small">
            <Puzzle size={32} />
            <p>{canEditBuiltin ? t('no_builtin_skills') : t('no_skills')}</p>
          </div>
        )}
      </div>
    </div>
  )
}

// ─── Gateway Panel ───

const PLATFORM_DEFS = {
  telegram: {
    label: 'Telegram',
    icon: '✈️',
    fields: [
      { key: 'bot_token', label: 'Bot Token', placeholder: '123456:ABC-DEF...', type: 'password' },
    ],
  },
  feishu: {
    label: 'Feishu / Lark',
    icon: '🐦',
    fields: [
      { key: 'app_id', label: 'App ID', placeholder: 'cli_xxxxx' },
      { key: 'app_secret', label: 'App Secret', placeholder: 'xxxxx', type: 'password' },
      { key: 'verification_token', label: 'Verification Token', placeholder: 'xxxxx', type: 'password' },
    ],
  },
  wechat: {
    label: 'WeChat (iLink)',
    icon: '💬',
    fields: [
      { key: 'bot_token', label: 'Bot Token', placeholder: 'iLink bot token', type: 'password' },
      { key: 'session_key', label: 'Session Key', placeholder: 'auto-filled after QR login', type: 'password' },
    ],
  },
}

function GatewayPanel({ flash, t }) {
  const [configs, setConfigs] = useState({})
  const [status, setStatus] = useState({})
  const [editingPlatform, setEditingPlatform] = useState(null)
  const [form, setForm] = useState({})
  const [qrUrl, setQrUrl] = useState(null)
  const [qrStatus, setQrStatus] = useState(null)

  const refresh = async () => {
    try {
      const [cfgRes, statusRes] = await Promise.all([
        API.listGatewayConfig(),
        API.gatewayStatus(),
      ])
      setConfigs(cfgRes.configs || {})
      setStatus(statusRes.status || {})
    } catch (e) {
      // ignore
    }
  }

  useEffect(() => { refresh() }, [])

  const handleSave = async (platform) => {
    try {
      const existing = configs[platform] || { enabled: false, settings: {} }
      const newSettings = { ...existing.settings }
      const def = PLATFORM_DEFS[platform]
      for (const field of def.fields) {
        if (form[field.key] !== undefined && form[field.key] !== '') {
          newSettings[field.key] = form[field.key]
        }
      }
      await API.setGatewayConfig(platform, { enabled: existing.enabled, settings: newSettings })
      flash('success', t('gateway_config_saved'))
      setEditingPlatform(null)
      setForm({})
      refresh()
    } catch (e) {
      flash('error', e.message)
    }
  }

  const handleToggle = async (platform) => {
    const cfg = configs[platform]
    if (!cfg) return
    try {
      await API.setGatewayConfig(platform, { ...cfg, enabled: !cfg.enabled })
      refresh()
    } catch (e) {
      flash('error', e.message)
    }
  }

  const handleStart = async (platform) => {
    try {
      await API.startGateway(platform)
      flash('success', `${platform} started`)
      refresh()
    } catch (e) {
      flash('error', e.message)
    }
  }

  const handleStop = async (platform) => {
    try {
      await API.stopGateway(platform)
      flash('success', `${platform} stopped`)
      refresh()
    } catch (e) {
      flash('error', e.message)
    }
  }

  const handleDelete = async (platform) => {
    if (!confirm(t('confirm_delete_gateway', { platform }))) return
    try {
      await API.deleteGatewayConfig(platform)
      flash('success', t('gateway_config_deleted'))
      refresh()
    } catch (e) {
      flash('error', e.message)
    }
  }

  const startEdit = (platform) => {
    const cfg = configs[platform] || { enabled: false, settings: {} }
    setForm({ ...cfg.settings })
    setEditingPlatform(platform)
    setQrUrl(null)
    setQrStatus(null)
  }

  const handleGetQR = async () => {
    try {
      const res = await API.getWechatQR()
      setQrUrl(res.qr_url)
      setQrStatus('waiting')
    } catch (e) {
      flash('error', e.message)
    }
  }

  const handleCheckQR = async () => {
    try {
      const res = await API.getWechatQRStatus()
      setQrStatus(res.status)
      if (res.status === 'confirmed' && res.session_key) {
        setForm(prev => ({ ...prev, session_key: res.session_key }))
        flash('success', t('wechat_login_success'))
      }
    } catch (e) {
      flash('error', e.message)
    }
  }

  return (
    <div>
      <div className="section-header">
        <h3>{t('gateway_platforms')}</h3>
        <p className="hint">{t('gateway_desc')}</p>
      </div>

      <div className="item-list">
        {Object.entries(PLATFORM_DEFS).map(([platform, def]) => {
          const cfg = configs[platform]
          const st = status[platform] || {}
          const isRunning = st.running
          const isConfigured = !!cfg

          return (
            <div key={platform} className={`item-card ${isRunning ? 'running' : ''}`}>
              <div className="item-info">
                <div className="item-title">
                  <span style={{ fontSize: 20 }}>{def.icon}</span>
                  {def.label}
                  {isRunning && <span className="tag" style={{ background: '#22c55e', color: '#fff' }}>{t('running')}</span>}
                  {!isRunning && isConfigured && <span className="tag">{t('stopped')}</span>}
                  {!isConfigured && <span className="tag">{t('not_configured')}</span>}
                </div>
                <div className="item-meta">
                  {cfg?.enabled !== undefined && (
                    <span className="tag">{cfg.enabled ? t('enabled') : t('disabled')}</span>
                  )}
                </div>
              </div>
              <div className="item-actions">
                {isConfigured && (
                  <>
                    <button className="btn-icon" onClick={() => handleToggle(platform)} title={cfg?.enabled ? t('disable') : t('enable')}>
                      {cfg?.enabled ? '🔴' : '🟢'}
                    </button>
                    {cfg?.enabled && !isRunning && (
                      <button className="btn-icon" onClick={() => handleStart(platform)} title={t('start')}>▶️</button>
                    )}
                    {isRunning && (
                      <button className="btn-icon" onClick={() => handleStop(platform)} title={t('stop')}>⏹️</button>
                    )}
                  </>
                )}
                <button className="btn-icon" onClick={() => startEdit(platform)}><Edit3 size={16} /></button>
                {isConfigured && (
                  <button className="btn-icon danger" onClick={() => handleDelete(platform)}><Trash2 size={16} /></button>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {editingPlatform && (
        <div className="form-panel">
          <h3>{PLATFORM_DEFS[editingPlatform]?.icon} {t('configure')} {PLATFORM_DEFS[editingPlatform]?.label}</h3>
          <div className="form-grid">
            {PLATFORM_DEFS[editingPlatform]?.fields.map(field => (
              <label key={field.key}>
                <span>{field.label}</span>
                <input
                  type={field.type || 'text'}
                  value={form[field.key] || ''}
                  onChange={e => setForm({ ...form, [field.key]: e.target.value })}
                  placeholder={field.placeholder}
                />
              </label>
            ))}
          </div>

          {editingPlatform === 'wechat' && (
            <div style={{ marginTop: 16, padding: 16, background: 'var(--bg-secondary, #f5f5f5)', borderRadius: 8 }}>
              <h4>{t('wechat_qr_login')}</h4>
              <p className="hint">{t('wechat_qr_desc')}</p>
              <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                <button className="btn btn-sm" onClick={handleGetQR}>{t('get_qr_code')}</button>
                <button className="btn btn-sm" onClick={handleCheckQR}>{t('check_qr_status')}</button>
              </div>
              {qrUrl && (
                <div style={{ marginTop: 12 }}>
                  <img src={qrUrl} alt="QR Code" style={{ maxWidth: 200, border: '1px solid #ccc', borderRadius: 4 }} />
                  {qrStatus && <p className="hint">{t('qr_status')}: {qrStatus}</p>}
                </div>
              )}
            </div>
          )}

          <div className="form-actions">
            <button className="btn btn-primary" onClick={() => handleSave(editingPlatform)}>
              <Save size={14} /> {t('save')}
            </button>
            <button className="btn" onClick={() => { setEditingPlatform(null); setForm({}); setQrUrl(null) }}>
              <X size={14} /> {t('cancel')}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
