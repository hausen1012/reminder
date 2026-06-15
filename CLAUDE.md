# CLAUDE.md

dd## 系统

系统windows。


## 语言要求

- 所有对话使用中文。
- 所有文档使用中文。
- 除了 powershell 脚本，所有代码注释使用中文。

## 执行要求

- 在生成说明、总结、计划、提交说明时，统一使用中文。
- 在新增或修改 Markdown 文档时，统一使用中文。
- 在新增或修改代码注释时，统一使用中文。

### Environment config (see .env.example)

| Variable | Default | Description |
|---|---|---|
| PORT | 8765 | Listen port |
| DB_PATH | /data/db/bedrock.db | SQLite database path |
| JWT_SECRET | auto-generated | HMAC signing key |
| USERNAME | admin | Seed admin username |
| PASSWORD | admin123 | Seed admin password |