import { useEffect } from 'react'
import { useAbout } from '@/contexts/AboutContext'
import { AboutSection } from '@/components/about/AboutSection'
import { PageHeader } from '@/components/common/PageHeader'

// 关于页：版本信息、更新检查与项目地址。
export default function About() {
  const { markSeen } = useAbout()

  // 打开关于页即视为已读，侧边栏红点收起
  useEffect(() => {
    markSeen()
  }, [markSeen])

  return (
    <div className="space-y-6">
      <PageHeader title="关于" />
      <AboutSection />
    </div>
  )
}
