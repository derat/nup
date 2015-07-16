#!/usr/bin/python

import calendar
import datetime
import httplib
import json

import constants
from song import Song

class Server:
    def __init__(self, music_host_port):
        self.conn = httplib.HTTPConnection(constants.HOSTNAME, constants.PORT)
        self.headers = {
            'Cookie': '%s=1' % constants.AUTH_COOKIE,
        }
        self.send('POST', '/config', json.dumps({
            'SongBaseUrl': 'http://%s:%d/' % music_host_port,
            'CoverBaseUrl': '',
            'CacheSongs': False,
            'CacheQueries': False,
            'CacheTags': False,
        }))

    def reset_config(self):
        self.send('POST', '/config')

    def send(self, method, path, body=None):
        self.conn.request(method, path, body, self.headers)
        resp = self.conn.getresponse()
        result = resp.read()
        if resp.status != httplib.OK:
            raise RuntimeError('Got %s: %s' % (resp.status, resp.reason))
        return result

    def clear_data(self):
        self.send('POST', '/clear')

    def import_songs(self, songs):
        self.send('POST', '/import?replaceUserData=1',
                  '\n'.join([json.dumps(s.to_dict()) for s in songs]))

    def export_songs(self):
        def get_objects(path):
            out = self.send('GET', path).strip()
            # ''.split('\n') returns a list with a single empty string. :-/
            return [json.loads(s) for s in out.split('\n')] if len(out) else []

        songs_by_id = {}
        for obj in get_objects('/export?type=song'):
            song = Song(obj['artist'], obj['title'], obj['album'],
                        obj['track'], obj['disc'], obj['rating'],
                        obj['filename'], obj['length'], obj['tags'] or [])
            song.sha1 = obj['sha1']
            song.album_id = obj['albumId']
            songs_by_id[obj['songId']] = song
        for obj in get_objects('/export?type=play'):
            song = songs_by_id[obj['songId']]
            play_obj = obj['play']
            dt = datetime.datetime.strptime(play_obj['t'].split('.')[0],
                                            '%Y-%m-%dT%H:%M:%S')
            # Why is there datetime.fromtimestamp() but not date.totimestamp()?
            song.plays.append((calendar.timegm(dt.timetuple()),
                               play_obj['ip']))

        return {s.sha1: s for s in songs_by_id.values()}
