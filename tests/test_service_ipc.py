import json
import struct
import unittest


class FakeTransport:
    def __init__(self, available=True):
        self.available = available
        self.frames = []

    def wait(self, timeout_ms):
        return self.available

    def send(self, frame):
        self.frames.append(frame)


class ServiceIpcTests(unittest.TestCase):
    def test_frame_is_bounded_length_prefixed_json(self):
        from aetheris.service_ipc import ServiceClient

        transport = FakeTransport()
        client = ServiceClient(transport, pid=42, session_id=2, clock=lambda: 0.1)
        client.connect()
        client.ready()
        length = struct.unpack("<I", transport.frames[0][:4])[0]
        body = json.loads(transport.frames[0][4:])
        self.assertEqual(length, len(transport.frames[0]) - 4)
        self.assertEqual(body, {"version": 1, "type": "ready", "pid": 42, "session_id": 2, "monotonic_ms": 100})
        self.assertEqual(client.status_snapshot()["last_message"], "ready")

    def test_service_unavailable_is_explicit(self):
        from aetheris.service_ipc import ServiceClient, ServiceUnavailable

        with self.assertRaises(ServiceUnavailable):
            ServiceClient(FakeTransport(False), 42, 2).connect(timeout_seconds=0)

    def test_unknown_message_cannot_be_sent(self):
        from aetheris.service_ipc import ServiceClient

        with self.assertRaises(ValueError):
            ServiceClient(FakeTransport(), 42, 2)._send("run_path")


if __name__ == "__main__":
    unittest.main()
