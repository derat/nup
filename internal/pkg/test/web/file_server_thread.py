#!/usr/bin/python

import os
import SimpleHTTPServer
import SocketServer
import threading

class FileServerThread(threading.Thread):
    def __init__(self, path):
        threading.Thread.__init__(self)
        os.chdir(path)
        handler = SimpleHTTPServer.SimpleHTTPRequestHandler
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
