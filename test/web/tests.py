#!/usr/bin/python
# coding=UTF-8

import time
import unittest
from selenium import webdriver

# Local imports.
import constants
from file_server_thread import FileServerThread
from server import Server
from song import Song

file_thread = None
server = None
driver = None

class TimeoutError(Exception):
    def __init__(self, value):
        self.value = value
    def __str__(self):
        return repr(self.value)

def wait(f, timeout_sec=10, sleep_sec=0.1):
    '''Waits for a function to return true.'''
    start = time.time()
    while True:
        if f():
            return
        if time.time() - start >= timeout_sec:
            raise TimeoutError('Timed out waiting for condition')
        time.sleep(sleep_sec)

def select_option(select, value):
    for option in select.find_elements_by_tag_name('option'):
        if option.text == value:
            option.click()
            return
    raise RuntimeError('Failed to find option "%s"' % value)

def setUpModule():
    global file_thread
    file_thread = FileServerThread(constants.MUSIC_PATH)
    file_thread.start()

    global server
    server = Server(file_thread.host_port())

    global driver
    driver = webdriver.Chrome()

    # Makes no sense: Chrome starts at a data: URL, so I get a "Cookies are
    # disabled inside 'data:' URLs" exception if I try to add the cookie before
    # loading a page.
    base_url = 'http://%s:%d' % (constants.HOSTNAME, constants.PORT)
    driver.get(base_url)
    driver.add_cookie({
        'name': constants.AUTH_COOKIE,
        'value': 1,
    })
    driver.get(base_url)

def tearDownModule():
    file_thread.stop()
    driver.close()

class Test(unittest.TestCase):
    def setUp(self):
        server.clear_data()
        self.base_music_url = 'http://%s:%d/' % file_thread.host_port()

    def get_results(self):
        results = []
        table = driver.find_element_by_id('searchResultsTable')
        rows = table.find_elements_by_tag_name('tr')
        for i in range(1, len(rows)):
            row = rows[i]
            cols = row.find_elements_by_tag_name('td')
            # TODO: Do something with the time from cols[4].text?
            results.append(Song(cols[1].text, cols[2].text, cols[3].text))
        return results

    def wait_for_results(self, songs):
        try:
            wait(lambda: self.get_results() == songs, timeout_sec=3)
        except TimeoutError as e:
            self.fail('Timed out waiting for expected results. Received:\n' +
                      self.get_results())

    def get_current_song(self):
        '''Returns information about the currently-playing song.

           Return values:
               song: Song being displayed
               src: string from <audio>
               paused: bool from <audio>
        '''
        audio = driver.find_element_by_id('audio')
        song = Song(driver.find_element_by_id('artistDiv').text,
                    driver.find_element_by_id('titleDiv').text,
                    driver.find_element_by_id('albumDiv').text)
        return (song, audio.get_attribute('src'),
                audio.get_attribute('paused') is not None)

    def wait_for_song(self, song, paused):
        def is_current():
            current, current_src, current_paused = self.get_current_song()
            return current == song and \
                   current_src == self.base_music_url + song.filename and \
                   current_paused == paused
        try:
            wait(is_current, timeout_sec=5)
        except TimeoutError as e:
            self.fail('Timed out waiting for song. Received:\n' +
                      self.get_current_song())

    def test_queries(self):
        album1 = [
            Song('ar1', 'tr1', 'al1', 1, 1, 0.5),
            Song('ar1', 'tr2', 'al1', 2, 1, 0.75),
            Song('ar1', 'tr3', 'al1', 3, 1, 0.25),
        ]
        album2 = [
            Song('ar2', 'tr1', 'al2', 1, 1, 1.0),
            Song('ar2', 'tr2', 'al2', 2, 1, 0.0),
        ]
        server.import_songs(album1 + album2)

        driver.find_element_by_id('resetButton').click()
        driver.find_element_by_id('keywordsInput').send_keys('album:al1')
        driver.find_element_by_id('searchButton').click()
        self.wait_for_results(album1)

        driver.find_element_by_id('resetButton').click()
        driver.find_element_by_id('keywordsInput').send_keys('album:al2')
        driver.find_element_by_id('searchButton').click()
        self.wait_for_results(album2)

        driver.find_element_by_id('resetButton').click()
        driver.find_element_by_id('keywordsInput').send_keys('tr2')
        driver.find_element_by_id('searchButton').click()
        self.wait_for_results([album1[1], album2[1]])

        driver.find_element_by_id('resetButton').click()
        driver.find_element_by_id('firstTrackCheckbox').click()
        driver.find_element_by_id('searchButton').click()
        self.wait_for_results([album1[0], album2[0]])

        driver.find_element_by_id('resetButton').click()
        select_option(driver.find_element_by_id('minRatingSelect'), u'★★★★')
        driver.find_element_by_id('searchButton').click()
        self.wait_for_results([album1[1], album2[0]])

    def test_playback(self):
        song = Song('artist', 'track', 'album')
        server.import_songs([song])

        driver.find_element_by_id('resetButton').click()
        driver.find_element_by_id('keywordsInput').send_keys(song.artist)
        driver.find_element_by_id('luckyButton').click()
        self.wait_for_song(song, False)
        driver.find_element_by_id('playPauseButton').click()
        self.wait_for_song(song, True)
        driver.find_element_by_id('playPauseButton').click()
        self.wait_for_song(song, False)

if __name__ == '__main__':
    unittest.main()
