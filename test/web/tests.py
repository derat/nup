#!/usr/bin/python
# coding=UTF-8

import pprint
import time
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

    def wait_for_search_results(self, page, songs, checked=None, msg=''):
        '''Waits until the page is displaying the expected search results.'''
        def is_expected():
            results = page.get_search_results()
            if results != songs:
                return False
            if checked is not None:
                for i in range(len(checked)):
                    if results[i].checked != checked[i]:
                        return False
            return True
        try:
            utils.wait(is_expected, timeout_sec=3)
        except utils.TimeoutError as e:
            msg = ' (' + msg + ')' if msg else ''
            self.fail('Timed out waiting for expected results' + msg +
                      '.\nReceived:\n' +
                      pprint.pformat(page.get_search_results()))

    def wait_for_playlist(self, page, songs, highlighted_index=-1, msg=''):
        '''Waits until the page is displaying the expected playlist.'''
        def is_expected():
            playlist = page.get_playlist()
            if playlist != songs:
                return False
            if highlighted_index >= 0:
                for i, song in enumerate(playlist):
                    if (highlighted_index == i and not song.highlighted) or \
                       (highlighted_index != i and song.highlighted):
                        return False
            return True
        try:
            utils.wait(is_expected, timeout_sec=3)
        except utils.TimeoutError as e:
            msg = ' (' + msg + ')' if msg else ''
            self.fail('Timed out waiting for expected playlist' + msg +
                      '.\nReceived:\n' + pprint.pformat(page.get_playlist()))

    def wait_for_song(self, page, song, paused, time_str='', msg=''):
        '''Waits until the page is playing the expected song.'''
        def is_current():
            current, current_src, current_paused, current_time_str = \
                page.get_current_song()
            return current == song and \
                   current_src == self.base_music_url + song.filename and \
                   current_paused == paused and \
                   (not time_str or current_time_str == time_str)
        try:
            utils.wait(is_current, timeout_sec=5)
        except utils.TimeoutError as e:
            msg = ' (' + msg + ')' if msg else ''
            self.fail('Timed out waiting for song' + msg + '.\nReceived ' +
                      str(page.get_current_song()))

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
        for kw, res in (('album:al1', album1),
                        ('album:al2', album2),
                        ('artist:ar1', album1),
                        ('artist:"artist with space"', album3),
                        ('ti2', [album1[1], album2[1]]),
                        ('AR2 ti1', [album2[0]]),
                        ('ar1 bogus', [])):
            page.keywords = kw
            page.click(page.SEARCH_BUTTON)
            self.wait_for_search_results(page, res, msg=kw)

    def test_tag_query(self):
        song1 = Song('ar1', 'ti1', 'al1', tags=['electronic', 'instrumental'])
        song2 = Song('ar2', 'ti2', 'al2', tags=['rock', 'guitar'])
        song3 = Song('ar3', 'ti3', 'al3', tags=['instrumental', 'rock'])
        server.import_songs([song1, song2, song3])

        page = Page(driver)
        for tags, res in (('electronic', [song1]),
                          ('guitar rock', [song2]),
                          ('instrumental', [song1, song3]),
                          ('instrumental -electronic', [song3])):
            page.tags = tags
            page.click(page.SEARCH_BUTTON)
            self.wait_for_search_results(page, res, msg=tags)

    def test_rating_query(self):
        song1 = Song('a', 't', 'al1', rating=0.0)
        song2 = Song('a', 't', 'al2', rating=0.25)
        song3 = Song('a', 't', 'al3', rating=0.5)
        song4 = Song('a', 't', 'al4', rating=0.75)
        song5 = Song('a', 't', 'al5', rating=1.0)
        song6 = Song('a', 't', 'al6', rating=-1.0)
        server.import_songs([song1, song2, song3, song4, song5, song6])

        page = Page(driver)
        # Need to set something to avoid an alert.
        page.keywords = 't'
        page.click(page.SEARCH_BUTTON)
        self.wait_for_search_results(
            page, [song1, song2, song3, song4, song5, song6])

        page.click(page.RESET_BUTTON)
        for rating, res in ((page.TWO_STARS, [song2, song3, song4, song5]),
                            (page.THREE_STARS, [song3, song4, song5]),
                            (page.FOUR_STARS, [song4, song5]),
                            (page.FIVE_STARS, [song5])):
            page.select(page.MIN_RATING_SELECT, rating)
            page.click(page.SEARCH_BUTTON)
            self.wait_for_search_results(page, res, msg=rating)

        page.click(page.RESET_BUTTON)
        page.click(page.UNRATED_CHECKBOX)
        page.click(page.SEARCH_BUTTON)
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
        page.click(page.FIRST_TRACK_CHECKBOX)
        page.click(page.SEARCH_BUTTON)
        self.wait_for_search_results(page, [album1[0], album2[0]])

    def test_max_plays_query(self):
        song1 = Song('ar1', 'ti1', 'al1', plays=[(1, ''), (2, '')])
        song2 = Song('ar2', 'ti2', 'al2', plays=[(1, ''), (2, ''), (3, '')])
        song3 = Song('ar3', 'ti3', 'al3', plays=[])
        server.import_songs([song1, song2, song3])

        page = Page(driver)
        for plays, res in (('2', [song1, song3]),
                           ('3', [song1, song2, song3]),
                           ('0', [song3])):
            page.max_plays = plays
            page.click(page.SEARCH_BUTTON)
            self.wait_for_search_results(page, res, msg=plays)

    def test_play_time_query(self):
        now = time.time()
        day = 86400
        song1 = Song('ar1', 'ti1', 'al1', plays=[(now - 5 * day, '')])
        song2 = Song('ar2', 'ti2', 'al2', plays=[(now - 90 * day, '')])
        server.import_songs([song1, song2])

        page = Page(driver)
        for first, last, res in \
                ((page.ONE_DAY, page.UNSET_TIME, []),
                 (page.ONE_WEEK, page.UNSET_TIME, [song1]),
                 (page.ONE_YEAR, page.UNSET_TIME, [song1, song2]),
                 (page.UNSET_TIME, page.ONE_YEAR, []),
                 (page.UNSET_TIME, page.ONE_MONTH, [song2]),
                 (page.UNSET_TIME, page.ONE_DAY, [song1, song2])):
            page.select(page.FIRST_PLAYED_SELECT, first)
            page.select(page.LAST_PLAYED_SELECT, last)
            page.click(page.SEARCH_BUTTON)
            self.wait_for_search_results(page, res, msg='%s/%s' % (first, last))

    def test_search_result_checkboxes(self):
        songs = [Song('a', 't1', 'al1', 1),
                 Song('a', 't2', 'al1', 2),
                 Song('a', 't3', 'al1', 3)]
        server.import_songs(songs)


        page = Page(driver)

        def check_selected(selected, opaque):
            checkbox = page.get(page.SEARCH_RESULTS_CHECKBOX)
            if checkbox.is_selected() != selected:
                self.fail('Selected state (%d) didn\'t match expected (%d)' %
                          (checkbox.is_selected(), selected))
            actual_opaque = 'transparent' not in checkbox.get_attribute('class')
            if actual_opaque != opaque:
                self.fail('Opaque state (%d) didn\'t match expected (%d)' %
                          (actual_opaque, opaque))

        page.keywords = songs[0].artist
        page.click(page.SEARCH_BUTTON)
        self.wait_for_search_results(page, songs, [True, True, True])
        check_selected(True, True)

        page.click(page.SEARCH_RESULTS_CHECKBOX)
        self.wait_for_search_results(page, songs, [False, False, False])
        check_selected(False, True)

        page.click(page.SEARCH_RESULTS_CHECKBOX)
        self.wait_for_search_results(page, songs, [True, True, True])
        check_selected(True, True)

        page.click_search_result_checkbox(0)
        self.wait_for_search_results(page, songs, [False, True, True])
        check_selected(True, False)

        page.click(page.SEARCH_RESULTS_CHECKBOX)
        self.wait_for_search_results(page, songs, [False, False, False])
        check_selected(False, True)

        page.click_search_result_checkbox(0)
        page.click_search_result_checkbox(1)
        self.wait_for_search_results(page, songs, [True, True, False])
        check_selected(True, False)

        page.click_search_result_checkbox(2)
        self.wait_for_search_results(page, songs, [True, True, True])
        check_selected(True, True)

    def test_add_to_playlist(self):
        song1 = Song('a', 't1', 'al1', 1)
        song2 = Song('a', 't2', 'al1', 2)
        song3 = Song('a', 't3', 'al2', 1)
        song4 = Song('a', 't4', 'al2', 2)
        song5 = Song('a', 't5', 'al3', 1)
        song6 = Song('a', 't6', 'al3', 2)
        server.import_songs([song1, song2, song3, song4, song5, song6])

        page = Page(driver)
        page.keywords = 'al1'
        page.click(page.SEARCH_BUTTON)
        self.wait_for_search_results(page, [song1, song2])
        page.click(page.APPEND_BUTTON)
        self.wait_for_playlist(page, [song1, song2], 0)

        # Pause so we don't advance through the playlist mid-test.
        self.wait_for_song(page, song1, False)
        page.click(page.PLAY_PAUSE_BUTTON)
        self.wait_for_song(page, song1, True)

        # Inserting should leave the current track paused.
        page.keywords = 'al2'
        page.click(page.SEARCH_BUTTON)
        self.wait_for_search_results(page, [song3, song4])
        page.click(page.INSERT_BUTTON)
        self.wait_for_playlist(page, [song1, song3, song4, song2], 0)
        self.wait_for_song(page, song1, True)

        # Replacing should result in the new first track being played.
        page.keywords = 'al3'
        page.click(page.SEARCH_BUTTON)
        self.wait_for_search_results(page, [song5, song6])
        page.click(page.REPLACE_BUTTON)
        self.wait_for_playlist(page, [song5, song6], 0)
        self.wait_for_song(page, song5, False)

        # Appending should leave the first track playing.
        page.keywords = 'al1'
        page.click(page.SEARCH_BUTTON)
        self.wait_for_search_results(page, [song1, song2])
        page.click(page.APPEND_BUTTON)
        self.wait_for_playlist(page, [song5, song6, song1, song2], 0)
        self.wait_for_song(page, song5, False)

        # The "I'm feeling lucky" button should replace the current playlist and
        # start playing the new first song.
        page.keywords = 'al2'
        page.click(page.LUCKY_BUTTON)
        self.wait_for_playlist(page, [song3, song4], 0)
        self.wait_for_song(page, song3, False)

    def test_playback_buttons(self):
        song1 = Song('artist', 'track1', 'album', 1)
        song2 = Song('artist', 'track2', 'album', 2)
        server.import_songs([song1, song2])

        page = Page(driver)
        page.keywords = song1.artist
        page.click(page.LUCKY_BUTTON)
        self.wait_for_song(page, song1, False)
        self.wait_for_playlist(page, [song1, song2], 0)

        page.click(page.PLAY_PAUSE_BUTTON)
        self.wait_for_song(page, song1, True)
        self.wait_for_playlist(page, [song1, song2], 0)

        page.click(page.PLAY_PAUSE_BUTTON)
        self.wait_for_song(page, song1, False)
        self.wait_for_playlist(page, [song1, song2], 0)

        page.click(page.NEXT_BUTTON)
        self.wait_for_song(page, song2, False)
        self.wait_for_playlist(page, [song1, song2], 1)

        page.click(page.NEXT_BUTTON)
        self.wait_for_song(page, song2, False)
        self.wait_for_playlist(page, [song1, song2], 1)

        page.click(page.PREV_BUTTON)
        self.wait_for_song(page, song1, False)
        self.wait_for_playlist(page, [song1, song2], 0)

        page.click(page.PREV_BUTTON)
        self.wait_for_song(page, song1, False)
        self.wait_for_playlist(page, [song1, song2], 0)

    def test_play_through_songs(self):
        song1 = Song('artist', 'track1', 'album', 1, filename=Song.FILE_1S)
        song2 = Song('artist', 'track2', 'album', 2, filename=Song.FILE_5S)
        server.import_songs([song1, song2])

        page = Page(driver)
        page.keywords = song1.artist
        page.click(page.LUCKY_BUTTON)
        self.wait_for_song(page, song1, False)
        self.wait_for_playlist(page, [song1, song2], 0)
        self.wait_for_song(page, song2, False)
        self.wait_for_playlist(page, [song1, song2], 1)

    def test_display_time_while_playing(self):
        song = Song('ar', 't', 'al', 1, filename=Song.FILE_5S, length=5.0)
        server.import_songs([song])

        page = Page(driver)
        page.keywords = song.artist
        page.click(page.LUCKY_BUTTON)
        self.wait_for_song(page, song, False, time_str='[0:00 / 0:05]')
        self.wait_for_song(page, song, False, time_str='[0:01 / 0:05]')
        self.wait_for_song(page, song, False, time_str='[0:02 / 0:05]')
        self.wait_for_song(page, song, False, time_str='[0:03 / 0:05]')
        self.wait_for_song(page, song, False, time_str='[0:04 / 0:05]')
        self.wait_for_song(page, song, True, time_str='[0:05 / 0:05]')

    def test_report_played(self):
        song1 = Song('ar', 't1', 'al', 1, filename=Song.FILE_5S, length=5.0)
        song2 = Song('ar', 't2', 'al', 2, filename=Song.FILE_1S, length=1.0)
        server.import_songs([song1, song2])

        # Skip the first song early on, but listen to all of the second song.
        page = Page(driver)
        page.keywords = song1.artist
        page.click(page.LUCKY_BUTTON)
        self.wait_for_song(page, song1, False, time_str='[0:01 / 0:05]')
        start_time = int(time.time())
        page.click(page.NEXT_BUTTON)
        self.wait_for_song(page, song2, True, time_str='[0:01 / 0:01]')
        end_time = int(time.time() + 1.0)  # Lawls.

        # Only the second song should've been reported.
        exported = server.export_songs()
        if len(exported[song1.sha1].plays) != 0:
            self.fail('Unexpected plays: ' + str(exported[song1.sha1].plays))
        if len(exported[song2.sha1].plays) != 1:
            self.fail('Missing plays: ' + str(exported[song2.sha1].plays))
        p = exported[song2.sha1].plays[0]
        if p[0] < start_time or p[0] > end_time:
            self.fail('Bad start time %d; expected [%d, %d]' %
                      (p[0], start_time, end_time))

        # Go back to the first song but pause it after one second.
        start_time = int(time.time())
        page.click(page.PREV_BUTTON)
        self.wait_for_song(page, song1, False, time_str='[0:01 / 0:05]')
        end_time = int(time.time() + 1.0)
        page.click(page.PLAY_PAUSE_BUTTON)
        exported = server.export_songs()
        if len(exported[song1.sha1].plays) != 0:
            self.fail('Unexpected plays: ' + str(exported[song1.sha1].plays))

        # After more than half of the first song has played, it should be
        # reported.
        page.click(page.PLAY_PAUSE_BUTTON)
        self.wait_for_song(page, song1, False, time_str='[0:04 / 0:05]')
        exported = server.export_songs()
        if len(exported[song1.sha1].plays) != 1:
            self.fail('Missing plays: ' + str(exported[song1.sha1].plays))
        p = exported[song1.sha1].plays[0]
        if p[0] < start_time or p[0] > end_time:
            self.fail('Bad start time %d; expected [%d, %d]' %
                      (p[0], start_time, end_time))

if __name__ == '__main__':
    unittest.main()
