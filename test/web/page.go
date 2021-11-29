// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/derat/nup/server/db"
	"github.com/tebeka/selenium"
)

// loc matches an element in the page.
// See selenium.WebDriver.FindElement().
type loc struct {
	by, value string
}

// joinLocs flattens locs, consisting of loc and []loc items, into a single slice.
func joinLocs(locs ...interface{}) []loc {
	var all []loc
	for _, l := range locs {
		if tl, ok := l.(loc); ok {
			all = append(all, tl)
		} else if tl, ok := l.([]loc); ok {
			all = append(all, tl...)
		} else {
			panic("Invalid type (must be loc or []loc)")
		}
	}
	return all
}

var (
	body = joinLocs(loc{selenium.ByTagName, "body"})

	overlayManager = joinLocs(loc{selenium.ByTagName, "overlay-manager"})
	menuPlay       = joinLocs(overlayManager, loc{selenium.ByID, "play"})
	menuRemove     = joinLocs(overlayManager, loc{selenium.ByID, "remove"})
	menuTruncate   = joinLocs(overlayManager, loc{selenium.ByID, "truncate"})

	musicPlayer     = joinLocs(loc{selenium.ByTagName, "music-player"})
	audio           = joinLocs(musicPlayer, loc{selenium.ByCSSSelector, "audio"})
	artistDiv       = joinLocs(musicPlayer, loc{selenium.ByID, "artist"})
	titleDiv        = joinLocs(musicPlayer, loc{selenium.ByID, "title"})
	albumDiv        = joinLocs(musicPlayer, loc{selenium.ByID, "album"})
	prevButton      = joinLocs(musicPlayer, loc{selenium.ByID, "prev"})
	playPauseButton = joinLocs(musicPlayer, loc{selenium.ByID, "play-pause"})
	nextButton      = joinLocs(musicPlayer, loc{selenium.ByID, "next"})

	playlistTable = joinLocs(musicPlayer, loc{selenium.ByID, "playlist"},
		loc{selenium.ByCSSSelector, "table"})

	musicSearcher      = joinLocs(loc{selenium.ByTagName, "music-searcher"})
	keywordsInput      = joinLocs(musicSearcher, loc{selenium.ByID, "keywords-input"})
	tagsInput          = joinLocs(musicSearcher, loc{selenium.ByID, "tags-input"})
	firstTrackCheckbox = joinLocs(musicSearcher, loc{selenium.ByID, "first-track-checkbox"})
	unratedCheckbox    = joinLocs(musicSearcher, loc{selenium.ByID, "unrated-checkbox"})
	minRatingSelect    = joinLocs(musicSearcher, loc{selenium.ByID, "min-rating-select"})
	maxPlaysInput      = joinLocs(musicSearcher, loc{selenium.ByID, "max-plays-input"})
	firstPlayedSelect  = joinLocs(musicSearcher, loc{selenium.ByID, "first-played-select"})
	lastPlayedSelect   = joinLocs(musicSearcher, loc{selenium.ByID, "last-played-select"})
	searchButton       = joinLocs(musicSearcher, loc{selenium.ByID, "search-button"})
	resetButton        = joinLocs(musicSearcher, loc{selenium.ByID, "reset-button"})
	luckyButton        = joinLocs(musicSearcher, loc{selenium.ByID, "lucky-button"})
	appendButton       = joinLocs(musicSearcher, loc{selenium.ByID, "append-button"})
	insertButton       = joinLocs(musicSearcher, loc{selenium.ByID, "insert-button"})
	replaceButton      = joinLocs(musicSearcher, loc{selenium.ByID, "replace-button"})

	searchResultsCheckbox = joinLocs(musicSearcher, loc{selenium.ByID, "results-table"},
		loc{selenium.ByCSSSelector, `th input[type="checkbox"]`})
	searchResultsTable = joinLocs(musicSearcher, loc{selenium.ByID, "results-table"},
		loc{selenium.ByCSSSelector, "table"})
)

const (
	// Text for minRatingSelect options and ratingOverlayDiv.
	oneStar    = "★"
	twoStars   = "★★"
	threeStars = "★★★"
	fourStars  = "★★★★"
	fiveStars  = "★★★★★"

	// Text for firstPlayedSelect and lastPlayedSelect options.
	unsetTime   = "..."
	oneDay      = "one day"
	oneWeek     = "one week"
	oneMonth    = "one month"
	threeMonths = "three months"
	sixMonths   = "six months"
	oneYear     = "one year"
	threeYears  = "three years"
	fiveYears   = "five years"
)

// isMissingAttrError returns true if err was returned by calling
// GetAttribute for an attribute that doesn't exist.
func isMissingAttrError(err error) bool {
	// https://github.com/tebeka/selenium/issues/143
	return err != nil && err.Error() == "nil return value"
}

// isStaleElementError returns true if err was caused by using a selenium.WebElement
// that refers to an element that no longer exists.
func isStaleElementError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "stale element reference")
}

// page is used by tests to interact with the web interface.
type page struct {
	t  *testing.T
	wd selenium.WebDriver
}

func newPage(t *testing.T, wd selenium.WebDriver) *page {
	if _, err := wd.ExecuteScript("document.test.setPlayDelayMs(10)", nil); err != nil {
		t.Fatal("Failed setting short play delay: ", err)
	}
	if _, err := wd.ExecuteScript("document.test.reset()", nil); err != nil {
		t.Fatal("Failed resetting page: ", err)
	}
	return &page{t, wd}
}

// get returns the element matched by locs.
func (p *page) get(locs []loc) (selenium.WebElement, error) {
	if err := wait(func() error {
		_, err := p.getNoWait(locs, nil)
		return err
	}); err != nil {
		return nil, err
	}
	return p.getNoWait(locs, nil)
}

// getOrFails calls get and fails the test on error.
func (p *page) getOrFail(locs []loc) selenium.WebElement {
	el, err := p.get(locs)
	if err != nil {
		p.t.Fatalf("Failed getting %v for %v: %v", locs, testInfo(), err)
	}
	return el
}

// getNoWait is called by get.
func (p *page) getNoWait(locs []loc, base selenium.WebElement) (selenium.WebElement, error) {
	var query string
	for len(locs) > 0 {
		if query != "" {
			query = "expand(" + query + ")"
		} else {
			query = "document"
		}
		by, value := locs[0].by, locs[0].value
		switch by {
		case selenium.ByID:
			query += ".getElementById('" + value + "')"
		case selenium.ByTagName:
			query += ".getElementsByTagName('" + value + "').item(0)"
		case selenium.ByCSSSelector:
			query += ".querySelector('" + value + "')"
		default:
			panic(fmt.Sprintf("Invalid 'by' %q", by))
		}
		locs = locs[1:]
	}

	script := "const expand = e => e.shadowRoot || e; return " + query
	res, err := p.wd.ExecuteScriptRaw(script, nil)
	if err != nil {
		if strings.Contains(err.Error(), "Cannot read properties of null (reading 'shadowRoot')") {
			return nil, errors.New("not found")
		}
		return nil, err
	}
	return p.wd.DecodeElement(res)
}

// getTextOrFail returns el's text, failing the test on error.
// If ignoreStale is true, errors caused by the element no longer existing are ignored.
func (p *page) getTextOrFail(el selenium.WebElement, ignoreStale bool) string {
	text, err := el.Text()
	if ignoreStale && isStaleElementError(err) {
		return ""
	} else if err != nil {
		p.t.Fatalf("Failed getting element text for %v: %v", testInfo(), err)
	}
	return text
}

// getTextOrFail returns the named attribute from el, failing the test on error.
// If ignoreStale is true, errors caused by the element no longer existing are ignored.
func (p *page) getAttrOrFail(el selenium.WebElement, name string, ignoreStale bool) string {
	val, err := el.GetAttribute(name)
	if isMissingAttrError(err) {
		return ""
	} else if ignoreStale && isStaleElementError(err) {
		return ""
	} else if err != nil {
		p.t.Fatalf("Failed getting attribute %q for %v: %v", name, testInfo(), err)
	}
	return val
}

// getSelectedOrFail returns whether el is selected, failing the test on error.
// If ignoreStale is true, errors caused by the element no longer existing are ignored.
func (p *page) getSelectedOrFail(el selenium.WebElement, ignoreStale bool) bool {
	sel, err := el.IsSelected()
	if ignoreStale && isStaleElementError(err) {
		return false
	} else if err != nil {
		p.t.Fatalf("Failed getting selected state for %v: %v", testInfo(), err)
	}
	return sel
}

// getSongsFromTable returns songInfos describing the supplied <table> within a <song-table>.
func (p *page) getSongsFromTable(table selenium.WebElement) []songInfo {
	var songs []songInfo
	rows, err := table.FindElements(selenium.ByTagName, "tr")
	if err != nil {
		p.t.Fatalf("Failed getting song rows for %v: %v", testInfo(), err)
	}
	if len(rows) == 0 {
		return nil
	}
	for _, row := range rows[1:] { // skip header
		cols, err := row.FindElements(selenium.ByTagName, "td")
		if isStaleElementError(err) {
			break // table was modified while we were reading it
		} else if err != nil {
			p.t.Fatalf("Failed getting song columns for %v: %v", testInfo(), err)
		}
		// Final column is time; first column may be checkbox.
		song := songInfo{
			artist: p.getTextOrFail(cols[len(cols)-4], true),
			title:  p.getTextOrFail(cols[len(cols)-3], true),
			album:  p.getTextOrFail(cols[len(cols)-2], true),
		}
		// TODO: Copy time from last column.
		class := p.getAttrOrFail(row, "class", true)
		song.active = strings.Contains(class, "active")
		song.menu = strings.Contains(class, "menu")
		if len(cols) == 5 {
			el, err := cols[0].FindElement(selenium.ByTagName, "input")
			if err == nil {
				song.checked = p.getSelectedOrFail(el, true)
			} else if !isStaleElementError(err) {
				p.t.Fatalf("Failed getting checkbox for %v: %v", testInfo(), err)
			}
		}
		songs = append(songs, song)
	}
	return songs
}

// searchResultsOpt is an option for checkSearchResults.
type searchResultsOpt func(cfg *searchResultsConfig)

// searchResultsChecked checks that songs' checkboxes match vals.
func searchResultsChecked(vals ...bool) searchResultsOpt {
	return func(cfg *searchResultsConfig) { cfg.checked = vals }
}

// searchResultsDesc supplied a descriptive string to be included in failure messages.
func searchResultsDesc(d string) searchResultsOpt {
	return func(cfg *searchResultsConfig) { cfg.desc = d }
}

type searchResultsConfig struct {
	desc    string
	checked []bool // checked state of each song
}

// checkSearchResults waits for the search results table to contain the songs in want.
func (p *page) checkSearchResults(want []db.Song, opts ...searchResultsOpt) {
	var cfg searchResultsConfig
	for _, o := range opts {
		o(&cfg)
	}
	table := p.getOrFail(searchResultsTable)
	if err := wait(func() error {
		got := p.getSongsFromTable(table)
		if !compareSongInfos(got, want, cfg.checked, -1 /* active */, -1 /* menu */) {
			return errors.New("songs don't match")
		}
		return nil
	}); err != nil {
		got := p.getSongsFromTable(table)
		msg := fmt.Sprintf("Bad search results for %v", testInfo())
		if cfg.desc != "" {
			msg += fmt.Sprintf(" (%v)", cfg.desc)
		}
		msg += "\nGot:\n"
		for _, s := range got {
			msg += "  " + s.String() + "\n"
		}
		msg += "Want:\n"
		for i, s := range want {
			// This matches songInfo.String().
			str := fmt.Sprintf("%q %q %q", s.Artist, s.Title, s.Album)
			if cfg.checked != nil && cfg.checked[i] {
				str += " checked"
			}
			msg += "  [" + str + "]\n"
		}
		p.t.Fatal(msg)
	}
}

// playlistOpt is an option for checkPlaylist.
type playlistOpt func(cfg *playlistConfig)

// playlistActive indicates that the song at idx should be active.
func playlistActive(idx int) playlistOpt {
	return func(cfg *playlistConfig) { cfg.active = idx }
}

// playlistMenu indicates that a context menu should be shown for the song at idx.
func playlistMenu(idx int) playlistOpt {
	return func(cfg *playlistConfig) { cfg.menu = idx }
}

type playlistConfig struct {
	active int // 0-based index of active song
	menu   int // 0-based index of song with context menu
}

// checkPlaylist waits for the playlist table to contain the songs in want.
func (p *page) checkPlaylist(want []db.Song, opts ...playlistOpt) {
	cfg := playlistConfig{active: -1, menu: -1}
	for _, o := range opts {
		o(&cfg)
	}
	table := p.getOrFail(playlistTable)
	if err := wait(func() error {
		got := p.getSongsFromTable(table)
		if !compareSongInfos(got, want, nil /* checked */, cfg.active, cfg.menu) {
			return errors.New("songs don't match")
		}
		return nil
	}); err != nil {
		got := p.getSongsFromTable(table)
		msg := fmt.Sprintf("Bad playlist for %v", testInfo())
		msg += "\nGot:\n"
		for _, s := range got {
			msg += "  " + s.String() + "\n"
		}
		msg += "Want:\n"
		for i, s := range want {
			// This matches songInfo.String().
			str := fmt.Sprintf("%q %q %q", s.Artist, s.Title, s.Album)
			if cfg.active >= 0 && cfg.active == i {
				str += " active"
			}
			msg += "  [" + str + "]\n"
		}
		p.t.Fatal(msg)
	}
}

// setText clears the element matched by locs and types text into it.
func (p *page) setText(locs []loc, text string) {
	el := p.getOrFail(locs)
	if err := el.Clear(); err != nil {
		p.t.Fatalf("Failed clearing %v for %v: %v", locs, testInfo(), err)
	}
	if err := el.SendKeys(text); err != nil {
		p.t.Fatalf("Failed sending keys to %v for %v: %v", locs, testInfo(), err)
	}
}

// click clicks on the element matched by locs.
func (p *page) click(locs []loc) {
	if err := p.getOrFail(locs).Click(); err != nil {
		p.t.Fatalf("Failed clicking %v for %v: %v", locs, testInfo(), err)
	}
}

// clickOption clicks the <option> with the supplied text in the <select> matched by sel.
func (p *page) clickOption(sel []loc, option string) {
	opts, err := p.getOrFail(sel).FindElements(selenium.ByTagName, "option")
	if err != nil {
		p.t.Fatalf("Failed getting %v options for %v: %v", sel, testInfo(), err)
	}
	for _, opt := range opts {
		if strings.TrimSpace(p.getTextOrFail(opt, false)) == option {
			if err := opt.Click(); err != nil {
				p.t.Fatalf("Failed clicking %v option %q for %v: %v", sel, option, testInfo(), err)
			}
			return
		}
	}
	p.t.Fatalf("Failed finding %v option %q for %v: %v", sel, option, testInfo(), err)
}

type checkboxState uint32

const (
	checkboxChecked     checkboxState = 1 << iota
	checkboxTransparent               // has "transparent" class
)

// checkCheckbox verifies that the checkbox element matched by locs has the specified state.
// TODO: The "check" in this name is ambiguous.
func (p *page) checkCheckbox(locs []loc, state checkboxState) {
	el := p.getOrFail(locs)
	if got, want := p.getSelectedOrFail(el, false), state&checkboxChecked != 0; got != want {
		p.t.Fatalf("Checkbox %v has checked state %v for %v; want %v", locs, got, testInfo(), want)
	}
	class := p.getAttrOrFail(el, "class", false)
	if got, want := strings.Contains(class, "transparent"), state&checkboxTransparent != 0; got != want {
		p.t.Fatalf("Checkbox %v has transparent state %v for %v; want %v", locs, got, testInfo(), want)
	}
}

// clickSearchResultsSongCheckbox clicks the checkbox for the song at 0-based index
// idx in the search results table. If key (e.g. selenium.ShiftKey) is non-empty,
// it is held while performing the click.
func (p *page) clickSearchResultsSongCheckbox(idx int, key string) {
	table := p.getOrFail(searchResultsTable)
	sel := fmt.Sprintf("tr:nth-child(%d) td:first-child input", idx+1)
	cb, err := table.FindElement(selenium.ByCSSSelector, sel)
	if err != nil {
		p.t.Fatalf("Failed finding checkbox %d (%q) for %v: %v", idx, sel, testInfo(), err)
	}
	if key != "" {
		if err := p.wd.KeyDown(key); err != nil {
			p.t.Fatalf("Failed pressing key before clicking checkbox %d for %v: %v", idx, testInfo(), err)
		}
		defer func() {
			if err := p.wd.KeyUp(key); err != nil {
				p.t.Fatalf("Failed releasing key after clicking checkbox %d for %v: %v", idx, testInfo(), err)
			}
		}()
	}
	if err := cb.Click(); err != nil {
		p.t.Fatalf("Failed clicking checkbox %d for %v: %v", idx, testInfo(), err)
	}
}

// rightClickPlaylistSong right-clicks on the playlist song at the specified index.
func (p *page) rightClickPlaylistSong(idx int) {
	table := p.getOrFail(playlistTable)
	sel := fmt.Sprintf("tbody tr:nth-child(%d)", idx+1)
	row, err := table.FindElement(selenium.ByCSSSelector, sel)
	if err != nil {
		p.t.Fatalf("Failed finding playlist song %d (%q) for %v: %v", idx, sel, testInfo(), err)
	}
	if err := row.MoveTo(3, 3); err != nil {
		p.t.Fatalf("Failed moving mouse to playlist song for %v: %v", testInfo(), err)
	}
	if err := p.wd.Click(selenium.RightButton); err != nil {
		p.t.Fatalf("Failed right-clicking on playlist song for %v: %v", testInfo(), err)
	}
}

// checkSong verifies that the current song matches want.
func (p *page) checkSong(want db.Song, flags songFlags) {
	getSong := func() songInfo {
		au := p.getOrFail(audio)
		return songInfo{
			artist: p.getTextOrFail(p.getOrFail(artistDiv), true),
			title:  p.getTextOrFail(p.getOrFail(titleDiv), true),
			album:  p.getTextOrFail(p.getOrFail(albumDiv), true),
			paused: p.getAttrOrFail(au, "paused", true) != "",
			ended:  p.getAttrOrFail(au, "ended", true) != "",
			src:    p.getAttrOrFail(au, "src", true),
		}
	}

	if err := wait(func() error {
		got := getSong()
		if !compareSongInfo(got, want, flags) {
			return errors.New("songs don't match")
		}
		return nil
	}); err != nil {
		got := getSong()
		msg := fmt.Sprintf("Bad song for %v", testInfo())
		msg += "\nGot: " + got.String() + "\n"
		msg += "Want:  "
		// This matches songInfo.String().
		str := fmt.Sprintf("%q %q %q", want.Artist, want.Title, want.Album)
		if flags&songEnded != 0 {
			str += " ended"
		}
		if flags&songPaused != 0 {
			str += " paused"
		}
		msg += "[" + str + "]\n"
		p.t.Fatal(msg)
	}
}
