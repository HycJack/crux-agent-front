import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './AuthContext'
import Login from './pages/Login'
import AgentList from './pages/AgentList'
import AgentForm from './pages/AgentForm'
import Teams from './pages/Teams'
import TeamForm from './pages/TeamForm'
import Chat from './pages/Chat'
import SettingsPage from './pages/Settings'
import './i18n'
import './App.css'

function ProtectedRoute({ children }) {
  const { isAuthenticated, loading } = useAuth()
  if (loading) return <div className="page"><div className="empty-state"><div className="spin" /></div></div>
  if (!isAuthenticated) return <Navigate to="/login" />
  return children
}

function AppRoutes() {
  const { isAuthenticated, loading } = useAuth()
  if (loading) return <div className="page"><div className="empty-state"><div className="spin" /></div></div>

  return (
    <Routes>
      <Route path="/login" element={isAuthenticated ? <Navigate to="/" /> : <Login />} />
      <Route path="/" element={<ProtectedRoute><AgentList /></ProtectedRoute>} />
      <Route path="/agents/new" element={<ProtectedRoute><AgentForm /></ProtectedRoute>} />
      <Route path="/agents/:id/edit" element={<ProtectedRoute><AgentForm /></ProtectedRoute>} />
      <Route path="/teams" element={<ProtectedRoute><Teams /></ProtectedRoute>} />
      <Route path="/teams/new" element={<ProtectedRoute><TeamForm /></ProtectedRoute>} />
      <Route path="/teams/:id/edit" element={<ProtectedRoute><TeamForm /></ProtectedRoute>} />
      <Route path="/chat/:agentId" element={<ProtectedRoute><Chat /></ProtectedRoute>} />
      <Route path="/chat" element={<ProtectedRoute><Chat /></ProtectedRoute>} />
      <Route path="/settings" element={<ProtectedRoute><SettingsPage /></ProtectedRoute>} />
      <Route path="*" element={<Navigate to="/" />} />
    </Routes>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </BrowserRouter>
  )
}
