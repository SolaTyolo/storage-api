-- Supabase Storage 风格元数据
-- 参考: https://github.com/supabase/storage/blob/master/migrations/tenant/0002-storage-schema.sql

CREATE SCHEMA IF NOT EXISTS storage;

-- 若曾跑过带 upload_sessions 的旧 schema，清理遗留表
DROP TABLE IF EXISTS storage.upload_sessions;

CREATE TABLE IF NOT EXISTS storage.buckets (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    owner           UUID,
    public          BOOLEAN NOT NULL DEFAULT FALSE,
    file_size_limit BIGINT,
    allowed_mime_types TEXT[],
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS storage.objects (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bucket_id        TEXT NOT NULL REFERENCES storage.buckets(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    owner            UUID,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata         JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (bucket_id, name)
);

CREATE INDEX IF NOT EXISTS objects_name_prefix ON storage.objects (bucket_id, name text_pattern_ops);
