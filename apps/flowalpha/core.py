#!/usr/bin/env python3
# core.py — FlowAlpha CORE (Flowork apps platform, runtime:process).
#
# WHITE-LABEL + SOVEREIGN: 100% ORIGINAL code (no upstream copied → no attribution owed).
# Market data = a PUBLIC exchange REST mirror (no key, no account). Indicators, strategies,
# backtest and parameter optimization run here in pure stdlib Python (no numpy, no DB), so
# FlowAlpha runs anywhere with zero config. The closed loop — data → indicator → strategy →
# backtest → optimize → metrics — lives in THIS process; the last backtest is shared state
# both the GUI and agents read.
#
# Protocol: read {"op","args"} per line on stdin, reply {"result", ...} or {"error"} per line.
import sys, json, os, time, math, itertools, urllib.request, urllib.parse, urllib.error

DATA_BASE = os.environ.get("QUANT_DATA_BASE", "https://data-api.binance.vision/api/v3")
HTTP_TIMEOUT = 12
STATE_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "state.json")
MAX_OPT_COMBOS = 60

_symbols_cache = {"at": 0.0, "list": []}
_ANN = {"1m": 525600, "5m": 105120, "15m": 35040, "1h": 8760, "4h": 2190, "1d": 365}


# ── data source ───────────────────────────────────────────────────────────────
def _get(path, params=None):
    url = DATA_BASE.rstrip("/") + path
    if params:
        url += "?" + urllib.parse.urlencode(params)
    req = urllib.request.Request(url, headers={"User-Agent": "flowalpha/0.3"})
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


# ── strategies → a target-position series (0 flat / 1 long; None during warm-up) ──
# Each strategy is a pure function of closes + params. The simulator turns position
# transitions into trades, so adding a strategy never touches the backtest engine.
STRATEGIES = {
    "sma_cross": {"params": {"fast": 10, "slow": 30}, "desc": "Long when fast SMA > slow SMA"},
    "ema_cross": {"params": {"fast": 12, "slow": 26}, "desc": "Long when fast EMA > slow EMA"},
    "rsi_threshold": {"params": {"period": 14, "buy_below": 30, "sell_above": 70}, "desc": "Long when RSI dips below buy_below, exit above sell_above"},
    "macd_cross": {"params": {"fast": 12, "slow": 26, "signal": 9}, "desc": "Long when MACD line > signal line"},
}


def _positions(closes, strategy, p):
    n = len(closes)
    pos = [None] * n
    if strategy == "sma_cross":
        fast, slow = int(p.get("fast", 10)), int(p.get("slow", 30))
        a, b = sma(closes, fast), sma(closes, slow)
        for i in range(n):
            pos[i] = None if (a[i] is None or b[i] is None) else (1 if a[i] > b[i] else 0)
    elif strategy == "ema_cross":
        fast, slow = int(p.get("fast", 12)), int(p.get("slow", 26))
        a, b = ema(closes, fast), ema(closes, slow)
        for i in range(n):
            pos[i] = None if (a[i] is None or b[i] is None) else (1 if a[i] > b[i] else 0)
    elif strategy == "rsi_threshold":
        period = int(p.get("period", 14))
        buy, sell = float(p.get("buy_below", 30)), float(p.get("sell_above", 70))
        r, cur = rsi(closes, period), 0
        for i in range(n):
            if r[i] is None:
                pos[i] = None
                continue
            if cur == 0 and r[i] < buy:
                cur = 1
            elif cur == 1 and r[i] > sell:
                cur = 0
            pos[i] = cur
    elif strategy == "macd_cross":
        line, sig, _ = macd(closes, int(p.get("fast", 12)), int(p.get("slow", 26)), int(p.get("signal", 9)))
        for i in range(n):
            pos[i] = None if (line[i] is None or sig[i] is None) else (1 if line[i] > sig[i] else 0)
    else:
        return None
    return pos


# ── backtest engine ─────────────────────────────────────────────────────────
def _simulate(closes, times, pos, fee):
    cur, equity, entry_px, entry_t = 0, 1.0, None, None
    eq_curve, trades, rets = [], [], []
    for i in range(len(closes)):
        if cur == 1 and i > 0:
            r = closes[i] / closes[i - 1] - 1
            equity *= (1 + r)
            rets.append(r)
        t = pos[i]
        if t is not None and i > 0 and t != cur:
            if cur == 0 and t == 1:
                cur, entry_px, entry_t = 1, closes[i], times[i]
                equity *= (1 - fee / 2)
            elif cur == 1 and t == 0:
                cur = 0
                equity *= (1 - fee / 2)
                trades.append({"entry_t": entry_t, "entry": entry_px, "exit_t": times[i],
                               "exit": closes[i], "ret_pct": (closes[i] / entry_px - 1) * 100})
                entry_px = None
        eq_curve.append({"t": times[i], "eq": round(equity, 6)})
    if cur == 1 and entry_px:
        trades.append({"entry_t": entry_t, "entry": entry_px, "exit_t": times[-1],
                       "exit": closes[-1], "ret_pct": (closes[-1] / entry_px - 1) * 100, "open": True})
    return equity, eq_curve, trades, rets


def _max_drawdown(eq):
    peak, mdd = eq[0], 0.0
    for v in eq:
        peak = max(peak, v)
        mdd = min(mdd, v / peak - 1)
    return mdd * 100


def _metrics(equity, closes, eq_curve, trades, rets, interval):
    closed = [t for t in trades if not t.get("open")]
    wins = [t for t in closed if t["ret_pct"] > 0]
    eqs = [p["eq"] for p in eq_curve]
    mean = sum(rets) / len(rets) if rets else 0.0
    var = sum((r - mean) ** 2 for r in rets) / len(rets) if rets else 0.0
    std = math.sqrt(var)
    sharpe = (mean / std * math.sqrt(_ANN.get(interval, 8760))) if std > 0 else 0.0
    return {
        "total_return_pct": round((equity - 1) * 100, 2),
        "buy_hold_pct": round((closes[-1] / closes[0] - 1) * 100, 2),
        "win_rate_pct": round(len(wins) / len(closed) * 100, 1) if closed else 0.0,
        "max_drawdown_pct": round(_max_drawdown(eqs), 2),
        "sharpe": round(sharpe, 2),
        "trades": len(closed) + (1 if (trades and trades[-1].get("open")) else 0),
        "candles": len(closes),
    }


# ── ops: data ─────────────────────────────────────────────────────────────────
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


# ── ops: indicators ───────────────────────────────────────────────────────────
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


# ── ops: strategies / backtest / optimize ──────────────────────────────────────
def op_list_strategies(a):
    return {"result": {"strategies": [{"name": k, "params": v["params"], "desc": v["desc"]} for k, v in STRATEGIES.items()]}}


def _run(symbol, interval, limit, strategy, params, fee):
    candles = _klines(symbol, interval, limit)
    closes = [c["c"] for c in candles]
    times = [c["t"] for c in candles]
    pos = _positions(closes, strategy, params)
    if pos is None:
        return None, "unknown strategy: " + strategy
    equity, eq_curve, trades, rets = _simulate(closes, times, pos, fee)
    metrics = _metrics(equity, closes, eq_curve, trades, rets, interval)
    return {"symbol": symbol, "interval": interval, "strategy": strategy, "params": params,
            "metrics": metrics, "equity": eq_curve, "trades": trades,
            "candles": [{"t": c["t"], "c": c["c"]} for c in candles]}, None


def op_run_backtest(a):
    sym = str(a.get("symbol", "")).upper().strip()
    if not sym:
        return {"error": "symbol required"}
    strategy = str(a.get("strategy") or "sma_cross")
    if strategy not in STRATEGIES:
        return {"error": "unknown strategy: %s (%s)" % (strategy, "|".join(STRATEGIES))}
    interval = str(a.get("interval") or "1h")
    limit = max(50, min(int(a.get("limit") or 300), 500))
    fee = float(a.get("fee_bps") or 10) / 10000.0
    params = dict(STRATEGIES[strategy]["params"])
    for k in params:
        if a.get(k) is not None:
            params[k] = a.get(k)
    res, err = _run(sym, interval, limit, strategy, params, fee)
    if err:
        return {"error": err}
    res["params"]["fee_bps"] = round(fee * 10000, 2)
    _save_state({"last_backtest": res, "at": int(time.time())})
    return {"result": res}


def _grid(strategy, custom):
    """Yield param dicts for a sweep. custom = {param: [values]} overrides defaults."""
    defaults = {
        "sma_cross": {"fast": [5, 10, 20], "slow": [30, 50, 100]},
        "ema_cross": {"fast": [8, 12, 20], "slow": [21, 26, 50]},
        "rsi_threshold": {"period": [14], "buy_below": [20, 30], "sell_above": [70, 80]},
        "macd_cross": {"fast": [8, 12], "slow": [21, 26], "signal": [9]},
    }.get(strategy, {})
    space = custom if isinstance(custom, dict) and custom else defaults
    keys = list(space.keys())
    for combo in itertools.product(*[space[k] for k in keys]):
        p = dict(zip(keys, combo))
        # skip invalid fast>=slow combos
        if "fast" in p and "slow" in p and p["fast"] >= p["slow"]:
            continue
        yield p


def op_run_optimize(a):
    sym = str(a.get("symbol", "")).upper().strip()
    if not sym:
        return {"error": "symbol required"}
    strategy = str(a.get("strategy") or "sma_cross")
    if strategy not in STRATEGIES:
        return {"error": "unknown strategy: %s" % strategy}
    interval = str(a.get("interval") or "1h")
    limit = max(50, min(int(a.get("limit") or 300), 500))
    fee = float(a.get("fee_bps") or 10) / 10000.0
    rank_by = str(a.get("rank_by") or "total_return_pct")
    candles = _klines(sym, interval, limit)
    closes = [c["c"] for c in candles]
    times = [c["t"] for c in candles]
    results = []
    for params in itertools.islice(_grid(strategy, a.get("grid")), MAX_OPT_COMBOS):
        pos = _positions(closes, strategy, params)
        if pos is None:
            continue
        equity, eq, trades, rets = _simulate(closes, times, pos, fee)
        m = _metrics(equity, closes, eq, trades, rets, interval)
        results.append({"params": params, "metrics": m})
    if not results:
        return {"error": "no valid parameter combos"}
    results.sort(key=lambda r: r["metrics"].get(rank_by, -1e9), reverse=True)
    best = results[0]
    return {"result": {"symbol": sym, "interval": interval, "strategy": strategy, "rank_by": rank_by,
                       "tested": len(results), "best": best, "buy_hold_pct": round((closes[-1] / closes[0] - 1) * 100, 2),
                       "results": results[:15]}}


def op_get_last_backtest(a):
    return {"result": _load_state().get("last_backtest") or {}}


# ── shared state ────────────────────────────────────────────────────────────
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
    "list_strategies": op_list_strategies, "run_backtest": op_run_backtest,
    "run_optimize": op_run_optimize, "get_last_backtest": op_get_last_backtest,
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
