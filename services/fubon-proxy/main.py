"""
Fubon MarketData Proxy Service
使用富邦新一代 API Python SDK 提供行情数据 REST API
"""
import os
import time
import json
import logging
from typing import List, Optional
from contextlib import asynccontextmanager

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


def get_sdk():
    """获取或创建SDK实例"""
    global sdk, rest_client, login_time
    
    # 检查是否需要重新登录
    if sdk is not None and login_time is not None:
        if time.time() - login_time < SESSION_TIMEOUT:
            return sdk, rest_client
        else:
            logger.info("Session expired, re-logging in...")
    
    # 创建新SDK实例
    from fubon_neo.sdk import FubonSDK
    
    personal_id = os.getenv("FUBON_PERSONAL_ID")
    api_key = os.getenv("FUBON_API_KEY")
    
    if not personal_id or not api_key:
        raise ValueError("FUBON_PERSONAL_ID and FUBON_API_KEY must be set")
    
    try:
        sdk = FubonSDK()
        
        # 使用 DMA 模式登录（无需凭证文件）
        result = sdk.apikey_dma_login(personal_id, api_key)
        
        if not result.is_success:
            raise Exception(f"Login failed: {result.message}")
        
        logger.info(f"Login successful: {result.data}")
        
        # 初始化实时行情
        sdk.init_realtime()
        
        # 获取 REST client
        rest_client = sdk.marketdata.rest_client.stock
        
        login_time = time.time()
        
        return sdk, rest_client
        
    except Exception as e:
        logger.error(f"SDK initialization failed: {e}")
        sdk = None
        rest_client = None
        raise


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
    """
    获取个股实时行情
    
    - symbol: 股票代码，例如 2330, 0050
    """
    try:
        _, client = get_sdk()
        result = client.intraday.quote(symbol=symbol)
        
        if not result or not result.data:
            raise HTTPException(status_code=404, detail=f"No data for symbol {symbol}")
        
        data = result.data
        return convert_quote(symbol, data)
        
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting quote for {symbol}: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/quotes", response_model=List[QuoteResponse])
async def get_quotes(symbols: str = Query(..., description="股票代码，逗号分隔，例如: 2330,2317,0050")):
    """
    批量获取个股实时行情
    
    - symbols: 逗号分隔的股票代码列表
    """
    symbol_list = [s.strip() for s in symbols.split(",")]
    
    try:
        _, client = get_sdk()
        quotes = []
        
        for symbol in symbol_list:
            try:
                result = client.intraday.quote(symbol=symbol)
                if result and result.data:
                    quotes.append(convert_quote(symbol, result.data))
            except Exception as e:
                logger.error(f"Error getting quote for {symbol}: {e}")
                # 继续获取其他股票
        
        return quotes
        
    except Exception as e:
        logger.error(f"Error getting quotes: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/market-status", response_model=MarketStatusResponse)
async def get_market_status():
    """获取市场状态"""
    try:
        _, client = get_sdk()
        # 使用 0050 作为市场状态指示器
        result = client.intraday.quote(symbol="0050")
        
        if not result or not result.data:
            raise HTTPException(status_code=503, detail="Market data unavailable")
        
        data = result.data
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
