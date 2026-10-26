package unblock

import (
	"testing"

	"netease-music-mygo/internal/netease"
)

// Kuwo's stream endpoint only accepts the query encrypted the way its own
// client does it, so the ciphertext is pinned to what that client sends.
func TestKuwoQueryEncryption(t *testing.T) {
	const query = "user=0&corp=kuwo&source=kwplayer_ar_5.1.0.0_B_jiakong_vh.apk&p2p=1&type=convert_url2&sig=0&format=mp3&rid=228908"
	const want = "3HxQnWXTNdQ6RbicYxLOyHAu64fVpKoBz43BshH4RFaGBPBi+8dZdGuvz4Hu9TAfA75CH9prKR/wLP/IiYIvJoWxgCvU/gETNIqiGvqcuuscRcbgESVmpm7oNjCqzuWEIZJuWAS1Zw6vdYHlcEZ102pjSIhT5B3R"
	if got := kuwoEncryptQuery(query); got != want {
		t.Fatalf("encrypt: got %q, want %q", got, want)
	}
}

// A hit only counts as the same recording when title, version, artist and
// length agree, or a search would play a cover or the wrong edit.
func TestMatch(t *testing.T) {
	qingtian := netease.Track{
		ID:       186016,
		Name:     "晴天",
		Artists:  []netease.Artist{{Name: "周杰伦"}},
		Duration: 269040,
	}
	lowercase := netease.Track{
		Name:     "abc",
		Artists:  []netease.Artist{{Name: "周杰伦"}},
		Duration: 269040,
	}
	cases := []struct {
		name     string
		track    netease.Track
		title    string
		artist   string
		duration int64
		want     bool
	}{
		{"exact", qingtian, "晴天", "周杰伦", 269040, true},
		{"six seconds longer", qingtian, "晴天", "周杰伦", 275040, false},
		{"live take", qingtian, "晴天 (Live)", "周杰伦", 269040, false},
		{"guest artist", qingtian, "晴天", "周杰伦/杨瑞代", 269040, true},
		{"full width title", lowercase, "ＡＢＣ", "周杰伦", 269040, true},
		{"other song", qingtian, "七里香", "周杰伦", 269040, false},
	}
	for _, c := range cases {
		if got := matchesTrack(c.track, c.title, c.artist, c.duration); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// convert_url2 answers with one key=value per line, and the URL is
// everything after "url=", ampersands included.
func TestKuwoStreamURL(t *testing.T) {
	body := "format=mp3\r\nbitrate=128\r\nurl=http://example/a.mp3&bitrate$128\r\n"
	const want = "http://example/a.mp3&bitrate$128"
	if got := parseKuwoStreamURL(body); got != want {
		t.Fatalf("parse: got %q, want %q", got, want)
	}
}

// With no source to ask, resolve finds nothing and reports no failure: the
// player says the song is unavailable on its own.
func TestResolveWithoutSources(t *testing.T) {
	result, err := Resolve(netease.Track{ID: 1, Name: "晴天"}, nil)
	if result != nil || err != nil {
		t.Fatalf("resolve: %v, %v", result, err)
	}
}
