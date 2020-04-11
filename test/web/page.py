#!/usr/bin/python
# coding=UTF-8

import selenium
import utils
from selenium.webdriver.common.action_chains import ActionChains
from selenium.webdriver.common.by import By
from selenium.webdriver.common.keys import Keys

from song import Song

# Loosely based on https://selenium-python.readthedocs.org/page-objects.html.
class InputElement(object):
    def __set__(self, obj, value):
        element = obj.get(self.locator)
        element.clear()
        element.send_keys(value)

    def __get__(self, obj, owner):
        return obj.get(self.locator).get_attribute('value')

    def send_keys(self, value):
        obj.get(self.locator).send_keys(value)

class KeywordsInput(InputElement):
    locator = (By.ID, 'keywordsInput')

class MaxPlaysInput(InputElement):
    locator = (By.ID, 'maxPlaysInput')

class TagsInput(InputElement):
    locator = (By.ID, 'tagsInput')

class Page(object):
    keywords = KeywordsInput()
    tags = TagsInput()
    max_plays = MaxPlaysInput()

    # Locators for various elements.
    ALBUM_DIV = (By.ID, 'albumDiv')
    APPEND_BUTTON = (By.ID, 'appendButton')
    ARTIST_DIV = (By.ID, 'artistDiv')
    AUDIO = (By.ID, 'audio')
    BODY = (By.TAG_NAME, 'body')
    COVER_IMAGE = (By.ID, 'coverImage')
    EDIT_TAGS_SUGGESTER = (By.ID, 'editTagsSuggester')
    EDIT_TAGS_TEXTAREA = (By.ID, 'editTagsTextarea')
    FIRST_PLAYED_SELECT = (By.ID, 'firstPlayedSelect')
    FIRST_TRACK_CHECKBOX = (By.ID, 'firstTrackCheckbox')
    INSERT_BUTTON = (By.ID, 'insertButton')
    LAST_PLAYED_SELECT = (By.ID, 'lastPlayedSelect')
    LUCKY_BUTTON = (By.ID, 'luckyButton')
    MIN_RATING_SELECT = (By.ID, 'minRatingSelect')
    NEXT_BUTTON = (By.ID, 'nextButton')
    OPTIONS_OK_BUTTON = (By.ID, 'dialogManager',
                         By.CSS_SELECTOR, '.dialog',
                         By.ID, 'ok-button')
    PLAY_PAUSE_BUTTON = (By.ID, 'playPauseButton')
    PLAYLIST_TABLE = (By.ID, 'playlistTable',
                      By.CSS_SELECTOR, 'table')
    PRESET_SELECT = (By.ID, 'presetSelect')
    PREV_BUTTON = (By.ID, 'prevButton')
    RATING_OVERLAY_DIV = (By.ID, 'ratingOverlayDiv')
    RATING_SPAN = (By.ID, 'ratingSpan')
    REPLACE_BUTTON = (By.ID, 'replaceButton')
    RESET_BUTTON = (By.ID, 'resetButton')
    SEARCH_BUTTON = (By.ID, 'searchButton')
    SEARCH_RESULTS_CHECKBOX = (By.ID, 'searchResultsTable',
                               By.CSS_SELECTOR, 'th input[type="checkbox"]')
    SEARCH_RESULTS_TABLE = (By.ID, 'searchResultsTable',
                            By.CSS_SELECTOR, 'table')
    TIME_DIV = (By.ID, 'timeDiv')
    TITLE_DIV = (By.ID, 'titleDiv')
    UNRATED_CHECKBOX = (By.ID, 'unratedCheckbox')
    UPDATE_CLOSE_IMAGE = (By.ID, 'updateCloseImage')
    VOLUME_RANGE = (By.ID, 'dialogManager',
                    By.CSS_SELECTOR, '.dialog',
                    By.ID, 'volume-range')
    VOLUME_SPAN = (By.ID, 'dialogManager',
                   By.CSS_SELECTOR, '.dialog',
                   By.ID, 'volume-span')

    # Values for FIRST_PLAYED_SELECT and LAST_PLAYED_SELECT.
    UNSET_TIME = '...'
    ONE_DAY = 'one day'
    ONE_WEEK = 'one week'
    ONE_MONTH = 'one month'
    THREE_MONTHS = 'three months'
    SIX_MONTHS = 'six months'
    ONE_YEAR = 'one year'
    THREE_YEARS = 'three years'

    # Values for MIN_RATING_SELECT and RATING_OVERLAY_DIV.
    ONE_STAR = u'★';
    TWO_STARS = u'★★';
    THREE_STARS = u'★★★';
    FOUR_STARS = u'★★★★';
    FIVE_STARS = u'★★★★★';

    # Values for PRESET_SELECT.
    PRESET_INSTRUMENTAL_OLD = 'instrumental old'
    PRESET_MELLOW = 'mellow'
    PRESET_NEW_ALBUMS = 'new albums'
    PRESET_UNRATED = 'unrated'
    PRESET_OLD = 'old'

    def __init__(self, driver):
        self.driver = driver
        self.reset()

    def reload(self):
        self.driver.refresh()

    def reset(self):
        self.driver.execute_script('document.test.reset()')

    def refresh_tags(self):
        self.driver.execute_script('document.test.updateTags()');

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
        except selenium.common.exceptions.StaleElementReferenceException:
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
        song = Song(self.get(Page.ARTIST_DIV).text,
                    self.get(Page.TITLE_DIV).text,
                    self.get(Page.ALBUM_DIV).text)
        return (song,
                audio.get_attribute('src'),
                audio.get_attribute('paused') is not None,
                audio.get_attribute('ended') is not None,
                self.get(Page.TIME_DIV).text,
                self.get(Page.RATING_OVERLAY_DIV).text,
                self.get(Page.COVER_IMAGE).get_attribute('title'))

    # Waits for and returns the element described by |locator|.
    # |locator| is typically a tuple like (By.ID, 'some-element') or
    # (By.CSS_SELECTOR, 'div.foo').
    #
    # To handle elements nested within one or more Shadow DOMs, |locator|
    # can also contain additional pairs using (only) By.ID or By.CSS_SELECTOR,
    # which will be used to search within nested Shadow DOMs.
    def get(self, locator):
        utils.wait(lambda: self.driver.find_element(locator[0], locator[1]))
        return self.get_nowait(locator)

    def get_nowait(self, locator):
        el = None
        while len(locator):
            if el:
                root = self.driver.execute_script(
                        'return arguments[0].shadowRoot', el)
            else:
                root = self.driver
            el = root.find_element(locator[0], locator[1])
            locator = locator[2:]

        return el

    def click(self, locator):
        self.get(locator).click()

    def select(self, locator, value):
        select = self.get(locator)
        for option in select.find_elements_by_tag_name('option'):
            if option.text == value:
                option.click()
                return
        raise RuntimeError('Failed to find option "%s"' % value)

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
        spans = root.find_elements_by_css_selector('span')
        return [s.text for s in spans]

    def show_options(self):
        # TODO: This ought to be using Alt+O, but my version of selenium is
        # broken and barfs when modifiers are sent.
        self.driver.execute_script('document.test.showOptions()')

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
