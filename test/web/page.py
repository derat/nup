#!/usr/bin/python
# coding=UTF-8

import selenium
import utils
from selenium.webdriver.common.action_chains import ActionChains
from selenium.webdriver.common.by import By

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
    EDIT_TAGS_SUGGESTIONS_DIV = (By.ID, 'editTagsSuggestionsDiv')
    EDIT_TAGS_TEXTAREA = (By.ID, 'editTagsTextarea')
    FIRST_PLAYED_SELECT = (By.ID, 'firstPlayedSelect')
    FIRST_TRACK_CHECKBOX = (By.ID, 'firstTrackCheckbox')
    INSERT_BUTTON = (By.ID, 'insertButton')
    LAST_PLAYED_SELECT = (By.ID, 'lastPlayedSelect')
    LUCKY_BUTTON = (By.ID, 'luckyButton')
    MIN_RATING_SELECT = (By.ID, 'minRatingSelect')
    NEXT_BUTTON = (By.ID, 'nextButton')
    OPTIONS_DIV = (By.ID, 'optionsDiv')
    OPTIONS_OK_BUTTON = (By.ID, 'optionsOkButton')
    PLAY_PAUSE_BUTTON = (By.ID, 'playPauseButton')
    PLAYLIST_TABLE = (By.ID, 'playlistTable')
    PREV_BUTTON = (By.ID, 'prevButton')
    RATING_OVERLAY_DIV = (By.ID, 'ratingOverlayDiv')
    RATING_SPAN = (By.ID, 'ratingSpan')
    REPLACE_BUTTON = (By.ID, 'replaceButton')
    RESET_BUTTON = (By.ID, 'resetButton')
    SEARCH_BUTTON = (By.ID, 'searchButton')
    SEARCH_RESULTS_CHECKBOX = (By.ID, 'searchResultsCheckbox')
    SEARCH_RESULTS_TABLE = (By.ID, 'searchResultsTable')
    TIME_DIV = (By.ID, 'timeDiv')
    TITLE_DIV = (By.ID, 'titleDiv')
    UNRATED_CHECKBOX = (By.ID, 'unratedCheckbox')
    UPDATE_CLOSE_IMAGE = (By.ID, 'updateCloseImage')
    VOLUME_RANGE = (By.ID, 'volumeRange')
    VOLUME_SPAN = (By.ID, 'volumeSpan')

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

    def __init__(self, driver):
        self.driver = driver
        self.reset()

    def reset(self):
        self.driver.execute_script('document.playlist.resetForTesting()')

    def refresh_tags(self):
        self.driver.execute_script(
            'document.player.updateTagsFromServer(false)');

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
                self.get(Page.TIME_DIV).text,
                self.get(Page.RATING_OVERLAY_DIV).text,
                self.get(Page.COVER_IMAGE).get_attribute('title'))

    def get(self, locator):
        utils.wait(lambda: self.driver.find_element(*locator))
        return self.driver.find_element(*locator)

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
                self.driver.find_element(*locator)
                return True
            except selenium.common.exceptions.NoSuchElementException:
                return False
        utils.wait(lambda: not exists())

    def focus(self, locator):
        self.driver.focus(locator)

    def click_search_result_checkbox(self, row_index):
        self.get(Page.SEARCH_RESULTS_TABLE).\
            find_elements_by_tag_name('tr')[row_index + 1].\
            find_elements_by_tag_name('td')[0].\
            find_elements_by_tag_name('input')[0].click()

    def click_rating(self, num_stars):
        stars = self.get(self.RATING_SPAN).find_elements_by_tag_name('a')
        stars[num_stars-1].click()

    def get_tag_suggestions(self, locator):
        spans = self.get(locator).find_elements_by_tag_name('span')
        return [s.text for s in spans]

    def show_options(self):
        # TODO: This ought to be using Alt+O, but my version of selenium is
        # broken and barfs when modifiers are sent.
        self.driver.execute_script('document.player.showOptions()')
