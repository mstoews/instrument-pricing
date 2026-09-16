import argparse
from .adapters.sample import SampleAdapter
from .client import RestSink

def main():
    parser = argparse.ArgumentParser(description="Submit a synthetic quote through REST")
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    parser.add_argument("--instrument-id", default="00000000-0000-0000-0000-000000000001")
    args = parser.parse_args()
    sink = RestSink(args.base_url)
    for quote in SampleAdapter().quotes(args.instrument_id):
        sink.submit_quote(quote)
    print("Sample quote submitted")

if __name__ == "__main__":
    main()
