"""
Fubon MarketData Proxy Service
使用富邦新一代 API Python SDK 提供行情数据 REST API
"""
import os
import sys
import glob
import time
import json
import logging
from typing import List, Optional
from contextlib import asynccontextmanager
from pathlib import Path

# 自動載入 ~/.config/atlas-go/.env
_env_path = Path.home() / ".config" / "atlas-go" / ".env"
if _env_path.exists():
    try:
        from dotenv import load_dotenv
        load_dotenv(str(_env_path))
        logger = logging.getLogger(__name__)
        logger.info(f"Loaded environment from {_env_path}")
    except ImportError:
        pass

from fastapi import FastAPI, HTTPException, Query
from fastapi.responses import JSONResponse
from pydantic import BaseModel

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# 全局状态
sdk = None
rest_client = None
login_time = None
SESSION_TIMEOUT = 3600  # 1小时重新登录


class QuoteResponse(BaseModel):
    """行情响应"""
    symbol: str
    name: str
    last: float
    open: float
    high: float
    low: float
    volume: int
    reference_price: float
    previous_close: float
    change: float
    change_percent: float
    bids: List[dict]
    asks: List[dict]
    is_open: bool
    is_close: bool
    timestamp: int
    source: str = "fubon"


class MarketStatusResponse(BaseModel):
    """市场状态响应"""
    status: str
    is_open: bool
    timestamp: int


def _find_cert():
    """Auto-detect newest .p12 certificate, preferring the new one."""
    # 1) env var if it exists
    cert_path = os.getenv("FUBON_CERT_PATH")
    if cert_path and os.path.exists(cert_path):
        return cert_path

    # 2) search known locations, newest first
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

    return cert_path  # may be None


def get_sdk():
    """Get or create SDK instance; first login uses password+cert, subsequent calls reuse token."""
    global sdk, rest_client, login_time

    # Reuse existing session
    if sdk is not None and login_time is not None:
        if time.time() - login_time < SESSION_TIMEOUT:
            return sdk, rest_client
        logger.info("Session expired, re-logging in...")

    from fubon_neo.sdk import FubonSDK

    personal_id = os.getenv("FUBON_PERSONAL_ID")
    password = os.getenv("FUBON_PASSWORD")
    cert_path = _find_cert()
    cert_password = os.getenv("FUBON_CERT_PASSWORD")

    if not personal_id or not password:
        raise ValueError("FUBON_PERSONAL_ID and FUBON_PASSWORD must be set")
    if not cert_path:
        raise ValueError("Certificate not found – set FUBON_CERT_PATH or place .p12 under ~/.config/atlas-go/.fubon-env/")

    try:
        sdk = FubonSDK()

        if cert_password:
            result = sdk.login(personal_id, password, cert_path, cert_password)
        else:
            result = sdk.login(personal_id, password, cert_path)

        if not result.is_success:
            raise Exception(f"Login failed: {result.message}")

        logger.info(f"Login successful: {result.data}")

        sdk.init_realtime()
        rest_client = sdk.marketdata.rest_client.stock
        login_time = time.time()

        return sdk, rest_client

    except Exception as e:
        logger.error(f"SDK initialization failed: {e}")
        sdk = None
        rest_client = None
        raise


def _unwrap(result):
    """Handle both SDK CustomReturnType (has .data) and plain dict returns."""
    if result is None:
        return None
    if hasattr(result, "data"):
        return result.data
    if isinstance(result, dict):
        return result
    return None


def convert_quote(symbol: str, data: dict) -> QuoteResponse:
    """转换富邦行情数据为标准格式"""
    return QuoteResponse(
        symbol=symbol,
        name=data.get("name", ""),
        last=data.get("closePrice", 0.0),
        open=data.get("openPrice", 0.0),
        high=data.get("highPrice", 0.0),
        low=data.get("lowPrice", 0.0),
        volume=data.get("total", {}).get("tradeVolume", 0),
        reference_price=data.get("referencePrice", 0.0),
        previous_close=data.get("previousClose", 0.0),
        change=data.get("change", 0.0),
        change_percent=data.get("changePercent", 0.0),
        bids=data.get("bids", []),
        asks=data.get("asks", []),
        is_open=data.get("isOpen", False),
        is_close=data.get("isClose", False),
        timestamp=data.get("lastUpdated", int(time.time() * 1000000)),
        source="fubon"
    )


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期管理"""
    logger.info("Starting Fubon MarketData Proxy...")
    
    try:
        get_sdk()
        logger.info("SDK initialized successfully")
    except Exception as e:
        logger.error(f"Failed to initialize SDK: {e}")
        # 不阻断启动，首次请求时再尝试
    
    yield
    
    # 关闭时登出
    global sdk
    if sdk:
        try:
            sdk.logout()
            logger.info("Logged out successfully")
        except:
            pass


app = FastAPI(
    title="Fubon MarketData Proxy",
    description="富邦新一代 API 行情数据代理服务",
    version="1.0.0",
    lifespan=lifespan
)


@app.get("/health")
async def health_check():
    """健康检查"""
    try:
        _, client = get_sdk()
        # 尝试获取 0050 测试连接
        result = client.intraday.quote(symbol="0050")
        return {
            "status": "healthy",
            "login_time": login_time,
            "session_age": time.time() - login_time if login_time else None
        }
    except Exception as e:
        logger.error(f"Health check failed: {e}")
        raise HTTPException(status_code=503, detail=str(e))


@app.get("/quote/{symbol}", response_model=QuoteResponse)
async def get_quote(symbol: str):
    try:
        _, client = get_sdk()
        result = client.intraday.quote(symbol=symbol)
        data = _unwrap(result)
        if data is None:
            raise HTTPException(status_code=404, detail=f"No data for symbol {symbol}")
        return convert_quote(symbol, data)
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting quote for {symbol}: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/quotes", response_model=List[QuoteResponse])
async def get_quotes(symbols: str = Query(..., description="股票代码，逗号分隔，例如: 2330,2317,0050")):
    symbol_list = [s.strip() for s in symbols.split(",")]
    try:
        _, client = get_sdk()
        quotes = []
        for symbol in symbol_list:
            try:
                result = client.intraday.quote(symbol=symbol)
                data = _unwrap(result)
                if data:
                    quotes.append(convert_quote(symbol, data))
            except Exception as e:
                logger.error(f"Error getting quote for {symbol}: {e}")
        return quotes
    except Exception as e:
        logger.error(f"Error getting quotes: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/market-status", response_model=MarketStatusResponse)
async def get_market_status():
    try:
        _, client = get_sdk()
        result = client.intraday.quote(symbol="0050")
        data = _unwrap(result)
        if data is None:
            raise HTTPException(status_code=503, detail="Market data unavailable")
        is_open = data.get("isOpen", False)
        is_close = data.get("isClose", False)
        if is_close:
            status = "closed"
        elif is_open:
            status = "open"
        else:
            status = "unknown"
        return MarketStatusResponse(
            status=status,
            is_open=is_open and not is_close,
            timestamp=int(time.time())
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting market status: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/")
async def root():
    """根路径"""
    return {
        "service": "Fubon MarketData Proxy",
        "version": "1.0.0",
        "endpoints": [
            "/health",
            "/quote/{symbol}",
            "/quotes?symbols=2330,2317",
            "/market-status"
        ]
    }


if __name__ == "__main__":
    import uvicorn
    port = int(os.getenv("FUBON_PROXY_PORT", "8081"))
    uvicorn.run(app, host="0.0.0.0", port=port)
