"""
utterdb Locust load test — raw TCP, 70% GET / 30% WRITE mix.

Run headless:
    locust -f src/test/benchmark.py \
        --headless -u 50 -r 10 \
        --run-time 60s \
        --host tcp://localhost:9000

Or with the web UI (omit --headless):
    locust -f src/test/benchmark.py --host tcp://localhost:9000
"""

import random
import socket
import threading
import time

from locust import User, between, events, task


class TCPKeyValueUser(User):
    # No artificial wait between requests — max throughput
    wait_time = between(0, 0)

    # Shared across all users, protected by a lock
    _seed_keys: list[str] = []
    _expected: dict[str, str] = {}
    _seeded = False
    _lock = threading.Lock()

    # ── Lifecycle ─────────────────────────────────────────────────────────────

    def on_start(self):
        # Parse host/port from self.host which Locust sets to --host value
        # Expected format: tcp://host:port  or  host:port
        raw = self.host.replace("tcp://", "")
        self._host, port_str = raw.rsplit(":", 1)
        self._port = int(port_str)

        self._sock = self._connect()

        # Only the first user seeds; others wait until seeding is done
        with TCPKeyValueUser._lock:
            if not TCPKeyValueUser._seeded:
                TCPKeyValueUser._seeded = True
                should_seed = True
            else:
                should_seed = False

        if should_seed:
            self._seed(500)

    def on_stop(self):
        try:
            self._sock.close()
        except Exception:
            pass

    # ── Connection helpers ────────────────────────────────────────────────────

    def _connect(self) -> socket.socket:
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        s.settimeout(5.0)
        s.connect((self._host, self._port))
        return s

    def _reconnect(self):
        try:
            self._sock.close()
        except Exception:
            pass
        try:
            self._sock = self._connect()
        except Exception as e:
            print(f"Reconnect failed: {e}")

    # ── Seeding ───────────────────────────────────────────────────────────────

    def _seed(self, n: int):
        print(f"Seeding {n} keys...")
        seeded = 0
        for i in range(n):
            key = f"seed_{i}"
            val = f"value_{i}"
            try:
                self._raw_send(f"WRITE|{key}|string|{val}|string")
                resp = self._raw_recv()
                if resp.startswith("OK"):
                    with TCPKeyValueUser._lock:
                        TCPKeyValueUser._seed_keys.append(key)
                        TCPKeyValueUser._expected[key] = val
                    seeded += 1
            except Exception as e:
                print(f"Seed error at key {i}: {e}")
                self._reconnect()
        print(f"Seeded {seeded}/{n} keys")

    # ── Raw TCP helpers ───────────────────────────────────────────────────────

    def _raw_send(self, msg: str):
        self._sock.sendall((msg + "\n").encode())

    def _raw_recv(self) -> str:
        buf = b""
        while not buf.endswith(b"\n"):
            chunk = self._sock.recv(4096)
            if not chunk:
                raise ConnectionError("Connection closed by server")
            buf += chunk
        return buf.decode().strip()

    def _send(self, cmd: str, name: str) -> tuple[bool, str]:
        """
        Send a command, record Locust stats, return (success, response).
        Locust expects response_time in milliseconds.
        """
        start_ns = time.perf_counter_ns()
        try:
            self._raw_send(cmd)
            resp = self._raw_recv()
            elapsed_ms = (time.perf_counter_ns() - start_ns) / 1_000_000

            failed = resp.startswith("ERR")
            events.request.fire(
                request_type="TCP",
                name=name,
                response_time=elapsed_ms,
                response_length=len(resp),
                exception=Exception(resp) if failed else None,
            )
            return not failed, resp

        except Exception as e:
            elapsed_ms = (time.perf_counter_ns() - start_ns) / 1_000_000
            events.request.fire(
                request_type="TCP",
                name=name,
                response_time=elapsed_ms,
                response_length=0,
                exception=e,
            )
            self._reconnect()
            return False, ""

    # ── Tasks ─────────────────────────────────────────────────────────────────

    @task(7)
    def read_task(self):
        with TCPKeyValueUser._lock:
            if not TCPKeyValueUser._seed_keys:
                return
            key = random.choice(TCPKeyValueUser._seed_keys)
            expected = TCPKeyValueUser._expected.get(key, "")

        ok, resp = self._send(f"GET|{key}|string", "GET")

        # Treat a value mismatch as a separate failure category
        if ok and expected and expected not in resp:
            events.request.fire(
                request_type="TCP",
                name="GET_MISMATCH",
                response_time=0,
                response_length=len(resp),
                exception=Exception(f"expected={expected} got={resp}"),
            )

    @task(3)
    def write_task(self):
        key = f"bench_{time.time_ns()}_{random.randint(0, 1_000_000)}"
        val = "bench_val"
        ok, _ = self._send(f"WRITE|{key}|string|{val}|string", "WRITE")
        if ok:
            with TCPKeyValueUser._lock:
                TCPKeyValueUser._expected[key] = val
                # Keep the shared key pool from growing unboundedly
                if random.random() < 0.3:
                    TCPKeyValueUser._seed_keys.append(key)
