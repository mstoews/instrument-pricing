import argparse
import sys
from datetime import date, timedelta
from .adapters.sample import SampleAdapter
from .client import RestSink


def sample(args, sink):
    for quote in SampleAdapter().quotes(args.instrument_id):
        sink.submit_quote(quote)
    print("Sample quote submitted")


def find_or_create_equity(sink: RestSink, massive, ticker: str) -> tuple[str, bool]:
    """Resolve by FIGI first (stable across ticker changes), then ticker@venue;
    create the equity from Massive reference data when neither matches."""
    from .adapters.massive import equity_payload
    payload = equity_payload(massive.ticker_details(ticker))
    lookups = [(x["scheme"], x["value"], x["venue"]) for x in payload["identifiers"] if x["scheme"] == "figi"]
    lookups.append(("ticker", ticker, payload["exchange"]))
    for scheme, value, venue in lookups:
        found = sink.resolve(scheme, value, venue)
        if len(found) == 1:
            return found[0]["id"], False
        if len(found) > 1:
            raise SystemExit(f"{ticker}: {scheme} {value} matches {len(found)} instruments; resolve it manually")
    return sink.create_equity(payload), True


def massive_bars(args, sink):
    from .adapters.massive import MassiveClient
    client = MassiveClient(calls_per_minute=args.calls_per_minute)
    for ticker in [t.strip().upper() for t in args.tickers.split(",") if t.strip()]:
        instrument_id, created = find_or_create_equity(sink, client, ticker)
        bars = client.bars(instrument_id, ticker, args.start, args.end)
        inserted = sink.submit_bars(bars) if bars else 0
        print(f"{ticker}: {'created' if created else 'found'} {instrument_id}; {len(bars)} bars, {inserted} new")


def treasury_curve(args, sink):
    from .adapters.treasury import TreasuryClient, parse_par_csv, snapshot
    if args.file:
        with open(args.file, encoding="utf-8") as f:
            curves = parse_par_csv(f.read())
    else:
        curves = TreasuryClient().par_curves(args.year)
    if args.latest:
        curves = curves[-1:]
    new = sum(sink.submit_curve(snapshot(c)) for c in curves)
    span = f"{curves[0].date}..{curves[-1].date}" if curves else "no dates"
    print(f"Treasury: {len(curves)} curve dates ({span}), {new} new snapshots")


def main(argv=None):
    parser = argparse.ArgumentParser(description="Normalize vendor data and submit it to the pricing API")
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    parser.add_argument("--api-key", default=None, help="ingest-scoped API key (default: $PRICING_API_KEY)")
    sub = parser.add_subparsers(dest="command")
    s = sub.add_parser("sample", help="submit one synthetic quote (default command)")
    s.add_argument("--instrument-id", default="00000000-0000-0000-0000-000000000001")
    m = sub.add_parser("massive-bars", help="daily bars from Massive; creates missing equities ($MASSIVE_API_KEY)")
    m.add_argument("--tickers", required=True, help="comma-separated, e.g. AAPL,MSFT")
    m.add_argument("--from", dest="start", type=date.fromisoformat, default=date.today() - timedelta(days=365))
    m.add_argument("--to", dest="end", type=date.fromisoformat, default=date.today())
    m.add_argument("--calls-per-minute", type=float, default=None, help="Massive request pacing (default 5, the free tier)")
    t = sub.add_parser("treasury-curve", help="Treasury par curve → bootstrapped zero curve snapshots")
    t.add_argument("--year", type=int, default=date.today().year)
    t.add_argument("--file", help="read a downloaded Treasury CSV instead of fetching")
    t.add_argument("--latest", action="store_true", help="submit only the most recent date")
    argv = list(sys.argv[1:] if argv is None else argv)
    commands = ("sample", "massive-bars", "treasury-curve", "-h", "--help")
    if not any(a in commands for a in argv):
        # Backward compatible: no subcommand means `sample`; global options stay in front.
        split = next((i for i, a in enumerate(argv) if a == "--instrument-id"), len(argv))
        argv = [*argv[:split], "sample", *argv[split:]]
    args = parser.parse_args(argv)
    sink = RestSink(args.base_url, args.api_key)
    {"sample": sample, "massive-bars": massive_bars, "treasury-curve": treasury_curve}[args.command](args, sink)


if __name__ == "__main__":
    main()
