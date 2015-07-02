#!/usr/bin/python

import pprint
import unittest
from selenium import webdriver

# Local imports.
import constants
import utils
from file_server_thread import FileServerThread
from page import Page
from server import Server
from song import Song

file_thread = None
server = None
driver = None

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

    def wait_for_search_results(self, page, songs):
        '''Waits until the page is displaying the expected search results.'''
        try:
            utils.wait(lambda: page.get_search_results() == songs,
                       timeout_sec=3)
        except utils.TimeoutError as e:
            self.fail('Timed out waiting for expected results. Received:\n' +
                      pprint.pformat(page.get_search_results()))

    def wait_for_song(self, page, song, paused):
        '''Waits until the page is playing the expected song.'''
        def is_current():
            current, current_src, current_paused = page.get_current_song()
            return current == song and \
                   current_src == self.base_music_url + song.filename and \
                   current_paused == paused
        try:
            utils.wait(is_current, timeout_sec=5)
        except utils.TimeoutError as e:
            self.fail('Timed out waiting for song. Received ' +
                      page.get_current_song())

    def test_keyword_query(self):
        album1 = [
            Song('ar1', 'ti1', 'al1', 1),
            Song('ar1', 'ti2', 'al1', 2),
            Song('ar1', 'ti3', 'al1', 3),
        ]
        album2 = [
            Song('ar2', 'ti1', 'al2', 1),
            Song('ar2', 'ti2', 'al2', 2),
        ]
        album3 = [
            Song('artist with space', 'ti1', 'al3', 1),
        ]
        server.import_songs(album1 + album2 + album3)

        page = Page(driver)
        page.click_reset_button()
        page.keywords = 'album:al1'
        page.click_search_button()
        self.wait_for_search_results(page, album1)

        page.click_reset_button()
        page.keywords = 'album:al2'
        page.click_search_button()
        self.wait_for_search_results(page, album2)

        page.click_reset_button()
        page.keywords = 'artist:ar1'
        page.click_search_button()
        self.wait_for_search_results(page, album1)

        page.click_reset_button()
        page.keywords = 'artist:"artist with space"'
        page.click_search_button()
        self.wait_for_search_results(page, album3)

        page.click_reset_button()
        page.keywords = 'ti2'
        page.click_search_button()
        self.wait_for_search_results(page, [album1[1], album2[1]])

        page.click_reset_button()
        page.keywords = 'AR2 ti1'
        page.click_search_button()
        self.wait_for_search_results(page, [album2[0]])

        page.click_reset_button()
        page.keywords = 'ar1 bogus'
        page.click_search_button()
        self.wait_for_search_results(page, [])

    def test_tag_query(self):
        song1 = Song('ar1', 'ti1', 'al1', tags=['electronic', 'instrumental'])
        song2 = Song('ar2', 'ti2', 'al2', tags=['rock', 'guitar'])
        song3 = Song('ar3', 'ti3', 'al3', tags=['instrumental', 'rock'])
        server.import_songs([song1, song2, song3])

        page = Page(driver)
        page.click_reset_button()
        page.tags = 'electronic'
        page.click_search_button()
        self.wait_for_search_results(page, [song1])

        page.click_reset_button()
        page.tags = 'guitar rock'
        page.click_search_button()
        self.wait_for_search_results(page, [song2])

        page.click_reset_button()
        page.tags = 'instrumental'
        page.click_search_button()
        self.wait_for_search_results(page, [song1, song3])

        page.click_reset_button()
        page.tags = 'instrumental -electronic'
        page.click_search_button()
        self.wait_for_search_results(page, [song3])

    def test_rating_query(self):
        song1 = Song('a', 't', 'al1', rating=0.0)
        song2 = Song('a', 't', 'al2', rating=0.25)
        song3 = Song('a', 't', 'al3', rating=0.5)
        song4 = Song('a', 't', 'al4', rating=0.75)
        song5 = Song('a', 't', 'al5', rating=1.0)
        song6 = Song('a', 't', 'al6', rating=-1.0)
        server.import_songs([song1, song2, song3, song4, song5, song6])

        page = Page(driver)
        page.click_reset_button()
        # Need to set something to avoid an alert.
        page.keywords = 't'
        page.click_search_button()
        self.wait_for_search_results(
            page, [song1, song2, song3, song4, song5, song6])

        page.click_reset_button()
        page.click_rating_select(3)
        page.click_search_button()
        self.wait_for_search_results(page, [song3, song4, song5])

        page.click_reset_button()
        page.click_rating_select(5)
        page.click_search_button()
        self.wait_for_search_results(page, [song5])

        page.click_reset_button()
        page.click_unrated_checkbox()
        page.click_search_button()
        self.wait_for_search_results(page, [song6])

    def test_first_track_query(self):
        album1 = [
            Song('ar1', 'ti1', 'al1', 1, 1),
            Song('ar1', 'ti2', 'al1', 2, 1),
            Song('ar1', 'ti3', 'al1', 3, 1),
        ]
        album2 = [
            Song('ar2', 'ti1', 'al2', 1, 1),
            Song('ar2', 'ti2', 'al2', 2, 1),
        ]
        server.import_songs(album1 + album2)

        page = Page(driver)
        page.click_reset_button()
        page.click_first_track_checkbox()
        page.click_search_button()
        self.wait_for_search_results(page, [album1[0], album2[0]])

    def test_playback(self):
        song = Song('artist', 'track', 'album')
        server.import_songs([song])

        page = Page(driver)
        page.click_reset_button()
        page.keywords = song.artist
        page.click_lucky_button()
        self.wait_for_song(page, song, False)
        page.click_play_pause_button()
        self.wait_for_song(page, song, True)
        page.click_play_pause_button()
        self.wait_for_song(page, song, False)

if __name__ == '__main__':
    unittest.main()
