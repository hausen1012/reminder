import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react'
import { getConfig, updateConfig as apiUpdateConfig } from '@/lib/api'

interface ConfigContextValue {
  config: Record<string, string>
  loading: boolean
  updateConfig: (cfg: Record<string, string>) => Promise<void>
}

const ConfigContext = createContext<ConfigContextValue | undefined>(undefined)

function applyBranding(cfg: Record<string, string>) {
  if (cfg.app_name) {
    document.title = cfg.app_name
  }
  const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (cfg.logo_svg) {
    const encoded = encodeURIComponent(cfg.logo_svg)
    const href = `data:image/svg+xml,${encoded}`
    if (!link) {
      const el = document.createElement('link')
      el.rel = 'icon'
      document.head.appendChild(el)
      el.href = href
    } else {
      link.href = href
    }
  } else if (link) {
    link.remove()
  }
}

export function ConfigProvider({ children }: { children: ReactNode }) {
  const [config, setConfig] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) {
      setLoading(false)
      return
    }
    getConfig()
      .then((cfg) => {
        setConfig(cfg)
        applyBranding(cfg)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const updateConfig = useCallback(async (cfg: Record<string, string>) => {
    const updated = await apiUpdateConfig(cfg)
    setConfig(updated)
    applyBranding(updated)
  }, [])

  return (
    <ConfigContext.Provider value={{ config, loading, updateConfig }}>
      {children}
    </ConfigContext.Provider>
  )
}

export function useConfig() {
  const ctx = useContext(ConfigContext)
  if (!ctx) throw new Error('useConfig must be used within ConfigProvider')
  return ctx
}
