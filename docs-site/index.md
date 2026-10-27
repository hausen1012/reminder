---
layout: home

hero:
  name: Reminder
  text: 自托管的个人提醒与通知系统
  tagline: 到点自动通过邮件、钉钉、企微或 Webhook 提醒你，点一下确认即完成闭环
  image:
    src: /logo.svg
    alt: Reminder
  actions:
    - theme: brand
      text: 快速开始
      link: /guide/getting-started
    - theme: alt
      text: 项目介绍
      link: /guide/introduction
    - theme: alt
      text: 外部 API
      link: /api/external
    - theme: alt
      text: GitHub
      link: https://github.com/hausen1012/reminder

features:
  - icon: ⏰
    title: 灵活调度
    details: 单次、周期（分/时/天/月/年）、Cron 表达式，公历与农历都支持，农历还能按农历月重复。
  - icon: 📮
    title: 多渠道通知
    details: SMTP 邮件、钉钉机器人、企业微信机器人、通用 Webhook，以及用于调试的日志通道。
  - icon: ✅
    title: 确认闭环
    details: 需要确认的提醒附带一次性链接，未点击则按配置自动重发，直到你确认或用完重试次数。
  - icon: 🛟
    title: 断点补发
    details: 进程重启期间错过的触发会被兜底扫描补发，超出容忍窗口则标记过期，不漏也不吵。
  - icon: 🧾
    title: 全程留痕
    details: 每次触发、每个通道的每次尝试都有记录，成功失败与耗时一目了然。
  - icon: 📦
    title: 单文件部署
    details: 前端已嵌入 Go 二进制，内置登录认证与 SQLite 存储，运行即服务，无需外部依赖。
---
