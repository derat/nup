#!/usr/bin/python

import datetime
import sha


class Song:
    # Various MP3 files that can be used as a song's filename.
    FILE_0S = '0s.mp3'
    FILE_1S = '1s.mp3'
    FILE_5S = '5s.mp3'

    def __init__(self, artist, title, album, track=0, disc=0, rating=-1,
                 filename=FILE_5S, length=5.0, tags=[], plays=[]):
        '''tags: List of strings.
           plays: List of (past_days, ip) tuples.
                  past_days is int, ip is string.
        '''
        self.artist = artist
        self.title = title
        self.album = album
        self.track = track
        self.disc = disc
        self.rating = rating
        self.filename = filename
        self.length = length
        self.tags = tags
        self.plays = plays

        self.sha1 = sha.new('%s-%s-%s' % (artist, album, title)).hexdigest()
        self.album_id = '%s-%s' % (artist, album)

        # Used for playlist entries and search results, respectively.
        self.highlighted = False
        self.checked = False

    def to_dict(self):
        def get_time(days_past):
            t = datetime.datetime.utcnow() - datetime.timedelta(days=days_past)
            return t.isoformat('T') + 'Z'

        return {
            'sha1': self.sha1,
            'filename': self.filename,
            'artist': self.artist,
            'title': self.title,
            'album': self.album,
            'albumId': self.album_id,
            'track': self.track,
            'disc': self.disc,
            'length': self.length,
            'rating': self.rating,
            'tags': self.tags,
            'plays': [{'t': get_time(p[0]), 'ip': p[1]} for p in self.plays],
        }

    def __str__(self):
        info = ''
        if self.highlighted:
            info += ' (highlighted)'
        if self.checked:
            info += ' (checked)'
        return '[%s, %s, %s%s]' % (self.artist, self.title, self.album, info)

    def __repr__(self):
        return self.__str__()

    def __eq__(self, other):
        return self.artist == other.artist and \
               self.title == other.title and \
               self.album == other.album
