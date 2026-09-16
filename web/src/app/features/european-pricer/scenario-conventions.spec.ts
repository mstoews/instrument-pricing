import { describe, expect, it } from 'vitest';
import { actual365Fixed, usableSpot } from './scenario-conventions';

describe('date-only ACT/365F scenario convention', () => {
  it.each([
    ['2026-01-01', '2027-01-01', '1.00000000'],
    ['2028-01-01', '2029-01-01', '1.00273973'],
    ['2028-02-28', '2028-03-01', '0.00547945'],
    ['2027-01-15', '2027-01-15', '0.00000000'],
    ['2026-03-07', '2026-03-09', '0.00547945'],
  ])('converts %s to %s without timezone or DST changes', (start, end, expected) => {
    expect(actual365Fixed(start, end)).toBe(expected);
  });
  it.each([
    '',
    '2027-02-29',
    '2028-02-30',
    '2028-13-01',
    '0000-01-01',
    '2028-1-01',
    '2028-01-01T00:00:00Z',
  ])('rejects invalid valuation date %s', (date) => {
    expect(actual365Fixed(date, '2029-01-01')).toBeNull();
  });
  it('rejects invalid expirations and valuation dates after expiry', () => {
    expect(actual365Fixed('2027-01-16', '2027-01-15')).toBeNull();
    expect(actual365Fixed('2027-01-01', '2027-02-29')).toBeNull();
  });
  it('only accepts positive quote prices within the API decimal format', () => {
    for (const value of ['', '0.00000000', '-1', 'NaN', '1e2', '1000000000000'])
      expect(usableSpot(value)).toBe(false);
    for (const value of ['0.00000001', '201.12345678', '999999999999.99999999'])
      expect(usableSpot(value)).toBe(true);
  });
});
