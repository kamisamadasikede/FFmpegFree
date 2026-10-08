package live

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureFLV 读预览 HTTP-FLV，原样保存字节（之后交给 ffmpeg 解码检查）并解析 tag 到达时间。
func captureFLV(ctx context.Context, rawURL string) (raw []byte, tags []arrivedTag, code int, err error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	req.Header.Set("Origin", "wails://wails")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, resp.StatusCode, nil
	}
	var buf bytes.Buffer
	tags, err = parseFLVArrivals(io.TeeReader(resp.Body, &buf))
	return buf.Bytes(), tags, resp.StatusCode, err
}

// 契约 v0.25.3（N2）：纯音频的拉流能播：转封装出只有音频的合法 FLV（头里只有音频标志），GOP 缓存不等视频关键帧，
// 第一个 FLV 头就发 playing，playing / PullSession / GetPreviewStream 都带 hasVideo=false、hasAudio=true。
// RTMP 和 HLS（MediaMTX 的 mpegts 变体）各走一遍。
func TestMeasurePullAudioOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	for _, proto := range []string{"rtmp", "hls"} {
		t.Run(proto, func(t *testing.T) {
			variant := ""
			if proto == "hls" {
				variant = "mpegts"
			}
			m := startMediaMTXHLS(t, variant)
			r := newRealFixture(t, 0)
			pub := exec.Command(r.ffmpeg, "-hide_banner", "-loglevel", "error", "-re", "-f", "lavfi", "-i", "sine=f=440:r=44100",
				"-c:a", "aac", "-b:a", "96k", "-f", "flv", m.rtmpURL("live/audio"))
			if err := pub.Start(); err != nil {
				t.Fatal(err)
			}
			defer pub.Process.Kill()
			src := m.rtmpURL("live/audio")
			if proto == "hls" {
				src = m.hlsURL("live/audio")
				deadline := time.Now().Add(40 * time.Second)
				for {
					resp, err := http.Get(src)
					if err == nil {
						ok := resp.StatusCode == 200
						resp.Body.Close()
						if ok {
							break
						}
					}
					if time.Now().After(deadline) {
						t.Skip("纯音频 HLS 没有就绪")
					}
					time.Sleep(time.Second)
				}
			} else {
				time.Sleep(1500 * time.Millisecond)
			}
			var mu sync.Mutex
			var events []PullEvent
			playing := make(chan PullEvent, 1)
			r.svc.cfg.Emit = func(name string, p any) {
				if name != "live:pull" {
					return
				}
				ev := p.(PullEvent)
				mu.Lock()
				events = append(events, ev)
				mu.Unlock()
				if ev.State == "playing" {
					playing <- ev
				}
			}
			started := time.Now()
			ps, err := r.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: src})
			if err != nil || ps.PreviewURL == "" {
				t.Fatalf("StartPullPreview: %+v %v", ps, err)
			}
			if ps.HasVideo != nil || ps.HasAudio != nil {
				t.Fatalf("新会话还没开始播放，PullSession 不应带 hasVideo / hasAudio: %+v", ps)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			// 播放器和 StartPullPreview 同时连上来（前端拿到 previewUrl 就连）
			type result struct {
				raw  []byte
				tags []arrivedTag
				code int
			}
			first := make(chan result, 1)
			readCtx, stopRead := context.WithTimeout(ctx, 14*time.Second)
			defer stopRead()
			go func() {
				raw, tags, code, _ := captureFLV(readCtx, ps.PreviewURL)
				first <- result{raw, tags, code}
			}()
			var ev PullEvent
			select {
			case ev = <-playing:
			case <-time.After(20 * time.Second):
				mu.Lock()
				t.Fatalf("纯音频拉流 20 秒内没有 playing（卡在“正在连接…”）: %+v", events)
			}
			toPlaying := time.Since(started)
			if ev.HasVideo == nil || ev.HasAudio == nil || *ev.HasVideo || !*ev.HasAudio {
				t.Fatalf("playing 应带 hasVideo=false hasAudio=true: %+v", ev)
			}
			again, err := r.svc.StartPullPreview(context.Background(), PullPreviewRequest{URL: src})
			if err != nil || again.ID != ps.ID || again.HasVideo == nil || *again.HasVideo || again.HasAudio == nil || !*again.HasAudio {
				t.Fatalf("已开始播放的会话，PullSession 应带 hasVideo=false hasAudio=true: %+v %v", again, err)
			}
			st, err := r.svc.GetPreviewStream(ps.ID)
			if err != nil || st.HasVideo || !st.HasAudio {
				t.Fatalf("GetPreviewStream: %+v %v", st, err)
			}
			// 后加入的客户端：不等视频关键帧，马上有音频
			time.Sleep(3 * time.Second)
			lateCtx, stopLate := context.WithTimeout(ctx, 4*time.Second)
			defer stopLate()
			lateAt := time.Now()
			_, late, code, _ := captureFLV(lateCtx, ps.PreviewURL)
			if code != http.StatusOK {
				t.Fatalf("后加入 HTTP %d", code)
			}
			var lateFirst time.Duration = -1
			lateAudio := 0
			for _, tg := range late {
				if tg.kind == 9 {
					t.Fatalf("纯音频流里出现了视频 tag: %+v", tg)
				}
				if tg.kind == 8 && tg.size > 2 {
					lateAudio++
					if lateFirst < 0 {
						lateFirst = tg.at.Sub(lateAt)
					}
				}
			}
			res := <-first
			if res.code != http.StatusOK || len(res.raw) < 13 {
				t.Fatalf("第一个客户端 HTTP %d，%d 字节", res.code, len(res.raw))
			}
			if string(res.raw[:3]) != "FLV" || res.raw[4] != 0x04 {
				t.Fatalf("FLV 头的音视频标志应只有音频（0x04），实际 %#x", res.raw[4])
			}
			audio, video := 0, 0
			var firstAudio time.Duration = -1
			for _, tg := range res.tags {
				switch tg.kind {
				case 8:
					audio++
					if firstAudio < 0 {
						firstAudio = tg.at.Sub(started)
					}
				case 9:
					video++
				}
			}
			// 解码检查：ffmpeg 读我们送出的 FLV，不能有错误。
			path := filepath.Join(t.TempDir(), "audio.flv")
			if err := os.WriteFile(path, res.raw, 0o644); err != nil {
				t.Fatal(err)
			}
			out, _ := exec.Command(r.ffmpeg, "-hide_banner", "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput()
			probe, _ := exec.Command(r.ffprobe, "-v", "error", "-show_entries", "stream=codec_type,codec_name", "-of", "csv=p=0", path).Output()
			t.Logf("MEASURE pull 纯音频（%s）: StartPullPreview 后 %v playing，第一个音频 tag %v；第一个客户端 %d 个音频 tag、%d 个视频 tag；"+
				"后加入的客户端 %v 收到第一个音频、4 秒内 %d 个；ffprobe=%q；解码错误=%q",
				proto, toPlaying.Round(10*time.Millisecond), firstAudio.Round(10*time.Millisecond), audio, video,
				lateFirst.Round(time.Millisecond), lateAudio, strings.TrimSpace(string(probe)), strings.TrimSpace(string(out)))
			if video != 0 || audio < 100 {
				t.Fatalf("应只有音频且持续送出: audio=%d video=%d", audio, video)
			}
			if strings.TrimSpace(string(probe)) != "aac,audio" && strings.TrimSpace(string(probe)) != "audio,aac" {
				t.Fatalf("送出的 FLV 应只有一路 AAC 音频: %q", probe)
			}
			if strings.TrimSpace(string(out)) != "" {
				t.Fatalf("解码出错: %s", out)
			}
			if lateFirst < 0 || lateFirst > time.Second || lateAudio < 100 {
				t.Fatalf("后加入的客户端应马上收到音频（不等关键帧）: first=%v n=%d", lateFirst, lateAudio)
			}
			// 远端停止：ended 或 interrupted；interrupted 带 LIVE_PUSH_INTERRUPTED，不是 INTERNAL。
			pub.Process.Kill()
			pub.Wait()
			deadline := time.Now().Add(25 * time.Second)
			var last PullEvent
			for time.Now().Before(deadline) {
				mu.Lock()
				last = events[len(events)-1]
				mu.Unlock()
				if last.State == "ended" || last.State == "interrupted" {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if last.State == "interrupted" && (last.Error == nil || last.Error.Code != "LIVE_PUSH_INTERRUPTED" || last.Error.Message != "拉流被中断，请重新拉流。") {
				t.Fatalf("interrupted 的错误: %+v", last.Error)
			}
			if last.State != "ended" && last.State != "interrupted" {
				t.Fatalf("远端停止后应 ended 或 interrupted: %+v", last)
			}
			if last.HasVideo != nil {
				t.Fatalf("只有 playing 带 hasVideo: %+v", last)
			}
		})
	}
}
