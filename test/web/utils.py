#!/usr/bin/python

import time

class TimeoutError(Exception):
    def __init__(self, value):
        self.value = value
    def __str__(self):
        return repr(self.value)

def wait(f, timeout_sec=10, sleep_sec=0.05):
    '''Waits for a function to return true.'''
    start = time.time()
    while True:
        if f():
            return
        if time.time() - start >= timeout_sec:
            raise TimeoutError('Timed out waiting for condition')
        time.sleep(sleep_sec)

def wait_equal(f, expected, timeout_sec=10, sleep_sec=0.1):
    actual = None
    def is_equal():
        actual = f()
        return actual == expected
    try:
        wait(is_equal, timeout_sec, sleep_sec)
    except TimeoutError as e:
        return TimeoutError(
            str(e) + ' (got "%s"; want "%s")' % (actual, expected))
