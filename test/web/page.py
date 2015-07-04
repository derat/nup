#!/usr/bin/python
# coding=UTF-8

import selenium
import utils
from selenium.webdriver.common.by import By

from song import Song

def get_element(driver, locator):
    utils.wait(lambda: driver.find_element(*locator))
    return driver.find_element(*locator)

# Loosely based on https://selenium-python.readthedocs.org/page-objects.html.
class InputElement(object):
    def __set__(self, obj, value):
        element = get_element(obj.driver, self.locator)
        element.clear()
        element.send_keys(value)

    def __get__(self, obj, owner):
        return get_element(obj.driver, self.locator).get_attribute("value")

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
    FIRST_PLAYED_SELECT = (By.ID, 'firstPlayedSelect')
    FIRST_TRACK_CHECKBOX = (By.ID, 'firstTrackCheckbox')
    INSERT_BUTTON = (By.ID, 'insertButton')
    LAST_PLAYED_SELECT = (By.ID, 'lastPlayedSelect')
    LUCKY_BUTTON = (By.ID, 'luckyButton')
    MIN_RATING_SELECT = (By.ID, 'minRatingSelect')
    NEXT_BUTTON = (By.ID, 'nextButton')
    PLAY_PAUSE_BUTTON = (By.ID, 'playPauseButton')
    PLAYLIST_TABLE = (By.ID, 'playlistTable')
    PREV_BUTTON = (By.ID, 'prevButton')
    REPLACE_BUTTON = (By.ID, 'replaceButton')
    RESET_BUTTON = (By.ID, 'resetButton')
    SEARCH_BUTTON = (By.ID, 'searchButton')
    SEARCH_RESULTS_CHECKBOX = (By.ID, 'searchResultsCheckbox')
    SEARCH_RESULTS_TABLE = (By.ID, 'searchResultsTable')
    TITLE_DIV = (By.ID, 'titleDiv')
    UNRATED_CHECKBOX = (By.ID, 'unratedCheckbox')

    # Values for FIRST_PLAYED_SELECT and LAST_PLAYED_SELECT.
    UNSET_TIME = '...'
    ONE_DAY = 'one day'
    ONE_WEEK = 'one week'
    ONE_MONTH = 'one month'
    THREE_MONTHS = 'three months'
    SIX_MONTHS = 'six months'
    ONE_YEAR = 'one year'
    THREE_YEARS = 'three years'

    # Values for MIN_RATING_SELECT.
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
                song.highlighted = 'playing' in row.get_attribute('class')
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
            get_element(self.driver, Page.SEARCH_RESULTS_TABLE))

    def get_playlist(self):
        return self.get_songs_from_table(
            get_element(self.driver, Page.PLAYLIST_TABLE))

    def get_current_song(self):
        '''Gets information about the currently-playing song.

           Returns a 3-tuple containing:
               Song being displayed
               <audio> src (string)
               <audio> paused state (bool)
        '''
        audio = self.driver.find_element(*Page.AUDIO)
        song = Song(get_element(self.driver, Page.ARTIST_DIV).text,
                    get_element(self.driver, Page.TITLE_DIV).text,
                    get_element(self.driver, Page.ALBUM_DIV).text)
        return (song, audio.get_attribute('src'),
                audio.get_attribute('paused') is not None)

    def click(self, locator):
        get_element(self.driver, locator).click()

    def select(self, locator, value):
        select = get_element(self.driver, locator)
        for option in select.find_elements_by_tag_name('option'):
            if option.text == value:
                option.click()
                return
        raise RuntimeError('Failed to find option "%s"' % value)
