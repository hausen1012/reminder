-- 001: 将 api_key 表名和列名统一为 token
--
-- 使用 sqlite3 命令行执行：sqlite3 data.db < migrations/001_rename_apikey_to_token.sql
-- 或在 DB Browser for SQLite 中打开并执行此文件。
--
-- 注意：SQLite 没有 IF EXISTS 的 ALTER TABLE，如果某条语句报错说明那步已经完成，
-- 跳过继续执行下一条即可。

-- 1. 表重命名
ALTER TABLE api_keys RENAME TO tokens;
ALTER TABLE api_key_default_channels RENAME TO token_default_channels;

-- 2. reminders 表：先删旧索引再重命名列
DROP INDEX IF EXISTS idx_reminders_api_key_id;
ALTER TABLE reminders RENAME COLUMN api_key_id TO token_id;

-- 3. token_default_channels 表：重命名主键列
ALTER TABLE token_default_channels RENAME COLUMN api_key_id TO token_id;