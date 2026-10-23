#!/usr/bin/env python3
"""Focused streaming regression, CLI and TLS controls for the installed wheel."""
import argparse
import contextlib
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
from pathlib import Path
import ssl
import subprocess
import sys
import tempfile
import threading

import pip
from pip._internal.network.utils import response_chunks
from pip._vendor import msgpack, requests, urllib3
from pip._vendor.urllib3.exceptions import ProtocolError


class TrackedBody(io.BytesIO):
    largest_readline = 0
    def readline(self, size=-1):
        data = super().readline(size)
        self.largest_readline = max(self.largest_readline, len(data))
        return data


class Socket:
    def __init__(self, body):
        self.body = body
    def makefile(self, *_):
        return self.body


def streamed(body):
    wire = TrackedBody(b"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n" + body)
    origin = http.client.HTTPResponse(Socket(wire), method="GET")
    origin.begin()
    response = requests.Response()
    response.status_code = 200
    response.raw = urllib3.response.HTTPResponse(
        body=origin, original_response=origin, headers=dict(origin.headers.items()),
        preload_content=False)
    return response, wire


def boundary_tests(expect_fixed):
    outcome = []
    for label, body in (
        ("chunk-size", b"f" * 131072),
        ("chunk-trailer", b"3\r\nfoo\r\n0\r\n" + b"X" * 131072),
    ):
        response, wire = streamed(body)
        error_description = "no exception"
        try:
            list(response_chunks(response))
        except Exception as error:
            error_description = repr(error)
            if expect_fixed:
                assert isinstance(error, ProtocolError), repr(error)
                assert "exceeded maximum allowed length" in str(error), str(error)
        if expect_fixed:
            assert error_description != "no exception", "malicious chunk line was accepted"
            assert wire.largest_readline <= 65537, wire.largest_readline
        else:
            assert wire.largest_readline == 131072, (wire.largest_readline, error_description)
        outcome.append({"path": label, "max_readline": wire.largest_readline})
    for body in (b"3\r\nfoo\r\n0\r\n\r\n", b"3;" + b"x" * 65532 + b"\r\nfoo\r\n0\r\n\r\n"):
        response, _ = streamed(body)
        assert b"".join(response_chunks(response)) == b"foo"
    return outcome


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--expect-vulnerable", action="store_true")
    args = parser.parse_args()
    result = {"pip": pip.__version__, "urllib3": urllib3.__version__,
              "streaming": boundary_tests(not args.expect_vulnerable)}
    if args.expect_vulnerable:
        print(json.dumps(result)); return
    assert pip.__version__ == "26.2.2+pasturestack.1"
    assert urllib3.__version__ == "2.8.0"
    assert msgpack.Unpacker.__module__ == "pip._vendor.msgpack.fallback"
    import importlib.util
    assert importlib.util.find_spec("pip._vendor.msgpack._cmsgpack") is None
    unpacker = msgpack.Unpacker()
    unpacker.feed(b"\x83\x73\xc4\x00")
    try:
        unpacker.unpack()
    except (ValueError, msgpack.OutOfData):
        pass
    try:
        unpacker.skip()
    except (ValueError, msgpack.OutOfData):
        pass
    with tempfile.TemporaryDirectory(prefix="pip-tls-") as name:
        root = Path(name)
        cert, key = root / "cert.pem", root / "key.pem"
        subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
                        "-days", "1", "-subj", "/CN=localhost", "-addext",
                        "subjectAltName=DNS:localhost", "-keyout", str(key), "-out", str(cert)],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                body = b"trusted fixture"
                self.send_response(200); self.send_header("Content-Length", str(len(body)))
                self.end_headers(); self.wfile.write(body)
            def log_message(self, *_):
                pass
        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        context.minimum_version = ssl.TLSVersion.TLSv1_2
        context.load_cert_chain(cert, key)
        server.socket = context.wrap_socket(server.socket, server_side=True)
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        try:
            port = server.server_address[1]
            session = requests.Session(); session.trust_env = False
            assert session.get(f"https://localhost:{port}/", verify=str(cert), timeout=5).status_code == 200
            for url, verify in ((f"https://localhost:{port}/", True),
                                (f"https://127.0.0.1:{port}/", str(cert))):
                try:
                    session.get(url, verify=verify, timeout=5)
                except requests.exceptions.SSLError:
                    pass
                else:
                    raise AssertionError("untrusted or wrong-host TLS was accepted")
        finally:
            server.shutdown(); server.server_close(); thread.join()
    subprocess.run([sys.executable, "-m", "pip", "--version"], check=True)
    subprocess.run([sys.executable, "-m", "pip", "check"], check=True)
    result.update(tls_trusted="accepted", tls_untrusted="rejected", tls_wrong_host="rejected",
                  msgpack_implementation=msgpack.Unpacker.__module__, cli="passed")
    print(json.dumps(result))


if __name__ == "__main__":
    if sys.flags.optimize:
        raise RuntimeError("Security regression tests must not run with Python optimization")
    main()
