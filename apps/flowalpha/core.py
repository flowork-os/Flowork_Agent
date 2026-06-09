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
import sys, json, os, time, math, ast, itertools, urllib.request, urllib.parse, urllib.error

DATA_BASE = os.environ.get("QUANT_DATA_BASE", "https://data-api.binance.vision/api/v3")
HTTP_TIMEOUT = 12
# AI runs through the Flowork router (sovereign, OpenAI-compatible) — NOT a third-party LLM key.
# Model is left to the router's default unless QUANT_LLM_MODEL is set (brand-neutral: no vendor
# model name baked in). Override the endpoint with FLOWORK_ROUTER_URL.
LLM_URL = os.environ.get("FLOWORK_ROUTER_URL", "http://127.0.0.1:2402/v1/chat/completions")
LLM_MODEL = os.environ.get("QUANT_LLM_MODEL", "")
LLM_TIMEOUT = 30
# Paper portfolio: virtual cash, no broker, no real money (live trading is a separate,
# owner-gated phase). Starting cash + per-order fee are configurable.
STARTING_CASH = float(os.environ.get("QUANT_PAPER_CASH", "10000"))
PAPER_FEE = float(os.environ.get("QUANT_PAPER_FEE_BPS", "10")) / 10000.0
# Live (real-money) trading is OWNER-GATED. It is DISABLED unless the OWNER sets this env on the
# HOST — an agent can never enable it (same doctrine as FLOWORK_POWER_ARMED). Even when armed, a
# real broker connector (with the owner's own keys) must be configured to route real orders;
# none is bundled, so real money is never at risk by default.
LIVE_ENABLED = os.environ.get("FLOWALPHA_LIVE_ENABLED", "").strip().lower() in ("1", "true", "yes", "on")
# Notifications: FlowAlpha does NOT send Telegram itself (the app is sandboxed, holds no token).
# It EMITS fired alerts/bot-actions to a webhook that the agent handling Telegram listens on
# (a Flowork trigger of type "webhook" with deliver=telegram → mr-flow relays → owner). Sovereign
# + plug-and-play. Config from env (QUANT_NOTIFY_URL + QUANT_NOTIFY_SECRET) OR, if unset, from
# ~/.flowork/flowalpha-notify.json {"url","secret"}. Empty = notifications off. Payload={"text"}.
def _notify_config():
    url = os.environ.get("QUANT_NOTIFY_URL", "").strip()
    secret = os.environ.get("QUANT_NOTIFY_SECRET", "").strip()
    if not url:
        try:
            with open(os.path.expanduser("~/.flowork/flowalpha-notify.json")) as f:
                cfg = json.load(f)
            url = str(cfg.get("url", "")).strip()
            secret = str(cfg.get("secret", "")).strip()
        except Exception:  # noqa
            pass
    return url, secret


def _notify(text):
    url, secret = _notify_config()
    if not url:
        return
    try:
        headers = {"Content-Type": "application/json"}
        if secret:
            headers["X-Flowork-Key"] = secret
        req = urllib.request.Request(url, data=json.dumps({"text": text}).encode("utf-8"), headers=headers)
        urllib.request.urlopen(req, timeout=6).read()
    except Exception:  # noqa
        pass
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


# ── multi-asset routing: crypto → Binance public mirror, stocks/forex/index → Yahoo ──
# Sovereign + key-free for ALL asset classes. Because every indicator/backtest/AI op goes
# through _klines/_last_price, routing here makes the whole engine multi-asset for free.
_CRYPTO_QUOTES = ("USDT", "BUSD", "USDC", "FDUSD", "TUSD", "DAI")
_YF = {"1m": ("1m", "5d"), "5m": ("5m", "1mo"), "15m": ("15m", "1mo"),
       "1h": ("60m", "3mo"), "4h": ("60m", "6mo"), "1d": ("1d", "2y")}


def _is_crypto(sym):
    s = sym.upper()
    if any(ch in s for ch in ("=", ".", "^", "-")):
        return False  # EURUSD=X, BBCA.JK, ^GSPC, BTC-USD → Yahoo
    return any(s.endswith(q) for q in _CRYPTO_QUOTES)


def _binance_klines(symbol, interval, limit):
    rows = _get("/klines", {"symbol": symbol, "interval": interval, "limit": limit})
    return [{"t": int(r[0]), "o": float(r[1]), "h": float(r[2]), "l": float(r[3]),
             "c": float(r[4]), "v": float(r[5])} for r in rows]


def _yahoo(symbol, interval, rng):
    url = "https://query1.finance.yahoo.com/v8/finance/chart/" + urllib.parse.quote(symbol) + "?interval=" + interval + "&range=" + rng
    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as r:
        return json.loads(r.read().decode("utf-8"))["chart"]["result"][0]


def _yahoo_klines(symbol, interval, limit):
    yi, rng = _YF.get(interval, ("1d", "2y"))
    res = _yahoo(symbol, yi, rng)
    ts = res.get("timestamp") or []
    q = res["indicators"]["quote"][0]
    out = []
    for i in range(len(ts)):
        o, h, l, c, v = q["open"][i], q["high"][i], q["low"][i], q["close"][i], q["volume"][i]
        if None in (o, h, l, c):
            continue
        out.append({"t": ts[i] * 1000, "o": float(o), "h": float(h), "l": float(l), "c": float(c), "v": float(v or 0)})
    return out[-limit:]


def _klines(symbol, interval, limit):
    return _binance_klines(symbol, interval, limit) if _is_crypto(symbol) else _yahoo_klines(symbol, interval, limit)


def _last_price(symbol):
    if _is_crypto(symbol):
        return float(_get("/ticker/price", {"symbol": symbol})["price"])
    res = _yahoo(symbol, "1d", "5d")
    mp = (res.get("meta") or {}).get("regularMarketPrice")
    if mp is not None:
        return float(mp)
    cl = [c for c in res["indicators"]["quote"][0]["close"] if c is not None]
    return float(cl[-1])


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


def stdev(xs, period):
    out = []
    for i in range(len(xs)):
        if i < period - 1:
            out.append(None)
            continue
        w = xs[i - period + 1:i + 1]
        m = sum(w) / period
        out.append(math.sqrt(sum((x - m) ** 2 for x in w) / period))
    return out


def bollinger(closes, period, k):
    mid, sd = sma(closes, period), stdev(closes, period)
    up = [(mid[i] + k * sd[i]) if (mid[i] is not None and sd[i] is not None) else None for i in range(len(closes))]
    lo = [(mid[i] - k * sd[i]) if (mid[i] is not None and sd[i] is not None) else None for i in range(len(closes))]
    return mid, up, lo


def atr(highs, lows, closes, period):
    n = len(closes)
    tr = [highs[0] - lows[0]] + [max(highs[i] - lows[i], abs(highs[i] - closes[i - 1]), abs(lows[i] - closes[i - 1])) for i in range(1, n)]
    out = [None] * n
    if n <= period:
        return out
    a = sum(tr[1:period + 1]) / period
    out[period] = a
    for i in range(period + 1, n):
        a = (a * (period - 1) + tr[i]) / period
        out[i] = a
    return out


def obv(closes, volumes):
    out = [0.0] * len(closes)
    for i in range(1, len(closes)):
        out[i] = out[i - 1] + (volumes[i] if closes[i] > closes[i - 1] else -volumes[i] if closes[i] < closes[i - 1] else 0)
    return out


def stoch(highs, lows, closes, period, d_period):
    n = len(closes)
    k = [None] * n
    for i in range(period - 1, n):
        hh, ll = max(highs[i - period + 1:i + 1]), min(lows[i - period + 1:i + 1])
        k[i] = 100 * (closes[i] - ll) / (hh - ll) if hh != ll else 50.0
    kvals = [v for v in k if v is not None]
    dsma = sma(kvals, d_period)
    d, j = [None] * n, 0
    for i in range(n):
        if k[i] is not None:
            d[i] = dsma[j]
            j += 1
    return k, d


def vwap(highs, lows, closes, vols):
    out, cumpv, cumv = [], 0.0, 0.0
    for i in range(len(closes)):
        tp = (highs[i] + lows[i] + closes[i]) / 3
        cumpv += tp * vols[i]
        cumv += vols[i]
        out.append(cumpv / cumv if cumv > 0 else None)
    return out


def _wilder(xs, period):
    out = [None] * len(xs)
    if len(xs) < period:
        return out
    s = sum(xs[:period])
    out[period - 1] = s / period
    for i in range(period, len(xs)):
        s = s - (s / period) + xs[i]
        out[i] = s / period
    return out


def adx(highs, lows, closes, period):
    n = len(closes)
    tr = [highs[0] - lows[0]] + [max(highs[i] - lows[i], abs(highs[i] - closes[i - 1]), abs(lows[i] - closes[i - 1])) for i in range(1, n)]
    pdm = [0.0] + [max(highs[i] - highs[i - 1], 0) if (highs[i] - highs[i - 1]) > (lows[i - 1] - lows[i]) else 0.0 for i in range(1, n)]
    ndm = [0.0] + [max(lows[i - 1] - lows[i], 0) if (lows[i - 1] - lows[i]) > (highs[i] - highs[i - 1]) else 0.0 for i in range(1, n)]
    atrw, pdmw, ndmw = _wilder(tr, period), _wilder(pdm, period), _wilder(ndm, period)
    pdi = [100 * pdmw[i] / atrw[i] if (atrw[i] and atrw[i] > 0) else None for i in range(n)]
    ndi = [100 * ndmw[i] / atrw[i] if (atrw[i] and atrw[i] > 0) else None for i in range(n)]
    dx = [(100 * abs(pdi[i] - ndi[i]) / (pdi[i] + ndi[i])) if (pdi[i] is not None and ndi[i] is not None and (pdi[i] + ndi[i]) > 0) else None for i in range(n)]
    dxv = [v for v in dx if v is not None]
    adxw = _wilder(dxv, period)
    out, j = [None] * n, 0
    for i in range(n):
        if dx[i] is not None:
            out[i] = adxw[j]
            j += 1
    return out, pdi, ndi


def supertrend(highs, lows, closes, period, mult):
    n = len(closes)
    a = atr(highs, lows, closes, period)
    st, dir_ = [None] * n, [None] * n
    fub = flb = None
    for i in range(n):
        if a[i] is None:
            continue
        mid = (highs[i] + lows[i]) / 2
        bub, blb = mid + mult * a[i], mid - mult * a[i]
        fub = bub if (fub is None or bub < fub or closes[i - 1] > fub) else fub
        flb = blb if (flb is None or blb > flb or closes[i - 1] < flb) else flb
        if st[i - 1] is None:
            dir_[i] = 1 if closes[i] >= mid else -1
        elif st[i - 1] == fub:
            dir_[i] = -1 if closes[i] <= fub else 1
        else:
            dir_[i] = 1 if closes[i] >= flb else -1
        st[i] = flb if dir_[i] == 1 else fub
    return st, dir_


# ── strategies → a target-position series (0 flat / 1 long; None during warm-up) ──
# Each strategy is a pure function of closes + params. The simulator turns position
# transitions into trades, so adding a strategy never touches the backtest engine.
STRATEGIES = {
    "sma_cross": {"params": {"fast": 10, "slow": 30}, "desc": "Long when fast SMA > slow SMA"},
    "ema_cross": {"params": {"fast": 12, "slow": 26}, "desc": "Long when fast EMA > slow EMA"},
    "rsi_threshold": {"params": {"period": 14, "buy_below": 30, "sell_above": 70}, "desc": "Long when RSI dips below buy_below, exit above sell_above"},
    "macd_cross": {"params": {"fast": 12, "slow": 26, "signal": 9}, "desc": "Long when MACD line > signal line"},
    "macd_zero": {"params": {"fast": 12, "slow": 26, "signal": 9}, "desc": "Long when the MACD line is above zero"},
    "bollinger_bounce": {"params": {"period": 20, "k": 2}, "desc": "Long when price dips below the lower Bollinger band, exit above the middle"},
    "triple_ma": {"params": {"fast": 8, "mid": 21, "slow": 55}, "desc": "Long when fast EMA > mid EMA > slow EMA (trend stack)"},
    "supertrend": {"params": {"period": 10, "mult": 3}, "desc": "Long when SuperTrend direction is up (ATR trend-following)"},
}


def _positions(candles, strategy, p):
    closes = [c["c"] for c in candles]
    highs = [c["h"] for c in candles]
    lows = [c["l"] for c in candles]
    n = len(closes)
    pos = [None] * n
    if strategy == "supertrend":
        _st, dr = supertrend(highs, lows, closes, int(p.get("period", 10)), float(p.get("mult", 3)))
        for i in range(n):
            pos[i] = None if dr[i] is None else (1 if dr[i] == 1 else 0)
    elif strategy == "sma_cross":
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
    elif strategy == "macd_zero":
        line, _, _ = macd(closes, int(p.get("fast", 12)), int(p.get("slow", 26)), int(p.get("signal", 9)))
        for i in range(n):
            pos[i] = None if line[i] is None else (1 if line[i] > 0 else 0)
    elif strategy == "bollinger_bounce":
        mid, _, lo = bollinger(closes, int(p.get("period", 20)), float(p.get("k", 2)))
        cur = 0
        for i in range(n):
            if mid[i] is None:
                pos[i] = None
                continue
            if cur == 0 and closes[i] < lo[i]:
                cur = 1
            elif cur == 1 and closes[i] > mid[i]:
                cur = 0
            pos[i] = cur
    elif strategy == "triple_ma":
        ef, em, es = ema(closes, int(p.get("fast", 8))), ema(closes, int(p.get("mid", 21))), ema(closes, int(p.get("slow", 55)))
        for i in range(n):
            pos[i] = None if (ef[i] is None or em[i] is None or es[i] is None) else (1 if (ef[i] > em[i] > es[i]) else 0)
    else:
        return None
    return pos


# ── backtest engine ─────────────────────────────────────────────────────────
def _simulate(closes, times, signal, fee, direction="long", sl=0.0, tp=0.0, trail=0.0, slip=0.0):
    # signal[i] = 1 (bullish) / 0 (bearish) / None (warm-up). direction maps it to a position:
    # long → long|flat, short → short|flat, both → long|short. SL/TP/trailing are close-based
    # (an intra-bar refinement is possible later); slippage moves fills against you.
    def target(s):
        if s is None:
            return None
        if direction == "short":
            return -1 if s == 0 else 0
        if direction == "both":
            return 1 if s == 1 else -1
        return 1 if s == 1 else 0
    state = {"pos": 0, "equity": 1.0, "entry": None, "entry_t": None, "best": None}
    eq_curve, trades, rets = [], [], []

    def do_open(p, px, t):
        state["pos"], state["entry"], state["entry_t"], state["best"] = p, px * (1 + slip * p), t, px
        state["equity"] *= (1 - fee / 2)

    def do_close(px, t, reason):
        p = state["pos"]
        ex = px * (1 - slip * p)
        state["equity"] *= (1 - fee / 2)
        trades.append({"entry_t": state["entry_t"], "entry": state["entry"], "exit_t": t, "exit": ex,
                       "ret_pct": p * (ex / state["entry"] - 1) * 100, "side": "long" if p > 0 else "short", "reason": reason})
        state["pos"], state["entry"], state["best"] = 0, None, None

    for i in range(len(closes)):
        p = state["pos"]
        if p != 0 and i > 0:
            r = p * (closes[i] / closes[i - 1] - 1)
            state["equity"] *= (1 + r)
            rets.append(r)
        if p > 0:
            state["best"] = max(state["best"], closes[i]) if state["best"] is not None else closes[i]
        elif p < 0:
            state["best"] = min(state["best"], closes[i]) if state["best"] is not None else closes[i]
        # SL / TP / trailing (close-based)
        if state["pos"] != 0 and state["entry"]:
            cp, e, b, reason = closes[i], state["entry"], state["best"], None
            if state["pos"] > 0:
                if sl > 0 and cp <= e * (1 - sl): reason = "SL"
                elif tp > 0 and cp >= e * (1 + tp): reason = "TP"
                elif trail > 0 and cp <= b * (1 - trail): reason = "trail"
            else:
                if sl > 0 and cp >= e * (1 + sl): reason = "SL"
                elif tp > 0 and cp <= e * (1 - tp): reason = "TP"
                elif trail > 0 and cp >= b * (1 + trail): reason = "trail"
            if reason:
                do_close(cp, times[i], reason)
        # signal-driven transitions
        tg = target(signal[i])
        if tg is not None and i > 0 and tg != state["pos"]:
            if state["pos"] != 0:
                do_close(closes[i], times[i], "signal")
            if tg != 0:
                do_open(tg, closes[i], times[i])
        eq_curve.append({"t": times[i], "eq": round(state["equity"], 6)})
    if state["pos"] != 0 and state["entry"]:
        p, ex = state["pos"], closes[-1] * (1 - slip * state["pos"])
        trades.append({"entry_t": state["entry_t"], "entry": state["entry"], "exit_t": times[-1], "exit": ex,
                       "ret_pct": p * (ex / state["entry"] - 1) * 100, "side": "long" if p > 0 else "short", "open": True})
    return state["equity"], eq_curve, trades, rets


def _max_drawdown(eq):
    peak, mdd = eq[0], 0.0
    for v in eq:
        peak = max(peak, v)
        mdd = min(mdd, v / peak - 1)
    return mdd * 100


def _metrics(equity, closes, eq_curve, trades, rets, interval):
    closed = [t for t in trades if not t.get("open")]
    wins = [t for t in closed if t["ret_pct"] > 0]
    losses = [t for t in closed if t["ret_pct"] <= 0]
    eqs = [p["eq"] for p in eq_curve]
    ann = _ANN.get(interval, 8760)
    mean = sum(rets) / len(rets) if rets else 0.0
    var = sum((r - mean) ** 2 for r in rets) / len(rets) if rets else 0.0
    std = math.sqrt(var)
    sharpe = (mean / std * math.sqrt(ann)) if std > 0 else 0.0
    dvar = sum(min(r, 0) ** 2 for r in rets) / len(rets) if rets else 0.0
    dstd = math.sqrt(dvar)
    sortino = (mean / dstd * math.sqrt(ann)) if dstd > 0 else 0.0
    mdd = _max_drawdown(eqs)
    ann_ret = ((equity ** (ann / max(1, len(rets)))) - 1) * 100 if rets else 0.0
    calmar = (ann_ret / abs(mdd)) if mdd != 0 else 0.0
    gw = sum(t["ret_pct"] for t in wins)
    gl = abs(sum(t["ret_pct"] for t in losses))
    pf = (gw / gl) if gl > 0 else (gw if gw > 0 else 0.0)
    return {
        "total_return_pct": round((equity - 1) * 100, 2),
        "buy_hold_pct": round((closes[-1] / closes[0] - 1) * 100, 2),
        "win_rate_pct": round(len(wins) / len(closed) * 100, 1) if closed else 0.0,
        "max_drawdown_pct": round(mdd, 2),
        "sharpe": round(sharpe, 2),
        "sortino": round(sortino, 2),
        "calmar": round(calmar, 2),
        "profit_factor": round(pf, 2),
        "avg_win_pct": round(sum(t["ret_pct"] for t in wins) / len(wins), 2) if wins else 0.0,
        "avg_loss_pct": round(sum(t["ret_pct"] for t in losses) / len(losses), 2) if losses else 0.0,
        "trades": len(closed) + (1 if (trades and trades[-1].get("open")) else 0),
        "candles": len(closes),
    }


# ── ops: data ─────────────────────────────────────────────────────────────────
def op_get_price(a):
    sym = str(a.get("symbol", "")).upper().strip()
    if not sym:
        return {"error": "symbol required"}
    return {"result": {"symbol": sym, "price": _last_price(sym), "asset": "crypto" if _is_crypto(sym) else "stock/fx"}}


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
        {"name": "bollinger", "params": {"period": 20, "k": 2}, "desc": "Bollinger Bands (middle/upper/lower)"},
        {"name": "atr", "params": {"period": 14}, "desc": "Average True Range (volatility)"},
        {"name": "obv", "params": {}, "desc": "On-Balance Volume"},
        {"name": "stoch", "params": {"period": 14, "d_period": 3}, "desc": "Stochastic %K/%D (0..100)"},
        {"name": "vwap", "params": {}, "desc": "Volume Weighted Average Price"},
        {"name": "adx", "params": {"period": 14}, "desc": "ADX trend strength + +DI/-DI"},
        {"name": "supertrend", "params": {"period": 10, "mult": 3}, "desc": "SuperTrend (trend + direction)"},
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
    highs = [c["h"] for c in candles]
    lows = [c["l"] for c in candles]
    vols = [c["v"] for c in candles]
    times = [c["t"] for c in candles]
    if ind == "bollinger":
        period, kk = int(a.get("period") or 20), float(a.get("k") or 2)
        mid, up, lo = bollinger(closes, period, kk)
        return {"result": {"symbol": sym, "indicator": "bollinger", "period": period, "k": kk,
                           "series": [{"t": times[i], "mid": mid[i], "upper": up[i], "lower": lo[i]} for i in range(len(mid))]}}
    if ind == "atr":
        period = int(a.get("period") or 14)
        series = atr(highs, lows, closes, period)
        return {"result": {"symbol": sym, "indicator": "atr", "period": period,
                           "series": [{"t": times[i], "v": series[i]} for i in range(len(series))]}}
    if ind == "obv":
        series = obv(closes, vols)
        return {"result": {"symbol": sym, "indicator": "obv",
                           "series": [{"t": times[i], "v": series[i]} for i in range(len(series))]}}
    if ind == "stoch":
        period, dp = int(a.get("period") or 14), int(a.get("d_period") or 3)
        kk, dd = stoch(highs, lows, closes, period, dp)
        return {"result": {"symbol": sym, "indicator": "stoch", "period": period, "d_period": dp,
                           "series": [{"t": times[i], "k": kk[i], "d": dd[i]} for i in range(len(kk))]}}
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
    if ind == "vwap":
        series = vwap(highs, lows, closes, vols)
        return {"result": {"symbol": sym, "indicator": "vwap", "series": [{"t": times[i], "v": series[i]} for i in range(len(series))]}}
    if ind == "adx":
        period = int(a.get("period") or 14)
        ax, pdi, ndi = adx(highs, lows, closes, period)
        return {"result": {"symbol": sym, "indicator": "adx", "period": period,
                           "series": [{"t": times[i], "adx": ax[i], "pdi": pdi[i], "ndi": ndi[i]} for i in range(len(ax))]}}
    if ind == "supertrend":
        period, mult = int(a.get("period") or 10), float(a.get("mult") or 3)
        st, dr = supertrend(highs, lows, closes, period, mult)
        return {"result": {"symbol": sym, "indicator": "supertrend", "period": period, "mult": mult,
                           "series": [{"t": times[i], "v": st[i], "dir": dr[i]} for i in range(len(st))]}}
    return {"error": "unknown indicator: " + ind + " (sma|ema|rsi|macd|bollinger|atr|obv|stoch|vwap|adx|supertrend)"}


# ── ops: custom indicator (SAFE formula — AST whitelist, NOT exec/eval) ─────────
# The owner/agent writes a formula over the arrays close/open/high/low/volume using a
# whitelisted set of functions (sma/ema/rsi/abs/min/max) and arithmetic. It is parsed to an
# AST and only safe node types are evaluated — no imports, attributes, or arbitrary calls — so
# arbitrary code can NEVER run (the dangerous part of QuantDinger's indicator IDE, done safely).
def _elemwise(a, b, fn):
    la, lb = isinstance(a, list), isinstance(b, list)
    if la and lb:
        n = min(len(a), len(b))
        return [None if (a[i] is None or b[i] is None) else fn(a[i], b[i]) for i in range(n)]
    if la:
        return [None if v is None else fn(v, b) for v in a]
    if lb:
        return [None if v is None else fn(a, v) for v in b]
    return fn(a, b)


def _vec1(fn):
    return lambda x: ([None if v is None else fn(v) for v in x] if isinstance(x, list) else fn(x))


_FORMULA_FUNCS = {
    "sma": lambda x, p: sma(x, int(p)), "ema": lambda x, p: ema(x, int(p)), "rsi": lambda x, p: rsi(x, int(p)),
    "stdev": lambda x, p: stdev(x, int(p)), "atr": lambda h, l, c, p: atr(h, l, c, int(p)), "obv": lambda c, v: obv(c, v),
    "abs": _vec1(abs), "log": _vec1(math.log), "sqrt": _vec1(math.sqrt),
}
_BINOPS = {ast.Add: lambda a, b: a + b, ast.Sub: lambda a, b: a - b, ast.Mult: lambda a, b: a * b,
           ast.Div: lambda a, b: (a / b if b else None), ast.Pow: lambda a, b: a ** b}


def _eval_formula(node, ctx):
    if isinstance(node, ast.Expression):
        return _eval_formula(node.body, ctx)
    if isinstance(node, ast.Constant) and isinstance(node.value, (int, float)):
        return node.value
    if isinstance(node, ast.Name):
        if node.id in ctx:
            return ctx[node.id]
        raise ValueError("unknown name '%s' (use close/open/high/low/volume)" % node.id)
    if isinstance(node, ast.BinOp) and type(node.op) in _BINOPS:
        return _elemwise(_eval_formula(node.left, ctx), _eval_formula(node.right, ctx), _BINOPS[type(node.op)])
    if isinstance(node, ast.UnaryOp) and isinstance(node.op, ast.USub):
        return _elemwise(0, _eval_formula(node.operand, ctx), lambda a, b: a - b)
    if isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and node.func.id in _FORMULA_FUNCS and not node.keywords:
        return _FORMULA_FUNCS[node.func.id](*[_eval_formula(a, ctx) for a in node.args])
    raise ValueError("disallowed expression (only +-*/ , numbers, close/open/high/low/volume, sma/ema/rsi/abs/log/sqrt)")


def op_custom_indicator(a):
    sym = str(a.get("symbol", "")).upper().strip()
    formula = str(a.get("formula", "")).strip()
    if not sym or not formula:
        return {"error": "symbol and formula required"}
    if len(formula) > 500:
        return {"error": "formula too long"}
    interval = str(a.get("interval") or "1h")
    limit = max(1, min(int(a.get("limit") or 200), 500))
    candles = _klines(sym, interval, limit)
    times = [c["t"] for c in candles]
    ctx = {"close": [c["c"] for c in candles], "open": [c["o"] for c in candles],
           "high": [c["h"] for c in candles], "low": [c["l"] for c in candles], "volume": [c["v"] for c in candles]}
    try:
        series = _eval_formula(ast.parse(formula, mode="eval"), ctx)
    except SyntaxError:
        return {"error": "formula syntax error"}
    except Exception as e:  # noqa
        return {"error": "formula error: " + str(e)}
    if not isinstance(series, list):
        series = [series] * len(times)  # constant → broadcast
    return {"result": {"symbol": sym, "formula": formula,
                       "series": [{"t": times[i], "v": series[i] if i < len(series) else None} for i in range(len(times))]}}


# ── ops: strategies / backtest / optimize ──────────────────────────────────────
def op_list_strategies(a):
    return {"result": {"strategies": [{"name": k, "params": v["params"], "desc": v["desc"]} for k, v in STRATEGIES.items()]}}


def _run(symbol, interval, limit, strategy, params, fee, risk=None):
    risk = risk or {}
    candles = _klines(symbol, interval, limit)
    closes = [c["c"] for c in candles]
    times = [c["t"] for c in candles]
    pos = _positions(candles, strategy, params)
    if pos is None:
        return None, "unknown strategy: " + strategy
    direction = str(risk.get("direction") or "long").lower()
    if direction not in ("long", "short", "both"):
        direction = "long"
    sl = max(0.0, float(risk.get("sl_pct") or 0)) / 100.0
    tp = max(0.0, float(risk.get("tp_pct") or 0)) / 100.0
    trail = max(0.0, float(risk.get("trail_pct") or 0)) / 100.0
    slip = max(0.0, float(risk.get("slippage_bps") or 0)) / 10000.0
    equity, eq_curve, trades, rets = _simulate(closes, times, pos, fee, direction, sl, tp, trail, slip)
    metrics = _metrics(equity, closes, eq_curve, trades, rets, interval)
    return {"symbol": symbol, "interval": interval, "strategy": strategy, "params": params,
            "direction": direction, "metrics": metrics, "equity": eq_curve, "trades": trades,
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
    risk = {"direction": a.get("direction"), "sl_pct": a.get("sl_pct"), "tp_pct": a.get("tp_pct"),
            "trail_pct": a.get("trail_pct"), "slippage_bps": a.get("slippage_bps")}
    res, err = _run(sym, interval, limit, strategy, params, fee, risk)
    if err:
        return {"error": err}
    res["params"]["fee_bps"] = round(fee * 10000, 2)
    res["risk"] = {"direction": res["direction"], "sl_pct": a.get("sl_pct") or 0, "tp_pct": a.get("tp_pct") or 0,
                   "trail_pct": a.get("trail_pct") or 0, "slippage_bps": a.get("slippage_bps") or 0}
    m = res["metrics"]
    hist = _load_state().get("bt_history") or []
    hist.insert(0, {"symbol": sym, "strategy": strategy, "direction": res["direction"], "interval": interval,
                    "params": dict(res["params"]), "return_pct": m["total_return_pct"], "sharpe": m["sharpe"],
                    "trades": m["trades"], "at": int(time.time())})
    _patch_state({"last_backtest": res, "bt_history": hist[:25]})
    return {"result": res}


def op_backtest_history(a):
    return {"result": {"history": _load_state().get("bt_history") or []}}


def op_multi_timeframe(a):
    sym = str(a.get("symbol", "")).upper().strip()
    if not sym:
        return {"error": "symbol required"}
    strategy = str(a.get("strategy") or "sma_cross")
    if strategy not in STRATEGIES:
        return {"error": "unknown strategy: %s" % strategy}
    tfs = a.get("timeframes") or ["1d", "4h", "1h"]
    out = []
    for tf in tfs:
        try:
            candles = _klines(sym, str(tf), 200)
        except Exception:  # noqa
            continue
        pos = _positions(candles, strategy, dict(STRATEGIES[strategy]["params"]))
        sig = next((v for v in reversed(pos) if v is not None), None)
        out.append({"tf": str(tf), "signal": "long" if sig == 1 else "flat"})
    longs = sum(1 for o in out if o["signal"] == "long")
    align = "all_long" if (out and longs == len(out)) else "all_flat" if longs == 0 else "mixed"
    return {"result": {"symbol": sym, "strategy": strategy, "timeframes": out, "alignment": align,
                       "hint": {"all_long": "strong long confirmation across timeframes",
                                "all_flat": "no long signal on any timeframe",
                                "mixed": "timeframes disagree — wait for alignment"}.get(align, "")}}


def op_regime_detection(a):
    sym = str(a.get("symbol", "")).upper().strip()
    if not sym:
        return {"error": "symbol required"}
    interval = str(a.get("interval") or "1h")
    limit = max(50, min(int(a.get("limit") or 200), 500))
    candles = _klines(sym, interval, limit)
    closes = [c["c"] for c in candles]
    highs = [c["h"] for c in candles]
    lows = [c["l"] for c in candles]
    ax, pdi, ndi = adx(highs, lows, closes, 14)
    a_atr = atr(highs, lows, closes, 14)
    last_adx = next((v for v in reversed(ax) if v is not None), 0.0)
    last_atr = next((v for v in reversed(a_atr) if v is not None), 0.0)
    atr_pct = last_atr / closes[-1] * 100 if closes[-1] else 0.0
    s = sma(closes, 20)
    sv = [v for v in s if v is not None]
    slope_up = len(sv) >= 6 and sv[-1] > sv[-6]
    p_last = next((v for v in reversed(pdi) if v is not None), 0.0)
    n_last = next((v for v in reversed(ndi) if v is not None), 0.0)
    trend = "up" if p_last >= n_last else "down"
    if last_adx >= 25:
        regime = "trending_" + trend
    elif atr_pct >= 4:
        regime = "volatile"
    else:
        regime = "ranging"
    return {"result": {"symbol": sym, "regime": regime, "adx": round(last_adx, 1), "atr_pct": round(atr_pct, 2),
                       "plus_di": round(p_last, 1), "minus_di": round(n_last, 1), "sma_slope": "up" if slope_up else "down",
                       "hint": {"trending_up": "trend-follow long", "trending_down": "trend-follow short", "ranging": "mean-reversion / range", "volatile": "reduce size / widen stops"}.get(regime, "")}}


def _grid(strategy, custom):
    """Yield param dicts for a sweep. custom = {param: [values]} overrides defaults."""
    defaults = {
        "sma_cross": {"fast": [5, 10, 20], "slow": [30, 50, 100]},
        "ema_cross": {"fast": [8, 12, 20], "slow": [21, 26, 50]},
        "rsi_threshold": {"period": [14], "buy_below": [20, 30], "sell_above": [70, 80]},
        "macd_cross": {"fast": [8, 12], "slow": [21, 26], "signal": [9]},
        "macd_zero": {"fast": [8, 12], "slow": [21, 26], "signal": [9]},
        "bollinger_bounce": {"period": [14, 20, 30], "k": [2, 2.5]},
        "triple_ma": {"fast": [5, 8, 13], "mid": [21, 34], "slow": [55, 100]},
        "supertrend": {"period": [7, 10, 14], "mult": [2, 3, 4]},
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
        pos = _positions(candles, strategy, params)
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


def op_compare_strategies(a):
    sym = str(a.get("symbol", "")).upper().strip()
    if not sym:
        return {"error": "symbol required"}
    interval = str(a.get("interval") or "1h")
    limit = max(50, min(int(a.get("limit") or 300), 500))
    fee = float(a.get("fee_bps") or 10) / 10000.0
    rows, buy_hold = [], 0.0
    for name in STRATEGIES:
        res, err = _run(sym, interval, limit, name, dict(STRATEGIES[name]["params"]), fee)
        if err:
            continue
        rows.append({"strategy": name, "params": STRATEGIES[name]["params"], "metrics": res["metrics"]})
        buy_hold = res["metrics"]["buy_hold_pct"]
    if not rows:
        return {"error": "no strategy produced a result"}
    rows.sort(key=lambda r: r["metrics"]["total_return_pct"], reverse=True)
    return {"result": {"symbol": sym, "interval": interval, "buy_hold_pct": buy_hold, "results": rows}}


# ── ops: AI (via the Flowork router — sovereign, no third-party key) ─────────────
def _llm(prompt, max_tokens=320):
    body = {"messages": [{"role": "user", "content": prompt}], "max_tokens": max_tokens}
    if LLM_MODEL:
        body["model"] = LLM_MODEL
    req = urllib.request.Request(LLM_URL, data=json.dumps(body).encode("utf-8"),
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=LLM_TIMEOUT) as r:
        d = json.loads(r.read().decode("utf-8"))
    return (d.get("choices") or [{}])[0].get("message", {}).get("content", "").strip()


def op_ai_analyze(a):
    sym = str(a.get("symbol", "")).upper().strip()
    if not sym:
        return {"error": "symbol required"}
    interval = str(a.get("interval") or "1h")
    limit = max(50, min(int(a.get("limit") or 300), 500))
    # gather neutral, factual context from our own engine (no hallucinated numbers).
    price = _get("/ticker/price", {"symbol": sym})
    bt, err = _run(sym, interval, limit, "sma_cross", {"fast": 10, "slow": 30}, 0.001)
    if err:
        return {"error": err}
    closes = [c["c"] for c in _klines(sym, interval, limit)]
    r = rsi(closes, 14)
    rsi_last = next((v for v in reversed(r) if v is not None), None)
    m = bt["metrics"]
    ctx = {"symbol": sym, "price": float(price["price"]), "rsi14": round(rsi_last, 1) if rsi_last else None,
           "backtest": {"strategy": "sma_cross(10/30)", **m}}
    prompt = (
        "You are a concise, neutral quantitative analyst. Use ONLY the data below — do not invent "
        "numbers. Give 3-4 short sentences in English covering: trend/momentum read, what the "
        "backtest implies about this regime, and the key risk. No hype, no disclaimers.\n\n"
        + json.dumps(ctx, indent=2)
    )
    try:
        text = _llm(prompt)
    except urllib.error.URLError as e:
        return {"error": "AI router unreachable (%s) — start the Flowork router or set FLOWORK_ROUTER_URL" % e.reason}
    if not text:
        return {"error": "AI router returned no content"}
    return {"result": {"symbol": sym, "analysis": text, "context": ctx}}


# ── ops: paper portfolio (virtual; no broker, no real money) ────────────────────
def _portfolio():
    p = _load_state().get("portfolio")
    if not p:
        p = {"cash": STARTING_CASH, "positions": {}, "orders": [], "realized": 0.0, "start_cash": STARTING_CASH}
    return p


def _price(sym):
    return _last_price(sym)  # multi-asset (crypto → Binance, stocks/forex → Yahoo)


def op_portfolio_get(a):
    p = _portfolio()
    positions, pos_value = [], 0.0
    for sym, pos in p["positions"].items():
        try:
            px = _price(sym)
        except Exception:  # noqa
            px = pos["avg"]
        val = pos["qty"] * px
        pos_value += val
        positions.append({"symbol": sym, "qty": round(pos["qty"], 8), "avg": pos["avg"], "price": px,
                          "value": round(val, 2), "unrealized": round(pos["qty"] * (px - pos["avg"]), 2),
                          "unrealized_pct": round((px / pos["avg"] - 1) * 100, 2) if pos["avg"] else 0.0})
    start = p.get("start_cash", STARTING_CASH)
    equity = p["cash"] + pos_value
    return {"result": {"cash": round(p["cash"], 2), "positions": positions, "equity": round(equity, 2),
                       "realized_pnl": round(p["realized"], 2), "start_cash": start,
                       "total_return_pct": round((equity / start - 1) * 100, 2)}}


def op_paper_buy(a):
    sym = str(a.get("symbol", "")).upper().strip()
    quote = float(a.get("quote_amount") or 0)
    if not sym or quote <= 0:
        return {"error": "symbol and positive quote_amount required"}
    p = _portfolio()
    if p["cash"] < quote:
        return {"error": "insufficient cash (have %.2f, need %.2f)" % (p["cash"], quote)}
    px = _price(sym)
    qty = (quote * (1 - PAPER_FEE)) / px
    pos = p["positions"].get(sym, {"qty": 0.0, "avg": 0.0})
    new_qty = pos["qty"] + qty
    pos["avg"] = (pos["qty"] * pos["avg"] + qty * px) / new_qty if new_qty > 0 else px
    pos["qty"] = new_qty
    p["positions"][sym] = pos
    p["cash"] -= quote
    order = {"side": "buy", "symbol": sym, "qty": round(qty, 8), "price": px, "quote": round(quote, 2), "t": int(time.time() * 1000)}
    p["orders"].append(order)
    _patch_state({"portfolio": p})
    return {"result": {"order": order, "cash": round(p["cash"], 2)}}


def op_paper_sell(a):
    sym = str(a.get("symbol", "")).upper().strip()
    p = _portfolio()
    pos = p["positions"].get(sym)
    if not pos or pos["qty"] <= 0:
        return {"error": "no open position in %s" % sym}
    qty = float(a.get("qty") or pos["qty"])
    qty = min(qty, pos["qty"])
    px = _price(sym)
    proceeds = qty * px
    realized = qty * (px - pos["avg"]) - proceeds * PAPER_FEE
    p["realized"] += realized
    p["cash"] += proceeds * (1 - PAPER_FEE)
    pos["qty"] -= qty
    if pos["qty"] <= 1e-12:
        del p["positions"][sym]
    else:
        p["positions"][sym] = pos
    order = {"side": "sell", "symbol": sym, "qty": round(qty, 8), "price": px, "realized": round(realized, 2), "t": int(time.time() * 1000)}
    p["orders"].append(order)
    _patch_state({"portfolio": p})
    return {"result": {"order": order, "realized": round(realized, 2), "cash": round(p["cash"], 2)}}


def op_paper_reset(a):
    _patch_state({"portfolio": {"cash": STARTING_CASH, "positions": {}, "orders": [], "realized": 0.0, "start_cash": STARTING_CASH}})
    return {"result": {"ok": True, "cash": STARTING_CASH}}


def op_list_paper_orders(a):
    p = _portfolio()
    limit = max(1, min(int(a.get("limit") or 50), 500))
    return {"result": {"orders": p["orders"][-limit:][::-1], "count": len(p["orders"])}}


# ── ops: live trading (OWNER-GATED; refuses unless the owner arms it) ────────────
def op_live_status(a):
    return {"result": {
        "live_enabled": LIVE_ENABLED,
        "broker": None,
        "mode": "armed-no-broker" if LIVE_ENABLED else "paper-only",
        "note": ("Live trading is OWNER-GATED. It is DISABLED unless the OWNER sets "
                 "FLOWALPHA_LIVE_ENABLED=1 in the host environment AND configures a broker "
                 "connector with their own keys. An agent can never enable it. Use paper_buy / "
                 "paper_sell for risk-free trading."),
    }}


def op_live_order(a):
    # Two-gate refusal (cannot lose real money by default):
    if not LIVE_ENABLED:
        return {"error": "live trading is DISABLED (owner-gated). The owner must set "
                         "FLOWALPHA_LIVE_ENABLED=1 in the host env to arm it — an agent cannot. "
                         "Use paper_buy / paper_sell for risk-free trading."}
    return {"error": "live trading is ARMED but no broker connector is configured. The owner "
                     "adds a broker (with their own keys) to route real orders; none is bundled, "
                     "so no real order is placed."}


# ── ops: market breadth ─────────────────────────────────────────────────────
def op_get_ticker_24h(a):
    sym = str(a.get("symbol", "")).upper().strip()
    if not sym:
        return {"error": "symbol required"}
    if not _is_crypto(sym):
        return {"error": "24h ticker is crypto-only; use get_klines for stocks/forex"}
    d = _get("/ticker/24hr", {"symbol": sym})
    return {"result": {"symbol": d["symbol"], "last": float(d["lastPrice"]),
                       "change_pct": float(d["priceChangePercent"]), "high": float(d["highPrice"]),
                       "low": float(d["lowPrice"]), "volume": float(d["volume"]),
                       "quote_volume": float(d["quoteVolume"]), "trades": int(d.get("count", 0))}}


def op_top_movers(a):
    quote = str(a.get("quote") or "USDT").upper().strip()
    limit = max(1, min(int(a.get("limit") or 10), 50))
    rows = [t for t in _get("/ticker/24hr") if t["symbol"].endswith(quote) and float(t.get("quoteVolume", 0)) > 0]
    rows.sort(key=lambda t: float(t["priceChangePercent"]), reverse=True)
    fmt = lambda t: {"symbol": t["symbol"], "change_pct": round(float(t["priceChangePercent"]), 2),
                     "last": float(t["lastPrice"]), "quote_volume": round(float(t["quoteVolume"]), 0)}
    return {"result": {"quote": quote, "gainers": [fmt(t) for t in rows[:limit]],
                       "losers": [fmt(t) for t in rows[-limit:][::-1]]}}


# ── ops: watchlist + price alerts (shared state) ────────────────────────────────
def op_watchlist_get(a):
    out = []
    for s in (_load_state().get("watchlist") or []):
        try:
            out.append({"symbol": s, "price": _last_price(s)})
        except Exception:  # noqa
            out.append({"symbol": s, "price": None})
    return {"result": {"watchlist": out}}


def op_watchlist_add(a):
    s = str(a.get("symbol", "")).upper().strip()
    if not s:
        return {"error": "symbol required"}
    wl = _load_state().get("watchlist") or []
    if s not in wl:
        wl.append(s)
    _patch_state({"watchlist": wl})
    return {"result": {"watchlist": wl}}


def op_watchlist_remove(a):
    s = str(a.get("symbol", "")).upper().strip()
    wl = [x for x in (_load_state().get("watchlist") or []) if x != s]
    _patch_state({"watchlist": wl})
    return {"result": {"watchlist": wl}}


def op_alert_add(a):
    s = str(a.get("symbol", "")).upper().strip()
    cond = str(a.get("cond", "above")).lower().strip()
    price = float(a.get("price") or 0)
    if not s or price <= 0 or cond not in ("above", "below"):
        return {"error": "symbol, cond (above|below), and positive price required"}
    al = _load_state().get("alerts") or []
    alert = {"id": int(time.time() * 1000), "symbol": s, "cond": cond, "price": price, "triggered": False}
    al.append(alert)
    _patch_state({"alerts": al})
    return {"result": {"alert": alert}}


def op_alert_list(a):
    return {"result": {"alerts": _load_state().get("alerts") or []}}


def op_alert_remove(a):
    aid = a.get("id")
    al = [x for x in (_load_state().get("alerts") or []) if x.get("id") != aid]
    _patch_state({"alerts": al})
    return {"result": {"alerts": al}}


def op_alert_check(a):
    al = _load_state().get("alerts") or []
    triggered, changed = [], False
    for alert in al:
        if alert.get("triggered"):
            continue
        try:
            px = _last_price(alert["symbol"])
        except Exception:  # noqa
            continue
        if (alert["cond"] == "above" and px >= alert["price"]) or (alert["cond"] == "below" and px <= alert["price"]):
            alert["triggered"] = True
            alert["fired_price"] = px
            changed = True
            triggered.append(dict(alert))
    if changed:
        _patch_state({"alerts": al})
        for t in triggered:
            _notify("\U0001f6a8 FlowAlpha alert: %s %s %s (now %s)" % (
                t["symbol"], t["cond"], t["price"], t.get("fired_price")))
    return {"result": {"triggered": triggered}}


# ── ops: paper bots (a strategy that runs continuously on the paper portfolio) ──
# A bot = {symbol, strategy, interval, quote, enabled}. bot_step evaluates each enabled bot's
# current signal and acts on the PAPER portfolio (buy when bullish + flat, sell when bearish +
# in-position). The GUI (or an agent) calls bot_step on a timer → the strategy "runs live" on
# paper. No real money — it just drives paper_buy/paper_sell.
def op_bot_add(a):
    sym = str(a.get("symbol", "")).upper().strip()
    strat = str(a.get("strategy") or "sma_cross")
    if not sym or strat not in STRATEGIES:
        return {"error": "symbol and valid strategy required"}
    bot = {"id": int(time.time() * 1000), "symbol": sym, "strategy": strat,
           "interval": str(a.get("interval") or "1h"), "quote": float(a.get("quote") or 1000), "enabled": True}
    bots = _load_state().get("bots") or []
    bots.append(bot)
    _patch_state({"bots": bots})
    return {"result": {"bot": bot}}


def op_bot_list(a):
    return {"result": {"bots": _load_state().get("bots") or [], "last_run": _load_state().get("last_bot_run")}}


def op_bot_remove(a):
    aid = a.get("id")
    bots = [b for b in (_load_state().get("bots") or []) if b.get("id") != aid]
    _patch_state({"bots": bots})
    return {"result": {"bots": bots}}


def op_bot_toggle(a):
    aid = a.get("id")
    bots = _load_state().get("bots") or []
    for b in bots:
        if b.get("id") == aid:
            b["enabled"] = not b.get("enabled")
    _patch_state({"bots": bots})
    return {"result": {"bots": bots}}


def op_bot_step(a):
    bots = _load_state().get("bots") or []
    actions = []
    for bot in bots:
        if not bot.get("enabled"):
            continue
        sym, strat = bot["symbol"], bot["strategy"]
        try:
            candles = _klines(sym, bot.get("interval", "1h"), 200)
        except Exception:  # noqa
            continue
        pos = _positions(candles, strat, dict(STRATEGIES[strat]["params"]))
        sig = next((v for v in reversed(pos) if v is not None), None)
        if sig is None:
            continue
        p = _portfolio()
        held = sym in p["positions"] and p["positions"][sym]["qty"] > 0
        if sig == 1 and not held:
            r = op_paper_buy({"symbol": sym, "quote_amount": bot.get("quote", 1000)})
            if "result" in r:
                actions.append({"symbol": sym, "strategy": strat, "action": "buy"})
                _notify("\U0001f916 FlowAlpha bot (%s) BUY %s" % (strat, sym))
        elif sig == 0 and held:
            r = op_paper_sell({"symbol": sym})
            if "result" in r:
                rz = r["result"].get("realized")
                actions.append({"symbol": sym, "strategy": strat, "action": "sell", "realized": rz})
                _notify("\U0001f916 FlowAlpha bot (%s) SELL %s (realized %s)" % (strat, sym, rz))
    # snapshot paper equity over time (the portfolio's own equity curve)
    pf = _portfolio()
    eq = pf["cash"]
    for s, posn in pf["positions"].items():
        try:
            eq += posn["qty"] * _price(s)
        except Exception:  # noqa
            eq += posn["qty"] * posn["avg"]
    eh = _load_state().get("equity_history") or []
    eh.append({"t": int(time.time() * 1000), "eq": round(eq, 2)})
    _patch_state({"last_bot_run": int(time.time()), "equity_history": eh[-300:]})
    return {"result": {"actions": actions, "bots": len(bots), "enabled": sum(1 for b in bots if b.get("enabled"))}}


def op_portfolio_equity(a):
    return {"result": {"equity_history": _load_state().get("equity_history") or []}}


# ── shared state ────────────────────────────────────────────────────────────
def _patch_state(updates):
    st = _load_state()
    st.update(updates)
    st["at"] = int(time.time())
    _save_state(st)


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
    "custom_indicator": op_custom_indicator,
    "list_strategies": op_list_strategies, "run_backtest": op_run_backtest,
    "run_optimize": op_run_optimize, "get_last_backtest": op_get_last_backtest, "compare_strategies": op_compare_strategies,
    "backtest_history": op_backtest_history, "regime_detection": op_regime_detection, "multi_timeframe": op_multi_timeframe,
    "ai_analyze": op_ai_analyze,
    "portfolio_get": op_portfolio_get, "paper_buy": op_paper_buy, "paper_sell": op_paper_sell,
    "paper_reset": op_paper_reset, "list_paper_orders": op_list_paper_orders,
    "live_status": op_live_status, "live_order": op_live_order,
    "get_ticker_24h": op_get_ticker_24h, "top_movers": op_top_movers,
    "watchlist_get": op_watchlist_get, "watchlist_add": op_watchlist_add, "watchlist_remove": op_watchlist_remove,
    "alert_add": op_alert_add, "alert_list": op_alert_list, "alert_remove": op_alert_remove, "alert_check": op_alert_check,
    "bot_add": op_bot_add, "bot_list": op_bot_list, "bot_remove": op_bot_remove, "bot_toggle": op_bot_toggle, "bot_step": op_bot_step,
    "portfolio_equity": op_portfolio_equity,
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
