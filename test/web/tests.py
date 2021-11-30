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

    def wait_for_playlist(self, page, songs, active_index=-1, menu_index=-1,
                          msg=''):
        '''Waits until the page is displaying the expected playlist.'''
        def is_expected():
            playlist = page.get_playlist()
            if playlist != songs:
                return False
            if active_index >= 0:
                for i, song in enumerate(playlist):
                    if (active_index == i and not song.active) or \
                       (active_index != i and song.active):
                        return False
            if menu_index >= 0:
                for i, song in enumerate(playlist):
                    if (menu_index == i and not song.menu) or \
                       (menu_index != i and song.menu):
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
