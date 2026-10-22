import { describe, expect, it } from 'vitest';

import { CURRENCY_OPTIONS, currencyFractionDigits, roundCurrencyAmount } from '@/constants/currencies';
import { formatMoney } from '@/utils';

describe('configured currencies', () => {
  it('offers the same standard currency codes with the correct minor units', () => {
    const codes = new Set(CURRENCY_OPTIONS.map(({ value }) => value));
    expect(codes.has('USD')).toBe(true);
    expect(codes.has('IRR')).toBe(true);
    expect(codes.has('IRT')).toBe(false);
    expect(currencyFractionDigits('JPY')).toBe(0);
    expect(currencyFractionDigits('KWD')).toBe(3);
  });

  it('rounds values according to the selected currency precision', () => {
    expect(roundCurrencyAmount(12.345, 'USD')).toBe(12.35);
    expect(roundCurrencyAmount(12.5, 'JPY')).toBe(13);
    expect(roundCurrencyAmount(12.3456, 'KWD')).toBe(12.346);
  });

  it('renders a currency code rather than silently labeling an amount as another unit', () => {
    expect(formatMoney(1234.5, 'USD')).toContain('USD');
    expect(formatMoney(1234.5, 'JPY')).toContain('JPY');
    expect(formatMoney(1234.567, 'KWD')).toContain('KWD');
    expect(formatMoney(100, 'IRT')).toContain('IRT');
  });
});
