"""
utterdb ultra-light raw TCP benchmark
Measures pure throughput + latency.

No locks.
No shared state.
No validation.
No bookkeeping.

Run:
    locust -f benchmark.py \
        --headless -u 200 -r 50 \
        --run-time 60s \
        --host tcp://127.0.0.1:9000
"""

import random
import socket
import time

from locust import User, constant, events, task


class TCPUser(User):
    # absolutely no waiting
    wait_time = constant(0)

    def on_start(self):
        host = self.host.replace("tcp://", "")
        self.host_addr, port = host.rsplit(":", 1)
        self.port = int(port)

        self._connect()

    def on_stop(self):
        try:
            self.sock.close()
        except Exception:
            pass

    # ---------------------------------------------------------------- #

    def _connect(self):
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)

        # critical for tiny packets
        sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)

        sock.settimeout(5)

        sock.connect((self.host_addr, self.port))

        self.sock = sock
        self.reader = sock.makefile("rb")

    # ---------------------------------------------------------------- #

    def _request(self, payload: bytes, name: str):
        start = time.perf_counter_ns()

        try:
            self.sock.sendall(payload)

            resp = self.reader.readline()

            if not resp:
                raise ConnectionError("closed")

            elapsed_ms = (time.perf_counter_ns() - start) / 1_000_000

            events.request.fire(
                request_type="TCP",
                name=name,
                response_time=elapsed_ms,
                response_length=len(resp),
                exception=None,
            )

        except Exception as e:
            elapsed_ms = (time.perf_counter_ns() - start) / 1_000_000

            events.request.fire(
                request_type="TCP",
                name=name,
                response_time=elapsed_ms,
                response_length=0,
                exception=e,
            )

            try:
                self.sock.close()
            except Exception:
                pass

            self._connect()

    # ---------------------------------------------------------------- #
    # 70% GET
    # 30% WRITE
    # ---------------------------------------------------------------- #

    @task(7)
    def get_task(self):
        key = f"k{random.randint(0, 100000)}"

        self._request(
            f"GET|{key}|string\n".encode(),
            "GET",
        )

    @task(3)
    def write_task(self):
        key = f"k{time.time_ns()}"

        self._request(
            f"WRITE|{key}|string|value|string\n".encode(),
            "WRITE",
        )