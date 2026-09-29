package store

import (
	"context"
	"fmt"
	"time"

	"FFmpegFree/internal/apperr"
)

// MediaInfo 是契约第 3 节的 MediaInfo（v0.8 扩展）。
//
// Width / Height 是**显示尺寸**（已按 Rotation 交换宽高），各条流的编码尺寸在 Streams 里。
// 只有 media 表里的列会持久化；Streams、Container、Fps 等只在 Probe 的返回值里出现，
// ListRecent 返回的记录里为零值。批量探测时单个文件失败，Error 有值，其他字段为空，不落库。
type MediaInfo struct {
	ID         string  `json:"id"`
	Path       string  `json:"path"`
	Name       string  `json:"name"`
	Size       int64   `json:"size"`
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	VideoCodec string  `json:"videoCodec"`
	AudioCodec string  `json:"audioCodec"`
	Bitrate    int64   `json:"bitrate"`
	ThumbURL   string  `json:"thumbUrl"`

	// 以下字段只在 Probe 时填充，不入库。
	Container  string           `json:"container,omitempty"`
	Fps        float64          `json:"fps,omitempty"`
	Rotation   int              `json:"rotation,omitempty"` // 0 / 90 / 180 / 270，逆时针显示旋转角度
	SampleRate int              `json:"sampleRate,omitempty"`
	Channels   int              `json:"channels,omitempty"`
	HasVideo   bool             `json:"hasVideo,omitempty"`
	HasAudio   bool             `json:"hasAudio,omitempty"`
	Streams    []StreamInfo     `json:"streams,omitempty"`
	Error      *apperr.AppError `json:"error,omitempty"`

	ProbedAt int64 `json:"probedAt"`
}

// StreamInfo 是一条流的信息（ffprobe -show_streams 的整理结果）。
type StreamInfo struct {
	Index         int     `json:"index"`
	Type          string  `json:"type"` // video | audio | subtitle | data | attachment
	Codec         string  `json:"codec"`
	Profile       string  `json:"profile,omitempty"`
	Width         int     `json:"width,omitempty"` // 编码宽高，不含旋转
	Height        int     `json:"height,omitempty"`
	PixFmt        string  `json:"pixFmt,omitempty"`
	Fps           float64 `json:"fps,omitempty"`
	Bitrate       int64   `json:"bitrate,omitempty"`
	Duration      float64 `json:"duration,omitempty"`
	Rotation      int     `json:"rotation,omitempty"`
	SampleRate    int     `json:"sampleRate,omitempty"`
	Channels      int     `json:"channels,omitempty"`
	ChannelLayout string  `json:"channelLayout,omitempty"`
	Language      string  `json:"language,omitempty"`
	AttachedPic   bool    `json:"attachedPic,omitempty"` // 封面图，不算视频画面
}

const (
	defaultRecentLimit = 20
	maxRecentLimit     = 200
)

// UpsertMedia 按 path_key 写入 media 表：同一个文件再次探测时保留原来的 id，只更新探测结果。
// pathKey 由调用方用 paths.Normalize 生成。返回写入后的记录（ID 是最终 id）。
func (s *Store) UpsertMedia(ctx context.Context, pathKey string, m MediaInfo) (MediaInfo, error) {
	if m.ProbedAt == 0 {
		m.ProbedAt = time.Now().UnixMilli()
	}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO media (id, path, path_key, name, size, duration, width, height, video_codec, audio_codec, bitrate, probed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(path_key) DO UPDATE SET
			path=excluded.path, name=excluded.name, size=excluded.size, duration=excluded.duration,
			width=excluded.width, height=excluded.height, video_codec=excluded.video_codec,
			audio_codec=excluded.audio_codec, bitrate=excluded.bitrate, probed_at=excluded.probed_at
		RETURNING id`,
		m.ID, m.Path, pathKey, m.Name, m.Size, m.Duration, m.Width, m.Height, m.VideoCodec, m.AudioCodec,
		m.Bitrate, m.ProbedAt).Scan(&m.ID)
	if err != nil {
		return MediaInfo{}, fmt.Errorf("写入媒体记录失败: %w", err)
	}
	return m, nil
}

// ListRecentMedia 按探测时间倒序返回最近的媒体记录。limit 默认 20，最大 200。
func (s *Store) ListRecentMedia(ctx context.Context, limit int) ([]MediaInfo, error) {
	if limit <= 0 {
		limit = defaultRecentLimit
	}
	if limit > maxRecentLimit {
		limit = maxRecentLimit
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, path, name, size, duration, width, height, video_codec, audio_codec, bitrate, probed_at
		FROM media ORDER BY probed_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询媒体记录失败: %w", err)
	}
	defer rows.Close()
	out := []MediaInfo{}
	for rows.Next() {
		var m MediaInfo
		if err := rows.Scan(&m.ID, &m.Path, &m.Name, &m.Size, &m.Duration, &m.Width, &m.Height,
			&m.VideoCodec, &m.AudioCodec, &m.Bitrate, &m.ProbedAt); err != nil {
			return nil, err
		}
		m.HasVideo = m.VideoCodec != ""
		m.HasAudio = m.AudioCodec != ""
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMedia 只删 media 记录，不碰文件。不存在的 id 忽略。
func (s *Store) DeleteMedia(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM media WHERE id = ?`, id); err != nil {
			return fmt.Errorf("删除媒体记录失败: %w", err)
		}
	}
	return tx.Commit()
}
