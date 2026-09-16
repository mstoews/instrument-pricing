#include "bridge.h"
#include "pricing/black_scholes.hpp"
#include <stdexcept>

extern "C" int pricing_european(int is_put, double strike, double time_to_expiry,
                                double spot, double volatility, double risk_free_rate,
                                double dividend_yield, double *price) {
    if (!price || (is_put != 0 && is_put != 1)) return 1;
    *price = 0;
    try {
        *price = pricing::black_scholes(
            {is_put ? pricing::OptionRight::Put : pricing::OptionRight::Call, strike, time_to_expiry},
            {spot, volatility, risk_free_rate, dividend_yield}).price;
        return 0;
    } catch (const std::invalid_argument&) {
        return 1;
    } catch (const std::overflow_error&) {
        return 2;
    } catch (...) {
        return 3;
    }
}
