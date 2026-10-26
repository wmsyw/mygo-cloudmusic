package unblock

import (
	"io"
	"net/http"
	"os"
	"testing"

	"netease-music-mygo/internal/netease"
)

// A VIP track resolves through Kuwo to the whole file: the length rules
// out the eleven-second trial its antiserver endpoint serves, and the
// first bytes must be audio rather than an error page.
func TestKuwoLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	track := netease.Track{
		ID:       186016,
		Name:     "晴天",
		Artists:  []netease.Artist{{Name: "周杰伦"}},
		Duration: 269040,
	}
	result, err := Resolve(track, []Source{Kuwo})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("no stream found")
	}
	if result.Source != Kuwo {
		t.Fatalf("source %s, want kuwo", result.Source)
	}
	if result.Size <= 1<<20 {
		t.Fatalf("size %d, which is a trial rather than the song", result.Size)
	}

	req, err := http.NewRequest(http.MethodGet, result.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-3")
	req.Header.Set("User-Agent", netease.UserAgent)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	head, err := io.ReadAll(io.LimitReader(res.Body, 4))
	if err != nil {
		t.Fatal(err)
	}
	id3 := len(head) >= 3 && string(head[:3]) == "ID3"
	sync := len(head) >= 2 && head[0] == 0xff && head[1]&0xe0 == 0xe0
	if !id3 && !sync {
		t.Fatalf("first bytes % x are neither ID3 nor an MP3 frame sync", head)
	}
}
