-- 契约 v0.24（6.17.2）：原地重新转换。旧行都是 0 / NULL。
ALTER TABLE tasks ADD COLUMN reconverting INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN reconvert_prev TEXT;
ALTER TABLE tasks ADD COLUMN reconvert_pending TEXT;
ALTER TABLE tasks ADD COLUMN last_reconvert_error TEXT;
