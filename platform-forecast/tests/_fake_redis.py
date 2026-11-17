from __future__ import annotations

import socket
import socketserver
import threading
import time


def _enc(v) -> bytes:
    if v is None:
        return b"$-1\r\n"
    if isinstance(v, bytes):
        return b"$" + str(len(v)).encode() + b"\r\n" + v + b"\r\n"
    if isinstance(v, int):
        return b":" + str(v).encode() + b"\r\n"
    if isinstance(v, list):
        return b"*" + str(len(v)).encode() + b"\r\n" + b"".join(_enc(x) for x in v)
    return b"+" + str(v).encode() + b"\r\n"


class _Handler(socketserver.StreamRequestHandler):
    def _read_cmd(self):
        line = self.rfile.readline()
        if not line:
            return None
        n = int(line[1:])
        out = []
        for _ in range(n):
            ln = int(self.rfile.readline()[1:])
            out.append(self.rfile.read(ln + 2)[:-2])
        return out

    def handle(self):
        store = self.server.store
        queued = None
        while True:
            cmd = self._read_cmd()
            if cmd is None:
                return
            name = cmd[0].upper().decode()
            if name == "MULTI":
                queued = []
                self.wfile.write(b"+OK\r\n")
                continue
            if name == "EXEC":
                res = [self._run(c) for c in queued or []]
                queued = None
                self.wfile.write(b"*" + str(len(res)).encode() + b"\r\n" + b"".join(res))
                continue
            if queued is not None:
                queued.append(cmd)
                self.wfile.write(b"+QUEUED\r\n")
                continue
            self.wfile.write(self._run(cmd))

    def _run(self, cmd) -> bytes:
        srv = self.server
        name = cmd[0].upper().decode()
        args = cmd[1:]
        if name in ("PING",):
            return b"+PONG\r\n"
        if name in ("SELECT", "CLIENT", "AUTH"):
            return b"+OK\r\n"
        if name == "HELLO":
            return b"-ERR unknown command 'HELLO'\r\n"
        writes = ("SET", "SETEX", "PSETEX", "DEL", "EXPIRE", "UNLINK", "FLUSHDB", "FLUSHALL")
        if name in writes:
            if srv.fail_writes:
                return b"-READONLY You can't write against a read only replica.\r\n"
            srv.write_log.append(name)
        if name == "SETEX":
            srv.store[args[0]] = (args[2], int(args[1]))
            return b"+OK\r\n"
        if name == "SET":
            ttl = -1
            up = [a.upper() for a in args]
            if b"EX" in up:
                ttl = int(args[up.index(b"EX") + 1])
            srv.store[args[0]] = (args[1], ttl)
            return b"+OK\r\n"
        if name == "GET":
            item = srv.store.get(args[0])
            return _enc(item[0] if item else None)
        if name == "TTL":
            item = srv.store.get(args[0])
            return _enc(item[1] if item else -2)
        if name in ("DEL", "UNLINK"):
            return _enc(sum(1 for a in args if srv.store.pop(a, None) is not None))
        if name == "EXISTS":
            return _enc(sum(1 for a in args if a in srv.store))
        if name == "KEYS":
            import fnmatch

            pat = args[0].decode()
            return _enc([k for k in srv.store if fnmatch.fnmatchcase(k.decode(), pat)])
        if name == "SCAN":
            import fnmatch

            pat = "*"
            up = [a.upper() for a in args]
            if b"MATCH" in up:
                pat = args[up.index(b"MATCH") + 1].decode()
            keys = [k for k in srv.store if fnmatch.fnmatchcase(k.decode(), pat)]
            return _enc([b"0", keys])
        return b"-ERR unknown command '" + name.encode() + b"'\r\n"


class FakeRedis(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True

    def __init__(self):
        super().__init__(("127.0.0.1", 0), _Handler)
        self.store: dict[bytes, tuple[bytes, int]] = {}
        self.fail_writes = False
        self.write_log: list[str] = []
        self._t = threading.Thread(target=self.serve_forever, daemon=True)
        self._t.start()

    @property
    def url(self) -> str:
        return f"redis://127.0.0.1:{self.server_address[1]}/3"

    def keys(self, prefix: str = "fc:v1") -> list[str]:
        return sorted(k.decode() for k in self.store if k.decode().startswith(prefix))

    def stop(self):
        self.shutdown()
        self.server_close()


def closed_port_url() -> str:
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return f"redis://127.0.0.1:{port}/3"
