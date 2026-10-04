package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/ericwyn/tagger/internal/domain"
	alac "github.com/mycophonic/saprobe-alac"
)

func TestBuildALACWAVHeader(t *testing.T) {
	cases := []struct {
		name     string
		format   alac.PCMFormat
		align    uint16
		byteRate uint32
		bits     uint16
	}{
		{"16bit stereo", alac.PCMFormat{SampleRate: 44100, BitDepth: 16, Channels: 2}, 4, 176400, 16},
		{"24bit stereo", alac.PCMFormat{SampleRate: 48000, BitDepth: 24, Channels: 2}, 6, 288000, 24},
		{"20bit packed in 24", alac.PCMFormat{SampleRate: 48000, BitDepth: 20, Channels: 2}, 6, 288000, 24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const dataLen int64 = 6000
			hdr := buildALACWAVHeader(tc.format, dataLen)
			if len(hdr) != 44 || string(hdr[:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" || string(hdr[36:40]) != "data" {
				t.Fatalf("WAV header malformed (%d bytes): %x", len(hdr), hdr)
			}
			if got := binary.LittleEndian.Uint32(hdr[4:8]); got != 36+uint32(dataLen) {
				t.Errorf("RIFF size = %d", got)
			}
			if got := binary.LittleEndian.Uint16(hdr[20:22]); got != 1 {
				t.Errorf("WAV codec = %d, want PCM=1", got)
			}
			if got := binary.LittleEndian.Uint16(hdr[32:34]); got != tc.align {
				t.Errorf("block align = %d, want %d", got, tc.align)
			}
			if got := binary.LittleEndian.Uint32(hdr[28:32]); got != tc.byteRate {
				t.Errorf("byte rate = %d, want %d", got, tc.byteRate)
			}
			if got := binary.LittleEndian.Uint16(hdr[34:36]); got != tc.bits {
				t.Errorf("bits per sample = %d, want %d", got, tc.bits)
			}
			if got := binary.LittleEndian.Uint32(hdr[40:44]); got != uint32(dataLen) {
				t.Errorf("PCM data size = %d", got)
			}
		})
	}
}

func TestAudioETagALAC(t *testing.T) {
	got := audioETagALAC("abc123")
	if got != `"abc123-al3"` || got == audioETag("abc123", false) || got == audioETag("abc123", true) {
		t.Errorf("ALAC ETag 未与其他音频区分: %q", got)
	}
}

// 两首合成 ALAC 由服务器 ffmpeg 生成：16bit/44100Hz 和 24bit/48000Hz；
// 与其他音频文件无关，可验证末包实际帧数、无缓存流式返回和非采样对齐 Range。
func TestALACStreamingAndRange(t *testing.T) {
	for _, tc := range []struct {
		name string
		bits int
	}{
		{"sine-16bit.m4a", 16},
		{"sine-24bit.m4a", 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.name)
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			dec, err := alac.NewDecoder(f)
			if err != nil {
				f.Close()
				t.Fatal(err)
			}
			if dec.Format().BitDepth != tc.bits {
				f.Close()
				t.Fatalf("decoded depth %d, want %d", dec.Format().BitDepth, tc.bits)
			}
			wantPCM, err := io.ReadAll(dec) // 测试只对 1.3s 样本执行整段对比；生产代码不这么做。
			f.Close()
			if err != nil {
				t.Fatal(err)
			}

			s := newTestServer(t)
			track := domain.Track{Revision: "synthetic-alac", Format: domain.FormatM4A, Properties: domain.TrackProperties{Codec: "ALAC"}}
			s.h.Engine.GET("/__test/alac", func(_ context.Context, c *app.RequestContext) {
				src, err := os.Open(path)
				if err != nil {
					c.SetStatusCode(500)
					return
				}
				s.serveALACWAV(c, src, track)
			})
			url := "/__test/alac"
			full := ut.PerformRequest(s.h.Engine, "GET", url, nil)
			if full.Code != 200 || full.Header().Get("Content-Type") != "audio/wav" {
				t.Fatalf("full HTTP %d type=%q body=%q", full.Code, full.Header().Get("Content-Type"), full.Body.String()[:min(full.Body.Len(), 100)])
			}
			body := full.Body.Bytes()
			if len(body) != 44+len(wantPCM) || !bytes.Equal(body[44:], wantPCM) {
				t.Fatalf("WAV/PCM mismatch: body=%d pcm=%d", len(body), len(wantPCM))
			}
			if int(binary.LittleEndian.Uint32(body[40:44])) != len(wantPCM) {
				t.Fatalf("WAV dataLen=%d actual=%d", binary.LittleEndian.Uint32(body[40:44]), len(wantPCM))
			}
			etag := full.Header().Get("ETag")
			for _, region := range [][2]int{{0, 43}, {44, 100}, {102, 504}, {len(body) / 2, len(body)/2 + 113}, {len(body) - 200, len(body) - 1}} {
				start, end := region[0], region[1]
				resp := ut.PerformRequest(s.h.Engine, "GET", url, nil,
					ut.Header{Key: "Range", Value: "bytes=" + itoa(start) + "-" + itoa(end)},
					ut.Header{Key: "If-Range", Value: etag})
				if resp.Code != 206 || !bytes.Equal(resp.Body.Bytes(), body[start:end+1]) {
					t.Fatalf("range %d-%d: HTTP %d length=%d want=%d", start, end, resp.Code, resp.Body.Len(), end-start+1)
				}
			}
			invalid := ut.PerformRequest(s.h.Engine, "GET", url, nil, ut.Header{Key: "Range", Value: "bytes=999999999-"})
			if invalid.Code != 416 || !strings.HasPrefix(invalid.Header().Get("Content-Range"), "bytes */") {
				t.Fatalf("invalid range: HTTP %d Content-Range=%s", invalid.Code, invalid.Header().Get("Content-Range"))
			}
			stale := ut.PerformRequest(s.h.Engine, "GET", url, nil,
				ut.Header{Key: "Range", Value: "bytes=0-99"}, ut.Header{Key: "If-Range", Value: `"old"`})
			if stale.Code != 200 || !bytes.Equal(stale.Body.Bytes(), body) {
				t.Fatalf("old If-Range should send full 200: got %d", stale.Code)
			}
			cached := ut.PerformRequest(s.h.Engine, "GET", url, nil, ut.Header{Key: "If-None-Match", Value: etag})
			if cached.Code != 304 {
				t.Fatalf("If-None-Match should return 304, got %d", cached.Code)
			}
		})
	}
}

// 用环境变量指向老板指定的 ALAC 单曲，在 Windows 上以只读方式验证真实文件。
// 在 CI / 云服务器无真实曲库时自动跳过；不在本地安装 Go，测试二进制从云服务器交叉编译。
func TestALACRealFileSmoke(t *testing.T) {
	path := os.Getenv("TAGGER_TEST_ALAC_FILE")
	if path == "" {
		t.Skip("需要 TAGGER_TEST_ALAC_FILE")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec, err := alac.NewDecoder(f)
	if err != nil {
		t.Fatal(err)
	}
	pcmSize, err := alacPCMSize(dec)
	if err != nil {
		t.Fatal(err)
	}
	if pcmSize <= 0 || pcmSize > int64(^uint32(0))-36 {
		t.Fatalf("PCM size invalid: %d", pcmSize)
	}
	if err := alacSeekBytes(dec, pcmSize/2); err != nil {
		t.Fatal(err)
	}
	middle := make([]byte, 64*1024)
	if _, err := io.ReadFull(dec, middle); err != nil {
		t.Fatal(err)
	}
	if err := alacSeekBytes(dec, 0); err != nil {
		t.Fatal(err)
	}
	begin := make([]byte, 64*1024)
	if _, err := io.ReadFull(dec, begin); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(begin, middle) {
		t.Fatal("真实 ALAC 中段与首段意外相同，可能未正确 Seek")
	}
	t.Logf("real ALAC: %+v PCM=%d, random seek and 64KiB decode OK", dec.Format(), pcmSize)
}

func itoa(n int) string { return strconv.Itoa(n) }
