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

// Poller は MediaMTX の API を1秒ごとなどに読んで、全カメラの Sample を1組にして作る。
// API の応答の本文は、ログにもエラーにも含めない（RTMP の接続一覧はパスワードを返すため）。
type Poller struct {
	baseURL string
	apiUser string
	apiPass string
	ids     []string // カメラの id。設定の順
	paths   []string // カメラの MediaMTX の path（ids と同じ順）。API の項目の名前と照合する
	client  *http.Client

	mu      sync.Mutex
	bases   []base // ids と同じ順
	hasLast bool
	lastAt  time.Time
}

// base はカメラごとの基準。
type base struct {
	has  bool // 基準があるか（最後に path の一覧を読めた回に、その path が一覧にあったか、で決まる）
	byte int64
	time time.Time
	id   string // publisher の識別子。なければ空
}

// NewPoller は Poller を作る。ids はカメラの id の列で設定の順、paths は同じ順の MediaMTX の path
// （`live/main` など。app が空なら id と同じ）。Sample の Camera は id。
// client が nil なら http.DefaultClient を使う。
func NewPoller(baseURL, apiUser, apiPass string, ids, paths []string, client *http.Client) *Poller {
	if len(paths) != len(ids) {
		paths = ids
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Poller{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiUser: apiUser,
		apiPass: apiPass,
		ids:     append([]string(nil), ids...),
		paths:   append([]string(nil), paths...),
		client:  client,
		bases:   make([]base, len(ids)),
	}
}

type pathItem struct {
	Name         string `json:"name"`
	Online       bool   `json:"online"`
	InboundBytes int64  `json:"inboundBytes"`
	Source       *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"source"`
}

type pathsListResponse struct {
	Items []pathItem `json:"items"`
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

// Poll は時刻 at での1回の読み取りを行い、全カメラの Sample を1組にして返す。
func (p *Poller) Poll(ctx context.Context, at time.Time) SampleSet {
	p.mu.Lock()
	defer p.mu.Unlock()

	var interval time.Duration
	if p.hasLast {
		interval = at.Sub(p.lastAt)
	}
	p.hasLast = true
	p.lastAt = at

	set := SampleSet{At: at, Cameras: make([]Sample, len(p.ids))}
	for i, id := range p.ids {
		set.Cameras[i] = Sample{Camera: id, At: at, Interval: interval}
	}

	var pl pathsListResponse
	if err := p.get(ctx, "/v3/paths/list", &pl); err != nil {
		// 失敗した回は基準を変えない
		for i := range set.Cameras {
			set.Cameras[i].Err = err
		}
		return set
	}
	byName := make(map[string]*pathItem, len(pl.Items))
	for i := range pl.Items {
		byName[pl.Items[i].Name] = &pl.Items[i]
	}

	needSRT := false
	for i := range p.ids {
		s := &set.Cameras[i]
		b := &p.bases[i]
		it := byName[p.paths[i]]
		if it == nil {
			// 一覧にない path: オフライン扱い。基準は取り直す
			*b = base{}
			continue
		}
		var srcType, srcID string
		if it.Source != nil {
			srcType, srcID = it.Source.Type, it.Source.ID
		}
		s.Online = it.Online

		// 基準からのビットレート。最初の読み取り、識別子の変化、減少のときは 0 で再基準
		if s.Online && b.has && srcID == b.id && it.InboundBytes >= b.byte {
			if sec := at.Sub(b.time).Seconds(); sec > 0 {
				s.Kbps = float64(it.InboundBytes-b.byte) * 8 / 1000 / sec
			}
		}
		*b = base{has: true, byte: it.InboundBytes, time: at, id: srcID}

		if !s.Online {
			continue
		}
		switch srcType {
		case "rtmpConn":
			s.Protocol = "rtmp"
		case "srtConn":
			s.Protocol = "srt"
			needSRT = true
		}
	}

	if needSRT {
		p.fillSRT(ctx, &set, byName)
	}
	return set
}

// fillSRT は SRT のカメラに、publish 中の接続の統計を振り分ける。一覧の失敗や該当なしは nil。
func (p *Poller) fillSRT(ctx context.Context, set *SampleSet, byName map[string]*pathItem) {
	var cr srtConnsResponse
	if err := p.get(ctx, "/v3/srt/conns/list", &cr); err != nil {
		return
	}
	for i := range set.Cameras {
		s := &set.Cameras[i]
		if s.Protocol != "srt" {
			continue
		}
		path := p.paths[i]
		sourceID := byName[path].Source.ID
		pick := -1
		for j, c := range cr.Items {
			if c.State != "publish" || c.Path != path {
				continue
			}
			if sourceID != "" && c.ID == sourceID {
				pick = j
				break
			}
			if pick < 0 {
				pick = j
			}
		}
		if pick >= 0 {
			c := cr.Items[pick]
			s.SRT = &SRTStats{RTTMs: c.RTT, LossRate: c.LossRate, RecvMbps: c.RecvMbps}
		}
	}
}

// Run は interval ごとに Poll して組を out へ送る。ctx が終わると戻る。
func (p *Poller) Run(ctx context.Context, interval time.Duration, out chan<- SampleSet) {
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
