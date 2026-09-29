-- 硬件编码接入（契约 v0.17，9.7）：任务实际使用的视频编码器与设备、是否回退 CPU 及原因。旧行默认为空。
ALTER TABLE tasks ADD COLUMN encoder TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN encoder_device TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN hw_fallback INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN hw_fallback_reason TEXT NOT NULL DEFAULT '';
