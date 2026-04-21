import random
import socket
import time

from locust import User, between, events, task


class TCPKeyValueUser(User):
    wait_time = between(0, 0)

    # shared seed pool (per process, not per VU)
    seed_keys = []

    def on_start(self):
        self.host = "16.59.40.30"
        self.port = 8080

        self.sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.sock.settimeout(2.0)

        try:
            self.sock.connect((self.host, self.port))
        except Exception as e:
            print(f"Connection failed: {e}")

        # populate seed keys once (only first VU does heavy lifting)
        if not TCPKeyValueUser.seed_keys:
            self._seed_keys()

    def _seed_keys(self):
        print("Seeding 500 keys...")

        for i in range(500):
            key = f"seed_{i}"
            val = f"value_{i}"

            try:
                self.sock.sendall(f"WRITE|{key}|string|{val}|string\n".encode("utf-8"))
                self.sock.recv(1024)
                TCPKeyValueUser.seed_keys.append(key)
            except Exception as e:
                print(f"Seeding error: {e}")
                break

        print(f"Seeded {len(TCPKeyValueUser.seed_keys)} keys")

    @task(7)  # ~70%
    def read_task(self):
        if not TCPKeyValueUser.seed_keys:
            return

        key = random.choice(TCPKeyValueUser.seed_keys)
        self._send_command(f"GET|{key}|string", "GET")

    @task(3)  # ~30%
    def write_task(self):
        key = f"write_{time.time()}_{random.randint(0, 1_000_000)}"
        val = "bench_val"

        self._send_command(f"WRITE|{key}|string|{val}|string", "WRITE")

        # optionally add to read pool (keeps dataset fresh)
        if random.random() < 0.5:
            TCPKeyValueUser.seed_keys.append(key)

    def _send_command(self, command, name):
        start_time = time.time()

        try:
            self.sock.sendall((command + "\n").encode("utf-8"))
            data = self.sock.recv(1024).decode("utf-8").strip()

            total_time = int((time.time() - start_time) * 1000)

            events.request.fire(
                request_type="TCP",
                name=name,
                response_time=total_time,
                response_length=len(data),
                exception=None,
            )

        except Exception as e:
            total_time = int((time.time() - start_time) * 1000)

            events.request.fire(
                request_type="TCP",
                name=name,
                response_time=total_time,
                response_length=0,
                exception=e,
            )

            self.on_start()  # reconnect on failure

    def on_stop(self):
        self.sock.close()
