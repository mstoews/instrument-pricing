#ifndef PRICING_BRIDGE_H
#define PRICING_BRIDGE_H
#ifdef __cplusplus
extern "C" {
#endif
// Status: 0 success, 1 invalid input, 2 numerical overflow, 3 unexpected failure.
// No exceptions or allocated memory cross the C ABI.
int pricing_european(int is_put, double strike, double time_to_expiry,
                     double spot, double volatility, double risk_free_rate,
                     double dividend_yield, double *price);
#ifdef __cplusplus
}
#endif
#endif
