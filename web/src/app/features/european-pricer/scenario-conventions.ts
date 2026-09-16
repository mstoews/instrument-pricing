// Date-only scenario convention: UTC calendar-day difference / 365, rounded to
// the REST contract's eight fractional digits. No exchange expiry time is implied.
export function actual365Fixed(valuation: string, expiration: string): string | null {
  const parse = (value: string) => {
    if (!/^[0-9]{4}-[0-9]{2}-[0-9]{2}$/.test(value) || value.startsWith('0000')) return NaN;
    const timestamp = Date.parse(value + 'T00:00:00Z');
    return Number.isFinite(timestamp) && new Date(timestamp).toISOString().slice(0, 10) === value
      ? timestamp
      : NaN;
  };
  const from = parse(valuation),
    to = parse(expiration);
  if (!Number.isFinite(from) || !Number.isFinite(to) || from > to) return null;
  return ((to - from) / 86400000 / 365).toFixed(8);
}

export function usableSpot(value: string): boolean {
  return /^[0-9]{1,12}(\.[0-9]{1,8})?$/.test(value) && !/^0+(\.0+)?$/.test(value);
}
