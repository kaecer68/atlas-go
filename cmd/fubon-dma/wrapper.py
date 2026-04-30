#!/usr/bin/env python3
"""
富邦 DMA 登入/下單 JSONL wrapper

透過 stdin/stdout JSONL 協定與 Go 程序通訊。
每行輸入為一個 JSON 指令，每行輸出為一個 JSON 回應。

指令：
  login         — DMA API Key 登入
  submit_order  — 送出委託單
  cancel_order  — 取消委託單
  query_orders  — 查詢今日委託
  logout        — 登出並結束行程
"""

import json
import sys
import traceback

# 嘗試匯入 fubon_neo SDK；若不存在則以 mock 模式運作
try:
    from fubon_neo.sdk import FubonSDK
    from fubon_neo.constant import (
        BSAction,
        MarketType,
        PriceType,
        TimeInForce,
        OrderType,
    )

    _SDK_AVAILABLE = True
except ImportError:
    _SDK_AVAILABLE = False


def _make_response(status, **kwargs):
    """建立標準 JSONL 回應字典"""
    resp = {"status": status}
    resp.update(kwargs)
    return resp


def _serialize_result(result):
    """將 SDK CustomReturnType 轉為可序列化字典"""
    data = None
    if result.data is not None:
        # data 可能是 Account 列表或 OrderResult
        try:
            # 嘗試迭代（帳號列表）
            data = [str(item) for item in result.data]
        except TypeError:
            data = str(result.data)
    return {
        "is_success": result.is_success,
        "message": result.message,
        "data": data,
    }


def _parse_side(side_str):
    """將字串 side 轉為 BSAction"""
    side_str = side_str.upper()
    if side_str in ("BUY", "B"):
        return BSAction.Buy
    elif side_str in ("SELL", "S"):
        return BSAction.Sell
    raise ValueError(f"不支援的 side: {side_str}")


def _parse_market_type(market_str):
    """將字串 market_type 轉為 MarketType"""
    market_str = market_str.upper()
    mapping = {
        "COMMON": MarketType.Common,
        "ODDLOT": MarketType.OddLot,
        "INBLOCK": MarketType.InBlock,
        "EMERGING": MarketType.Emerging,
        "AUCTION": MarketType.Auction,
    }
    if market_str in mapping:
        return mapping[market_str]
    return MarketType.Common


def _parse_price_type(price_str):
    """將字串 price_type 轉為 PriceType"""
    price_str = price_str.upper()
    mapping = {
        "LIMIT": PriceType.Limit,
        "MARKET": PriceType.Market,
        "LIMIT_UP": PriceType.LimitUp,
        "LIMIT_DOWN": PriceType.LimitDown,
    }
    if price_str in mapping:
        return mapping[price_str]
    return PriceType.Limit


def _parse_time_in_force(tif_str):
    """將字串 time_in_force 轉為 TimeInForce"""
    tif_str = tif_str.upper()
    mapping = {
        "ROD": TimeInForce.ROD,
        "IOC": TimeInForce.IOC,
        "FOK": TimeInForce.FOK,
    }
    if tif_str in mapping:
        return mapping[tif_str]
    return TimeInForce.ROD


def _parse_order_type(ot_str):
    """將字串 order_type 轉為 OrderType"""
    ot_str = ot_str.upper()
    mapping = {
        "STOCK": OrderType.Stock,
        "MARGIN": OrderType.Margin,
        "SHORT": OrderType.Short,
    }
    if ot_str in mapping:
        return mapping[ot_str]
    return OrderType.Stock


def handle_login(sdk, req):
    """處理 login 指令"""
    personal_id = req.get("personal_id", "")
    api_key = req.get("api_key", "")

    if not personal_id or not api_key:
        return _make_response("error", code="MISSING_PARAMS",
                              message="personal_id 和 api_key 為必填")

    try:
        result = sdk.apikey_dma_login(personal_id, api_key)
        serialized = _serialize_result(result)

        if result.is_success:
            # 登入成功，回傳帳號資訊
            accounts = []
            if result.data is not None:
                try:
                    for acct in result.data:
                        accounts.append(str(acct))
                except TypeError:
                    accounts.append(str(result.data))

            return _make_response("ok",
                                  is_success=True,
                                  message=result.message,
                                  accounts=accounts)
        else:
            return _make_response("error",
                                  code="LOGIN_FAILED",
                                  message=result.message or "登入失敗",
                                  is_success=False)
    except Exception as e:
        return _make_response("error", code="LOGIN_EXCEPTION",
                              message=str(e))


def handle_submit_order(sdk, session_data, req):
    """處理 submit_order 指令"""
    if session_data is None:
        return _make_response("error", code="NOT_LOGGED_IN",
                              message="尚未登入，請先執行 login")

    symbol = req.get("symbol", "")
    side = req.get("side", "BUY")
    quantity = req.get("quantity", 0)
    price = req.get("price", None)
    market_type = req.get("market_type", "COMMON")
    price_type = req.get("price_type", "LIMIT")
    time_in_force = req.get("time_in_force", "ROD")
    order_type = req.get("order_type", "STOCK")
    user_def = req.get("user_def", None)

    if not symbol:
        return _make_response("error", code="MISSING_SYMBOL",
                              message="symbol 為必填")
    if quantity <= 0:
        return _make_response("error", code="INVALID_QUANTITY",
                              message="quantity 必須為正整數")

    try:
        order = Order(
            buy_sell=_parse_side(side),
            symbol=symbol,
            quantity=int(quantity),
            market_type=_parse_market_type(market_type),
            price_type=_parse_price_type(price_type),
            time_in_force=_parse_time_in_force(time_in_force),
            order_type=_parse_order_type(order_type),
        )
        if price is not None:
            order.price = str(price)
        if user_def is not None:
            order.user_def = str(user_def)

        # session_data 是登入回傳的帳號列表中的第一個帳號
        account = session_data
        result = sdk.stock.place_order(account, order)

        serialized = _serialize_result(result)
        if result.is_success:
            order_info = serialized.get("data")
            return _make_response("ok",
                                  order_id=str(result.data) if result.data else None,
                                  is_success=True,
                                  message=result.message,
                                  detail=order_info)
        else:
            return _make_response("error",
                                  code="ORDER_FAILED",
                                  message=result.message or "下單失敗",
                                  is_success=False)
    except Exception as e:
        return _make_response("error", code="ORDER_EXCEPTION",
                              message=str(e))


def handle_cancel_order(sdk, session_data, req):
    """處理 cancel_order 指令"""
    if session_data is None:
        return _make_response("error", code="NOT_LOGGED_IN",
                              message="尚未登入，請先執行 login")

    # cancel_order 需要 account 和 order_result 物件
    # 由於 JSONL 協定限制，取消委託需透過 query_orders 取得 order_result 後再取消
    return _make_response("error", code="NOT_IMPLEMENTED",
                          message="cancel_order 需透過 query_orders 取得 order_result 後操作")


def handle_query_orders(sdk, session_data, req):
    """處理 query_orders 指令"""
    if session_data is None:
        return _make_response("error", code="NOT_LOGGED_IN",
                              message="尚未登入，請先執行 login")

    try:
        account = session_data
        result = sdk.stock.get_order_results(account)
        serialized = _serialize_result(result)

        if result.is_success:
            orders = []
            if result.data is not None:
                try:
                    for order_res in result.data:
                        orders.append(str(order_res))
                except TypeError:
                    orders.append(str(result.data))
            return _make_response("ok",
                                  is_success=True,
                                  message=result.message,
                                  orders=orders)
        else:
            return _make_response("error",
                                  code="QUERY_FAILED",
                                  message=result.message or "查詢失敗",
                                  is_success=False)
    except Exception as e:
        return _make_response("error", code="QUERY_EXCEPTION",
                              message=str(e))


def handle_logout(sdk):
    """處理 logout 指令"""
    try:
        sdk.logout()
    except Exception:
        pass
    return _make_response("ok", message="已登出")


def main():
    """主迴圈：從 stdin 讀取 JSONL 指令，回應至 stdout"""
    if not _SDK_AVAILABLE:
        # Mock 模式：SDK 不存在時仍可回應基本指令
        print(json.dumps({"status": "warn", "message": "fubon_neo SDK 未安裝，以 mock 模式運作"}),
              flush=True)

    sdk = FubonSDK() if _SDK_AVAILABLE else None
    session_data = None  # 登入後的帳號物件

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        try:
            req = json.loads(line)
        except json.JSONDecodeError as e:
            print(json.dumps(_make_response("error", code="INVALID_JSON",
                                            message=f"JSON 解析失敗: {e}")),
                  flush=True)
            continue

        cmd = req.get("cmd", "")

        if cmd == "login":
            if not _SDK_AVAILABLE:
                print(json.dumps(_make_response("error", code="SDK_NOT_AVAILABLE",
                                                message="fubon_neo SDK 未安裝")),
                      flush=True)
                continue

            result = handle_login(sdk, req)
            if result.get("status") == "ok" and result.get("is_success"):
                # 登入成功，儲存第一個帳號作為預設交易帳號
                try:
                    login_result = sdk.apikey_dma_login(
                        req.get("personal_id", ""),
                        req.get("api_key", "")
                    )
                    if login_result.data is not None:
                        try:
                            session_data = list(login_result.data)[0]
                        except (TypeError, IndexError):
                            session_data = login_result.data
                except Exception:
                    pass
            print(json.dumps(result, ensure_ascii=False), flush=True)

        elif cmd == "submit_order":
            if not _SDK_AVAILABLE:
                print(json.dumps(_make_response("error", code="SDK_NOT_AVAILABLE",
                                                message="fubon_neo SDK 未安裝")),
                      flush=True)
                continue
            result = handle_submit_order(sdk, session_data, req)
            print(json.dumps(result, ensure_ascii=False), flush=True)

        elif cmd == "cancel_order":
            if not _SDK_AVAILABLE:
                print(json.dumps(_make_response("error", code="SDK_NOT_AVAILABLE",
                                                message="fubon_neo SDK 未安裝")),
                      flush=True)
                continue
            result = handle_cancel_order(sdk, session_data, req)
            print(json.dumps(result, ensure_ascii=False), flush=True)

        elif cmd == "query_orders":
            if not _SDK_AVAILABLE:
                print(json.dumps(_make_response("error", code="SDK_NOT_AVAILABLE",
                                                message="fubon_neo SDK 未安裝")),
                      flush=True)
                continue
            result = handle_query_orders(sdk, session_data, req)
            print(json.dumps(result, ensure_ascii=False), flush=True)

        elif cmd == "logout":
            if _SDK_AVAILABLE and sdk is not None:
                result = handle_logout(sdk)
            else:
                result = _make_response("ok", message="已登出（mock 模式）")
            print(json.dumps(result, ensure_ascii=False), flush=True)
            break

        elif cmd == "ping":
            print(json.dumps(_make_response("ok", message="pong")), flush=True)

        else:
            print(json.dumps(_make_response("error", code="UNKNOWN_CMD",
                                            message=f"未知指令: {cmd}")),
                  flush=True)


if __name__ == "__main__":
    main()