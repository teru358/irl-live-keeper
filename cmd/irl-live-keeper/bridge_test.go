package main

import (
	"context"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

// つなぎ: 組から先頭のカメラの Sample だけを、順に core へ渡す。
func TestForwardFirstCamera(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan ingest.SampleSet)
	out := make(chan ingest.Sample)
	done := make(chan struct{})
	go func() { forwardFirstCamera(ctx, in, out); close(done) }()

	now := time.Now()
	in <- ingest.SampleSet{At: now, Cameras: []ingest.Sample{{Camera: "a", Kbps: 1}, {Camera: "b", Kbps: 2}}}
	if s := <-out; s.Camera != "a" || s.Kbps != 1 {
		t.Errorf("sample = %+v", s)
	}
	in <- ingest.SampleSet{At: now} // カメラなしは捨てる
	in <- ingest.SampleSet{At: now, Cameras: []ingest.Sample{{Camera: "a", Kbps: 3}}}
	if s := <-out; s.Kbps != 3 {
		t.Errorf("sample = %+v", s)
	}
	in <- ingest.SampleSet{Cameras: []ingest.Sample{{Camera: "a"}}} // 受け手がいなくても止まれる
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("止まらない")
	}
}
