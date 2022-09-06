// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
	"github.com/tebeka/selenium"
)

// testEmail is used to log in to the dev_appserver.py's fake login page:
// https://cloud.google.com/appengine/docs/standard/go111/users
const testEmail = "testuser@example.org"

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
	// Fake login page served by dev_appserver.py.
	loginEmail  = joinLocs(loc{selenium.ByID, "email"})
	loginButton = joinLocs(loc{selenium.ByID, "submit-login"})

	// Note that selenium.ByTagName doesn't seem to work within shadow roots.
	// Use selenium.ByCSSSelector instead for referencing deeply-nested elements.
	document = []loc(nil)
	body     = joinLocs(loc{selenium.ByTagName, "body"})

	optionsDialog   = joinLocs(body, loc{selenium.ByCSSSelector, "dialog.options > span"})
	optionsOKButton = joinLocs(optionsDialog, loc{selenium.ByID, "ok-button"})
	themeSelect     = joinLocs(optionsDialog, loc{selenium.ByID, "theme-select"})
	gainTypeSelect  = joinLocs(optionsDialog, loc{selenium.ByID, "gain-type-select"})
	preAmpRange     = joinLocs(optionsDialog, loc{selenium.ByID, "pre-amp-range"})

	infoDialog        = joinLocs(body, loc{selenium.ByCSSSelector, "dialog.song-info > span"})
	infoArtist        = joinLocs(infoDialog, loc{selenium.ByID, "artist"})
	infoTitle         = joinLocs(infoDialog, loc{selenium.ByID, "title"})
	infoAlbum         = joinLocs(infoDialog, loc{selenium.ByID, "album"})
	infoTrack         = joinLocs(infoDialog, loc{selenium.ByID, "track"})
	infoDate          = joinLocs(infoDialog, loc{selenium.ByID, "date"})
	infoLength        = joinLocs(infoDialog, loc{selenium.ByID, "length"})
	infoRating        = joinLocs(infoDialog, loc{selenium.ByID, "rating"})
	infoTags          = joinLocs(infoDialog, loc{selenium.ByID, "tags"})
	infoDismissButton = joinLocs(infoDialog, loc{selenium.ByID, "dismiss-button"})

	statsDialog       = joinLocs(body, loc{selenium.ByCSSSelector, "dialog.stats > span"})
	statsDecadesChart = joinLocs(statsDialog, loc{selenium.ByID, "decades-chart"})
	statsRatingsChart = joinLocs(statsDialog, loc{selenium.ByID, "ratings-chart"})

	menu           = joinLocs(body, loc{selenium.ByCSSSelector, "dialog.menu > span"})
	menuFullscreen = joinLocs(menu, loc{selenium.ByID, "fullscreen"})
	menuOptions    = joinLocs(menu, loc{selenium.ByID, "options"})
	menuStats      = joinLocs(menu, loc{selenium.ByID, "stats"})
	menuInfo       = joinLocs(menu, loc{selenium.ByID, "info"})
	menuPlay       = joinLocs(menu, loc{selenium.ByID, "play"})
	menuRemove     = joinLocs(menu, loc{selenium.ByID, "remove"})
	menuTruncate   = joinLocs(menu, loc{selenium.ByID, "truncate"})

	playView         = joinLocs(loc{selenium.ByTagName, "play-view"})
	menuButton       = joinLocs(playView, loc{selenium.ByID, "menu-button"})
	coverImage       = joinLocs(playView, loc{selenium.ByID, "cover-img"})
	ratingOverlayDiv = joinLocs(playView, loc{selenium.ByID, "rating-overlay"})
	artistDiv        = joinLocs(playView, loc{selenium.ByID, "artist"})
	titleDiv         = joinLocs(playView, loc{selenium.ByID, "title"})
	albumDiv         = joinLocs(playView, loc{selenium.ByID, "album"})
	timeDiv          = joinLocs(playView, loc{selenium.ByID, "time"})
	prevButton       = joinLocs(playView, loc{selenium.ByID, "prev"})
	playPauseButton  = joinLocs(playView, loc{selenium.ByID, "play-pause"})
	nextButton       = joinLocs(playView, loc{selenium.ByID, "next"})

	audioWrapper = joinLocs(playView, loc{selenium.ByCSSSelector, "audio-wrapper"})
	audio        = joinLocs(audioWrapper, loc{selenium.ByCSSSelector, "audio"})

	playlistTable = joinLocs(playView, loc{selenium.ByID, "playlist"},
		loc{selenium.ByCSSSelector, "table"})

	updateDialog       = joinLocs(body, loc{selenium.ByCSSSelector, "dialog.update > span"})
	updateOneStar      = joinLocs(updateDialog, loc{selenium.ByCSSSelector, "#rating a:nth-child(1)"})
	updateTwoStars     = joinLocs(updateDialog, loc{selenium.ByCSSSelector, "#rating a:nth-child(2)"})
	updateThreeStars   = joinLocs(updateDialog, loc{selenium.ByCSSSelector, "#rating a:nth-child(3)"})
	updateFourStars    = joinLocs(updateDialog, loc{selenium.ByCSSSelector, "#rating a:nth-child(4)"})
	updateFiveStars    = joinLocs(updateDialog, loc{selenium.ByCSSSelector, "#rating a:nth-child(5)"})
	updateTagsTextarea = joinLocs(updateDialog, loc{selenium.ByID, "tags-textarea"})
	updateTagSuggester = joinLocs(updateDialog, loc{selenium.ByID, "tag-suggester"})
	updateCloseImage   = joinLocs(updateDialog, loc{selenium.ByID, "close-icon"})

	fullscreenOverlay = joinLocs(playView, loc{selenium.ByCSSSelector, "fullscreen-overlay"})
	currentArtistDiv  = joinLocs(fullscreenOverlay, loc{selenium.ByID, "current-artist"})
	currentTitleDiv   = joinLocs(fullscreenOverlay, loc{selenium.ByID, "current-title"})
	currentAlbumDiv   = joinLocs(fullscreenOverlay, loc{selenium.ByID, "current-album"})
	nextArtistDiv     = joinLocs(fullscreenOverlay, loc{selenium.ByID, "next-artist"})
	nextTitleDiv      = joinLocs(fullscreenOverlay, loc{selenium.ByID, "next-title"})
	nextAlbumDiv      = joinLocs(fullscreenOverlay, loc{selenium.ByID, "next-album"})

	searchView                = joinLocs(loc{selenium.ByTagName, "search-view"})
	keywordsInput             = joinLocs(searchView, loc{selenium.ByID, "keywords-input"})
	tagsInput                 = joinLocs(searchView, loc{selenium.ByID, "tags-input"})
	minDateInput              = joinLocs(searchView, loc{selenium.ByID, "min-date-input"})
	maxDateInput              = joinLocs(searchView, loc{selenium.ByID, "max-date-input"})
	firstTrackCheckbox        = joinLocs(searchView, loc{selenium.ByID, "first-track-checkbox"})
	unratedCheckbox           = joinLocs(searchView, loc{selenium.ByID, "unrated-checkbox"})
	minRatingSelect           = joinLocs(searchView, loc{selenium.ByID, "min-rating-select"})
	orderByLastPlayedCheckbox = joinLocs(searchView, loc{selenium.ByID, "order-by-last-played-checkbox"})
	maxPlaysInput             = joinLocs(searchView, loc{selenium.ByID, "max-plays-input"})
	firstPlayedSelect         = joinLocs(searchView, loc{selenium.ByID, "first-played-select"})
	lastPlayedSelect          = joinLocs(searchView, loc{selenium.ByID, "last-played-select"})
	presetSelect              = joinLocs(searchView, loc{selenium.ByID, "preset-select"})
	searchButton              = joinLocs(searchView, loc{selenium.ByID, "search-button"})
	resetButton               = joinLocs(searchView, loc{selenium.ByID, "reset-button"})
	luckyButton               = joinLocs(searchView, loc{selenium.ByID, "lucky-button"})
	appendButton              = joinLocs(searchView, loc{selenium.ByID, "append-button"})
	insertButton              = joinLocs(searchView, loc{selenium.ByID, "insert-button"})
	replaceButton             = joinLocs(searchView, loc{selenium.ByID, "replace-button"})

	searchResultsCheckbox = joinLocs(searchView, loc{selenium.ByID, "results-table"},
		loc{selenium.ByCSSSelector, `th input[type="checkbox"]`})
	searchResultsTable = joinLocs(searchView, loc{selenium.ByID, "results-table"},
		loc{selenium.ByCSSSelector, "table"})
)

const (
	// Text for minRatingSelect option. Note hacky U+2009 (THIN SPACE) characters.
	oneStar    = "★"
	twoStars   = "★ ★"
	threeStars = "★ ★ ★"
	fourStars  = "★ ★ ★ ★"
	fiveStars  = "★ ★ ★ ★ ★"

	// Text for firstPlayedSelect and lastPlayedSelect options.
	unsetTime   = ""
	oneDay      = "one day"
	oneWeek     = "one week"
	oneMonth    = "one month"
	threeMonths = "three months"
	sixMonths   = "six months"
	oneYear     = "one year"
	threeYears  = "three years"
	fiveYears   = "five years"

	// Text and values for themeSelect options.
	themeAuto       = "Auto"
	themeLight      = "Light"
	themeDark       = "Dark"
	themeAutoValue  = "0"
	themeLightValue = "1"
	themeDarkValue  = "2"

	// Text and values for gainTypeSelect options.
	gainAuto       = "Auto"
	gainAlbum      = "Album"
	gainTrack      = "Track"
	gainNone       = "None"
	gainAutoValue  = "3"
	gainAlbumValue = "0"
	gainTrackValue = "1"
	gainNoneValue  = "2"

	// Text for presetSelect options.
	// These match the presets defined in sendConfig() in web_test.go.
	presetInstrumentalOld = "instrumental old"
	presetMellow          = "mellow"
	presetPlayedOnce      = "played once"
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

func newPage(t *testing.T, wd selenium.WebDriver, baseURL string) *page {
	p := page{t, wd, ""}
	if err := wd.Get(baseURL); err != nil {
		t.Fatalf("Failed loading %v: %v", baseURL, err)
	}
	p.configPage()
	return &p
}

// configPage configures the page for testing. This is called automatically.
func (p *page) configPage() {
	// If we're at dev_appserver.py's fake login page, log in to get to the app.
	if btn, err := p.getNoWait(loginButton); err == nil {
		p.setText(loginEmail, testEmail)
		if err := btn.Click(); err != nil {
			p.t.Fatalf("Failed clicking login button at %v: %v", p.desc(), err)
		}
		p.getOrFail(playView)
	}
	if _, err := p.wd.ExecuteScript("document.test.setPlayDelayMs(10)", nil); err != nil {
		p.t.Fatalf("Failed setting short play delay at %v: %v", p.desc(), err)
	}
	if _, err := p.wd.ExecuteScript("document.test.reset()", nil); err != nil {
		p.t.Fatalf("Failed resetting page at %v: %v", p.desc(), err)
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
		p.t.Fatalf("Reloading page at %v failed: %v", p.desc(), err)
	}
	p.configPage()
}

// refreshTags instructs the page to refresh the list of available tags from the server.
func (p *page) refreshTags() {
	if _, err := p.wd.ExecuteScript("document.test.updateTags()", nil); err != nil {
		p.t.Fatalf("Failed refreshing tags at %v: %v", p.desc(), err)
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
		p.t.Fatalf("Failed getting %v at %v: %v", locs, p.desc(), err)
	}
	return el
}

// getNoWait returns the first element matched by locs.
//
// If there is more than one element in locs, they will be used successively, e.g.
// loc[1] is used to search inside the element matched by loc[0].
//
// If an element has a shadow root (per its 'shadowRoot' property),
// the shadow root will be used for the next search.
//
// If locs is empty, the document element is returned.
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
	if len(locs) == 0 {
		query = "document.documentElement"
	} else {
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

// checkGone waits for the element described by locs to not be present in the document tree.
// It fails the test if the element remains present.
// Use checkDisplayed for elements that use e.g. display:none.
func (p *page) checkGone(locs []loc) {
	if err := wait(func() error {
		_, err := p.getNoWait(locs)
		if err == nil {
			return errors.New("still exists")
		}
		return nil
	}); err != nil {
		p.t.Fatalf("Failed waiting for element to be gone at %v: %v", p.desc(), err)
	}
}

// click clicks on the element matched by locs.
func (p *page) click(locs []loc) {
	if err := p.getOrFail(locs).Click(); err != nil {
		p.t.Fatalf("Failed clicking %v at %v: %v", locs, p.desc(), err)
	}
}

// clickOption clicks the <option> with the supplied text in the <select> matched by sel.
func (p *page) clickOption(sel []loc, option string) {
	opts, err := p.getOrFail(sel).FindElements(selenium.ByTagName, "option")
	if err != nil {
		p.t.Fatalf("Failed getting %v options at %v: %v", sel, p.desc(), err)
	} else if len(opts) == 0 {
		p.t.Fatalf("No options for %v at %v: %v", sel, p.desc(), err)
	}
	names := make([]string, 0, len(opts))
	for _, opt := range opts {
		name := strings.TrimSpace(p.getTextOrFail(opt, false))
		if name == option {
			if err := opt.Click(); err != nil {
				p.t.Fatalf("Failed clicking %v option %q at %v: %v", sel, option, p.desc(), err)
			}
			return
		}
		names = append(names, name)
	}
	p.t.Fatalf("Failed finding %v option %q among %q at %v", sel, option, names, p.desc())
}

// getTextOrFail returns el's text, failing the test on error.
// If ignoreStale is true, errors caused by the element no longer existing are ignored.
// Tests should consider calling checkText instead.
func (p *page) getTextOrFail(el selenium.WebElement, ignoreStale bool) string {
	text, err := el.Text()
	if ignoreStale && isStaleElementError(err) {
		return ""
	} else if err != nil {
		p.t.Fatalf("Failed getting element text at %v: %v", p.desc(), err)
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
		p.t.Fatalf("Failed getting attribute %q at %v: %v", name, p.desc(), err)
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
		p.t.Fatalf("Failed getting selected state at %v: %v", p.desc(), err)
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
			p.t.Fatalf("Failed clearing %v at %v: %v", locs, p.desc(), err)
		}
	}
	if err := el.SendKeys(text); err != nil {
		p.t.Fatalf("Failed sending keys to %v at %v: %v", locs, p.desc(), err)
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
		p.t.Fatalf("Failed emitting %q key down event at %v: %v", key, p.desc(), err)
	}
}

// clickSongRowCheckbox clicks the checkbox for the song at 0-based index
// idx in the table matched by locs. If key (e.g. selenium.ShiftKey) is non-empty,
// it is held while performing the click.
func (p *page) clickSongRowCheckbox(locs []loc, idx int, key string) {
	cb, err := p.getSongRow(locs, idx).FindElement(selenium.ByCSSSelector, "td:first-child input")
	if err != nil {
		p.t.Fatalf("Failed finding checkbox in song %d at %v: %v", idx, p.desc(), err)
	}
	if key != "" {
		if err := p.wd.KeyDown(key); err != nil {
			p.t.Fatalf("Failed pressing key before clicking checkbox %d at %v: %v", idx, p.desc(), err)
		}
		defer func() {
			if err := p.wd.KeyUp(key); err != nil {
				p.t.Fatalf("Failed releasing key after clicking checkbox %d at %v: %v", idx, p.desc(), err)
			}
		}()
	}
	if err := cb.Click(); err != nil {
		p.t.Fatalf("Failed clicking checkbox %d at %v: %v", idx, p.desc(), err)
	}
}

// rightClickSongRow right-clicks on the song at the specified index in the table matched by locs.
func (p *page) rightClickSongRow(locs []loc, idx int) {
	row := p.getSongRow(locs, idx)
	// The documentation says "MoveTo moves the mouse to relative coordinates from center of
	// element", but these coordinates seem to be relative to the element's upper-left corner.
	if err := row.MoveTo(3, 3); err != nil {
		p.t.Fatalf("Failed moving mouse to song at %v: %v", p.desc(), err)
	}
	if err := p.wd.Click(selenium.RightButton); err != nil {
		p.t.Fatalf("Failed right-clicking on song at %v: %v", p.desc(), err)
	}
}

// dragSongRow drags the song at srcIdx at table locs to dstIdx.
// dstOffsetY describes the Y offset from the center of dstIdx.
func (p *page) dragSongRow(locs []loc, srcIdx, dstIdx, dstOffsetY int) {
	// I initially tried using MoveTo, ButtonDown, and ButtonUp to drag the row, but this seems to
	// be broken in WebDriver:
	//
	//  https://github.com/seleniumhq/selenium-google-code-issue-archive/issues/3604
	//  https://github.com/SeleniumHQ/selenium/issues/9878
	//  https://github.com/w3c/webdriver/issues/1488
	//
	// The dragstart event is emitted when I start the drag, but the page never receives dragenter,
	// dragover, or dragend, and the button seems to remain depressed even after calling ButtonUp.
	// Some commenters say they were able to get this to work by calling MoveTo twice, but it
	// doesn't seem to change anything for me (regardless of whether I move relative to the source
	// or destination row).
	//
	// It seems like the state of the art is to just emit fake drag events from JavaScript:
	//
	//  https://gist.github.com/rcorreia/2362544
	//  https://stackoverflow.com/questions/61448931/selenium-drag-and-drop-issue-in-chrome
	//
	// Sigh.
	src := p.getSongRow(locs, srcIdx)
	dst := p.getSongRow(locs, dstIdx)
	if _, err := p.wd.ExecuteScript(
		"document.test.dragElement(arguments[0], arguments[1], 0, arguments[2])",
		[]interface{}{src, dst, dstOffsetY}); err != nil {
		p.t.Fatalf("Failed dragging song %v to %v at %v: %v", srcIdx, dstIdx, p.desc(), err)
	}
}

// getSongRow returns the row for song at the 0-based specified index in the table matched by locs.
func (p *page) getSongRow(locs []loc, idx int) selenium.WebElement {
	table := p.getOrFail(locs)
	sel := fmt.Sprintf("tbody tr:nth-child(%d)", idx+1)
	row, err := table.FindElement(selenium.ByCSSSelector, sel)
	if err != nil {
		p.t.Fatalf("Failed finding song %d (%q) at %v: %v", idx, sel, p.desc(), err)
	}
	return row
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
		p.t.Fatalf("Bad text in element at %v: %v", p.desc(), err)
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
		p.t.Fatalf("Bad %q attribute at %v: %v", attr, p.desc(), err)
	}
}

// checkDisplayed checks that the element matched by locs is or isn't displayed.
// Note that the element must be present in the document tree.
func (p *page) checkDisplayed(locs []loc, want bool) {
	el := p.getOrFail(locs)
	if err := wait(func() error {
		if got, err := el.IsDisplayed(); err != nil {
			p.t.Fatalf("Failed getting displayed state at %v: %v", p.desc(), err)
		} else if got != want {
			return fmt.Errorf("got %v; want %v", got, want)
		}
		return nil
	}); err != nil {
		p.t.Fatalf("Bad displayed state at %v: %v", p.desc(), err)
	}
}

// checkCheckbox verifies that the checkbox element matched by locs has the specified state.
// TODO: The "check" in this name is ambiguous.
func (p *page) checkCheckbox(locs []loc, state checkboxState) {
	el := p.getOrFail(locs)
	if got, want := p.getSelectedOrFail(el, false), state&checkboxChecked != 0; got != want {
		p.t.Fatalf("Checkbox %v has checked state %v at %v; want %v", locs, got, p.desc(), want)
	}
	class := p.getAttrOrFail(el, "class", false)
	if got, want := strings.Contains(class, "transparent"), state&checkboxTransparent != 0; got != want {
		p.t.Fatalf("Checkbox %v has transparent state %v at %v; want %v", locs, got, p.desc(), want)
	}
}

// getSongsFromTable returns songInfos describing the supplied <table> within a <song-table>.
func (p *page) getSongsFromTable(table selenium.WebElement) []songInfo {
	var songs []songInfo
	rows, err := table.FindElements(selenium.ByTagName, "tr")
	if err != nil {
		p.t.Fatalf("Failed getting song rows at %v: %v", p.desc(), err)
	}
	if len(rows) == 0 {
		return nil
	}
	for _, row := range rows[1:] { // skip header
		cols, err := row.FindElements(selenium.ByTagName, "td")
		if isStaleElementError(err) {
			break // table was modified while we were reading it
		} else if err != nil {
			p.t.Fatalf("Failed getting song columns at %v: %v", p.desc(), err)
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
				p.t.Fatalf("Failed getting checkbox at %v: %v", p.desc(), err)
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
		msg := fmt.Sprintf("Bad search results at %v: %v\n", p.desc(), err.Error())
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
		msg := fmt.Sprintf("Bad playlist at %v\n", p.desc())
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

// checkFullscreenOverlay waits for fullscreen-overlay to display the specified songs.
func (p *page) checkFullscreenOverlay(cur, next *db.Song) {
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
			p.t.Fatalf("Failed checking visibility of current artist at %v: %v", p.desc(), err)
		} else if d {
			cur = &songInfo{
				artist: p.getTextOrFail(p.getOrFail(currentArtistDiv), false),
				title:  p.getTextOrFail(p.getOrFail(currentTitleDiv), false),
				album:  p.getTextOrFail(p.getOrFail(currentAlbumDiv), false),
			}
		}
		if d, err := p.getOrFail(nextArtistDiv).IsDisplayed(); err != nil {
			p.t.Fatalf("Failed checking visibility of next artist at %v: %v", p.desc(), err)
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
		msg := fmt.Sprintf("Bad fullscreen-overlay songs at %v\n", p.desc())
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
	if err := waitFull(func() error {
		imgTitle := p.getAttrOrFail(p.getOrFail(coverImage), "title", false)
		time := p.getTextOrFail(p.getOrFail(timeDiv), false)
		au := p.getOrFail(audio)
		paused := p.getAttrOrFail(au, "paused", true) != ""
		ended := p.getAttrOrFail(au, "ended", true) != ""

		// Count the rating overlay's children to find the displayed rating.
		var rating int
		var err error
		stars := p.getAttrOrFail(p.getOrFail(ratingOverlayDiv), "childElementCount", false)
		if rating, err = strconv.Atoi(stars); err != nil {
			return fmt.Errorf("stars: %v", err)
		}

		var filename string
		src := p.getAttrOrFail(au, "src", true)
		if u, err := url.Parse(src); err == nil {
			filename = u.Query().Get("filename")
		}

		got = songInfo{
			artist:   p.getTextOrFail(p.getOrFail(artistDiv), false),
			title:    p.getTextOrFail(p.getOrFail(titleDiv), false),
			album:    p.getTextOrFail(p.getOrFail(albumDiv), false),
			paused:   &paused,
			ended:    &ended,
			filename: &filename,
			rating:   &rating,
			imgTitle: &imgTitle,
			timeStr:  &time,
		}
		if !songInfosEqual(want, got) {
			return errors.New("songs don't match")
		}
		return nil
	}, want.getTimeout(waitTimeout), waitSleep); err != nil {
		msg := fmt.Sprintf("Bad song at %v: %v\n", p.desc(), err)
		msg += "Want: " + want.String() + "\n"
		msg += "Got:  " + got.String()
		p.t.Fatal(msg)
	}
}

// Describes a bar within a chart in the stats dialog.
type statsChartBar struct {
	pct   int // rounded within [0, 100]
	title string
}

// Matches e.g. "55.3" from the stats bar style attribute "opacity: 0.55; width: 55.3%".
var statsPctRegexp = regexp.MustCompile(`width:\s*(?:calc\()?([^%]+)%`)

// checkStatsChart verifies that the stats dialog chart at locs contains want.
func (p *page) checkStatsChart(locs []loc, want []statsChartBar) {
	chart := p.getOrFail(locs)
	if err := wait(func() error {
		els, err := chart.FindElements(selenium.ByTagName, "span")
		if err != nil {
			return err
		}
		got := make([]statsChartBar, len(els))
		for i, el := range els {
			var bar statsChartBar
			bar.title, _ = el.GetAttribute("title")
			style, _ := el.GetAttribute("style")
			if ms := statsPctRegexp.FindStringSubmatch(style); ms != nil {
				val, _ := strconv.ParseFloat(ms[1], 64)
				bar.pct = int(math.Round(val))
			}
			got[i] = bar
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("got %v; want %v", got, want)
		}
		return nil
	}); err != nil {
		p.t.Fatalf("Bad %v chart at %v: %v", locs, p.desc(), err)
	}
}
