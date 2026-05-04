import os
import time
import json
import hmac
import hashlib
import threading
import requests
from typing import Any, Optional

BASE_URL = "https://www.fubon.com/Fubon-Premium-Pattern/external"


class FubonDMASession:
    _local = threading.local()

    def __init__(self, api_key: str, secret_key: str, use_mock: bool = False):
        self.api_key = api_key
        self.secret_key = secret_key.encode()
        self.use_mock = use_mock
        self._lock = threading.Lock()
        self._session = None

    @classmethod
    def get_instance(cls) -> Optional["FubonDMASession"]:
        return getattr(cls._local, "instance", None)

    @classmethod
    def set_instance(cls, session: "FubonDMASession") -> None:
        cls._local.instance = session

    def send_request(self, method: str, path: str, params: Optional[dict] = None) -> dict:
        with self._lock:
            if self.use_mock:
                return self._mock_response(method, path, params)

            session = self._get_session()
            timestamp = str(int(time.time() * 1000))
            body = json.dumps(params) if params else ""
            sign = self._sign(method, path, timestamp, body)

            headers = {
                "Content-Type": "application/json",
                "X-FB-API-KEY": self.api_key,
                "X-FB-TIMESTAMP": timestamp,
                "X-FB-SIGNATURE": sign,
            }

            url = f"{BASE_URL}{path}"
            if method == "GET" and params:
                resp = session.get(url, params=params, headers=headers, timeout=10)
            elif method == "POST":
                resp = session.post(url, data=body, headers=headers, timeout=10)
            else:
                resp = session.request(method, url, timeout=10)

            resp.raise_for_status()
            return resp.json()

    def _get_session(self) -> requests.Session:
        if self._session is None:
            self._session = requests.Session()
        return self._session

    def _sign(self, method: str, path: str, timestamp: str, body: str) -> str:
        msg = f"{method}{path}{timestamp}{body}"
        return hmac.new(self.secret_key, msg.encode(), hashlib.sha256).hexdigest()

    def _mock_response(self, method: str, path: str, params: Optional[dict]) -> dict:
        if "order" in path:
            return {
                "status": 0,
                "message": "mock order accepted",
                "order_id": "MOCK-" + str(int(time.time())),
            }
        if "positions" in path:
            return {"positions": []}
        if "balances" in path:
            return {"balance": 1000000.0}
        return {"status": 0, "data": {}}


def init_session(api_key: Optional[str] = None, secret_key: Optional[str] = None) -> FubonDMASession:
    if api_key is None:
        api_key = os.getenv("FUBON_API_KEY", "")
    if secret_key is None:
        secret_key = os.getenv("FUBON_SECRET_KEY", "")

    use_mock = os.getenv("FUBON_MOCK_MODE", "false").lower() == "true"
    session = FubonDMASession(api_key, secret_key, use_mock=use_mock)
    FubonDMASession.set_instance(session)
    return session
