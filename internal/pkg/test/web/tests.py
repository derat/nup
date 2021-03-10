#!/usr/bin/python
# coding=UTF-8

import distutils.spawn
import pprint
import tempfile
import time
import unittest

from selenium import webdriver
from selenium.webdriver.common.desired_capabilities import DesiredCapabilities

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
    file_thread.daemon = True  # avoid hanging on SIGINT
    file_thread.start()

    global server
    server = Server(file_thread.host_port())

    # https://www.programcreek.com/python/example/100025/selenium.webdriver.ChromeOptions
    # https://github.com/obsproject/obs-browser/issues/105
    opts = webdriver.ChromeOptions()
    opts.add_argument('--autoplay-policy=no-user-gesture-required')

    # Collect browser logs:
    # https://intellipaat.com/community/5478/getting-console-log-output-from-chrome-with-selenium-python-api-bindings
    caps = DesiredCapabilities.CHROME
    caps['goog:loggingPrefs'] = { 'browser':'ALL' }

    # For some reason, even when the chromedriver executable is in $PATH, I get
    # an error like the following here:
    #
    # WebDriverException: Message: 'chromedriver' executable needs to be in
    # PATH. Please see https://sites.google.com/a/chromium.org/chromedriver/home
    #
    # Passing the path manually seems to work:
    # https://stackoverflow.com/a/12611523
    global driver
    driver = webdriver.Chrome(distutils.spawn.find_executable('chromedriver'),
                              chrome_options=opts, desired_capabilities=caps)

    # Makes no sense: Chrome starts at a data: URL, so I get a "Cookies are
    # disabled inside 'data:' URLs" exception if I try to add the cookie before
    # loading a page.
    global base_url
    base_url = 'http://%s:%d' % (constants.HOSTNAME, constants.PORT)
    driver.get(base_url)
    driver.add_cookie({
        'name': constants.AUTH_COOKIE,
        'value': '1',
    })
    driver.get(base_url)

    global log_file
    log_file = tempfile.NamedTemporaryFile(prefix='nup_chrome.',
                                           suffix='.txt',
                                           delete=False)
    print('Writing Chrome logs to %s' % log_file.name)

def tearDownModule():
    file_thread.stop()
    driver.close()
    server.reset_config()
    log_file.close()

class Test(unittest.TestCase):
    def setUp(self):
        # https://stackoverflow.com/a/4506296
        log_file.write(self._testMethodName + '\n')
        log_file.write('-' * 80 + '\n')

        server.reset_connection()
        server.clear_data()

        # The filename really ought to be escaped, but I'm not sure how to get
        # Python to escape in the same way as Go.
        self.base_music_url = base_url + '/song?filename='

    def tearDown(self):
        for entry in driver.get_log('browser'):
            log_file.write(entry['message'] + '\n')
        log_file.write('\n')
        log_file.flush()

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

    def wait_for_song(self, page, song, paused=None, ended=None, time=None,
                      rating=None, title=None, msg='', timeout_sec=5):
        '''Waits until the page is playing the expected song.'''
        def is_current():
            current, current_src, current_paused, current_ended, current_time, \
                current_rating, current_title = page.get_current_song()
            return current == song and \
                   current_src == self.base_music_url + song.filename and \
                   (paused is None or current_paused == paused) and \
                   (ended is None or current_ended == ended) and \
                   (time is None or current_time == time) and \
                   (rating is None or current_rating == rating) and \
                   (title is None or current_title == title)
        try:
            utils.wait(is_current, timeout_sec=timeout_sec)
        except utils.TimeoutError as e:
            msg = ' (' + msg + ')' if msg else ''
            self.fail('Timed out waiting for song' + msg + '.\nReceived ' +
                      str(page.get_current_song()))

    def wait_for_server_user_data(self, songs, msg=''):
        '''Waits until the server contains the expected user data.

        Takes a map from song SHA1s to 3-tuples, each of which consists of:

            rating: float
            tags: list of strings
            plays: list of float start_timestamp or
                (start_timestamp, end_timestamp) tuples

        Values may be None to be ignored.
        '''
        def is_expected():
            exported = server.export_songs()
            for sha1, expected in songs.items():
                exp_rating, exp_tags, exp_plays = expected
                exp_tags = sorted(exp_tags) if exp_tags else exp_tags
                exp_plays = sorted(exp_plays) if exp_plays else exp_plays
                actual = exported[sha1]
                if (exp_rating is not None and actual.rating != exp_rating) or \
                   (exp_tags is not None and sorted(actual.tags) != exp_tags):
                    return False
                if exp_plays is not None:
                    if len(exp_plays) != len(actual.plays):
                        return False
                    for i, play in enumerate(sorted(actual.plays)):
                        actual_ts = play[0]
                        if isinstance(exp_plays[i], (list, tuple)):
                            exp_start, exp_end = exp_plays[i]
                            if actual_ts < exp_start or actual_ts > exp_end:
                                return False
                        else:
                            if actual_ts != exp_plays[i]:
                                return False
            return True
        try:
            utils.wait(is_expected, timeout_sec=5)
        except utils.TimeoutError as e:
            msg = ' (' + msg + ')' if msg else ''
            actual = ['[%s, %s, %s, %s]' %
                      (str(s), str(s.rating), str(s.tags), str(s.plays))
                      for s in server.export_songs().values()]
            self.fail('Timed out waiting for server data' + msg + '.\n' +
                      'Received ' + str(actual))

    def wait_for_presentation(self, page, cur_song, next_song):
        '''Waits until the the expected songs are displayed.'''
        def is_ready():
            songs = page.get_presentation_songs()
            return songs[0] == cur_song and songs[1] == next_song
        try:
            utils.wait(is_ready, timeout_sec=3)
        except utils.TimeoutError as e:
            self.fail('Timed out waiting for songs.\nReceived ' +
                      str(page.get_presentation_songs()))

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
            page.select(page.MIN_RATING_SELECT, text=rating)
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
            page.select(page.FIRST_PLAYED_SELECT, text=first)
            page.select(page.LAST_PLAYED_SELECT, text=last)
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

        # Shift-click downward.
        page.click(page.SEARCH_RESULTS_CHECKBOX)
        self.wait_for_search_results(page, songs, [False, False, False])
        page.click_search_result_checkbox(0, True)
        page.click_search_result_checkbox(2, True)
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
        song1 = Song('artist', 'track1', 'album', 1, filename=Song.FILE_5S)
        song2 = Song('artist', 'track2', 'album', 2, filename=Song.FILE_1S)
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
        song1 = Song('artist', 'track1', 'album', 1, filename=Song.FILE_5S)
        song2 = Song('artist', 'track2', 'album', 2, filename=Song.FILE_1S)
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
        self.wait_for_song(page, song, False, time='[ 0:00 / 0:05 ]')
        self.wait_for_song(page, song, False, time='[ 0:01 / 0:05 ]')
        self.wait_for_song(page, song, False, time='[ 0:02 / 0:05 ]')
        self.wait_for_song(page, song, False, time='[ 0:03 / 0:05 ]')
        self.wait_for_song(page, song, False, time='[ 0:04 / 0:05 ]')
        self.wait_for_song(page, song, True, time='[ 0:05 / 0:05 ]')

    def test_report_played(self):
        song1 = Song('ar', 't1', 'al', 1, filename=Song.FILE_5S, length=5.0)
        song2 = Song('ar', 't2', 'al', 2, filename=Song.FILE_1S, length=1.0)
        server.import_songs([song1, song2])

        # Skip the first song early on, but listen to all of the second song.
        page = Page(driver)
        page.keywords = song1.artist
        page.click(page.LUCKY_BUTTON)
        self.wait_for_song(page, song1, paused=False)
        song2_start_time = time.time()
        page.click(page.NEXT_BUTTON)
        self.wait_for_song(page, song2, paused=True)
        song2_end_time = time.time()

        # Only the second song should've been reported.
        self.wait_for_server_user_data({
            song1.sha1: (None, None, []),
            song2.sha1: (None, None, [(song2_start_time, song2_end_time)]),
        })

        # Go back to the first song but pause it immediately.
        song1_start_time = time.time()
        page.click(page.PREV_BUTTON)
        self.wait_for_song(page, song1, paused=False)
        song1_end_time = time.time()
        page.click(page.PLAY_PAUSE_BUTTON)
        # TODO: This doesn't test that it won't be reported later.
        self.wait_for_server_user_data({
            song1.sha1: (None, None, []),
            song2.sha1: (None, None, [(song2_start_time, song2_end_time)]),
        })

        # After more than half of the first song has played, it should be
        # reported.
        page.click(page.PLAY_PAUSE_BUTTON)
        self.wait_for_song(page, song1, paused=False)
        self.wait_for_server_user_data({
            song1.sha1: (None, None, [(song1_start_time, song1_end_time)]),
            song2.sha1: (None, None, [(song2_start_time, song2_end_time)]),
        })

    def test_report_replay(self):
        song = Song('ar', 't1', 'al', 1, filename=Song.FILE_1S, length=1.0)
        server.import_songs([song])

        # Play the song to completion.
        page = Page(driver)
        page.keywords = song.artist
        first_start_time = time.time()
        page.click(page.LUCKY_BUTTON)
        self.wait_for_song(page, song, ended=True)

        # Replay the song.
        second_start_time = time.time()
        page.click(page.PLAY_PAUSE_BUTTON)

        # Both playbacks should be reported.
        self.wait_for_server_user_data({
            song.sha1: (None, None, [
                (first_start_time, second_start_time),
                (second_start_time, second_start_time + 2)]),
        })

    def test_rate_and_tag(self):
        song = Song('ar', 't1', 'al', rating=0.5, tags=['rock', 'guitar'])
        server.import_songs([song])

        page = Page(driver)
        page.refresh_tags()
        page.keywords = song.artist
        page.click(page.LUCKY_BUTTON)
        self.wait_for_song(page, song, rating=page.THREE_STARS,
                           title=u'Rating: ★★★☆☆\nTags: guitar rock')

        page.show_update_div()
        page.click_rating(4)
        page.click(page.UPDATE_CLOSE_IMAGE)
        self.wait_for_song(page, song, rating=page.FOUR_STARS,
                           title=u'Rating: ★★★★☆\nTags: guitar rock')
        self.wait_for_server_user_data({
            song.sha1: (0.75, ['guitar', 'rock'], None),
        })

        page.show_update_div()
        page.get(page.EDIT_TAGS_TEXTAREA).send_keys(' +metal')
        page.click(page.UPDATE_CLOSE_IMAGE)
        self.wait_for_server_user_data({
            song.sha1: (0.75, ['guitar', 'metal', 'rock'], None),
        })
        self.wait_for_song(page, song, rating=page.FOUR_STARS,
                           title=u'Rating: ★★★★☆\nTags: guitar metal rock')

    def test_retry_updates(self):
        song = Song('ar', 't1', 'al', rating=0.5, tags=['rock', 'guitar'])
        server.import_songs([song])
        song_id = server.get_song_id(song.sha1)

        # Make the server always report failure.
        page = Page(driver)
        server.send_config(force_update_failures=True)
        rating = 0.75
        tags = ['jazz', 'mellow']
        page.rate_and_tag_song(song_id, rating, tags)
        first_start_time = 1437526417
        page.report_play(song_id, first_start_time)
        time.sleep(0.1)

        # Let the updates succeed.
        server.send_config(force_update_failures=False)
        self.wait_for_server_user_data({
            song.sha1: (rating, tags, [first_start_time]),
        })

        # Queue some more failed updates.
        server.send_config(force_update_failures=True)
        rating = 0.25
        tags = ['lively', 'soul']
        page.rate_and_tag_song(song_id, rating, tags)
        second_start_time = 1437526778
        page.report_play(song_id, second_start_time)
        time.sleep(0.1)

        # The queued updates should be sent if the page is reloaded.
        page.reload()
        server.send_config(force_update_failures=False)
        self.wait_for_server_user_data({
            song.sha1: (rating, tags, [first_start_time, second_start_time]),
        })

        # In the case of multiple queued updates, the last one should take
        # precedence.
        server.send_config(force_update_failures=True)
        page.rate_and_tag_song(song_id, 0.5)
        time.sleep(0.1)
        page.rate_and_tag_song(song_id, 0.75)
        time.sleep(0.1)
        page.rate_and_tag_song(song_id, 1.0)
        server.send_config(force_update_failures=False)
        self.wait_for_server_user_data({ song.sha1: (1.0, None, None) })

    def test_edit_tags_autocomplete(self):
        song1 = Song('ar', 't1', 'al', tags=['a0', 'a1', 'b'])
        song2 = Song('ar', 't2', 'al', tags=['c0', 'c1', 'd'])
        server.import_songs([song1, song2])

        page = Page(driver)
        page.refresh_tags()
        page.keywords = song1.title
        page.click(page.LUCKY_BUTTON)
        self.wait_for_song(page, song1)

        page.show_update_div()
        textarea = page.get(page.EDIT_TAGS_TEXTAREA)

        def check_textarea(expected):
            utils.wait_equal(lambda: textarea.get_attribute('value'),
                             expected,
                             timeout_sec=3)

        def check_suggestions(expected):
            utils.wait_equal(
                lambda: page.get_tag_suggestions(page.EDIT_TAGS_SUGGESTER),
                expected,
                timeout_sec=3)

        check_textarea('a0 a1 b ')

        TAB = u'\ue004'
        textarea.send_keys('d' + TAB)
        check_textarea('a0 a1 b d ')

        textarea.send_keys('c' + TAB)
        check_textarea('a0 a1 b d c')
        check_suggestions(['c0', 'c1'])

        textarea.send_keys('1' + TAB)
        check_textarea('a0 a1 b d c1 ')

    def test_options(self):
        page = Page(driver)
        page.show_options()

        def get_gain_type():
            return page.get(page.GAIN_TYPE_SELECT).get_attribute('value')
        def get_pre_amp():
            return page.get(page.PRE_AMP_RANGE).get_attribute('value')

        self.assertEqual(get_gain_type(), page.GAIN_ALBUM)
        page.select(page.GAIN_TYPE_SELECT, value=page.GAIN_TRACK)
        self.assertEqual(get_gain_type(), page.GAIN_TRACK)

        # I *think* that this clicks the middle of the range. This might be a
        # no-op since it should be 0, which is the default. :-/
        page.get(page.PRE_AMP_RANGE).click()
        orig_pre_amp = get_pre_amp()

        page.click(page.OPTIONS_OK_BUTTON)
        page.wait_until_gone(page.OPTIONS_OK_BUTTON)

        # Escape should dismiss the dialog.
        page.show_options()
        ESCAPE = u'\ue00c'
        page.get(page.BODY).send_keys(ESCAPE)
        page.wait_until_gone(page.OPTIONS_OK_BUTTON)

        # Now that we're using GainNode instead of setting the <audio> element's
        # volume, I'm not sure how to check that the setting was actually
        # applied. Just check that it was saved, since that seems better than
        # nothing.
        page.reload()
        page.show_options()
        self.assertEqual(get_gain_type(), page.GAIN_TRACK)
        self.assertEqual(get_pre_amp(), orig_pre_amp)
        page.click(page.OPTIONS_OK_BUTTON)

    def test_presets(self):
        song1 = Song('a', 't1', 'unrated')
        song2 = Song('a', 't1', 'new', rating=0.25, track=1, disc=1,
                     plays=[(time.time(), '')])
        song3 = Song('a', 't2', 'new', rating=1.0, track=2, disc=1,
                     plays=[(time.time(), '')])
        song4 = Song('a', 't1', 'old', rating=0.75, plays=[(1, '')])
        song5 = Song('a', 't2', 'old', rating=0.75, tags=['instrumental'],
                     plays=[(1, '')])
        song6 = Song('a', 't1', 'mellow', rating=0.75, tags=['mellow'])
        server.import_songs([song1, song2, song3, song4, song5, song6])

        page = Page(driver)
        page.select(page.PRESET_SELECT, text=page.PRESET_INSTRUMENTAL_OLD)
        self.wait_for_song(page, song5)
        page.select(page.PRESET_SELECT, text=page.PRESET_MELLOW)
        self.wait_for_song(page, song6)
        page.select(page.PRESET_SELECT, text=page.PRESET_NEW_ALBUMS)
        self.wait_for_search_results(page, [song2])
        page.select(page.PRESET_SELECT, text=page.PRESET_UNRATED)
        self.wait_for_song(page, song1)
        # TODO: Test PRESET_OLD? Not sure how to, since it shuffles and
        # autoplays (i.e. either song4 or song5 will play)...

        if page.is_focused(page.PRESET_SELECT):
            self.fail('Preset select still focused after click')

    def test_presentation(self):
        song1 = Song('artist', 'track1', 'album1', track=1, filename=Song.FILE_5S)
        song2 = Song('artist', 'track2', 'album1', track=2, filename=Song.FILE_5S)
        song3 = Song('artist', 'track3', 'album2', track=1, filename=Song.FILE_5S)
        server.import_songs([song1, song2, song3])

        page = Page(driver)
        page.keywords = 'album:album1'
        page.click(page.LUCKY_BUTTON)
        self.wait_for_playlist(page, [song1, song2], 0)

        page.show_presentation()
        self.wait_for_presentation(page, song1, song2)

        ESCAPE = u'\ue00c'
        page.get(page.BODY).send_keys(ESCAPE)
        # TODO: Wait to ensure that it's not visible.

        page.keywords = 'album:album2'
        page.click(page.SEARCH_BUTTON)
        self.wait_for_search_results(page, [song3])
        page.click(page.INSERT_BUTTON)
        self.wait_for_playlist(page, [song1, song3, song2], 0)

        page.show_presentation()
        self.wait_for_presentation(page, song1, song3)
        page.get(page.BODY).send_keys(ESCAPE)


if __name__ == '__main__':
    unittest.main()
