import { useState, useEffect, useContext, type FormEvent } from 'react'
import { Loader2, User as UserIcon } from 'lucide-react'
import { extractApiError, updatePassword } from '@/lib/api'
import { useConfig } from '@/contexts/ConfigContext'
import { AuthContext } from '@/contexts/AuthContext'
import { COMMON_TIMEZONES } from '@/lib/constants'
import { formatInZone } from '@/lib/datetime'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card'
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

// 设置页分区：账户信息、基础设置、AI 助手
const SECTIONS = [
  {
    id: 'account',
    label: '账户信息',
    description: '管理当前登录账户与密码安全',
  },
  {
    id: 'general',
    label: '基础设置',
    description: '配置站点外观、站点地址与时区',
  },
  {
    id: 'ai',
    label: 'AI 助手',
    description: '使用自然语言创建提醒与通知渠道',
  },
] as const

type SectionId = (typeof SECTIONS)[number]['id']

export default function Profile() {
  const [active, setActive] = useState<SectionId>('account')
  const auth = useContext(AuthContext)
  const user = auth?.user

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
  // 站点对外地址：决定确认链接（/c/:token）用哪个域名拼接。
  // 用户点开链接的通常是手机，所以这个地址必须是手机能访问到的，不能是 localhost。
  const [siteUrl, setSiteUrl] = useState('')
  const [saving, setSaving] = useState(false)
  const [savingTz, setSavingTz] = useState(false)
  const [savingSiteUrl, setSavingSiteUrl] = useState(false)

  useEffect(() => {
    setAppName(config.app_name ?? '')
    setLogoSvg(config.logo_svg ?? '')
    setTimezone(config.timezone ?? 'Asia/Shanghai')
    setSiteUrl(config.site_url ?? '')
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

  async function handleSiteUrlSave(e: FormEvent) {
    e.preventDefault()
    setSavingSiteUrl(true)
    try {
      await updateConfig({ site_url: siteUrl })
      toast({
        title: '站点地址已更新',
        description: '之后发送的提醒会用新地址拼接确认链接',
      })
    } catch (err) {
      toast({ title: '保存失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setSavingSiteUrl(false)
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
        {/* 分区导航：移动端横向可滚动并吸顶，桌面端纵向列表 */}
        <nav
          aria-label="设置分区"
          className="sticky top-0 z-10 -mx-4 flex gap-1 overflow-x-auto bg-background px-4 pb-1.5 pt-1.5 md:static md:-ml-3 md:mx-0 md:w-52 md:shrink-0 md:flex-col md:overflow-visible md:bg-transparent md:px-0 md:pb-0 md:pt-0"
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
                <span className="inline-flex items-center gap-1.5">
                  {section.label}
                </span>
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

              {/* 账户信息 */}
              {section.id === 'account' && (
                <div className="space-y-6">
                  {/* 基本信息 */}
                  <Card>
                    <CardHeader>
                      <CardTitle className="text-base font-medium">基本信息</CardTitle>
                      <CardDescription>当前登录系统的账户与角色信息</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <div className="flex items-center gap-4">
                        <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-primary font-semibold text-lg">
                          {user?.username ? user.username.slice(0, 2).toUpperCase() : <UserIcon className="h-6 w-6" />}
                        </div>
                        <div className="space-y-1">
                          <div className="flex items-center gap-2">
                            <span className="font-medium text-foreground">{user?.username || 'admin'}</span>
                            <span className="rounded-full bg-secondary px-2 py-0.5 text-xs text-muted-foreground font-normal">
                              UID: {user?.id ?? 1}
                            </span>
                          </div>
                          <p className="text-xs text-muted-foreground">
                            注册时间：{user?.created_at ? formatInZone(user.created_at, timezone, 'datetime') : '—'}
                          </p>
                        </div>
                      </div>
                    </CardContent>
                  </Card>

                  {/* 修改密码 */}
                  <Card>
                    <CardHeader>
                      <CardTitle className="text-base font-medium">修改密码</CardTitle>
                      <CardDescription>定期更换密码以确保系统安全，长度不少于 6 位</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <form onSubmit={handlePassword} className="space-y-4 max-w-xl">
                        <div className="space-y-2">
                          <Label htmlFor="old-password">原密码</Label>
                          <Input
                            id="old-password"
                            type="password"
                            value={oldPassword}
                            onChange={(e) => setOldPassword(e.target.value)}
                            required
                            autoComplete="current-password"
                          />
                        </div>
                        <div className="grid gap-4 sm:grid-cols-2">
                          <div className="space-y-2">
                            <Label htmlFor="new-password">新密码</Label>
                            <Input
                              id="new-password"
                              type="password"
                              value={newPassword}
                              onChange={(e) => setNewPassword(e.target.value)}
                              required
                              minLength={6}
                              autoComplete="new-password"
                            />
                          </div>
                          <div className="space-y-2">
                            <Label htmlFor="confirm-password">确认新密码</Label>
                            <Input
                              id="confirm-password"
                              type="password"
                              value={confirmPassword}
                              onChange={(e) => setConfirmPassword(e.target.value)}
                              required
                              minLength={6}
                              autoComplete="new-password"
                            />
                          </div>
                        </div>
                        {pwdMsg && <p className="text-sm text-green-600 dark:text-green-400">{pwdMsg}</p>}
                        {pwdError && <p className="text-sm text-destructive">{pwdError}</p>}
                        <div>
                          <Button type="submit" size="sm" disabled={pwdLoading}>
                            {pwdLoading && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
                            {pwdLoading ? '修改中…' : '修改密码'}
                          </Button>
                        </div>
                      </form>
                    </CardContent>
                  </Card>
                </div>
              )}

              {/* 基础设置 */}
              {section.id === 'general' && (
                <div className="space-y-6">
                  {/* 站点信息 */}
                  <Card>
                    <CardHeader>
                      <CardTitle className="text-base font-medium">站点信息</CardTitle>
                      <CardDescription>配置站点名称和浏览器标签图标</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <form onSubmit={handleBrandSave} className="space-y-5 max-w-xl">
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
                    </CardContent>
                  </Card>

                  {/* 站点地址 */}
                  <Card>
                    <CardHeader>
                      <CardTitle className="text-base font-medium">站点地址</CardTitle>
                      <CardDescription>确认链接使用这个地址，必须能在手机上打开</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <form onSubmit={handleSiteUrlSave} className="space-y-5 max-w-xl">
                        <div className="space-y-2">
                          <Label htmlFor="site-url">对外访问地址</Label>
                          <Input
                            id="site-url"
                            value={siteUrl}
                            onChange={(e) => setSiteUrl(e.target.value)}
                            placeholder="https://reminder.example.com"
                            inputMode="url"
                          />
                          <p className="text-xs text-muted-foreground">
                            用于生成确认提醒的访问链接，请填写设备可访问的地址或域名。
                          </p>
                          {siteUrl && (
                            <p className="text-xs text-muted-foreground break-all">
                              预览：{siteUrl.replace(/\/+$/, '')}/c/example-token
                            </p>
                          )}
                        </div>
                        <Button type="submit" size="sm" disabled={savingSiteUrl}>
                          {savingSiteUrl ? '保存中...' : '保存'}
                        </Button>
                      </form>
                    </CardContent>
                  </Card>

                  {/* 时区设置 */}
                  <Card>
                    <CardHeader>
                      <CardTitle className="text-base font-medium">时区设置</CardTitle>
                      <CardDescription>决定所有提醒的时间如何解释与展示</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <form onSubmit={handleTimezoneSave} className="space-y-5 max-w-xl">
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
                            所有提醒均按此时区计算，修改后立即生效。
                          </p>
                        </div>
                        <Button type="submit" size="sm" disabled={savingTz}>
                          {savingTz ? '保存中...' : '保存'}
                        </Button>
                      </form>
                    </CardContent>
                  </Card>
                </div>
              )}

              {/* AI 助手 */}
              {section.id === 'ai' && <AISettingsSection />}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
