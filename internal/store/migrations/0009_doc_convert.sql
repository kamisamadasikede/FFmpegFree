-- 契约 v0.26（6.12.21）：文档多格式转换。只加列、加索引，不改不删任何旧数据。
-- 旧行 kind 默认 media（转换页）；文档页写入 kind=doc。
ALTER TABLE convert_sources ADD COLUMN kind TEXT NOT NULL DEFAULT 'media';
ALTER TABLE convert_sources ADD COLUMN sheet_count INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_convert_sources_kind_activity ON convert_sources(kind, last_activity_at);
