#include "pricing/black_scholes.hpp"
#include <cmath>
#include <iostream>
#include <limits>
#include <stdexcept>
using namespace pricing;
void near(double a,double b,double tol=1e-6){if(std::abs(a-b)>tol||!std::isfinite(a))throw std::runtime_error("price mismatch");}
int main(){try{
 MarketSnapshot m{100,0.2,0.05,0};
 EuropeanOption c{OptionRight::Call,100,1},p{OptionRight::Put,100,1};
 near(black_scholes(c,m).price,10.450583572);near(black_scholes(p,m).price,5.573526022);
 m.dividend_yield=0.02;near(black_scholes(c,m).price-black_scholes(p,m).price,100*std::exp(-0.02)-100*std::exp(-0.05));
 c.time_to_expiry=0;near(black_scholes(c,m).price,0);c.strike=90;near(black_scholes(c,m).price,10);
 c={OptionRight::Call,100,1};m.volatility=0;near(black_scholes(c,m).price,100*std::exp(-0.02)-100*std::exp(-0.05));
 for(double bad:{-1.0,std::numeric_limits<double>::quiet_NaN()}){m.volatility=bad;bool rejected=false;try{black_scholes(c,m);}catch(const std::invalid_argument&){rejected=true;}if(!rejected)throw std::runtime_error("invalid input accepted");}
 std::cout<<"Black-Scholes tests passed\n";return 0;
 }catch(const std::exception& e){std::cerr<<e.what()<<'\n';return 1;}}
