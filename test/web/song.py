#!/usr/bin/python

import sha

MUSIC_FILE = '5s.mp3'

class Song:
    def __init__(self, artist, title, album, track=0, disc=0, rating=None,
                 tags=None):
        self.artist = artist
        self.title = title
        self.album = album
        self.track = track
        self.disc = disc
        self.rating = rating
        self.tags = tags

        self.sha1 = sha.new('%s-%s-%s' % (artist, album, title)).hexdigest()
        self.album_id = '%s-%s' % (artist, album)
        self.filename = MUSIC_FILE

    def to_dict(self):
        return {
            'sha1': self.sha1,
            'filename': self.filename,
            'artist': self.artist,
            'title': self.title,
            'album': self.album,
            'albumId': self.album_id,
            'track': self.track,
            'disc': self.disc,
            'rating': self.rating,
            'tags': self.tags,
        }

    def __str__(self):
        return '[%s, %s, %s]' % (self.artist, self.title, self.album)

    def __repr__(self):
        return self.__str__()

    def __eq__(self, other):
        return self.artist == other.artist and \
               self.title == other.title and \
               self.album == other.album
