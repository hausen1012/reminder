// 关于信息（版本与更新状态）的全局状态。
//
// 侧边栏的「设置」红点与设置页的「关于」分区共用同一份数据：
// 登录后读一次（后端已缓存最近一次检查结论），进入关于分区时把当前最新版本标记为已读。
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react'
import { checkUpdate, getAboutInfo, setUpdateCheckEnabled } from '@/lib/api'
import { useAuth } from '@/contexts/AuthContext'
import type { AboutInfo } from '@/types'

/** 已读版本在 localStorage 中的键；值是最新版本号，如 v1.5.0。 */
const SEEN_VERSION_KEY = 'update_seen_version'

interface AboutContextValue {
  info: AboutInfo | null
  /** 是否还有未读的新版本（决定侧边栏红点） */
  hasUnreadUpdate: boolean
  /** 立即检查一次更新，返回最新状态 */
  checkNow: () => Promise<AboutInfo>
  /** 开关每日自动检查 */
  setEnabled: (enabled: boolean) => Promise<AboutInfo>
  /** 把当前最新版本标记为已读，红点随之消失 */
  markSeen: () => void
}

const AboutContext = createContext<AboutContextValue | undefined>(undefined)

function readSeenVersion(): string {
  try {
    return localStorage.getItem(SEEN_VERSION_KEY) ?? ''
  } catch {
    return ''
  }
}

export function AboutProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth()
  const [info, setInfo] = useState<AboutInfo | null>(null)
  const [seenVersion, setSeenVersion] = useState(readSeenVersion)

  // 登录后读一次缓存的结论；未登录时不请求
  useEffect(() => {
    if (!user) {
      setInfo(null)
      return
    }
    let cancelled = false
    getAboutInfo()
      .then((next) => {
        if (!cancelled) setInfo(next)
      })
      .catch(() => {
        // 关于信息不影响主流程，读取失败时保持无红点
      })
    return () => {
      cancelled = true
    }
  }, [user])

  const checkNow = useCallback(async () => {
    const next = await checkUpdate()
    setInfo(next)
    return next
  }, [])

  const setEnabled = useCallback(async (enabled: boolean) => {
    const next = await setUpdateCheckEnabled(enabled)
    setInfo(next)
    return next
  }, [])

  const markSeen = useCallback(() => {
    const latest = info?.update.latest_version
    if (!latest) return
    try {
      localStorage.setItem(SEEN_VERSION_KEY, latest)
    } catch {
      // 隐私模式下写入可能失败，本次会话内仍然收起红点
    }
    setSeenVersion(latest)
  }, [info])

  const latest = info?.update.update_available ? info.update.latest_version : ''
  const hasUnreadUpdate = latest !== '' && latest !== seenVersion

  return (
    <AboutContext.Provider value={{ info, hasUnreadUpdate, checkNow, setEnabled, markSeen }}>
      {children}
    </AboutContext.Provider>
  )
}

export function useAbout() {
  const ctx = useContext(AboutContext)
  if (!ctx) throw new Error('useAbout must be used within AboutProvider')
  return ctx
}
