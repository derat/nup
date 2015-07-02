#!/usr/bin/python

import httplib
import json

import constants

class Server:
    def __init__(self, music_host_port):
        self.conn = httplib.HTTPConnection(constants.HOSTNAME, constants.PORT)
        self.headers = {
            'Cookie': '%s=1' % constants.AUTH_COOKIE,
        }
        self.post_request('/config', json.dumps({
            'SongBaseUrl': 'http://%s:%d/' % music_host_port,
            'CoverBaseUrl': '',
            'CacheSongs': False,
            'CacheQueries': False,
            'CacheTags': False,
        }))

    def post_request(self, path, body):
        self.conn.request('POST', path, body, self.headers)
        resp = self.conn.getresponse()
        resp.read()
        if resp.status != httplib.OK:
            raise RuntimeError('Got %s: %s' % (resp.status, resp.reason))

    def clear_data(self):
        self.post_request('/clear', None)

    def import_songs(self, songs):
        self.post_request('/import?replaceUserData=1',
                          '\n'.join([json.dumps(s.to_dict()) for s in songs]))
