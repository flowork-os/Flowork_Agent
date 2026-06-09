#!/usr/bin/env python3
# core.py — FlowAlpha CORE (Flowork apps platform, runtime:process).
#
# WHITE-LABEL + SOVEREIGN: 100% ORIGINAL code (no upstream copied → no attribution owed).
# Market data = a PUBLIC exchange REST mirror (no key, no account). Indicators + backtest
# are computed here in pure stdlib Python (no numpy, no DB), so FlowAlpha runs anywhere with
# zero config. The closed loop — data → indicator → strategy → backtest → metrics — lives in
# THIS process; the last backtest is shared state both the GUI and agents read.
#
# Protocol: read {"op","args"} per line on stdin, reply {"result", ...} or {"error"} per line.
import sys, json, os, time, math, urllib.request, urllib.parse, urllib.error

# Public market-data endpoint (Binance public DATA mirror — reachable where api.binance.com is
# geo-blocked). Same REST shape. Override via env QUANT_DATA_BASE for any other venue.
DATA_BASE = os.environ.get("QUANT_DATA_BASE", "https://data-api.binance.vision/api/v3")
HTTP_TIMEOUT = 12
STATE_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "state.json")

_symbols_cache = {"at": 0.0, "list": []}
# candles-per-year per interval, for annualising the Sharpe ratio.
_ANN = {"1m": 525600, "5m": 105120, "15m": 35040, "1h": 8760, "4h": 2190, "1d": 365}


# ── data source ───────────────────────────────────────────────────────────────
def _get(path, params=None):
    url = DATA_BASE.rstrip("/") + path
    if params:
        url += "?" + urllib.parse.urlencode(params)
    req = urllib.request.Request(url, headers={"User-Agent": "flowalpha/0.2"})
    with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as r:
        return json.loads(r.read().decode("utf-8"))


def _load_symbols():
    now = time.time()
    if _symbols_cache["list"] and (now - _symbols_cache["at"] < 3600):
        return _symbols_cache["list"]
    info = _get("/exchangeInfo")
    syms = [s["symbol"] for s in info.get("symbols", []) if s.get("status") == "TRADING"]
    _symbols_cache.update(at=now, list=syms)
    return syms


def _klines(symbol, interval, limit):
    rows = _get("/klines", {"symbol": symbol, "interval": interval, "limit": limit})
    return [{"t": int(r[0]), "o": float(r[1]), "h": float(r[2]), "l": float(r[3]),
             "c": float(r[4]), "v": float(r[5])} for r in rows]


# ── indicators (pure python; None during warm-up) ───────────────────────────────
def sma(xs, period):
    out, s = [], 0.0
    for i, x in enumerate(xs):
        s += x
        if i >= period:
            s -= xs[i - period]
        out.append(s / period if i >= period - 1 else None)
    return out


def ema(xs, period):
    out, k, prev = [], 2.0 / (period + 1), None
    for i, x in enumerate(xs):
        if i < period - 1:
            out.append(None)
        elif i == period - 1:
            prev = sum(xs[:period]) / period
            out.append(prev)
        else:
            prev = x * k + prev * (1 - k)
            out.append(prev)
    return out


def rsi(xs, period):
    out = [None] * len(xs)
    if len(xs) <= period:
        return out
    gains = losses = 0.0
    for i in range(1, period + 1):
        d = xs[i] - xs[i - 1]
        gains += max(d, 0.0)
        losses += max(-d, 0.0)
    ag, al = gains / period, losses / period
    out[period] = 100.0 if al == 0 else 100 - 100 / (1 + ag / al)
    for i in range(period + 1, len(xs)):
        d = xs[i] - xs[i - 1]
        ag = (ag * (period - 1) + max(d, 0.0)) / period
        al = (al * (period - 1) + max(-d, 0.0)) / period
        out[i] = 100.0 if al == 0 else 100 - 100 / (1 + ag / al)
    return out


def macd(xs, fast, slow, signal):
    ef, es = ema(xs, fast), ema(xs, slow)
    line = [(ef[i] - es[i]) if (ef[i] is not None and es[i] is not None) else None for i in range(len(xs))]
    vals = [v for v in line if v is not None]
    sig_vals = ema(vals, signal)
    sig, j = [None] * len(xs), 0
    for i in range(len(xs)):
        if line[i] is not None:
            sig[i] = sig_vals[j]
            j += 1
    hist = [(line[i] - sig[i]) if (line[i] is not None and sig[i] is not None) else None for i in range(len(xs))]
    return line, sig, hist


# ── ops ─────────────────────────────────────────────────────────────────────
def op_get_price(a):
    sym = str(a.get("symbol", "")).upper().strip()
    if not sym:
        return {"error": "symbol required"}
    d = _get("/ticker/price", {"symbol": sym})
    return {"result": {"symbol": d["symbol"], "price": float(d["price"]), "source": DATA_BASE}}


def op_get_klines(a):
    sym = str(a.get("symbol", "")).upper().strip()
    if not sym:
        return {"error": "symbol required"}
    interval = str(a.get("interval") or "1h")
    limit = max(1, min(int(a.get("limit") or 200), 500))
    return {"result": {"symbol": sym, "interval": interval, "candles": _klines(sym, interval, limit)}}


def op_search_symbols(a):
    q = str(a.get("query", "")).upper().strip()
    if not q:
        return {"error": "query required"}
    limit = max(1, min(int(a.get("limit") or 25), 100))
    hits = [s for s in _load_symbols() if q in s][:limit]
    return {"result": {"query": q, "count": len(hits), "symbols": hits}}


def op_list_indicators(a):
    return {"result": {"indicators": [
        {"name": "sma", "params": {"period": 20}, "desc": "Simple moving average"},
        {"name": "ema", "params": {"period": 20}, "desc": "Exponential moving average"},
        {"name": "rsi", "params": {"period": 14}, "desc": "Relative strength index (0..100)"},
        {"name": "macd", "params": {"fast": 12, "slow": 26, "signal": 9}, "desc": "MACD line/signal/histogram"},
    ]}}


def op_compute_indicator(a):
    sym = str(a.get("symbol", "")).upper().strip()
    ind = str(a.get("indicator", "")).lower().strip()
    if not sym or not ind:
        return {"error": "symbol and indicator required"}
    interval = str(a.get("interval") or "1h")
    limit = max(1, min(int(a.get("limit") or 200), 500))
    candles = _klines(sym, interval, limit)
    closes = [c["c"] for c in candles]
    times = [c["t"] for c in candles]
    if ind in ("sma", "ema"):
        period = int(a.get("period") or 20)
        series = (sma if ind == "sma" else ema)(closes, period)
        return {"result": {"symbol": sym, "indicator": ind, "period": period,
                           "series": [{"t": times[i], "v": series[i]} for i in range(len(series))]}}
    if ind == "rsi":
        period = int(a.get("period") or 14)
        series = rsi(closes, period)
        return {"result": {"symbol": sym, "indicator": "rsi", "period": period,
                           "series": [{"t": times[i], "v": series[i]} for i in range(len(series))]}}
    if ind == "macd":
        fast, slow, signal = int(a.get("fast") or 12), int(a.get("slow") or 26), int(a.get("signal") or 9)
        line, sig, hist = macd(closes, fast, slow, signal)
        return {"result": {"symbol": sym, "indicator": "macd", "fast": fast, "slow": slow, "signal": signal,
                           "series": [{"t": times[i], "macd": line[i], "signal": sig[i], "hist": hist[i]} for i in range(len(line))]}}
    return {"error": "unknown indicator: " + ind + " (sma|ema|rsi|macd)"}


def _max_drawdown(eq):
    peak, mdd = eq[0], 0.0
    for v in eq:
        peak = max(peak, v)
        mdd = min(mdd, v / peak - 1)
    return mdd * 100


def op_run_backtest(a):
    sym = str(a.get("symbol", "")).upper().strip()
    if not sym:
        return {"error": "symbol required"}
    interval = str(a.get("interval") or "1h")
    limit = max(50, min(int(a.get("limit") or 300), 500))
    fast = max(2, int(a.get("fast") or 10))
    slow = max(fast + 1, int(a.get("slow") or 30))
    fee = float(a.get("fee_bps") or 10) / 10000.0  # round-trip fraction
    candles = _klines(sym, interval, limit)
    closes = [c["c"] for c in candles]
    times = [c["t"] for c in candles]
    if len(closes) < slow + 2:
        return {"error": "not enough candles for slow=%d" % slow}
    fs, ss = sma(closes, fast), sma(closes, slow)

    pos, equity, entry_px, entry_t = 0, 1.0, None, None
    eq_curve, trades, rets = [], [], []
    for i in range(len(closes)):
        if pos == 1 and i > 0:
            r = closes[i] / closes[i - 1] - 1
            equity *= (1 + r)
            rets.append(r)
        if i > 0 and None not in (fs[i], ss[i], fs[i - 1], ss[i - 1]):
            up = fs[i - 1] <= ss[i - 1] and fs[i] > ss[i]
            down = fs[i - 1] >= ss[i - 1] and fs[i] < ss[i]
            if pos == 0 and up:
                pos, entry_px, entry_t = 1, closes[i], times[i]
                equity *= (1 - fee / 2)
            elif pos == 1 and down:
                pos = 0
                equity *= (1 - fee / 2)
                trades.append({"entry_t": entry_t, "entry": entry_px, "exit_t": times[i],
                               "exit": closes[i], "ret_pct": (closes[i] / entry_px - 1) * 100})
                entry_px = None
        eq_curve.append({"t": times[i], "eq": round(equity, 6)})
    if pos == 1 and entry_px:
        trades.append({"entry_t": entry_t, "entry": entry_px, "exit_t": times[-1],
                       "exit": closes[-1], "ret_pct": (closes[-1] / entry_px - 1) * 100, "open": True})

    closed = [t for t in trades if not t.get("open")]
    wins = [t for t in closed if t["ret_pct"] > 0]
    eqs = [p["eq"] for p in eq_curve]
    mean = sum(rets) / len(rets) if rets else 0.0
    var = sum((r - mean) ** 2 for r in rets) / len(rets) if rets else 0.0
    std = math.sqrt(var)
    ann = _ANN.get(interval, 8760)
    sharpe = (mean / std * math.sqrt(ann)) if std > 0 else 0.0
    metrics = {
        "total_return_pct": round((equity - 1) * 100, 2),
        "buy_hold_pct": round((closes[-1] / closes[0] - 1) * 100, 2),
        "win_rate_pct": round(len(wins) / len(closed) * 100, 1) if closed else 0.0,
        "max_drawdown_pct": round(_max_drawdown(eqs), 2),
        "sharpe": round(sharpe, 2),
        "trades": len(closed) + (1 if (trades and trades[-1].get("open")) else 0),
        "candles": len(closes),
    }
    result = {"symbol": sym, "interval": interval, "strategy": "sma_cross",
              "params": {"fast": fast, "slow": slow, "fee_bps": round(fee * 10000, 2)},
              "metrics": metrics, "equity": eq_curve, "trades": trades,
              "candles": [{"t": c["t"], "c": c["c"]} for c in candles]}
    _save_state({"last_backtest": result, "at": int(time.time())})
    return {"result": result}


def op_get_last_backtest(a):
    st = _load_state()
    return {"result": st.get("last_backtest") or {}}


# ── shared state (last backtest) ────────────────────────────────────────────
def _load_state():
    try:
        with open(STATE_FILE, "r", encoding="utf-8") as f:
            return json.load(f)
    except Exception:  # noqa
        return {}


def _save_state(d):
    try:
        tmp = STATE_FILE + ".tmp"
        with open(tmp, "w", encoding="utf-8") as f:
            json.dump(d, f)
        os.replace(tmp, STATE_FILE)
    except Exception:  # noqa
        pass


HANDLERS = {
    "get_price": op_get_price, "get_klines": op_get_klines, "search_symbols": op_search_symbols,
    "list_indicators": op_list_indicators, "compute_indicator": op_compute_indicator,
    "run_backtest": op_run_backtest, "get_last_backtest": op_get_last_backtest,
}


def handle(op, args):
    fn = HANDLERS.get(op)
    if not fn:
        return {"error": "unknown op: " + str(op)}
    try:
        return fn(args)
    except urllib.error.HTTPError as e:
        return {"error": "data source %d: %s" % (e.code, e.reason)}
    except Exception as e:  # noqa
        return {"error": str(e)}


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
            out = handle(req.get("op", ""), req.get("args") or {})
        except Exception as e:  # noqa
            out = {"error": str(e)}
        sys.stdout.write(json.dumps(out) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
