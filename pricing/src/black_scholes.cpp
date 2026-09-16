#include "pricing/black_scholes.hpp"
#include <algorithm>
#include <cmath>
#include <stdexcept>
namespace pricing {
PriceResult black_scholes(const EuropeanOption& o, const MarketSnapshot& m) {
 for(double x : {o.strike,o.time_to_expiry,m.spot,m.volatility,m.risk_free_rate,m.dividend_yield})
  if(!std::isfinite(x)) throw std::invalid_argument("Inputs must be finite");
 if(o.strike<=0 || m.spot<=0 || o.time_to_expiry<0 || m.volatility<0)
  throw std::invalid_argument("Invalid strike, spot, expiry or volatility");
 if(o.right!=OptionRight::Call && o.right!=OptionRight::Put) throw std::invalid_argument("Invalid option right");
 const double sign=o.right==OptionRight::Call?1.0:-1.0;
 const double s=m.spot*std::exp(-m.dividend_yield*o.time_to_expiry);
 const double k=o.strike*std::exp(-m.risk_free_rate*o.time_to_expiry);
 if(!std::isfinite(s)||!std::isfinite(k)) throw std::overflow_error("Discount overflow");
 if(o.time_to_expiry==0 || m.volatility==0) return {std::max(sign*(s-k),0.0)};
 const double vol=m.volatility*std::sqrt(o.time_to_expiry);
 const double d1=(std::log(m.spot/o.strike)+(m.risk_free_rate-m.dividend_yield+0.5*m.volatility*m.volatility)*o.time_to_expiry)/vol;
 const double d2=d1-vol;
 auto normal=[](double x){return 0.5*std::erfc(-x/std::sqrt(2.0));};
 double price=sign*(s*normal(sign*d1)-k*normal(sign*d2));
 if(!std::isfinite(price)) throw std::overflow_error("Price overflow");
 return {price};
}
}
