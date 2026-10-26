package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// A song NetEase won't play plays once unlocking is on, and not while it is
// off or no source allows it.
func TestUnlockSettings(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	gray := netease.Track{ID: 1, Fee: 1}

	a.settings.Unlock.Enabled = false
	if a.playable(&gray) {
		t.Fatal("a song NetEase won't play played with unlocking off")
	}
	// The player asks for sources only while unlocking is on, whatever
	// the sources below say.
	if sources := a.settings.unblockSources(); len(sources) != 0 {
		t.Errorf("sources with unlocking off: %v", sources)
	}
	a.settings.Unlock.Enabled = true
	if !a.playable(&gray) {
		t.Fatal("a song NetEase won't play stayed gray with unlocking on")
	}

	a.settings.Unlock.Pyncmd, a.settings.Unlock.Kuwo, a.settings.Unlock.Kugou = false, false, false
	if a.settings.canUnblock() {
		t.Fatal("unblocks with no source allowed")
	}
	if sources := a.settings.unblockSources(); len(sources) != 0 {
		t.Errorf("sources with none allowed: %v", sources)
	}
}

// The switch in the settings turns unlocking on and off.
func TestUnlockSwitch(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Unlock.Enabled = false
	// The theme waits on the app's main thread, which a test does not run.
	old := applyTheme
	applyTheme = func(settings) {}
	defer func() { applyTheme = old }()
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	// The sources a song may fall back to show only with unlocking on.
	for _, label := range []string{"pyncmd", "酷我音乐", "酷狗音乐"} {
		if _, ok := tt.Find(label); ok {
			t.Errorf("source row %s with unlocking off", label)
		}
	}
	// The row's title is labelled as its switch is: click the switch, at
	// the row's end.
	top, ok1 := tt.Find("灰色歌曲解锁")
	bottom, ok2 := tt.Find("无版权或下架的歌曲将从第三方音源匹配播放")
	if !ok1 || !ok2 {
		t.Fatal("no row 灰色歌曲解锁")
	}
	tt.ClickAt(560-28-16-12, (top.Y+bottom.Y+bottom.H)/2)
	tt.Frame()
	if !a.settings.Unlock.Enabled {
		t.Fatal("didn't turn on unlocking")
	}
	tt.Frame()
	// The sources appear with unlocking on: each is one the player may
	// fall back to.
	for _, label := range []string{"pyncmd", "酷我音乐", "酷狗音乐"} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("no source row %s", label)
		}
	}
}
