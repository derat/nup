// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
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
			panic(fmt.Sprintf("Invalid type %T (must be loc or []loc)", l))
		}
	}
	return all
}

var (
	body = joinLocs(loc{selenium.ByTagName, "body"})

	overlayManager = joinLocs(loc{selenium.ByTagName, "overlay-manager"})

	optionsDialog   = joinLocs(overlayManager, loc{selenium.ByCSSSelector, ".dialog"})
	optionsOKButton = joinLocs(optionsDialog, loc{selenium.ByID, "ok-button"})
	gainTypeSelect  = joinLocs(optionsDialog, loc{selenium.ByID, "gain-type-select"})
	preAmpRange     = joinLocs(optionsDialog, loc{selenium.ByID, "pre-amp-range"})

	menuPlay     = joinLocs(overlayManager, loc{selenium.ByID, "play"})
	menuRemove   = joinLocs(overlayManager, loc{selenium.ByID, "remove"})
	menuTruncate = joinLocs(overlayManager, loc{selenium.ByID, "truncate"})

	musicPlayer       = joinLocs(loc{selenium.ByTagName, "music-player"})
	audio             = joinLocs(musicPlayer, loc{selenium.ByCSSSelector, "audio"})
	coverImage        = joinLocs(musicPlayer, loc{selenium.ByID, "cover-img"})
	ratingOverlayDiv  = joinLocs(musicPlayer, loc{selenium.ByID, "rating-overlay"})
	ratingOneStar     = joinLocs(musicPlayer, loc{selenium.ByCSSSelector, "#rating a:nth-child(1)"})
	ratingTwoStars    = joinLocs(musicPlayer, loc{selenium.ByCSSSelector, "#rating a:nth-child(2)"})
	ratingThreeStars  = joinLocs(musicPlayer, loc{selenium.ByCSSSelector, "#rating a:nth-child(3)"})
	ratingFourStars   = joinLocs(musicPlayer, loc{selenium.ByCSSSelector, "#rating a:nth-child(4)"})
	ratingFiveStars   = joinLocs(musicPlayer, loc{selenium.ByCSSSelector, "#rating a:nth-child(5)"})
	editTagsTextarea  = joinLocs(musicPlayer, loc{selenium.ByID, "edit-tags"})
	editTagsSuggester = joinLocs(musicPlayer, loc{selenium.ByID, "edit-tags-suggester"})
	updateCloseImage  = joinLocs(musicPlayer, loc{selenium.ByID, "update-close"})
	artistDiv         = joinLocs(musicPlayer, loc{selenium.ByID, "artist"})
	titleDiv          = joinLocs(musicPlayer, loc{selenium.ByID, "title"})
	albumDiv          = joinLocs(musicPlayer, loc{selenium.ByID, "album"})
	timeDiv           = joinLocs(musicPlayer, loc{selenium.ByID, "time"})
	prevButton        = joinLocs(musicPlayer, loc{selenium.ByID, "prev"})
	playPauseButton   = joinLocs(musicPlayer, loc{selenium.ByID, "play-pause"})
	nextButton        = joinLocs(musicPlayer, loc{selenium.ByID, "next"})

	playlistTable = joinLocs(musicPlayer, loc{selenium.ByID, "playlist"},
		loc{selenium.ByCSSSelector, "table"})

	presentationLayer = joinLocs(musicPlayer, loc{selenium.ByCSSSelector, "presentation-layer"})
	currentArtistDiv  = joinLocs(presentationLayer, loc{selenium.ByID, "current-artist"})
	currentTitleDiv   = joinLocs(presentationLayer, loc{selenium.ByID, "current-title"})
	currentAlbumDiv   = joinLocs(presentationLayer, loc{selenium.ByID, "current-album"})
	nextArtistDiv     = joinLocs(presentationLayer, loc{selenium.ByID, "next-artist"})
	nextTitleDiv      = joinLocs(presentationLayer, loc{selenium.ByID, "next-title"})
	nextAlbumDiv      = joinLocs(presentationLayer, loc{selenium.ByID, "next-album"})

	musicSearcher      = joinLocs(loc{selenium.ByTagName, "music-searcher"})
	keywordsInput      = joinLocs(musicSearcher, loc{selenium.ByID, "keywords-input"})
	tagsInput          = joinLocs(musicSearcher, loc{selenium.ByID, "tags-input"})
	firstTrackCheckbox = joinLocs(musicSearcher, loc{selenium.ByID, "first-track-checkbox"})
	unratedCheckbox    = joinLocs(musicSearcher, loc{selenium.ByID, "unrated-checkbox"})
	minRatingSelect    = joinLocs(musicSearcher, loc{selenium.ByID, "min-rating-select"})
	maxPlaysInput      = joinLocs(musicSearcher, loc{selenium.ByID, "max-plays-input"})
	firstPlayedSelect  = joinLocs(musicSearcher, loc{selenium.ByID, "first-played-select"})
	lastPlayedSelect   = joinLocs(musicSearcher, loc{selenium.ByID, "last-played-select"})
	presetSelect       = joinLocs(musicSearcher, loc{selenium.ByID, "preset-select"})
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

	// Text and values for gainTypeSelect options.
	gainAlbum      = "Album"
	gainTrack      = "Track"
	gainNone       = "None"
	gainAlbumValue = "0"
	gainTrackValue = "1"
	gainNoneValue  = "2"

	// Text for presetSelect options.
	// These match the presets defined in sendConfig() in web_test.go.
	presetInstrumentalOld = "instrumental old"
	presetMellow          = "mellow"
	presetNewAlbums       = "new albums"
	presetUnrated         = "unrated"
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
	t     *testing.T
	wd    selenium.WebDriver
	stage string
}

func newPage(t *testing.T, wd selenium.WebDriver) *page {
	p := page{t, wd, ""}
	p.configPage()
	return &p
}

// configPage configures the page for testing. This is called automatically.
func (p *page) configPage() {
	if _, err := p.wd.ExecuteScript("document.test.setPlayDelayMs(10)", nil); err != nil {
		p.t.Fatalf("Failed setting short play delay for %v: %v", p.desc(), err)
	}
	if _, err := p.wd.ExecuteScript("document.test.reset()", nil); err != nil {
		p.t.Fatalf("Failed resetting page for %v: %v", p.desc(), err)
	}
}

// setStage sets a short human-readable string that will be included in failure messages.
// This is useful for tests that iterate over multiple cases.
func (p *page) setStage(stage string) {
	p.stage = stage
}

func (p *page) desc() string {
	s := test.Caller()
	if p.stage != "" {
		s += " (" + p.stage + ")"
	}
	return s
}

// reload reloads the page.
func (p *page) reload() {
	if err := p.wd.Refresh(); err != nil {
		p.t.Fatalf("Reloading page for %v failed: %v", p.desc(), err)
	}
	p.configPage()
}

// refreshTags instructs the page to refresh the list of available tags from the server.
func (p *page) refreshTags() {
	if _, err := p.wd.ExecuteScript("document.test.updateTags()", nil); err != nil {
		p.t.Fatalf("Failed refreshing tags for %v: %v", p.desc(), err)
	}
}

// getOrFail waits until getNoWait returns the first element matched by locs.
// If the element isn't found in a reasonable amount of time, it fails the test.
func (p *page) getOrFail(locs []loc) selenium.WebElement {
	var el selenium.WebElement
	if err := wait(func() error {
		var err error
		if el, err = p.getNoWait(locs); err != nil {
			return err
		}
		return nil
	}); err != nil {
		p.t.Fatalf("Failed getting %v for %v: %v", locs, p.desc(), err)
	}
	return el
}

// getNoWait returns the first element matched by locs.
//
// If there is more than one element in locs, they will be used successively, e.g.
// loc[1] is used to search inside the element matched by loc[0].
// If an element has a shadow root (per its 'shadowRoot' property),
// the shadow root will be used for the next search.
//
// This is based on Python code that initially used the 'return arguments[0].shadowRoot' approach
// described at https://stackoverflow.com/a/37253205/6882947, but that seems to have broken as a
// result of either upgrading to python-selenium 3.14.1 (from 3.8.0, I think) or upgrading to Chrome
// (and chromedriver) 96.0.4664.45 (from 94, I think).
//
// After upgrading, I would get back a dictionary like {u'shadow-6066-11e4-a52e-4f735466cecf':
// u'9ab4aaee-8108-45c2-9341-c232a9685355'} when evaluating shadowRoot. Trying to recursively call
// find_element() on it as before yielded "AttributeError: 'dict' object has no attribute
// 'find_element'". (For all I know, the version of Selenium that I was using was just too old for
// the current chromedriver, or this was a bug in python-selenium.)
//
// So, what we have instead here is an approximate JavaScript reimplementation of Selenium's
// element-finding code. :-/ It's possible that this could be switched back to using Selenium to
// find elements, but the current approach seems to work for now.
func (p *page) getNoWait(locs []loc) (selenium.WebElement, error) {
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
			return nil, fmt.Errorf("invalid 'by' %q", by)
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

// checkGone waits for the element described by locs to not be found.
// It fails the test if the element remains present.
func (p *page) checkGone(locs []loc) {
	if err := wait(func() error {
		_, err := p.getNoWait(locs)
		if err == nil {
			return errors.New("still exists")
		}
		return nil
	}); err != nil {
		p.t.Fatalf("Failed waiting for element to disappear for %v: %v", p.desc(), err)
	}
}

// click clicks on the element matched by locs.
func (p *page) click(locs []loc) {
	if err := p.getOrFail(locs).Click(); err != nil {
		p.t.Fatalf("Failed clicking %v for %v: %v", locs, p.desc(), err)
	}
}

// clickOption clicks the <option> with the supplied text in the <select> matched by sel.
func (p *page) clickOption(sel []loc, option string) {
	opts, err := p.getOrFail(sel).FindElements(selenium.ByTagName, "option")
	if err != nil {
		p.t.Fatalf("Failed getting %v options for %v: %v", sel, p.desc(), err)
	}
	for _, opt := range opts {
		if strings.TrimSpace(p.getTextOrFail(opt, false)) == option {
			if err := opt.Click(); err != nil {
				p.t.Fatalf("Failed clicking %v option %q for %v: %v", sel, option, p.desc(), err)
			}
			return
		}
	}
	p.t.Fatalf("Failed finding %v option %q for %v: %v", sel, option, p.desc(), err)
}

// getTextOrFail returns el's text, failing the test on error.
// If ignoreStale is true, errors caused by the element no longer existing are ignored.
// Tests should consider calling checkText instead.
func (p *page) getTextOrFail(el selenium.WebElement, ignoreStale bool) string {
	text, err := el.Text()
	if ignoreStale && isStaleElementError(err) {
		return ""
	} else if err != nil {
		p.t.Fatalf("Failed getting element text for %v: %v", p.desc(), err)
	}
	return text
}

// getAttrOrFail returns the named attribute from el, failing the test on error.
// If ignoreStale is true, errors caused by the element no longer existing are ignored.
// Tests should consider calling checkAttr instead.
func (p *page) getAttrOrFail(el selenium.WebElement, name string, ignoreStale bool) string {
	val, err := el.GetAttribute(name)
	if isMissingAttrError(err) {
		return ""
	} else if ignoreStale && isStaleElementError(err) {
		return ""
	} else if err != nil {
		p.t.Fatalf("Failed getting attribute %q for %v: %v", name, p.desc(), err)
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
		p.t.Fatalf("Failed getting selected state for %v: %v", p.desc(), err)
	}
	return sel
}

// sendKeys sends text to the element matched by locs.
//
// Note that this doesn't work right on systems without a US Qwerty layout due to a ChromeDriver bug
// that will never be fixed:
//
//  https://bugs.chromium.org/p/chromedriver/issues/detail?id=553
//  https://chromedriver.chromium.org/help/keyboard-support
//  https://github.com/SeleniumHQ/selenium/issues/4523
//
// Specifically, the requested text is sent to the element, but JavaScript key events contain
// incorrect values (e.g. when sending 'z' with Dvorak, the JS event will contain '/').
func (p *page) sendKeys(locs []loc, text string, clearFirst bool) {
	el := p.getOrFail(locs)
	if clearFirst {
		if err := el.Clear(); err != nil {
			p.t.Fatalf("Failed clearing %v for %v: %v", locs, p.desc(), err)
		}
	}
	if err := el.SendKeys(text); err != nil {
		p.t.Fatalf("Failed sending keys to %v for %v: %v", locs, p.desc(), err)
	}
}

// setText clears the element matched by locs and types text into it.
func (p *page) setText(locs []loc, text string) {
	p.sendKeys(locs, text, true /* clearFirst */)
}

// emitKeyDown emits a 'keydown' JavaScript event with the supplied data.
// This avoids the ChromeDriver bug described in sendKeys.
func (p *page) emitKeyDown(key string, keyCode int, alt bool) {
	s := fmt.Sprintf(
		"document.body.dispatchEvent("+
			"new KeyboardEvent('keydown', { key: '%s', keyCode: %d, altKey: %v }))",
		key, keyCode, alt)
	if _, err := p.wd.ExecuteScript(s, nil); err != nil {
		p.t.Fatalf("Failed emitting %q key down event for %v: %v", key, p.desc(), err)
	}
}

// clickSongRowCheckbox clicks the checkbox for the song at 0-based index
// idx in the table matched by locs. If key (e.g. selenium.ShiftKey) is non-empty,
// it is held while performing the click.
func (p *page) clickSongRowCheckbox(locs []loc, idx int, key string) {
	table := p.getOrFail(locs)
	sel := fmt.Sprintf("tr:nth-child(%d) td:first-child input", idx+1)
	cb, err := table.FindElement(selenium.ByCSSSelector, sel)
	if err != nil {
		p.t.Fatalf("Failed finding checkbox %d (%q) for %v: %v", idx, sel, p.desc(), err)
	}
	if key != "" {
		if err := p.wd.KeyDown(key); err != nil {
			p.t.Fatalf("Failed pressing key before clicking checkbox %d for %v: %v", idx, p.desc(), err)
		}
		defer func() {
			if err := p.wd.KeyUp(key); err != nil {
				p.t.Fatalf("Failed releasing key after clicking checkbox %d for %v: %v", idx, p.desc(), err)
			}
		}()
	}
	if err := cb.Click(); err != nil {
		p.t.Fatalf("Failed clicking checkbox %d for %v: %v", idx, p.desc(), err)
	}
}

// rightClickSongRow right-clicks on the playlist song at the specified index
// in the table matched by locs.
func (p *page) rightClickSongRow(locs []loc, idx int) {
	table := p.getOrFail(playlistTable)
	sel := fmt.Sprintf("tbody tr:nth-child(%d)", idx+1)
	row, err := table.FindElement(selenium.ByCSSSelector, sel)
	if err != nil {
		p.t.Fatalf("Failed finding song %d (%q) for %v: %v", idx, sel, p.desc(), err)
	}
	// These coordinates are apparently relative to the element's upper-left corner.
	if err := row.MoveTo(3, 3); err != nil {
		p.t.Fatalf("Failed moving mouse to song for %v: %v", p.desc(), err)
	}
	if err := p.wd.Click(selenium.RightButton); err != nil {
		p.t.Fatalf("Failed right-clicking on song for %v: %v", p.desc(), err)
	}
}

type checkboxState uint32

const (
	checkboxChecked     checkboxState = 1 << iota
	checkboxTransparent               // has "transparent" class
)

// checkText checks that text of the element matched by locs is matched by wantRegexp.
// Spacing can be weird if the text is spread across multiple child nodes.
func (p *page) checkText(locs []loc, wantRegexp string) {
	el := p.getOrFail(locs)
	want := regexp.MustCompile(wantRegexp)
	if err := wait(func() error {
		if got := p.getTextOrFail(el, false); !want.MatchString(got) {
			return fmt.Errorf("got %q; want %q", got, want)
		}
		return nil
	}); err != nil {
		p.t.Fatalf("Bad text in element for %v: %v", p.desc(), err)
	}
}

// checkAttr checks that attribute attr of the element matched by locs equals want.
func (p *page) checkAttr(locs []loc, attr, want string) {
	el := p.getOrFail(locs)
	if err := wait(func() error {
		if got := p.getAttrOrFail(el, attr, false); got != want {
			return fmt.Errorf("got %q; want %q", got, want)
		}
		return nil
	}); err != nil {
		p.t.Fatalf("Bad %v attribute for %v: %v", attr, p.desc(), err)
	}
}

// checkCheckbox verifies that the checkbox element matched by locs has the specified state.
// TODO: The "check" in this name is ambiguous.
func (p *page) checkCheckbox(locs []loc, state checkboxState) {
	el := p.getOrFail(locs)
	if got, want := p.getSelectedOrFail(el, false), state&checkboxChecked != 0; got != want {
		p.t.Fatalf("Checkbox %v has checked state %v for %v; want %v", locs, got, p.desc(), want)
	}
	class := p.getAttrOrFail(el, "class", false)
	if got, want := strings.Contains(class, "transparent"), state&checkboxTransparent != 0; got != want {
		p.t.Fatalf("Checkbox %v has transparent state %v for %v; want %v", locs, got, p.desc(), want)
	}
}

// getSongsFromTable returns songInfos describing the supplied <table> within a <song-table>.
func (p *page) getSongsFromTable(table selenium.WebElement) []songInfo {
	var songs []songInfo
	rows, err := table.FindElements(selenium.ByTagName, "tr")
	if err != nil {
		p.t.Fatalf("Failed getting song rows for %v: %v", p.desc(), err)
	}
	if len(rows) == 0 {
		return nil
	}
	for _, row := range rows[1:] { // skip header
		cols, err := row.FindElements(selenium.ByTagName, "td")
		if isStaleElementError(err) {
			break // table was modified while we were reading it
		} else if err != nil {
			p.t.Fatalf("Failed getting song columns for %v: %v", p.desc(), err)
		}
		// Final column is time; first column may be checkbox.
		song := songInfo{
			artist: p.getTextOrFail(cols[len(cols)-4], true),
			title:  p.getTextOrFail(cols[len(cols)-3], true),
			album:  p.getTextOrFail(cols[len(cols)-2], true),
		}

		// TODO: Copy time from last column.
		class := p.getAttrOrFail(row, "class", true)
		active := strings.Contains(class, "active")
		song.active = &active
		menu := strings.Contains(class, "menu")
		song.menu = &menu

		if len(cols) == 5 {
			el, err := cols[0].FindElement(selenium.ByTagName, "input")
			if err == nil {
				checked := p.getSelectedOrFail(el, true)
				song.checked = &checked
			} else if !isStaleElementError(err) {
				p.t.Fatalf("Failed getting checkbox for %v: %v", p.desc(), err)
			}
		}
		songs = append(songs, song)
	}
	return songs
}

// checkSearchResults waits for the search results table to contain songs.
func (p *page) checkSearchResults(songs []db.Song, checks ...songListCheck) {
	want := make([]songInfo, len(songs))
	for i := range songs {
		want[i] = makeSongInfo(songs[i])
	}
	for _, c := range checks {
		c(want)
	}

	table := p.getOrFail(searchResultsTable)
	if err := wait(func() error {
		got := p.getSongsFromTable(table)
		if !songInfoSlicesEqual(want, got) {
			return errors.New("songs don't match")
		}
		return nil
	}); err != nil {
		got := p.getSongsFromTable(table)
		msg := fmt.Sprintf("Bad search results for %v\n", p.desc())
		msg += "Want:\n"
		for _, s := range want {
			msg += "  " + s.String() + "\n"
		}
		msg += "Got:\n"
		for _, s := range got {
			msg += "  " + s.String() + "\n"
		}
		p.t.Fatal(msg)
	}
}

// checkPlaylist waits for the playlist table to contain songs.
func (p *page) checkPlaylist(songs []db.Song, checks ...songListCheck) {
	want := make([]songInfo, len(songs))
	for i := range songs {
		want[i] = makeSongInfo(songs[i])
	}
	for _, c := range checks {
		c(want)
	}

	table := p.getOrFail(playlistTable)
	if err := wait(func() error {
		got := p.getSongsFromTable(table)
		if !songInfoSlicesEqual(want, got) {
			return errors.New("songs don't match")
		}
		return nil
	}); err != nil {
		got := p.getSongsFromTable(table)
		msg := fmt.Sprintf("Bad playlist for %v\n", p.desc())
		msg += "Want:\n"
		for _, s := range want {
			msg += "  " + s.String() + "\n"
		}
		msg += "Got:\n"
		for _, s := range got {
			msg += "  " + s.String() + "\n"
		}
		p.t.Fatal(msg)
	}
}

// checkPresentation waits for the presentation layer to display the specified songs.
func (p *page) checkPresentation(cur, next *db.Song) {
	var curWant, nextWant *songInfo
	if cur != nil {
		s := makeSongInfo(*cur)
		curWant = &s
	}
	if next != nil {
		s := makeSongInfo(*next)
		nextWant = &s
	}

	getSongs := func() (cur, next *songInfo) {
		if d, err := p.getOrFail(currentArtistDiv).IsDisplayed(); err != nil {
			p.t.Fatalf("Failed checking visibility of current artist for %v: %v", p.desc(), err)
		} else if d {
			cur = &songInfo{
				artist: p.getTextOrFail(p.getOrFail(currentArtistDiv), false),
				title:  p.getTextOrFail(p.getOrFail(currentTitleDiv), false),
				album:  p.getTextOrFail(p.getOrFail(currentAlbumDiv), false),
			}
		}
		if d, err := p.getOrFail(nextArtistDiv).IsDisplayed(); err != nil {
			p.t.Fatalf("Failed checking visibility of next artist for %v: %v", p.desc(), err)
		} else if d {
			next = &songInfo{
				artist: p.getTextOrFail(p.getOrFail(nextArtistDiv), false),
				title:  p.getTextOrFail(p.getOrFail(nextTitleDiv), false),
				album:  p.getTextOrFail(p.getOrFail(nextAlbumDiv), false),
			}
		}
		return cur, next
	}
	equal := func(want, got *songInfo) bool {
		if (want == nil) != (got == nil) {
			return false
		}
		if want == nil {
			return true
		}
		return songInfosEqual(*want, *got)
	}
	if err := wait(func() error {
		curGot, nextGot := getSongs()
		if !equal(curWant, curGot) || !equal(nextWant, nextGot) {
			return errors.New("songs don't match")
		}
		return nil
	}); err != nil {
		curGot, nextGot := getSongs()
		msg := fmt.Sprintf("Bad presentation songs for %v\n", p.desc())
		msg += "Want:\n"
		msg += "  " + curWant.String() + "\n"
		msg += "  " + nextWant.String() + "\n"
		msg += "Got:\n"
		msg += "  " + curGot.String() + "\n"
		msg += "  " + nextGot.String() + "\n"
		p.t.Fatal(msg)
	}
}

// checkSong verifies that the current song matches s.
// By default, just the artist, title, and album are examined,
// but additional checks can be specified.
func (p *page) checkSong(s db.Song, checks ...songCheck) {
	want := makeSongInfo(s)
	for _, c := range checks {
		c(&want)
	}

	var got songInfo
	if err := wait(func() error {
		imgTitle := p.getAttrOrFail(p.getOrFail(coverImage), "title", false)
		rating := p.getTextOrFail(p.getOrFail(ratingOverlayDiv), false)
		time := p.getTextOrFail(p.getOrFail(timeDiv), false)
		au := p.getOrFail(audio)
		paused := p.getAttrOrFail(au, "paused", true) != ""
		ended := p.getAttrOrFail(au, "ended", true) != ""

		var filename string
		src := p.getAttrOrFail(au, "src", true)
		if u, err := url.Parse(src); err == nil {
			filename = u.Query().Get("filename")
		}

		got = songInfo{
			artist:    p.getTextOrFail(p.getOrFail(artistDiv), false),
			title:     p.getTextOrFail(p.getOrFail(titleDiv), false),
			album:     p.getTextOrFail(p.getOrFail(albumDiv), false),
			paused:    &paused,
			ended:     &ended,
			filename:  &filename,
			ratingStr: &rating,
			imgTitle:  &imgTitle,
			timeStr:   &time,
		}
		if !songInfosEqual(want, got) {
			return errors.New("songs don't match")
		}
		return nil
	}); err != nil {
		msg := fmt.Sprintf("Bad song for %v: %v\n", p.desc(), err)
		msg += "Want: " + want.String() + "\n"
		msg += "Got:  " + got.String()
		p.t.Fatal(msg)
	}
}
