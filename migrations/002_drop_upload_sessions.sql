-- 若曾跑过带 upload_sessions 的旧 schema，执行此迁移清理
DROP TABLE IF EXISTS storage.upload_sessions;
