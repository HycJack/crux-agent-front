import { createContext, useContext, useState, useEffect } from 'react'
import { API } from './api'

const AuthContext = createContext(null)

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null)
  const [token, setToken] = useState(localStorage.getItem('token'))
  const [loading, setLoading] = useState(true)

  // Verify token on mount
  useEffect(() => {
    if (token) {
      API.setToken(token)
      API.getMe()
        .then(data => setUser(data.user))
        .catch(() => {
          localStorage.removeItem('token')
          setToken(null)
          API.setToken(null)
        })
        .finally(() => setLoading(false))
    } else {
      setLoading(false)
    }
  }, [token])

  const login = async (username, password) => {
    const data = await API.login(username, password)
    setUser(data.user)
    setToken(data.token)
    localStorage.setItem('token', data.token)
    API.setToken(data.token)
    return data
  }

  const register = async (username, password, nickname, inviteCode) => {
    const data = await API.register(username, password, nickname, inviteCode)
    setUser(data.user)
    setToken(data.token)
    localStorage.setItem('token', data.token)
    API.setToken(data.token)
    return data
  }

  const logout = () => {
    setUser(null)
    setToken(null)
    localStorage.removeItem('token')
    API.setToken(null)
  }

  const updateProfile = async (data) => {
    await API.updateProfile(data)
    setUser(prev => ({ ...prev, ...data }))
  }

  return (
    <AuthContext.Provider value={{
      user, token, loading,
      login, register, logout, updateProfile,
      isAuthenticated: !!token && !!user,
    }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be inside AuthProvider')
  return ctx
}
