package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// requestTimeout は API の各リクエストのタイムアウト。
const requestTimeout = 500 * time.Millisecond

// Poller は MediaMTX の API を読んで Sample を作る。
// API の応答の本文は、ログにもエラーにも含めない（RTMP の接続一覧はパスワードを返すため）。
type Poller struct {
	baseURL  string
	apiUser  string
	apiPass  string
	pathName string
	client   *http.Client

	mu       sync.Mutex
	hasBase  bool // 基準があるか（最後に path の API を読めた回）
	baseByte int64
	baseTime time.Time
	baseID   string // publisher の識別子。なければ空
	hasLast  bool
	lastAt   time.Time
}

// NewPoller は Poller を作る。client が nil なら http.DefaultClient を使う。
func NewPoller(baseURL, apiUser, apiPass, pathName string, client *http.Client) *Poller {
	if client == nil {
		client = http.DefaultClient
	}
	return &Poller{
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiUser:  apiUser,
		apiPass:  apiPass,
		pathName: pathName,
		client:   client,
	}
}

type pathResponse struct {
	Online       bool  `json:"online"`
	InboundBytes int64 `json:"inboundBytes"`
	Source       *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"source"`
}

type srtConnsResponse struct {
	Items []struct {
		ID       string  `json:"id"`
		State    string  `json:"state"`
		Path     string  `json:"path"`
		RTT      float64 `json:"msRTT"`
		LossRate float64 `json:"packetsReceivedLossRate"`
		RecvMbps float64 `json:"mbpsReceiveRate"`
	} `json:"items"`
}

// get は GET して JSON を out に読む。エラーには本文を含めない。
func (p *Poller) get(ctx context.Context, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path, nil)
	if err != nil {
		return errors.New("リクエストを作れない")
	}
	req.SetBasicAuth(p.apiUser, p.apiPass)
	resp, err := p.client.Do(req)
	if err != nil {
		// url.Error は URL を含むので、内側の原因だけを返す
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("GET %s: 本文を読めない", path)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("GET %s: JSON を読めない", path)
	}
	return nil
}

// Poll は時刻 at での1回の読み取りを行い Sample を返す。
func (p *Poller) Poll(ctx context.Context, at time.Time) Sample {
	p.mu.Lock()
	defer p.mu.Unlock()

	s := Sample{At: at}
	if p.hasLast {
		s.Interval = at.Sub(p.lastAt)
	}
	p.hasLast = true
	p.lastAt = at

	var pr pathResponse
	if err := p.get(ctx, "/v3/paths/get/"+url.PathEscape(p.pathName), &pr); err != nil {
		// 失敗した回は基準を変えない
		s.Err = err
		return s
	}

	var srcType, srcID string
	if pr.Source != nil {
		srcType, srcID = pr.Source.Type, pr.Source.ID
	}
	s.Online = pr.Online

	// 基準からのビットレート。最初の読み取り、識別子の変化、減少のときは 0 で再基準
	if s.Online && p.hasBase && srcID == p.baseID && pr.InboundBytes >= p.baseByte {
		if sec := at.Sub(p.baseTime).Seconds(); sec > 0 {
			s.Kbps = float64(pr.InboundBytes-p.baseByte) * 8 / 1000 / sec
		}
	}
	p.hasBase = true
	p.baseByte, p.baseTime, p.baseID = pr.InboundBytes, at, srcID

	if !s.Online {
		return s
	}
	switch srcType {
	case "rtmpConn":
		s.Protocol = "rtmp"
	case "srtConn":
		s.Protocol = "srt"
		s.SRT = p.readSRT(ctx, srcID)
	}
	return s
}

// readSRT は publish 中の SRT 接続の統計を読む。失敗や該当なしは nil。
func (p *Poller) readSRT(ctx context.Context, sourceID string) *SRTStats {
	var cr srtConnsResponse
	if err := p.get(ctx, "/v3/srt/conns/list", &cr); err != nil {
		return nil
	}
	pick := -1
	for i, c := range cr.Items {
		if c.State != "publish" || c.Path != p.pathName {
			continue
		}
		if sourceID != "" && c.ID == sourceID {
			pick = i
			break
		}
		if pick < 0 {
			pick = i
		}
	}
	if pick < 0 {
		return nil
	}
	c := cr.Items[pick]
	return &SRTStats{RTTMs: c.RTT, LossRate: c.LossRate, RecvMbps: c.RecvMbps}
}

// Run は interval ごとに Poll して out へ送る。ctx が終わると戻る。
func (p *Poller) Run(ctx context.Context, interval time.Duration, out chan<- Sample) {
	tk := time.NewTicker(interval)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
		}
		s := p.Poll(ctx, time.Now())
		select {
		case out <- s:
		case <-ctx.Done():
			return
		}
	}
}
