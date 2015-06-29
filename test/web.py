#!/usr/bin/python
# coding=UTF-8

import httplib
import json
import os
import pprint
import sha
import SimpleHTTPServer
import SocketServer
import subprocess
import threading
import time
import unittest

from selenium import webdriver

HOSTNAME = 'localhost:8080'
BASE_URL = 'http://' + HOSTNAME
AUTH_COOKIE = 'webdriver'
MUSIC_PATH = 'data/music'
MUSIC_FILE = '5s.mp3'

file_thread = None
server = None
driver = None

class TimeoutError(Exception):
    def __init__(self, value):
        self.value = value
    def __str__(self):
        return repr(self.value)

def make_song(artist, title, album, track=0, disc=0, rating=-1, tags=[]):
    '''Returns a dictionary describing a song, suitable for JSONification.'''
    return {
        'sha1': sha.new('%s-%s-%s' % (artist, album, title)).hexdigest(),
        'filename': MUSIC_FILE,
        'artist': artist,
        'title': title,
        'album': album,
        'albumId': '%s-%s' % (artist, album),
        'track': track,
        'disc': disc,
        'rating': rating,
        'tags': tags,
    }

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

class FileServerThread(threading.Thread):
    def __init__(self, path):
        threading.Thread.__init__(self)
        os.chdir(path)
        handler = SimpleHTTPServer.SimpleHTTPRequestHandler
        self.server = SocketServer.TCPServer(("", 0), handler)

    def server_address(self):
        return self.server.server_address

    def run(self):
        self.server.serve_forever()

    def stop(self):
        self.server.shutdown()
        self.join()

class Server:
    def __init__(self, hostname):
        self.conn = httplib.HTTPConnection(hostname)
        self.headers = {
            'Cookie': '%s=1' % AUTH_COOKIE,
        }
        # TODO: Need to configure server to give URLs to local file server.

    def clear_data(self):
        self.conn.request('POST', '/clear', None, self.headers)
        resp = self.conn.getresponse()
        resp.read()
        if resp.status != httplib.OK:
            raise RuntimeError('Got %s: %s' % (resp.status, resp.reason))

    def import_songs(self, songs):
        path = '/import?replaceUserData=1'
        body = '\n'.join([json.dumps(s) for s in songs])
        self.conn.request('POST', path, body, self.headers)
        resp = self.conn.getresponse()
        resp.read()
        if resp.status != httplib.OK:
            raise RuntimeError('Got %s: %s' % (resp.status, resp.reason))

def setUpModule():
    global file_thread
    file_thread = FileServerThread(MUSIC_PATH)
    file_thread.start()

    global server
    server = Server(HOSTNAME)

    global driver
    driver = webdriver.Chrome()

    # Makes no sense: Chrome starts at a data: URL, so I get a "Cookies are
    # disabled inside 'data:' URLs" exception if I try to add the cookie before
    # loading a page.
    driver.get(BASE_URL)
    driver.add_cookie({
        'name': AUTH_COOKIE,
        'value': 1,
    })
    driver.get(BASE_URL)

def tearDownModule():
    file_thread.stop()
    driver.close()

class Test(unittest.TestCase):
    def setUp(self):
        server.clear_data()

    def get_results(self):
        results = []
        table = driver.find_element_by_id('searchResultsTable')
        rows = table.find_elements_by_tag_name('tr')
        for i in range(1, len(rows)):
            row = rows[i]
            cols = row.find_elements_by_tag_name('td')
            results.append({
                'artist': cols[1].text,
                'title': cols[2].text,
                'album': cols[3].text,
                'time': cols[4].text,
            })
        return results

    def wait_for_results(self, songs):
        def results_match():
            results = self.get_results()
            if len(results) != len(songs):
                return False
            for i in range(len(songs)):
                if songs[i]['artist'] != results[i]['artist'] or \
                   songs[i]['title'] != results[i]['title'] or \
                   songs[i]['album'] != results[i]['album']:
                    return False
            return True

        try:
            wait(results_match, timeout_sec=3)
        except TimeoutError as e:
            self.fail("Timed out waiting for expected results. Received:\n" +
                      pprint.pformat(self.get_results()))

    def get_current_song(self):
        return {
            'artist': driver.find_element_by_id('artistDiv').text,
            'title': driver.find_element_by_id('titleDiv').text,
            'album': driver.find_element_by_id('albumDiv').text,
        }

    def wait_for_song(self, song):
        def is_current():
            current = self.get_current_song()
            # TODO: Make sure that it's actually playing once music-file-serving
            # is working.
            return current['artist'] == song['artist'] and \
                   current['title'] == song['title'] and \
                   current['album'] == song['album']
        try:
            wait(is_current, timeout_sec=5)
        except TimeoutError as e:
            self.fail("Timed out waiting for song. Received:\n" +
                      pprint.pformat(self.get_current_song()))

    def test_queries(self):
        album1 = [
            make_song('ar1', 'tr1', 'al1', 1, 1, 0.5),
            make_song('ar1', 'tr2', 'al1', 2, 1, 0.75),
            make_song('ar1', 'tr3', 'al1', 3, 1, 0.25),
        ]
        album2 = [
            make_song('ar2', 'tr1', 'al2', 1, 1, 1.0),
            make_song('ar2', 'tr2', 'al2', 2, 1, 0.0),
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
        song = make_song('artist', 'track', 'album')
        server.import_songs([song])

        driver.find_element_by_id('resetButton').click()
        driver.find_element_by_id('keywordsInput').send_keys(song['artist'])
        driver.find_element_by_id('luckyButton').click()
        self.wait_for_song(song)

if __name__ == '__main__':
    unittest.main()

