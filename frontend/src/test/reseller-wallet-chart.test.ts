import { describe, expect, it } from 'vitest';

import type { ResellerTransaction } from '@/api/queries/useSession';
import { buildWalletActivity } from '@/pages/resellers/ResellerWalletChart';

function transaction(id: number, type: string, amount: number, date: Date): ResellerTransaction {
  return { id, resellerId: 1, type, amount, balance: 0, createdAt: date.getTime() };
}

describe('reseller wallet activity chart data', () => {
  it('aggregates deposits and withdrawals into the last twelve calendar months', () => {
    const now = new Date(2025, 2, 20);
    const rows = buildWalletActivity([
      transaction(1, 'deposit', 5.105, new Date(2025, 2, 2)),
      transaction(2, 'deposit', 4.895, new Date(2025, 2, 4)),
      transaction(3, 'withdraw', -3, new Date(2025, 2, 8)),
      transaction(4, 'adjust', -2, new Date(2025, 2, 10)),
      transaction(5, 'deposit', 100, new Date(2024, 2, 1)),
      transaction(6, 'deposit', 200, new Date(2025, 3, 1)),
    ], now, 'en-US');

    expect(rows).toHaveLength(12);
    expect(rows[0].month).toBe('Apr');
    expect(rows[11]).toEqual({ month: 'Mar', deposits: 10, withdrawals: 5 });
    expect(rows.slice(0, 11).every((point) => point.deposits === 0 && point.withdrawals === 0)).toBe(true);
  });

  it('returns a zero-filled twelve-month range for an empty ledger', () => {
    const rows = buildWalletActivity([], new Date(2025, 2, 20), 'en-US');

    expect(rows).toHaveLength(12);
    expect(rows.every((point) => point.deposits === 0 && point.withdrawals === 0)).toBe(true);
  });
});
