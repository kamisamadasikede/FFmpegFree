-- 契约 v0.23.4（走查 G3）：源文件行持久化完整的媒体信息。只加列，不改不删任何旧数据。
-- media    = 探测结果 MediaInfo 的 JSON（含 hasVideo / hasAudio / sampleRate / channels / streams，不含 thumbUrl 和 error）；NULL = 没有可用结果。
-- media_fp = 探测时文件的指纹 "<大小>:<修改时间纳秒>"；'' = 从没探测过。media 为 NULL 而 media_fp 非空 = 这个版本的文件探测失败过，文件不变就不再重探。
-- 旧行两列都是空，由 ListSources / GetSource / AddSources 在文件还在时懒探测补上（见契约 6.14.8）。
ALTER TABLE convert_sources ADD COLUMN media TEXT;
ALTER TABLE convert_sources ADD COLUMN media_fp TEXT NOT NULL DEFAULT '';
