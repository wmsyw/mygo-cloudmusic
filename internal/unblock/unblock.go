// Package unblock finds a track to stream from third-party sources when
// NetEase will not serve it.
package unblock

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"

	"netease-music-mygo/internal/netease"
)

// Source names a third-party place a stream comes from.
type Source string

const (
	Pyncmd Source = "pyncmd"
	Kuwo   Source = "kuwo"
	Kugou  Source = "kugou"
)

// Sources is the order sources are tried in. Kuwo comes before Kugou
// because Kuwo hands out whole VIP tracks through its mobile client's
// convert_url2 endpoint, while Kugou's tracker answers status 2, with no
// URL, for a VIP hash.
var Sources = []Source{Pyncmd, Kuwo, Kugou}

// Label is how a source is named to the listener.
func (s Source) Label() string {
	switch s {
	case Pyncmd:
		return "pyncmd"
	case Kuwo:
		return "酷我音乐"
	case Kugou:
		return "酷狗音乐"
	}
	return string(s)
}

// Label names the source a stream came from. Streams carry it as a plain
// string, so this is the form callers holding a netease.Stream use.
func Label(source string) string { return Source(source).Label() }

// ErrNoMatch reports that a source found nothing like the track asked for.
var ErrNoMatch = errors.New("no matching track on this source")

// Result is a stream found on a source.
type Result struct {
	Source Source
	URL    string
	Size   int64 // in bytes, 0 when the source would not say
}

// Resolve asks sources, in order, for the track and returns the first
// stream found. A source that finds no matching track is skipped; when
// nothing is found the last network or parse failure is returned, or no
// error at all if every source simply did not know the track.
func Resolve(t netease.Track, sources []Source) (*Result, error) {
	var failure error
	for _, source := range sources {
		var (
			result *Result
			err    error
		)
		switch source {
		case Pyncmd:
			result, err = resolvePyncmd(t)
		case Kuwo:
			result, err = resolveKuwo(t)
		case Kugou:
			result, err = resolveKugou(t)
		default:
			continue
		}
		if err != nil {
			if !errors.Is(err, ErrNoMatch) {
				failure = err
			}
			continue
		}
		return result, nil
	}
	return nil, failure
}

// A search answer is a few hundred kilobytes; a larger body is a broken
// source rather than a track.
const maxBody = 8 << 20

var httpClient = &http.Client{Timeout: 15 * time.Second}

// get reads what is at rawURL, with agent as the User-Agent, and refuses
// anything but a 200.
func get(rawURL, agent string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", agent)
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", req.URL.Host, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, maxBody))
}

// headSize asks the CDN how long a file is, and answers 0 when it will
// not say, which costs the seek bar its length but not the stream. The
// probe is optional, so it gives up quickly rather than hold up playback.
func headSize(rawURL string) int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("User-Agent", netease.UserAgent)
	res, err := httpClient.Do(req)
	if err != nil {
		return 0
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK || res.ContentLength <= 0 {
		return 0
	}
	return res.ContentLength
}

// secure upgrades a URL to HTTPS, which the players require.
func secure(rawURL string) string {
	return strings.Replace(rawURL, "http://", "https://", 1)
}

// resolvePyncmd asks the public gdstudio API, which serves NetEase songs
// from its own cache.
func resolvePyncmd(t netease.Track) (*Result, error) {
	rawURL := "https://music-api.gdstudio.xyz/api.php?types=url&source=netease&id=" +
		strconv.FormatInt(t.ID, 10) + "&br=320"
	body, err := get(rawURL, netease.UserAgent)
	if err != nil {
		return nil, err
	}
	var res struct {
		Bitrate int    `json:"br"`
		URL     string `json:"url"`
		Size    int64  `json:"size"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("pyncmd: %w", err)
	}
	// The API answers with an empty record for a song it does not have,
	// which is not a failure of the whole resolution.
	if res.Bitrate <= 0 || res.URL == "" {
		return nil, ErrNoMatch
	}
	return &Result{Source: Pyncmd, URL: secure(res.URL), Size: res.Size}, nil
}

// resolveKugou searches Kugou, then asks its tracker for a playable file
// for each hit that matches.
func resolveKugou(t netease.Track) (*Result, error) {
	searchURL := "http://mobilecdn.kugou.com/api/v3/search/song?format=json&keyword=" +
		url.QueryEscape(keyword(t)) + "&page=1&pagesize=10"
	body, err := get(searchURL, netease.UserAgent)
	if err != nil {
		return nil, err
	}
	var res struct {
		Data struct {
			Info []struct {
				Hash     string      `json:"hash"`
				Name     string      `json:"songname"`
				Artist   string      `json:"singername"`
				Duration int64       `json:"duration"` // in seconds
				AlbumID  looseString `json:"album_id"`
			} `json:"info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("kugou: %w", err)
	}
	var matches []kugouSong
	for _, song := range res.Data.Info {
		if song.Hash == "" || !matchesTrack(t, song.Name, song.Artist, song.Duration*1000) {
			continue
		}
		matches = append(matches, kugouSong{hash: song.Hash, albumID: string(song.AlbumID)})
		if len(matches) == 5 {
			break
		}
	}
	var failure error
	for _, song := range matches {
		result, err := kugouStream(song)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, ErrNoMatch) {
			continue
		}
		failure = err
	}
	if failure != nil {
		return nil, failure
	}
	return nil, ErrNoMatch
}

type kugouSong struct {
	hash    string
	albumID string
}

// kugouStream asks Kugou's tracker for the file behind a hash. A VIP
// hash is answered with status 2 and no URL, so another hit may still be
// the free upload of the same song.
func kugouStream(song kugouSong) (*Result, error) {
	sum := md5.Sum([]byte(song.hash + "kgcloudv2"))
	albumID := song.albumID
	if albumID == "" {
		albumID = "0"
	}
	rawURL := "https://trackercdn.kugou.com/i/v2/?key=" + hex.EncodeToString(sum[:]) +
		"&hash=" + song.hash + "&appid=1005&pid=2&cmd=25&behavior=play&album_id=" + albumID
	body, err := get(rawURL, netease.UserAgent)
	if err != nil {
		return nil, err
	}
	var res struct {
		URLs     stringList `json:"url"`
		FileSize int64      `json:"fileSize"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("kugou: %w", err)
	}
	if len(res.URLs) == 0 || res.URLs[0] == "" {
		return nil, ErrNoMatch
	}
	return &Result{Source: Kugou, URL: secure(res.URLs[0]), Size: res.FileSize}, nil
}

// stringList reads a field a source sends either as one string or as a
// list of them, as Kugou's tracker does for its URLs.
type stringList []string

func (l *stringList) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '[' {
		var values []string
		if err := json.Unmarshal(b, &values); err != nil {
			return err
		}
		*l = values
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var value string
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		*l = stringList{value}
		return nil
	}
	*l = nil
	return nil
}

// looseString reads a field a source sends either as a string or as a
// number, as Kugou does for its album IDs.
type looseString string

func (s *looseString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var value string
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		*s = looseString(value)
		return nil
	}
	var value json.Number
	if err := json.Unmarshal(b, &value); err != nil {
		return err
	}
	*s = looseString(value.String())
	return nil
}

// keyword is what a search asks for: the title and the first artist.
func keyword(t netease.Track) string {
	name := strings.TrimSpace(t.Name)
	if len(t.Artists) == 0 {
		return name
	}
	return strings.TrimSpace(name + " " + t.Artists[0].Name)
}

// matchWindowMS is how far a hit's length may be from NetEase's for it to
// still be the same recording.
const matchWindowMS = 5000

// matchesTrack reports whether a search hit is the same recording: same
// title, same version, the same first artist, and about the same length.
// A cover, live take or remix is a different recording, so a version
// marker on one side and not the other rules a hit out.
func matchesTrack(t netease.Track, title, artist string, durationMS int64) bool {
	if t.Duration <= 0 || durationMS <= 0 || absInt64(durationMS-t.Duration) > matchWindowMS {
		return false
	}
	if normalize(title) != normalize(t.Name) || markers(t.Name) != markers(title) {
		return false
	}
	if len(t.Artists) == 0 {
		return false
	}
	want := normalize(t.Artists[0].Name)
	if want == "" {
		return false
	}
	for _, name := range strings.FieldsFunc(artist, func(r rune) bool {
		return strings.ContainsRune("/&、,，;；", r)
	}) {
		if normalize(name) == want {
			return true
		}
	}
	return false
}

// versionMarkers name the edits of a song that are songs of their own.
var versionMarkers = []string{"live", "remix", "伴奏", "dj", "cover", "翻唱", "instrumental", "karaoke"}

// markers is the set of version markers in a title. The titles are
// compared normalized, and every marker is ASCII or Chinese, so all of
// them survive normalization.
func markers(title string) uint32 {
	title = normalize(title)
	var set uint32
	for i, marker := range versionMarkers {
		if strings.Contains(title, marker) {
			set |= 1 << uint(i)
		}
	}
	return set
}

// normalize folds a name to what identifies it: full-width to half-width,
// no accents, no punctuation or spaces, lower case.
func normalize(name string) string {
	name = norm.NFD.String(width.Fold.String(name))
	var b strings.Builder
	for _, r := range name {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
