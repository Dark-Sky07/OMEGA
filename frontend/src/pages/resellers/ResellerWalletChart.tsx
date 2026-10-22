import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Empty } from 'antd';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import type { ResellerTransaction } from '@/api/queries/useSession';
import { formatMoney, roundCurrencyAmount } from '@/utils';

export interface WalletActivityPoint {
  month: string;
  deposits: number;
  withdrawals: number;
}

/** Aggregate the last twelve calendar months of ledger movements. */
export function buildWalletActivity(
  transactions: ResellerTransaction[],
  now = new Date(),
  locale?: string,
  currency = 'USD',
): WalletActivityPoint[] {
  const monthStart = new Date(now.getFullYear(), now.getMonth() - 11, 1);
  const points: WalletActivityPoint[] = [];
  const indices = new Map<string, WalletActivityPoint>();
  const monthFormatter = new Intl.DateTimeFormat(locale, { month: 'short' });

  for (let offset = 0; offset < 12; offset += 1) {
    const date = new Date(monthStart.getFullYear(), monthStart.getMonth() + offset, 1);
    const key = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`;
    const point = { month: monthFormatter.format(date), deposits: 0, withdrawals: 0 };
    points.push(point);
    indices.set(key, point);
  }

  for (const transaction of transactions) {
    if (!Number.isFinite(transaction.createdAt) || !Number.isFinite(transaction.amount) || transaction.createdAt <= 0) continue;
    const date = new Date(transaction.createdAt);
    if (date > now) continue;
    const key = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`;
    const point = indices.get(key);
    if (!point) continue;

    const amount = Math.abs(transaction.amount);
    if (transaction.type === 'withdraw' || transaction.amount < 0) {
      point.withdrawals += amount;
    } else {
      point.deposits += amount;
    }
  }

  return points.map((point) => ({
    ...point,
    deposits: roundCurrencyAmount(point.deposits, currency),
    withdrawals: roundCurrencyAmount(point.withdrawals, currency),
  }));
}

interface ResellerWalletChartProps {
  transactions: ResellerTransaction[];
  currency: string;
}

export default function ResellerWalletChart({ transactions, currency }: ResellerWalletChartProps) {
  const { t, i18n } = useTranslation();
  const data = useMemo(
    () => buildWalletActivity(transactions, new Date(), i18n.resolvedLanguage || i18n.language, currency),
    [transactions, i18n.language, i18n.resolvedLanguage, currency],
  );

  return (
    <Card size="small" title={t('resellers.walletActivity')}>
      {transactions.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('resellers.noTransactions')} />
      ) : (
        <div role="img" aria-label={t('resellers.walletActivity')} style={{ width: '100%', height: 260 }}>
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={data} margin={{ top: 8, right: 12, bottom: 4, left: 8 }}>
              <CartesianGrid strokeDasharray="3 3" vertical={false} />
              <XAxis dataKey="month" interval="preserveStartEnd" minTickGap={12} />
              <YAxis width={92} tickFormatter={(value: number) => formatMoney(value, currency)} />
              <Tooltip formatter={(value, name) => [formatMoney(Number(value ?? 0), currency), String(name)]} />
              <Legend />
              <Bar dataKey="deposits" name={t('resellers.deposit')} fill="#52c41a" radius={[3, 3, 0, 0]} />
              <Bar dataKey="withdrawals" name={t('resellers.withdraw')} fill="#fa8c16" radius={[3, 3, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}
    </Card>
  );
}
