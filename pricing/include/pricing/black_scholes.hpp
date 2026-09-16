#pragma once
namespace pricing {
enum class OptionRight { Call, Put };
// European exercise only. Rates continuously compounded; time is a year fraction.
struct EuropeanOption { OptionRight right; double strike; double time_to_expiry; };
struct MarketSnapshot { double spot; double volatility; double risk_free_rate; double dividend_yield; };
struct PriceResult { double price; };
PriceResult black_scholes(const EuropeanOption&, const MarketSnapshot&);
}
