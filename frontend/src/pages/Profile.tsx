import { useState, useEffect, type FormEvent } from 'react'
import { updatePassword } from '@/lib/api'
import { useConfig } from '@/contexts/ConfigContext'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useToast } from '@/components/ui/use-toast'

export default function Profile() {
  // --- 修改密码 ---
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [pwdMsg, setPwdMsg] = useState('')
  const [pwdError, setPwdError] = useState('')

  async function handlePassword(e: FormEvent) {
    e.preventDefault()
    setPwdMsg('')
    setPwdError('')
    if (newPassword !== confirmPassword) {
      setPwdError('两次输入的新密码不一致')
      return
    }
    try {
      await updatePassword(oldPassword, newPassword)
      setPwdMsg('密码修改成功')
      setOldPassword('')
      setNewPassword('')
      setConfirmPassword('')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message || '修改失败'
      setPwdError(msg)
    }
  }

  // --- 站点设置 ---
  const { config, updateConfig } = useConfig()
  const { toast } = useToast()
  const [appName, setAppName] = useState('')
  const [logoSvg, setLogoSvg] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setAppName(config.app_name ?? '')
    setLogoSvg(config.logo_svg ?? '')
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

  function isDataUrl(v: string) {
    return v.startsWith('data:image/')
  }

  return (
    <div className="space-y-6">
      <h1 className="text-3xl font-bold tracking-tight">设置</h1>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>修改密码</CardTitle>
            <CardDescription>请定期更换密码以确保安全</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handlePassword} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="old-password">原密码</Label>
                <Input
                  id="old-password"
                  type="password"
                  value={oldPassword}
                  onChange={(e) => setOldPassword(e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-password">新密码</Label>
                <Input
                  id="new-password"
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  required
                  minLength={6}
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
                />
              </div>
              {pwdMsg && <p className="text-sm text-green-600 dark:text-green-400">{pwdMsg}</p>}
              {pwdError && <p className="text-sm text-destructive">{pwdError}</p>}
              <Button type="submit" size="sm">修改密码</Button>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>品牌信息</CardTitle>
            <CardDescription>配置站点名称、描述和 Logo</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleBrandSave} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="app-name">站点名称</Label>
                <Input
                  id="app-name"
                  value={appName}
                  onChange={(e) => setAppName(e.target.value)}
                  placeholder="Reminder"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="logo-upload">Logo</Label>
                <Input
                  id="logo-upload"
                  type="file"
                  accept="image/*,.svg"
                  onChange={handleLogoUpload}
                />
                {logoSvg && (
                  <div className="mt-2 flex items-center justify-center rounded-lg border bg-background p-3">
                    {isDataUrl(logoSvg) ? (
                      <img src={logoSvg} alt="Logo" className="max-h-16 max-w-48 object-contain" />
                    ) : (
                      <div dangerouslySetInnerHTML={{ __html: logoSvg }} />
                    )}
                  </div>
                )}
              </div>
              <Button type="submit" size="sm" disabled={saving}>
                {saving ? '保存中...' : '保存'}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
