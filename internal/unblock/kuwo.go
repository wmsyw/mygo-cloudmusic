package unblock

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"netease-music-mygo/internal/netease"
)

// resolveKuwo searches Kuwo, then asks its mobile client for the file of
// each hit that matches.
func resolveKuwo(t netease.Track) (*Result, error) {
	searchURL := "https://search.kuwo.cn/r.s?&correct=1&vipver=1&stype=comprehensive&encoding=utf8" +
		"&rformat=json&mobi=1&show_copyright_off=1&searchapi=6&all=" + url.QueryEscape(keyword(t))
	body, err := get(searchURL, netease.UserAgent)
	if err != nil {
		return nil, err
	}
	// The answer groups its hits by kind, and the songs are not always in
	// the same group, so every group's list is searched.
	var res struct {
		Content []struct {
			MusicPage struct {
				AbsList []struct {
					MusicRID string  `json:"MUSICRID"`
					Name     string  `json:"SONGNAME"`
					Artist   string  `json:"ARTIST"`
					Duration seconds `json:"DURATION"`
				} `json:"abslist"`
			} `json:"musicpage"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("kuwo: %w", err)
	}
	var rids []string
	for _, group := range res.Content {
		for _, song := range group.MusicPage.AbsList {
			if song.MusicRID == "" || !matchesTrack(t, song.Name, song.Artist, int64(song.Duration)*1000) {
				continue
			}
			rids = append(rids, kuwoRID(song.MusicRID))
			if len(rids) == 5 {
				break
			}
		}
		if len(rids) == 5 {
			break
		}
	}
	var failure error
	for _, rid := range rids {
		result, err := kuwoStream(rid)
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

// kuwoRID is the number Kuwo's stream endpoint wants, from "MUSIC_228908".
func kuwoRID(value string) string {
	if i := strings.LastIndexByte(value, '_'); i >= 0 {
		return value[i+1:]
	}
	return value
}

// kuwoConvertQuery is what Kuwo's mobile client asks for a track's file,
// before the request is encrypted.
const kuwoConvertQuery = "user=0&corp=kuwo&source=kwplayer_ar_5.1.0.0_B_jiakong_vh.apk" +
	"&p2p=1&type=convert_url2&sig=0&format=mp3&rid="

// kuwoStream turns a Kuwo RID into a file URL. The antiserver.kuwo.cn/anti.s
// endpoint is simpler but useless here: for a VIP track it answers with
// an eleven-second trial instead of the song, while the mobile client's
// convert_url2 serves the whole file.
func kuwoStream(rid string) (*Result, error) {
	query := kuwoEncryptQuery(kuwoConvertQuery + rid)
	rawURL := "http://mobi.kuwo.cn/mobi.s?f=kuwo&q=" + url.QueryEscape(query)
	body, err := get(rawURL, "okhttp/3.10.0")
	if err != nil {
		return nil, err
	}
	streamURL := parseKuwoStreamURL(string(body))
	if streamURL == "" {
		return nil, ErrNoMatch
	}
	streamURL = secure(streamURL)
	return &Result{Source: Kuwo, URL: streamURL, Size: headSize(streamURL)}, nil
}

// parseKuwoStreamURL reads the URL out of convert_url2's answer, which is
// one key=value per line. The URL is the rest of its line, ampersands and
// all, so it must not be taken for a query string.
func parseKuwoStreamURL(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "url="); ok {
			return value
		}
	}
	return ""
}

// seconds reads Kuwo's lengths, which the search sends as either a JSON
// number or a string of digits.
type seconds int64

func (s *seconds) UnmarshalJSON(b []byte) error {
	text := strings.Trim(string(b), `"`)
	if text == "" || text == "null" {
		return nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		// An unreadable length only means this hit never matches.
		return nil
	}
	*s = seconds(value)
	return nil
}
