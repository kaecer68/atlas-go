"""
Fubon MarketData Proxy Service
"""
import os, sys, glob, time, json, asyncio, signal, logging, threading
from typing import List, Optional, Dict, Any
from contextlib import asynccontextmanager
from pathlib import Path

_env_path = Path.home() / ".config" / "atlas-go" / ".env"
if _env_path.exists():
    try:
        from dotenv import load_dotenv
        load_dotenv(str(_env_path))
    except ImportError:
        pass

from fastapi import FastAPI, HTTPException, Query
from pydantic import BaseModel

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

sdk = None
rest_client = None
login_time = None
SESSION_TIMEOUT = 3600
SDK_INIT_TIMEOUT = 5
_cache: Dict[str, Dict[str, Any]] = {}
_cache_lock = threading.Lock()
CACHE_TTL = 30

class _SDKTimeout(Exception):
    pass

def _sdk_alarm_handler(signum, frame):
    raise _SDKTimeout()

class QuoteResponse(BaseModel):
    symbol: str; name: str; last: float; open: float; high: float; low: float
    volume: int; reference_price: float; previous_close: float; change: float
    change_percent: float; bids: List[dict]; asks: List[dict]
    is_open: bool; is_close: bool; timestamp: int; source: str = "fubon"

class MarketStatusResponse(BaseModel):
    status: str; is_open: bool; timestamp: int

def _find_cert():
    cert_path = os.getenv("FUBON_CERT_PATH")
    if cert_path and os.path.exists(cert_path):
        return cert_path
    search_dirs = [
        os.path.expanduser("~/.config/atlas-go/.fubon-env"),
        os.path.expanduser("~/.local/share/atlas-go/fubon_neo"),
    ]
    candidates = []
    for d in search_dirs:
        if os.path.isdir(d):
            candidates.extend(glob.glob(os.path.join(d, "*.p12")))
    if candidates:
        candidates.sort(key=os.path.getmtime, reverse=True)
        logger.info(f"Auto-detected cert: {candidates[0]}")
        return candidates[0]
    return cert_path

def _init_sdk_sync():
    global sdk, rest_client, login_time
    import socket
    socket.setdefaulttimeout(SDK_INIT_TIMEOUT)
    from fubon_neo.sdk import FubonSDK
    personal_id = os.getenv("FUBON_PERSONAL_ID")
    password = os.getenv("FUBON_PASSWORD")
    cert_path = _find_cert()
    cert_password = os.getenv("FUBON_CERT_PASSWORD")
    if not personal_id or not password:
        raise ValueError("FUBON_PERSONAL_ID and FUBON_PASSWORD must be set")
    if not cert_path:
        raise ValueError("Certificate not found")
    new_sdk = FubonSDK()
    if cert_password:
        result = new_sdk.login(personal_id, password, cert_path, cert_password)
    else:
        result = new_sdk.login(personal_id, password, cert_path)
    if not result.is_success:
        raise Exception(f"Login failed: {result.message}")
    logger.info("Login successful: %s", result.data)
    new_sdk.init_realtime()
    new_rest_client = new_sdk.marketdata.rest_client.stock
    sdk = new_sdk
    rest_client = new_rest_client
    login_time = time.time()

async def _init_sdk_async():
    global sdk, rest_client, login_time
    old_handler = signal.signal(signal.SIGALRM, _sdk_alarm_handler)
    signal.alarm(SDK_INIT_TIMEOUT)
    try:
        _init_sdk_sync()
    except _SDKTimeout:
        logger.error("SDK init timed out after %ds", SDK_INIT_TIMEOUT)
        sdk = None; rest_client = None
        raise HTTPException(status_code=503, detail="SDK init timed out")
    except Exception as e:
        logger.error("SDK init failed: %s", e)
        sdk = None; rest_client = None
        raise
    finally:
        signal.alarm(0)
        signal.signal(signal.SIGALRM, old_handler)

def get_sdk():
    global sdk, rest_client, login_time
    if sdk is not None and login_time is not None:
        if time.time() - login_time < SESSION_TIMEOUT:
            return sdk, rest_client
        logger.info("Session expired")
        sdk = None; rest_client = None
    return None, None

def _unwrap(result):
    if result is None: return None
    if hasattr(result, "data"): return result.data
    if isinstance(result, dict): return result
    return None

def convert_quote(symbol: str, data: dict) -> QuoteResponse:
    return QuoteResponse(
        symbol=symbol, name=data.get("name", ""),
        last=data.get("closePrice", 0.0), open=data.get("openPrice", 0.0),
        high=data.get("highPrice", 0.0), low=data.get("lowPrice", 0.0),
        volume=data.get("total", {}).get("tradeVolume", 0),
        reference_price=data.get("referencePrice", 0.0),
        previous_close=data.get("previousClose", 0.0),
        change=data.get("change", 0.0), change_percent=data.get("changePercent", 0.0),
        bids=data.get("bids", []), asks=data.get("asks", []),
        is_open=data.get("isOpen", False), is_close=data.get("isClose", False),
        timestamp=data.get("lastUpdated", int(time.time() * 1000000)),
        source="fubon"
    )

def _cache_get(symbol: str) -> Optional[dict]:
    with _cache_lock:
        entry = _cache.get(symbol)
        if entry and time.time() - entry["ts"] < CACHE_TTL:
            return entry["data"]
        return None

def _cache_set(symbol: str, data: dict):
    with _cache_lock:
        _cache[symbol] = {"data": data, "ts": time.time()}

@asynccontextmanager
async def lifespan(app: FastAPI):
    logger.info("Starting Fubon MarketData Proxy (SDK deferred)...")
    yield
    global sdk
    if sdk:
        try: sdk.logout(); logger.info("Logged out")
        except Exception: pass

app = FastAPI(title="Fubon MarketData Proxy", version="1.0.0", lifespan=lifespan)

@app.get("/health")
async def health_check():
    return {"status": "alive", "sdk_initialized": sdk is not None,
            "login_time": login_time,
            "session_age": time.time() - login_time if login_time else None}

@app.get("/health/deep")
async def health_check_deep():
    try:
        _, client = get_sdk()
        if client is None: await _init_sdk_async(); _, client = get_sdk()
        result = client.intraday.quote(symbol="0050")
        return {"status": "healthy", "login_time": login_time,
                "session_age": time.time() - login_time if login_time else None}
    except Exception as e:
        logger.error("Deep health check failed: %s", e)
        raise HTTPException(status_code=503, detail=str(e))

async def _ensure_sdk():
    _, client = get_sdk()
    if client is None: await _init_sdk_async(); _, client = get_sdk()
    return client

@app.get("/quote/{symbol}", response_model=QuoteResponse)
async def get_quote(symbol: str):
    try:
        client = await _ensure_sdk()
        result = client.intraday.quote(symbol=symbol)
        data = _unwrap(result)
        if data is None:
            cached = _cache_get(symbol)
            if cached: return QuoteResponse(**cached)
            raise HTTPException(status_code=404, detail=f"No data for {symbol}")
        quote = convert_quote(symbol, data)
        _cache_set(symbol, quote.model_dump())
        return quote
    except HTTPException: raise
    except Exception as e:
        cached = _cache_get(symbol)
        if cached: return QuoteResponse(**cached)
        logger.error("Error quote %s: %s", symbol, e)
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/quotes", response_model=List[QuoteResponse])
async def get_quotes(symbols: str = Query(...)):
    symbol_list = [s.strip() for s in symbols.split(",")]
    try:
        client = await _ensure_sdk()
        quotes = []
        for symbol in symbol_list:
            try:
                result = client.intraday.quote(symbol=symbol)
                data = _unwrap(result)
                if data:
                    q = convert_quote(symbol, data)
                    quotes.append(q)
                    _cache_set(symbol, q.model_dump())
                else:
                    cached = _cache_get(symbol)
                    if cached: quotes.append(QuoteResponse(**cached))
            except Exception as e:
                cached = _cache_get(symbol)
                if cached: quotes.append(QuoteResponse(**cached))
        return quotes
    except Exception as e:
        cached_quotes = [QuoteResponse(**c) for s in symbol_list if (c := _cache_get(s))]
        if cached_quotes: return cached_quotes
        logger.error("Error quotes: %s", e)
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/market-status", response_model=MarketStatusResponse)
async def get_market_status():
    try:
        client = await _ensure_sdk()
        result = client.intraday.quote(symbol="0050")
        data = _unwrap(result)
        if data is None: raise HTTPException(status_code=503, detail="Market data unavailable")
        is_open = data.get("isOpen", False); is_close = data.get("isClose", False)
        status = "closed" if is_close else ("open" if is_open else "unknown")
        return MarketStatusResponse(status=status, is_open=is_open and not is_close, timestamp=int(time.time()))
    except HTTPException: raise
    except Exception as e:
        logger.error("Error market status: %s", e)
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/")
async def root():
    return {"service": "Fubon MarketData Proxy", "version": "1.0.0",
            "endpoints": ["/health", "/health/deep", "/quote/{symbol}", "/quotes", "/market-status"]}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("FUBON_PROXY_PORT", "8081")))
