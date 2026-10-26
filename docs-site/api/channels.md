# 通道接口

外部 API 只能**读取**通道列表，用于挑选 `channel_ids`。通道的增删改需要在面板里操作。

## 获取通道列表

`GET /api/external/v1/channels`

```bash
curl http://localhost:8765/api/external/v1/channels \
  -H "X-AUTH: bdrk_xxx"
```

```json
{
  "code": 200,
  "message": "ok",
  "data": [
    {
      "id": 1,
      "name": "邮件通知",
      "type": "smtp",
      "enabled": true,
      "config": {
        "host": "***",
        "port": "***",
        "from_addr": "***"
      },
      "created_at": "2026-06-30T12:00:00Z",
      "updated_at": "2026-06-30T12:00:00Z"
    }
  ]
}
```

::: warning 敏感字段被脱敏
`config` 中以 `_enc` 结尾的字段，以及部分关键字段，会被替换为 `***`。
外部 API 拿到的不是完整配置，仅用于识别通道用途。
:::

## 如何选定 channel_ids

1. 调上面的接口拿到通道列表
2. 按 `name` 与 `type` 找到目标通道的 `id`
3. 在创建提醒时把它填进 `channel_ids`

更省事的做法是在面板的**令牌**页给令牌配置默认通道，之后调用创建接口时完全不传 `channel_ids`。

::: tip 禁用通道会被跳过
`enabled` 为 `false` 的通道即使被绑定也不会参与发送。提醒触发时如果所有绑定通道都不可用，
日志状态会是 `failed`。
:::
