import { Component } from 'react'
import { AlertTriangle, RefreshCw } from 'lucide-react'

export default class ErrorBoundary extends Component {
  state = { error: null }

  static getDerivedStateFromError(error) {
    return { error }
  }

  render() {
    if (this.state.error) {
      return (
        <div style={{
          display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
          minHeight: '100vh', gap: 16, padding: 24, background: '#0f0f1a', color: '#e0e0e0',
          fontFamily: '-apple-system, BlinkMacSystemFont, sans-serif',
        }}>
          <AlertTriangle size={48} color="#ff6b6b" />
          <h2 style={{ margin: 0, fontSize: 20 }}>页面出错了</h2>
          <p style={{ margin: 0, color: '#888', fontSize: 14, maxWidth: 400, textAlign: 'center' }}>
            {this.state.error.message || '未知错误'}
          </p>
          <button
            onClick={() => { this.setState({ error: null }); window.location.reload() }}
            style={{
              display: 'flex', alignItems: 'center', gap: 8, padding: '8px 16px',
              border: '1px solid #444', borderRadius: 8, background: '#1a1a2e',
              color: '#e0e0e0', cursor: 'pointer', fontSize: 14,
            }}
          >
            <RefreshCw size={14} /> 刷新页面
          </button>
        </div>
      )
    }
    return this.props.children
  }
}
