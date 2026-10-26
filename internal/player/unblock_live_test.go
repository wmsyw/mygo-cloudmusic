package player

import (
	"errors"
	"io"
	"os"
	"testing"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/unblock"
)

// A gray song, which NetEase will not stream, opens through Player.open
// from a third-party source and decodes to sound: the fallback the player
// runs in front of the listener is wired, not just the resolver.
func TestUnblockLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	track := netease.Track{
		ID:       186016,
		Name:     "晴天",
		Artists:  []netease.Artist{{Name: "周杰伦"}},
		Duration: 269000,
	}
	p := &Player{Unblock: func() []unblock.Source { return []unblock.Source{unblock.Kuwo} }}
	info, s, err := p.open(track, "exhigh", p.unblockSources())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if info.Source != string(unblock.Kuwo) {
		t.Fatalf("stream source %q, want kuwo", info.Source)
	}
	// The first ~46 ms of the track is encoder silence, so the sound is
	// looked for over the whole first second, past it.
	const want = 44100 * 2 // interleaved samples in a second
	buf := make([]float32, 8192)
	got, sound := 0, false
	for got < want {
		n, err := s.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal("decoded nothing")
		}
		got += n
		for _, f := range buf[:n] {
			if f != 0 {
				sound = true
			}
		}
	}
	if !sound {
		t.Error("decoded silence")
	}
}
