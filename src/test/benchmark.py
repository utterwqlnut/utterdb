import random
import socket
import threading
import time

from locust import User, between, events, task


class TCPKeyValueUser(User):
    wait_time = between(0, 0)

    seed_keys = []
    expected = {}
    lock = threading.Lock()

    def on_start(self):
        self.host = "172.31.5.172"
        self.port = 8080

        self.sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.sock.settimeout(2.0)

        try:
            self.sock.connect((self.host, self.port))
        except Exception as e:
            print(f"Connection failed: {e}")

        if not TCPKeyValueUser.seed_keys:
            self._seed_keys()

    def _seed_keys(self):
        print("Seeding 500 keys...")

        for i in range(500):
            key = f"seed_{i}"
            val = f"value_{i}"

            try:
                self.sock.sendall(f"WRITE|{key}|string|{val}|string\n".encode())
                self.sock.recv(1024)

                with TCPKeyValueUser.lock:
                    TCPKeyValueUser.seed_keys.append(key)
                    TCPKeyValueUser.expected[key] = val

            except Exception as e:
                print(f"Seeding error: {e}")
                break

        print(f"Seeded {len(TCPKeyValueUser.seed_keys)} keys")

    @task(7)
    def read_task(self):
        if not TCPKeyValueUser.seed_keys:
            return

        key = random.choice(TCPKeyValueUser.seed_keys)

        with TCPKeyValueUser.lock:
            expected_val = TCPKeyValueUser.expected.get(key)

        self._send_get(key, expected_val)

    @task(3)
    def write_task(self):
        key = f"write_{time.time()}_{random.randint(0, 1_000_000)}"
        val = "bench_val"

        success = self._send_command(f"WRITE|{key}|string|{val}|string", "WRITE (µs)")

        if success:
            with TCPKeyValueUser.lock:
                TCPKeyValueUser.expected[key] = val
                if random.random() < 0.5:
                    TCPKeyValueUser.seed_keys.append(key)

    def _now_us(self):
        return time.perf_counter_ns() // 1000

    def _send_get(self, key, expected_val):
        start = self._now_us()

        try:
            self.sock.sendall(f"GET|{key}|string\n".encode())
            data = self.sock.recv(1024).decode().strip()

            if expected_val not in data:
                events.request.fire(
                    request_type="TCP",
                    name="GET_MISMATCH (µs)",
                    response_time=self._now_us() - start,  # µs
                    response_length=len(data),
                    exception=Exception(f"Expected {expected_val}, got {data}"),
                )
            else:
                events.request.fire(
                    request_type="TCP",
                    name="GET (µs)",
                    response_time=self._now_us() - start,  # µs
                    response_length=len(data),
                    exception=None,
                )

        except Exception as e:
            events.request.fire(
                request_type="TCP",
                name="GET (µs)",
                response_time=self._now_us() - start,
                response_length=0,
                exception=e,
            )
            self.on_start()

    def _send_command(self, command, name):
        start = self._now_us()

        try:
            self.sock.sendall((command + "\n").encode())
            data = self.sock.recv(1024).decode().strip()

            events.request.fire(
                request_type="TCP",
                name=name,
                response_time=self._now_us() - start,  # µs
                response_length=len(data),
                exception=None,
            )
            return True

        except Exception as e:
            events.request.fire(
                request_type="TCP",
                name=name,
                response_time=self._now_us() - start,
                response_length=0,
                exception=e,
            )
            self.on_start()
            return False

    def on_stop(self):
        self.sock.close()
