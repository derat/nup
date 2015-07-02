#!/usr/bin/python
# coding=UTF-8

import utils
from selenium.webdriver.common.by import By

# Based on https://selenium-python.readthedocs.org/page-objects.html.

class Locators(object):
    FIRST_TRACK_CHECKBOX = (By.ID, 'firstTrackCheckbox')
    LUCKY_BUTTON = (By.ID, 'luckyButton')
    MIN_RATING_SELECT = (By.ID, 'minRatingSelect')
    PLAYING_ALBUM = (By.ID, 'albumDiv')
    PLAYING_ARTIST = (By.ID, 'artistDiv')
    PLAYING_TITLE = (By.ID, 'titleDiv')
    PLAY_PAUSE_BUTTON = (By.ID, 'playPauseButton')
    RESET_BUTTON = (By.ID, 'resetButton')
    SEARCH_BUTTON = (By.ID, 'searchButton')

class PageElement(object):
    def __set__(self, obj, value):
        self.get_element(obj.driver).send_keys(value)

    def __get__(self, obj, owner):
        return self.get_element(obj.driver).get_attribute("value")

    def get_element(self, driver):
        utils.wait(lambda: driver.find_element(*self.locator))
        return driver.find_element(*self.locator)

class Audio(PageElement):
    locator = (By.ID, 'audio')

class KeywordsInput(PageElement):
    locator = (By.ID, 'keywordsInput')

class Page(object):
    audio = Audio()
    keywords = KeywordsInput()

    def __init__(self, driver):
        self.driver = driver

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
