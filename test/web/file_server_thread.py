#!/usr/bin/python

import os
from SimpleHTTPServer import SimpleHTTPRequestHandler
import SocketServer
import threading

# See https://stackoverflow.com/a/21957017.
class CORSRequestHandler (SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header('Access-Control-Allow-Credentials', 'true')
        self.send_header('Access-Control-Allow-Origin', 'http://localhost:8080')
        SimpleHTTPRequestHandler.end_headers(self)

class FileServerThread(threading.Thread):
    def __init__(self, path):
        threading.Thread.__init__(self)
        os.chdir(path)
        handler = CORSRequestHandler
        handler.log_message = self.log
        self.requests = []
        self.server = SocketServer.TCPServer(('localhost', 0), handler)

    def log(self, fmt, *args):
        self.requests.append(fmt % args)

    def host_port(self):
        return self.server.server_address

    def run(self):
        self.server.serve_forever()

    def stop(self):
        self.server.shutdown()
        self.join()
