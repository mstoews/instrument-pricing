"""U.S. Treasury daily par yield curve (CMT) → bootstrapped zero curve.

Treasury publishes par yields on a bond-equivalent basis. The API stores
continuously compounded zero rates by tenor in days, so each day's par curve is
bootstrapped:

* par yields are interpolated linearly in maturity;
* maturities up to six months are treated as bills: discount factor 1/(1 + y·t);
* longer maturities are semiannual par bonds on a 0.5-year grid,
  DF(tₙ) = (1 − c/2·ΣDF(tₖ<ₙ)) / (1 + c/2), with c the par yield at tₙ;
* zero rate z(t) = −ln DF(t) / t, at tenor_days = round(t × 365) (ACT/365).

Approximations: the published curve is itself a fitted par curve; actual
coupon dates, day counts and the bill/coupon basis differences are ignored.
"""
import csv
import io
import math
import re
from dataclasses import dataclass
from datetime import date, datetime, time, timezone
from decimal import Decimal

from ..http import HttpGetter
from ..models import CurveSnapshot

SOURCE_ID = "treasury"
CURVE_NAME = "UST zero (bootstrapped from par CMT)"
CSV_URL = ("https://home.treasury.gov/resource-center/data-chart-center/interest-rates/"
           "daily-treasury-rates.csv/{year}/all?type=daily_treasury_yield_curve&field_tdr_date_value={year}&page&_format=csv")
_TENOR = re.compile(r"^\s*(\d+(?:\.\d+)?)\s*(wk|wks|week|weeks|mo|mos|month|months|yr|yrs|year|years)\s*$", re.I)
_UNITS = {"w": 7 / 365, "m": 1 / 12, "y": 1.0}


@dataclass(frozen=True)
class ParCurve:
    date: date
    yields: dict[float, float]  # maturity in years → par yield, decimal fraction


def tenor_years(header: str) -> float | None:
    """'1 Mo', '1.5 Month', '6 Wk', '10 Yr', '30 Year' → years; anything else → None."""
    m = _TENOR.match(header)
    return float(m.group(1)) * _UNITS[m.group(2)[0].lower()] if m else None


def _parse_date(text: str) -> date:
    for fmt in ("%m/%d/%Y", "%Y-%m-%d"):
        try:
            return datetime.strptime(text.strip(), fmt).date()
        except ValueError:
            pass
    raise ValueError(f"unrecognized Treasury date {text!r}")


def parse_par_csv(text: str) -> list[ParCurve]:
    """Parse Treasury's CSV, reading tenors from the header text so added or
    renamed columns (e.g. 1.5 Mo) work without code changes. Blank cells are
    skipped: some tenors were not published on every date."""
    reader = csv.reader(io.StringIO(text.lstrip("﻿")))
    header = next(reader)
    columns = [(i, tenor_years(h)) for i, h in enumerate(header)]
    if header[0].strip().lower() != "date" or not any(t for _, t in columns):
        raise ValueError("not a Treasury par yield curve CSV: " + ",".join(header))
    curves = []
    for row in reader:
        if not row or not row[0].strip():
            continue
        yields = {}
        for i, t in columns:
            if t and i < len(row) and row[i].strip() not in ("", "N/A"):
                yields[t] = float(row[i]) / 100
        if yields:
            curves.append(ParCurve(_parse_date(row[0]), yields))
    return sorted(curves, key=lambda c: c.date)


def _interpolate(points: list[tuple[float, float]], t: float) -> float:
    if t <= points[0][0]:
        return points[0][1]
    for (t0, y0), (t1, y1) in zip(points, points[1:]):
        if t <= t1:
            return y0 + (t - t0) / (t1 - t0) * (y1 - y0)
    return points[-1][1]


def bootstrap(par: dict[float, float]) -> list[tuple[int, float]]:
    """Return (tenor_days, continuously compounded zero rate) pairs: published
    tenors under six months plus a semiannual grid out to the longest tenor."""
    points = sorted(par.items())
    out: dict[int, float] = {}
    for t, y in points:
        if t < 0.5:
            out[round(t * 365)] = math.log1p(y * t) / t
    dfs: list[float] = []
    for n in range(1, int(round(points[-1][0] * 2)) + 1):
        t = n / 2
        c = _interpolate(points, t)
        df = 1 / (1 + c * t) if t <= 0.5 else (1 - c / 2 * sum(dfs)) / (1 + c / 2)
        if df <= 0:
            raise ValueError(f"bootstrap failed at {t} years (discount factor {df})")
        dfs.append(df)
        out[round(t * 365)] = -math.log(df) / t
    return sorted(out.items())


def observation_time(d: date) -> datetime:
    """Treasury's CMT yields reflect indicative quotes near 3:30 PM New York time."""
    try:
        from zoneinfo import ZoneInfo
        return datetime.combine(d, time(15, 30), ZoneInfo("America/New_York")).astimezone(timezone.utc)
    except Exception:  # no tz database (e.g. Windows without tzdata): use the EST equivalent
        return datetime.combine(d, time(20, 30), timezone.utc)


def snapshot(curve: ParCurve) -> CurveSnapshot:
    points = tuple((days, Decimal(f"{rate:.8f}")) for days, rate in bootstrap(curve.yields))
    return CurveSnapshot(CURVE_NAME, "USD", SOURCE_ID, observation_time(curve.date), points)


class TreasuryClient:
    def __init__(self, http: HttpGetter | None = None, url_template: str = CSV_URL):
        self.http = http or HttpGetter(min_interval=1.0)
        self.url_template = url_template

    def par_curves(self, year: int) -> list[ParCurve]:
        return parse_par_csv(self.http.get(self.url_template.format(year=year)).decode("utf-8"))
