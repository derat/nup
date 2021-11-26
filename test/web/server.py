#!/usr/bin/python

import calendar
import datetime
import httplib
import json

import constants
from song import Song

class Server:
    def __init__(self, music_host_port):
        self.reset_connection()
        self.music_host_port = music_host_port
        self.headers = {
            'Cookie': '%s=1' % constants.AUTH_COOKIE,
        }
        self.send_config()

    def reset_connection(self):
        self.conn = httplib.HTTPConnection(constants.HOSTNAME, constants.PORT)

    def send_config(self, force_update_failures=False):
        self.send('POST', '/config', json.dumps({
            'songBaseUrl': 'http://%s:%d/' % self.music_host_port,
            'coverBaseUrl': '',
            'forceUpdateFailures': force_update_failures,
            'presets': [
                {
                    "name": "instrumental old",
                    "tags": "instrumental",
                    "minRating": 4,
                    "lastPlayed": 6,
                    "shuffle": True,
                    "play": True,
                },
                {
                    "name": "mellow",
                    "tags": "mellow",
                    "minRating": 4,
                    "shuffle": True,
                    "play": True,
                },
                {
                    "name": "new albums",
                    "firstPlayed": 3,
                    "firstTrack": True,
                },
                {
                    "name": "unrated",
                    "unrated": True,
                    "play": True,
                },
            ],
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

    def get_objects(self, path):
        out = self.send('GET', path).strip()
        # ''.split('\n') returns a list with a single empty string. :-/
        return [json.loads(s) for s in out.split('\n')] if len(out) else []

    def export_songs(self):
        def parse_time(s):
            dt = datetime.datetime.strptime(s,
                '%Y-%m-%dT%H:%M:%S.%fZ' if '.' in s else '%Y-%m-%dT%H:%M:%SZ')
            # Why is there datetime.fromtimestamp() but not totimestamp()?
            # Instead there's just this stupid timegm() function that doesn't
            # know about microseconds.
            t = calendar.timegm(dt.timetuple()) + float(dt.strftime('0.%f'))
            return t

        songs_by_id = {}
        for obj in self.get_objects('/export?type=song'):
            song = Song(obj['artist'], obj['title'], obj['album'],
                        obj['track'], obj['disc'], obj['rating'],
                        obj['filename'], obj['length'], obj['tags'] or [])
            song.sha1 = obj['sha1']
            song.album_id = obj['albumId']
            songs_by_id[obj['songId']] = song
        for obj in self.get_objects('/export?type=play'):
            song = songs_by_id[obj['songId']]
            play_obj = obj['play']
            song.plays.append((parse_time(play_obj['t']), play_obj['ip']))

        for s in songs_by_id.values():
            s.plays.sort()

        return {s.sha1: s for s in songs_by_id.values()}

    def get_song_id(self, sha1):
        for obj in self.get_objects('/export?type=song'):
            if obj['sha1'] == sha1:
                return obj['songId']
        raise RuntimeError('Didn\'t get song with SHA1 "%s"' % sha1)
