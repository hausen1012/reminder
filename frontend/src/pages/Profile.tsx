import { useState, useEffect, type FormEvent } from 'react'
import { Loader2, Github, BookOpen, ExternalLink } from 'lucide-react'
import { extractApiError, updatePassword } from '@/lib/api'
import { useConfig } from '@/contexts/ConfigContext'
import { COMMON_TIMEZONES, DOCS_URL, GITHUB_URL } from '@/lib/constants'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/use-toast'
import { AppLogo } from '@/components/common/AppLogo'
import { AISettingsSection } from '@/components/ai/AISettingsSection'
import { PageHeader } from '@/components/common/PageHeader'
import { cn } from '@/lib/utils'

// 设置页的分区导航：左侧列出设置项，右侧只展示当前选中项的表单
const SECTIONS = [
  {
    id: 'password',
    label: '修改密码',
    description: '请定期更换密码以确保安全',
  },
  {
    id: 'brand',
    label: '站点信息',
    description: '配置站点名称和浏览器标签图标',
  },
  {
    id: 'timezone',
    label: '时区',
    description: '决定所有提醒的时间如何解释与展示',
  },
  {
    id: 'ai',
    label: 'AI 助手',
    description: '开启后可用自然语言对话来创建通知通道与提醒；助手生成的写操作需要你确认后才生效',
  },
  {
    id: 'about',
    label: '关于项目',
    description: '开源仓库与完整文档',
  },
] as const

type SectionId = (typeof SECTIONS)[number]['id']

export default function Profile() {
  const [active, setActive] = useState<SectionId>('password')

  // --- 修改密码 ---
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [pwdMsg, setPwdMsg] = useState('')
  const [pwdError, setPwdError] = useState('')
  const [pwdLoading, setPwdLoading] = useState(false)

  async function handlePassword(e: FormEvent) {
    e.preventDefault()
    setPwdMsg('')
    setPwdError('')
    if (newPassword !== confirmPassword) {
      setPwdError('两次输入的新密码不一致')
      return
    }
    setPwdLoading(true)
    try {
      await updatePassword(oldPassword, newPassword)
      setPwdMsg('密码修改成功')
      setOldPassword('')
      setNewPassword('')
      setConfirmPassword('')
    } catch (err: unknown) {
      setPwdError(extractApiError(err, '修改失败'))
    } finally {
      setPwdLoading(false)
    }
  }

  // --- 站点设置 ---
  const { config, updateConfig } = useConfig()
  const { toast } = useToast()
  const [appName, setAppName] = useState('')
  const [logoSvg, setLogoSvg] = useState('')
  const [timezone, setTimezone] = useState('Asia/Shanghai')
  const [saving, setSaving] = useState(false)
  const [savingTz, setSavingTz] = useState(false)

  useEffect(() => {
    setAppName(config.app_name ?? '')
    setLogoSvg(config.logo_svg ?? '')
    setTimezone(config.timezone ?? 'Asia/Shanghai')
  }, [config])

  async function handleBrandSave(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    try {
      await updateConfig({
        app_name: appName,
        logo_svg: logoSvg,
      })
      toast({ title: '保存成功' })
    } catch {
      toast({ title: '保存失败', variant: 'destructive' })
    } finally {
      setSaving(false)
    }
  }

  async function handleTimezoneSave(e: FormEvent) {
    e.preventDefault()
    setSavingTz(true)
    try {
      await updateConfig({ timezone })
      toast({ title: '时区已更新', description: '所有提醒将按新时区重新计算下次触发时间' })
    } catch (err) {
      toast({ title: '保存失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setSavingTz(false)
    }
  }

  function handleLogoUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    if (file.type === 'image/svg+xml') {
      const reader = new FileReader()
      reader.onload = () => setLogoSvg(reader.result as string)
      reader.readAsText(file)
    } else if (file.type.startsWith('image/')) {
      const reader = new FileReader()
      reader.onload = () => setLogoSvg(reader.result as string)
      reader.readAsDataURL(file)
    }
  }

  function clearLogoSvg() {
    setLogoSvg('')
  }

  return (
    <div className="space-y-6">
      <PageHeader title="设置" />
      <div className="flex flex-col gap-6 md:flex-row md:gap-8">
        {/* 分区导航：移动端横向可滚动，桌面端纵向列表 */}
        <nav
          aria-label="设置分区"
          className="flex gap-1 overflow-x-auto pb-1 md:w-52 md:shrink-0 md:flex-col md:overflow-visible md:pb-0"
        >
          {SECTIONS.map((section) => {
            const isActive = active === section.id
            return (
              <button
                key={section.id}
                type="button"
                onClick={() => setActive(section.id)}
                aria-current={isActive ? 'page' : undefined}
                className={cn(
                  'shrink-0 rounded-md px-3 py-2 text-left text-sm transition-colors md:w-full',
                  isActive
                    ? 'bg-secondary font-medium text-secondary-foreground'
                    : 'text-muted-foreground hover:bg-secondary/50 hover:text-foreground',
                )}
              >
                {section.label}
              </button>
            )
          })}
        </nav>

        {/* 当前分区：所有分区都保持挂载，切换后未提交的输入不会丢失 */}
        <div className="min-w-0 flex-1 md:border-l md:pl-8">
          {SECTIONS.map((section) => (
            <div
              key={section.id}
              hidden={active !== section.id}
              className="space-y-6"
            >
              <div className="space-y-1.5">
                <h2 className="text-lg font-semibold leading-none tracking-tight">
                  {section.label}
                </h2>
                <p className="text-sm text-muted-foreground">{section.description}</p>
              </div>

              {/* 修改密码 */}
              {section.id === 'password' && (
                <form onSubmit={handlePassword} className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="old-password">原密码</Label>
                    <Input id="old-password" type="password" value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} required />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="new-password">新密码</Label>
                    <Input id="new-password" type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} required minLength={6} />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="confirm-password">确认新密码</Label>
                    <Input id="confirm-password" type="password" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} required minLength={6} />
                  </div>
                  {pwdMsg && <p className="text-sm text-green-600 dark:text-green-400 sm:col-span-2">{pwdMsg}</p>}
                  {pwdError && <p className="text-sm text-destructive sm:col-span-2">{pwdError}</p>}
                  <div className="sm:col-span-2">
                    <Button type="submit" size="sm" disabled={pwdLoading}>
                      {pwdLoading && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
                      {pwdLoading ? '修改中…' : '修改密码'}
                    </Button>
                  </div>
                </form>
              )}

              {/* 站点信息 */}
              {section.id === 'brand' && (
                <form onSubmit={handleBrandSave} className="space-y-5">
                  <div className="space-y-2">
                    <Label htmlFor="app-name">站点名称</Label>
                    <Input id="app-name" value={appName} onChange={(e) => setAppName(e.target.value)} placeholder="Reminder" />
                  </div>
                  <div className="space-y-2">
                    <Label>浏览器图标</Label>
                    <div className="flex items-center gap-3">
                      <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg border bg-background">
                        {logoSvg ? (
                          <AppLogo svg={logoSvg} className="h-8 w-8 object-contain" alt="图标" />
                        ) : (
                          <span className="text-xs text-muted-foreground">无</span>
                        )}
                      </div>
                      <div className="flex gap-2">
                        <Button type="button" variant="outline" size="sm" onClick={() => document.getElementById('logo-upload')?.click()}>
                          上传
                        </Button>
                        {logoSvg && (
                          <Button type="button" variant="outline" size="sm" onClick={clearLogoSvg}>
                            清除
                          </Button>
                        )}
                        <input id="logo-upload" type="file" accept="image/*,.svg" className="hidden" onChange={handleLogoUpload} />
                      </div>
                    </div>
                    <p className="text-xs text-muted-foreground">支持 PNG、JPG、SVG，将显示在浏览器标签和侧边栏</p>
                  </div>
                  <Button type="submit" size="sm" disabled={saving}>
                    {saving ? '保存中...' : '保存'}
                  </Button>
                </form>
              )}

              {/* 时区 */}
              {section.id === 'timezone' && (
                <form onSubmit={handleTimezoneSave} className="space-y-5">
                  <div className="space-y-2">
                    <Label htmlFor="site-tz">站点时区</Label>
                    <Select value={timezone} onValueChange={setTimezone}>
                      <SelectTrigger id="site-tz">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {COMMON_TIMEZONES.map((tz) => (
                          <SelectItem key={tz} value={tz}>
                            {tz}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <p className="text-xs text-muted-foreground">
                      提醒里填写的时间（如 09:00）按此时区解释。修改后所有提醒的下次触发时间会立即重算，无需重启。
                    </p>
                  </div>
                  <Button type="submit" size="sm" disabled={savingTz}>
                    {savingTz ? '保存中...' : '保存'}
                  </Button>
                </form>
              )}

              {/* AI 助手 */}
              {section.id === 'ai' && <AISettingsSection />}

              {/* 关于项目 */}
              {section.id === 'about' && (
                <div className="space-y-3">
                  <a
                    href={GITHUB_URL}
                    target="_blank"
                    rel="noreferrer noopener"
                    className="flex items-center gap-2 text-sm text-primary hover:underline"
                  >
                    <Github className="h-4 w-4" />
                    github.com/hausen1012/reminder
                    <ExternalLink className="h-3 w-3" />
                  </a>
                  <a
                    href={DOCS_URL}
                    target="_blank"
                    rel="noreferrer noopener"
                    className="flex items-center gap-2 text-sm text-primary hover:underline"
                  >
                    <BookOpen className="h-4 w-4" />
                    hausen1012.github.io/reminder
                    <ExternalLink className="h-3 w-3" />
                  </a>
                  <p className="text-xs text-muted-foreground">
                    遇到问题或有建议，欢迎到 GitHub 提 Issue；具体用法见文档站点。
                  </p>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
