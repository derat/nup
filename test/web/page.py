#!/usr/bin/python
# coding=UTF-8

import utils
from selenium.webdriver.common.by import By

from song import Song

# Based on https://selenium-python.readthedocs.org/page-objects.html.

class Locators(object):
    ALBUM_DIV = (By.ID, 'albumDiv')
    ARTIST_DIV = (By.ID, 'artistDiv')
    AUDIO = (By.ID, 'audio')
    FIRST_TRACK_CHECKBOX = (By.ID, 'firstTrackCheckbox')
    LUCKY_BUTTON = (By.ID, 'luckyButton')
    MIN_RATING_SELECT = (By.ID, 'minRatingSelect')
    PLAY_PAUSE_BUTTON = (By.ID, 'playPauseButton')
    RESET_BUTTON = (By.ID, 'resetButton')
    SEARCH_BUTTON = (By.ID, 'searchButton')
    SEARCH_RESULTS_TABLE = (By.ID, 'searchResultsTable')
    TITLE_DIV = (By.ID, 'titleDiv')

class InputElement(object):
    def __set__(self, obj, value):
        self.get_element(obj.driver).send_keys(value)

    def __get__(self, obj, owner):
        return self.get_element(obj.driver).get_attribute("value")

    def get_element(self, driver):
        utils.wait(lambda: driver.find_element(*self.locator))
        return driver.find_element(*self.locator)

class KeywordsInput(InputElement):
    locator = (By.ID, 'keywordsInput')

class Page(object):
    keywords = KeywordsInput()

    def __init__(self, driver):
        self.driver = driver

    def get_search_results(self):
        results = []
        table = self.driver.find_element(*Locators.SEARCH_RESULTS_TABLE)
        # Skip header.
        for row in table.find_elements_by_tag_name('tr')[1:]:
            cols = row.find_elements_by_tag_name('td')
            # TODO: Do something with the time from cols[4].text?
            results.append(Song(cols[1].text, cols[2].text, cols[3].text))
        return results

    def get_current_song(self):
        '''Gets information about the currently-playing song.

           Returns a 3-tuple containing:
               Song being displayed
               <audio> src (string)
               <audio> paused state (bool)
        '''
        audio = self.driver.find_element(*Locators.AUDIO)
        song = Song(self.driver.find_element(*Locators.ARTIST_DIV).text,
                    self.driver.find_element(*Locators.TITLE_DIV).text,
                    self.driver.find_element(*Locators.ALBUM_DIV).text)
        return (song, audio.get_attribute('src'),
                audio.get_attribute('paused') is not None)

    def click_first_track_checkbox(self):
        self.driver.find_element(*Locators.FIRST_TRACK_CHECKBOX).click()

    def click_lucky_button(self):
        self.driver.find_element(*Locators.LUCKY_BUTTON).click()

    def click_play_pause_button(self):
        self.driver.find_element(*Locators.PLAY_PAUSE_BUTTON).click()

    def click_rating_select(self, num_stars):
        self.select_option(
            self.driver.find_element(*Locators.MIN_RATING_SELECT),
            u'★' * num_stars)

    def click_reset_button(self):
        self.driver.find_element(*Locators.RESET_BUTTON).click()

    def click_search_button(self):
        self.driver.find_element(*Locators.SEARCH_BUTTON).click()

    def select_option(self, select, value):
        for option in select.find_elements_by_tag_name('option'):
            if option.text == value:
                option.click()
                return
        raise RuntimeError('Failed to find option "%s"' % value)
