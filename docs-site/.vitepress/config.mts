import { defineConfig } from 'vitepress'

// 站点基础配置。
// base 与仓库名保持一致：GitHub Pages 的项目站点路径是 /<repo>/。
// 若绑定自定义域名或改用用户站点（<user>.github.io），把 base 改为 '/'。
export default defineConfig({
  lang: 'zh-CN',
  title: 'Reminder',
  description: '自托管的个人提醒 / 通知系统：多渠道发送、公历与农历调度、确认闭环',
  base: '/reminder/',
  cleanUrls: true,
  lastUpdated: true,

  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/reminder/favicon.svg' }],
    ['meta', { name: 'theme-color', content: '#0f172a' }],
  ],

  themeConfig: {
    logo: '/logo.svg',

    nav: [
      { text: '指南', link: '/guide/introduction' },
      { text: '参考', link: '/reference/variables' },
      { text: '外部 API', link: '/api/external' },
      { text: '常见问题', link: '/faq' },
    ],

    sidebar: {
      '/guide/': [
        {
          text: '开始使用',
          items: [
            { text: '项目介绍', link: '/guide/introduction' },
            { text: '快速开始', link: '/guide/getting-started' },
            { text: '部署', link: '/guide/deployment' },
            { text: '配置项', link: '/guide/configuration' },
          ],
        },
        {
          text: '功能说明',
          items: [
            { text: '提醒与调度', link: '/guide/reminders' },
            { text: '通知渠道', link: '/guide/channels' },
            { text: '内容格式与占位符', link: '/guide/content-format' },
            { text: '确认机制', link: '/guide/confirmation' },
            { text: '日志与监控', link: '/guide/logs-and-monitoring' },
            { text: '令牌与外部调用', link: '/guide/tokens' },
          ],
        },
        {
          text: '其他',
          items: [
            { text: '架构说明', link: '/guide/architecture' },
            { text: '开发与测试', link: '/guide/development' },
          ],
        },
      ],
      '/reference/': [
        {
          text: '参考手册',
          items: [
            { text: '模板变量', link: '/reference/variables' },
            { text: '响应格式与错误码', link: '/reference/response-and-errors' },
            { text: '内置 API', link: '/reference/internal-api' },
            { text: '发送状态与重试', link: '/reference/delivery-status' },
          ],
        },
      ],
      '/api/': [
        {
          text: '外部 API',
          items: [
            { text: '概览与鉴权', link: '/api/external' },
            { text: '提醒接口', link: '/api/reminders' },
            { text: '通道接口', link: '/api/channels' },
          ],
        },
      ],
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/hausen1012/reminder' },
    ],

    outline: { level: [2, 3], label: '本页目录' },

    docFooter: { prev: '上一页', next: '下一页' },
    darkModeSwitchLabel: '主题',
    lightModeSwitchTitle: '切换到浅色',
    darkModeSwitchTitle: '切换到深色',
    sidebarMenuLabel: '目录',
    returnToTopLabel: '回到顶部',

    search: {
      provider: 'local',
      options: {
        translations: {
          button: { buttonText: '搜索文档', buttonAriaLabel: '搜索文档' },
          modal: {
            noResultsText: '没有找到结果',
            resetButtonTitle: '清除条件',
            footer: { selectText: '选择', navigateText: '切换', closeText: '关闭' },
          },
        },
      },
    },

    footer: {
      message: '基于 MIT 许可发布',
      copyright: 'Copyright © 2026 Reminder',
    },

    editLink: {
      pattern: 'https://github.com/hausen1012/reminder/edit/main/docs-site/:path',
      text: '在 GitHub 上编辑此页',
    },
  },
})
