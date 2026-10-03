package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/ericwyn/tagger/internal/domain"
)

// cueOffsetBytes 必须「四舍五入 + blockAlign 对齐」，不能直接 int64() 截断。
//
// 真实踩坑（2026-10-03，老板报「Windows 版播放全是白噪音、foobar 正常」）：
// 陳百強《一生何求》整轨 cue 第 4 轨 11:44:60 = 704.8 秒，float64 里 704.8 实为
// 704.7999999999999545…，× byteRate 176400 得 124326719.99999999 —— 截断后是
// 124326719（%4 = 3），段 PCM 起点比正确位置早 1 字节 → 16bit 立体声采样点错位
// 半个样本 → 浏览器解码出来就是白噪音。foobar2000 按 cue 帧号定位，所以不受影响。
func TestCueOffsetBytesRoundsAndAligns(t *testing.T) {
	cases := []struct {
		name       string
		seconds    float64
		byteRate   int64
		blockAlign int64
		want       int64
	}{
		{"真实踩坑 11:44:60 @176400", 704.8, 176400, 4, 124326720},
		{"真实踩坑 00:02:60 @176400", 2.8, 176400, 4, 493920},
		{"00:08:03 @600", 8.04, 600, 4, 4824},
		{"起点为零", 0, 176400, 4, 0},
		{"byteRate 缺失时返回 0", 12.5, 0, 4, 0},
		{"blockAlign 缺失时只四舍五入", 2.8, 1000, 0, 2800},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cueOffsetBytes(tc.seconds, tc.byteRate, tc.blockAlign)
			if got != tc.want {
				t.Fatalf("cueOffsetBytes(%v, %d, %d) = %d, want %d", tc.seconds, tc.byteRate, tc.blockAlign, got, tc.want)
			}
			if tc.blockAlign > 1 && got%tc.blockAlign != 0 {
				t.Fatalf("偏移 %d 未按 blockAlign=%d 对齐", got, tc.blockAlign)
			}
		})
	}
}

// 端到端：合成一个「会踩浮点截断」的父音频，确认切片服务取的是对齐后的 PCM 区间。
func TestCueWAVSliceServesSampleAlignedPCM(t *testing.T) {
	s := newTestServer(t)
	root := s.writer.Root()

	const (
		byteRate   = 176400
		blockAlign = 4
		dataStart  = 44
		dataSize   = 600000
		startSec   = 2.8 // cue 00:02:60；精确字节偏移 493920，截断会得到 493919
	)
	parent := buildTestWAV(byteRate, blockAlign, dataStart, dataSize)
	name := "CueAligned.wav"
	if err := os.WriteFile(filepath.Join(root, name), parent, 0o644); err != nil {
		t.Fatal(err)
	}

	track := domain.Track{
		ID:                 "trk-cue-aligned",
		Format:             domain.FormatWAV,
		CuePath:            "CueAligned.cue",
		CueTrackNumber:     4,
		StartOffsetSeconds: startSec,
		EndOffsetSeconds:   startSec + 1,
		Revision:           "rev-test",
	}
	s.h.Engine.GET("/__test/cueslice", func(_ context.Context, c *app.RequestContext) {
		file, err := os.Open(filepath.Join(root, name))
		if err != nil {
			c.SetStatusCode(500)
			return
		}
		if !s.serveCueWAVSlice(c, file, int64(len(parent)), track) {
			_ = file.Close()
			c.SetStatusCode(500)
		}
	})

	resp := ut.PerformRequest(s.h.Engine, "GET", "/__test/cueslice", nil)
	if resp.Code != 200 {
		t.Fatalf("cue slice status = %d body=%q", resp.Code, resp.Body.String())
	}
	body := resp.Body.Bytes()
	if len(body) < dataStart+16 || string(body[0:4]) != "RIFF" || string(body[8:12]) != "WAVE" {
		t.Fatalf("合成头异常: %q", body[:min(len(body), 48)])
	}

	aligned := dataStart + 493920
	if !bytes.Equal(body[dataStart:dataStart+16], parent[aligned:aligned+16]) {
		t.Fatalf("段 PCM 起点未对齐: got %x want %x", body[dataStart:dataStart+16], parent[aligned:aligned+16])
	}
	if bytes.Equal(body[dataStart:dataStart+16], parent[aligned-1:aligned+15]) {
		t.Fatalf("段 PCM 用了截断偏移 %d（早 1 字节 → 采样点错位 → 白噪音）", aligned-1)
	}
	if got := binary.LittleEndian.Uint32(body[dataStart:]); got != uint32(493920/blockAlign) {
		t.Fatalf("首个采样块号 = %d, want %d", got, 493920/blockAlign)
	}
}

// buildTestWAV 造一个标准 PCM WAV，并把「4 字节块序号」写进每个采样块，
// 便于断言切片起点落在哪个块上。
func buildTestWAV(byteRate, blockAlign, dataStart, dataSize int64) []byte {
	buf := make([]byte, dataStart+dataSize)
	copy(buf[0:], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(buf)-8))
	copy(buf[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)                          // PCM
	binary.LittleEndian.PutUint16(buf[22:], 2)                          // 立体声
	binary.LittleEndian.PutUint32(buf[24:], uint32(byteRate/blockAlign)) // 采样率
	binary.LittleEndian.PutUint32(buf[28:], uint32(byteRate))
	binary.LittleEndian.PutUint16(buf[32:], uint16(blockAlign))
	binary.LittleEndian.PutUint16(buf[34:], 16)
	copy(buf[36:], "data")
	binary.LittleEndian.PutUint32(buf[40:], uint32(dataSize))
	for i := int64(0); i*blockAlign+blockAlign <= dataSize; i++ {
		binary.LittleEndian.PutUint32(buf[dataStart+i*blockAlign:], uint32(i))
	}
	return buf
}
