#!/usr/bin/python
# coding=UTF-8

import selenium
import time
import utils
from selenium.common.exceptions import StaleElementReferenceException
from selenium.webdriver.common.action_chains import ActionChains
from selenium.webdriver.common.by import By
from selenium.webdriver.common.keys import Keys

from song import Song

# Loosely based on https://selenium-python.readthedocs.org/page-objects.html.
class InputElement(object):
    def __init__(self, locator):
        self.locator = locator

    def __set__(self, obj, value):
        element = obj.get(self.locator)
        element.clear()
        element.send_keys(value)

    def __get__(self, obj, owner):
        return obj.get(self.locator).get_attribute('value')

    def send_keys(self, value):
        obj.get(self.locator).send_keys(value)

class Page(object):
    # Locators for various elements.
    BODY = (By.TAG_NAME, 'body')

    DIALOG_MANAGER = (By.TAG_NAME, 'dialog-manager')
    OPTIONS_DIALOG = DIALOG_MANAGER + (By.CSS_SELECTOR, '.dialog')
    OPTIONS_OK_BUTTON = OPTIONS_DIALOG + (By.ID, 'ok-button')
    GAIN_TYPE_SELECT = OPTIONS_DIALOG + (By.ID, 'gain-type-select')
    PRE_AMP_RANGE = OPTIONS_DIALOG + (By.ID, 'pre-amp-range')
    PRE_AMP_SPAN = OPTIONS_DIALOG + (By.ID, 'pre-amp-span')

    MUSIC_PLAYER = (By.TAG_NAME, 'music-player')
    ALBUM_DIV = MUSIC_PLAYER + (By.ID, 'album')
    ARTIST_DIV = MUSIC_PLAYER + (By.ID, 'artist')
    AUDIO = MUSIC_PLAYER + (By.CSS_SELECTOR, 'audio')
    COVER_IMAGE = MUSIC_PLAYER + (By.ID, 'cover-img')
    EDIT_TAGS_SUGGESTER = MUSIC_PLAYER + (By.ID, 'edit-tags-suggester')
    EDIT_TAGS_TEXTAREA = MUSIC_PLAYER + (By.ID, 'edit-tags')
    NEXT_BUTTON = MUSIC_PLAYER + (By.ID, 'next')
    PLAY_PAUSE_BUTTON = MUSIC_PLAYER + (By.ID, 'play-pause')
    PLAYLIST_TABLE = MUSIC_PLAYER + (
        By.ID, 'playlist', By.CSS_SELECTOR, 'table')
    PREV_BUTTON = MUSIC_PLAYER + (By.ID, 'prev')
    RATING_OVERLAY_DIV = MUSIC_PLAYER + (By.ID, 'rating-overlay')
    RATING_SPAN = MUSIC_PLAYER + (By.ID, 'rating')
    TIME_DIV = MUSIC_PLAYER + (By.ID, 'time')
    TITLE_DIV = MUSIC_PLAYER + (By.ID, 'title')
    UPDATE_CLOSE_IMAGE = MUSIC_PLAYER + (By.ID, 'update-close')

    MUSIC_SEARCHER = (By.TAG_NAME, 'music-searcher')
    APPEND_BUTTON = MUSIC_SEARCHER + (By.ID, 'append-button')
    FIRST_PLAYED_SELECT = MUSIC_SEARCHER + (By.ID, 'first-played-select')
    FIRST_TRACK_CHECKBOX = MUSIC_SEARCHER + (By.ID, 'first-track-checkbox')
    INSERT_BUTTON = MUSIC_SEARCHER + (By.ID, 'insert-button')
    LAST_PLAYED_SELECT = MUSIC_SEARCHER + (By.ID, 'last-played-select')
    LUCKY_BUTTON = MUSIC_SEARCHER + (By.ID, 'lucky-button')
    MIN_RATING_SELECT = MUSIC_SEARCHER + (By.ID, 'min-rating-select')
    PRESET_SELECT = MUSIC_SEARCHER + (By.ID, 'preset-select')
    REPLACE_BUTTON = MUSIC_SEARCHER + (By.ID, 'replace-button')
    RESET_BUTTON = MUSIC_SEARCHER + (By.ID, 'reset-button')
    SEARCH_BUTTON = MUSIC_SEARCHER + (By.ID, 'search-button')
    SEARCH_RESULTS_CHECKBOX = MUSIC_SEARCHER + (
        By.ID, 'results-table', By.CSS_SELECTOR, 'th input[type="checkbox"]')
    SEARCH_RESULTS_TABLE = MUSIC_SEARCHER + (
        By.ID, 'results-table', By.CSS_SELECTOR, 'table')
    UNRATED_CHECKBOX = MUSIC_SEARCHER + (By.ID, 'unrated-checkbox')

    PRESENTATION_LAYER = MUSIC_PLAYER + (By.CSS_SELECTOR, 'presentation-layer')
    CURRENT_ARTIST_DIV = PRESENTATION_LAYER + (By.ID, 'current-artist')
    CURRENT_TITLE_DIV = PRESENTATION_LAYER + (By.ID, 'current-title')
    CURRENT_ALBUM_DIV = PRESENTATION_LAYER + (By.ID, 'current-album')
    NEXT_ARTIST_DIV = PRESENTATION_LAYER + (By.ID, 'next-artist')
    NEXT_TITLE_DIV = PRESENTATION_LAYER + (By.ID, 'next-title')
    NEXT_ALBUM_DIV = PRESENTATION_LAYER + (By.ID, 'next-album')

    keywords = InputElement(MUSIC_SEARCHER + (By.ID, 'keywords-input'))
    max_plays = InputElement(MUSIC_SEARCHER + (By.ID, 'max-plays-input'))
    tags = InputElement(MUSIC_SEARCHER + (By.ID, 'tags-input'))

    # Text for FIRST_PLAYED_SELECT and LAST_PLAYED_SELECT options.
    UNSET_TIME = '...'
    ONE_DAY = 'one day'
    ONE_WEEK = 'one week'
    ONE_MONTH = 'one month'
    THREE_MONTHS = 'three months'
    SIX_MONTHS = 'six months'
    ONE_YEAR = 'one year'
    THREE_YEARS = 'three years'

    # Text for MIN_RATING_SELECT options and RATING_OVERLAY_DIV.
    ONE_STAR = u'★'
    TWO_STARS = u'★★'
    THREE_STARS = u'★★★'
    FOUR_STARS = u'★★★★'
    FIVE_STARS = u'★★★★★'

    # Text for PRESET_SELECT  options.
    PRESET_INSTRUMENTAL_OLD = 'instrumental old'
    PRESET_MELLOW = 'mellow'
    PRESET_NEW_ALBUMS = 'new albums'
    PRESET_UNRATED = 'unrated'
    PRESET_OLD = 'old'

    # Values for GAIN_TYPE_SELECT options.
    GAIN_ALBUM = '0'
    GAIN_TRACK = '1'
    GAIN_NONE = '2'

    def __init__(self, driver):
        self.driver = driver
        self.reset()

    def reload(self):
        self.driver.refresh()

    def reset(self):
        self.driver.execute_script('document.test.reset()')

    def refresh_tags(self):
        self.driver.execute_script('document.test.updateTags()')

    def get_songs_from_table(self, table):
        songs = []
        try:
            # Skip header.
            for row in table.find_elements_by_tag_name('tr')[1:]:
                cols = row.find_elements_by_tag_name('td')
                # Final column is time; first column may be checkbox.
                song = Song(cols[len(cols)-4].text,
                            cols[len(cols)-3].text,
                            cols[len(cols)-2].text)
                # TODO: Copy time from last column.
                song.highlighted = 'highlight' in row.get_attribute('class')
                song.checked = \
                    len(cols) == 5 and \
                    cols[0].find_elements_by_tag_name('input')[0].is_selected()
                songs.append(song)
        except StaleElementReferenceException:
            # Handle the case where the table is getting rewritten. :-/
            return songs
        return songs

    def get_search_results(self):
        return self.get_songs_from_table(
            self.get(Page.SEARCH_RESULTS_TABLE))

    def get_playlist(self):
        return self.get_songs_from_table(
            self.get(Page.PLAYLIST_TABLE))

    def get_current_song(self):
        '''Gets information about the currently-playing song.

           Returns a tuple containing:
               Song being displayed
               string <audio> src
               bool <audio> paused state
               bool <audio> ended state
               string displaying playback time (e.g. "[0:05 / 3:23]")
               string from rating overlay (e.g. THREE_STARS)
               string cover image title/tooltip text
        '''
        audio = self.get(Page.AUDIO)
        try:
            src = audio.get_attribute('src')
            paused = audio.get_attribute('paused') is not None
            ended = audio.get_attribute('ended') is not None
        except StaleElementReferenceException:
            # Handle the audio element getting swapped.
            src = ''
            paused = False
            ended = False

        song = Song(self.get(Page.ARTIST_DIV).text,
                    self.get(Page.TITLE_DIV).text,
                    self.get(Page.ALBUM_DIV).text)
        return (song, src, paused, ended,
                self.get(Page.TIME_DIV).text,
                self.get(Page.RATING_OVERLAY_DIV).text,
                self.get(Page.COVER_IMAGE).get_attribute('title'))

    def get_presentation_songs(self):
        '''Gets tuple of current and next Song from presentation layer.'''
        return (Song(self.get(Page.CURRENT_ARTIST_DIV).text,
                     self.get(Page.CURRENT_TITLE_DIV).text,
                     self.get(Page.CURRENT_ALBUM_DIV).text),
                Song(self.get(Page.NEXT_ARTIST_DIV).text,
                     self.get(Page.NEXT_TITLE_DIV).text,
                     self.get(Page.NEXT_ALBUM_DIV).text))


    # Waits for and returns the element described by |locator|.
    # |locator| is typically a tuple like (By.ID, 'some-element') or
    # (By.CSS_SELECTOR, 'div.foo').
    #
    # To handle elements nested within one or more Shadow DOMs, |locator|
    # can also contain additional By a <criterion>) pairs. Note that shadow DOMs
    # only support certain mechanisms (just By.ID or By.CSS_SELECTOR?).
    def get(self, locator):
        utils.wait(lambda: self.driver.find_element(locator[0], locator[1]))
        return self.get_nowait(locator)

    def get_nowait(self, locator, el=None):
        if not len(locator) or len(locator) % 2 != 0:
            raise RuntimeError('Invalid locator %s', locator)

        el = (el if el else self.driver).find_element(locator[0], locator[1])
        if len(locator) == 2:
            return el

        root = self.driver.execute_script(
            'return arguments[0].shadowRoot', el)
        return self.get_nowait(locator[2:], root if root else el)

    def click(self, locator):
        self.get(locator).click()

    def select(self, locator, text=None, value=None):
        select = self.get(locator)
        for option in select.find_elements_by_tag_name('option'):
            if (text and option.text == text) or \
                    (value and option.get_attribute('value') == value):
                option.click()
                return
        raise RuntimeError(
            'Failed to find option with text "%s" or value "%s"' % \
            (text, value))

    def wait_until_gone(self, locator):
        def exists():
            try:
                self.get_nowait(locator)
                return True
            except selenium.common.exceptions.NoSuchElementException:
                return False
        utils.wait(lambda: not exists())

    def focus(self, locator):
        self.driver.focus(locator)

    def is_focused(self, locator):
        return self.get(locator) == self.driver.switch_to.active_element

    def click_search_result_checkbox(self, row_index, shift=False):
        action = ActionChains(self.driver)
        if shift:
            action.key_down(Keys.SHIFT)

        checkbox = self.get(Page.SEARCH_RESULTS_TABLE).\
            find_elements_by_tag_name('tr')[row_index + 1].\
            find_elements_by_tag_name('td')[0].\
            find_elements_by_tag_name('input')[0]
        action.click(checkbox)

        if shift:
            action.key_up(Keys.SHIFT)
        action.perform()

    def click_rating(self, num_stars):
        stars = self.get(self.RATING_SPAN).find_elements_by_tag_name('a')
        stars[num_stars-1].click()

    def get_tag_suggestions(self, locator):
        suggester = self.get(locator)
        root = self.driver.execute_script('return arguments[0].shadowRoot',
                                          suggester)
        items = root.find_elements_by_css_selector('#suggestions div')
        return [i.text for i in items]

    def show_options(self):
        # Ideally, something like this could be used here:
        #   ActionChains(self.driver).send_keys(Keys.ALT, 'O').perform()
        # However, ChromeDriver apparently requires that a US keyboard layout is
        # present in able to be able to send the requested key:
        # https://chromedriver.chromium.org/help/keyboard-support
        # If only Dvorak-based layouts are available, then the JS event handler
        # receives 's' instead of 'o' (since the physical 's' key produces 'o'
        # in Dvorak).
        self.driver.execute_script('''
            document.body.dispatchEvent(
                new KeyboardEvent('keydown', {
                    key: 'o',
                    keyCode: 79,
                    altKey: true,
                })
            )
        ''')

    def show_presentation(self):
        self.driver.execute_script('''
            document.body.dispatchEvent(
                new KeyboardEvent('keydown', {
                    key: 'v',
                    keyCode: 86,
                    altKey: true,
                })
            )
        ''')

    def show_update_div(self):
        # Send a keyboard event instead of clicking on COVER_IMAGE since the
        # image will be hidden if the song's cover is missing.
        self.driver.execute_script('''
            document.body.dispatchEvent(
                new KeyboardEvent('keydown', {
                    key: 'r',
                    keyCode: 82,
                    altKey: true,
                })
            )
        ''')

    def rate_and_tag_song(self, song_id, rating=None, tags=None):
        '''Rates and/or tags a song, bypassing the UI.

           Arguments:
               song_id: string
               rating: float in [0.0, 1.0], or None to avoid rating
               tags: list of strings, or None to avoid tagging
        '''
        if rating is None:
            rating = 'null'
        if tags is None:
            tags = 'null'
        self.driver.execute_script(
            'document.test.rateAndTag(%s, %s, %s)' %
            (song_id, rating, tags))

    def report_play(self, song_id, start_time):
        '''Reports a song having been played, bypassing the UI.

           Arguments:
               song_id: string
               start_time: float timestamp
        '''
        self.driver.execute_script(
            'document.test.reportPlay(%s, %f)' % (song_id, start_time))
