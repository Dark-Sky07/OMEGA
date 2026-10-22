export interface CurrencyOption {
  value: string;
  label: string;
}

// Keep in sync with internal/util/currency. IRT is retained only to make old
// installations editable; it is not an ISO 4217 currency code.
export const CURRENCY_OPTIONS: CurrencyOption[] = [
  ['USD', 'US Dollar'], ['EUR', 'Euro'], ['GBP', 'Pound sterling'], ['JPY', 'Japanese yen'],
  ['CNY', 'Chinese yuan'], ['INR', 'Indian rupee'], ['CAD', 'Canadian dollar'], ['AUD', 'Australian dollar'],
  ['NZD', 'New Zealand dollar'], ['CHF', 'Swiss franc'], ['SEK', 'Swedish krona'], ['NOK', 'Norwegian krone'],
  ['DKK', 'Danish krone'], ['PLN', 'Polish złoty'], ['CZK', 'Czech koruna'], ['HUF', 'Hungarian forint'],
  ['RON', 'Romanian leu'], ['TRY', 'Turkish lira'], ['RUB', 'Russian ruble'], ['UAH', 'Ukrainian hryvnia'],
  ['IRR', 'Iranian rial'], ['AED', 'UAE dirham'], ['SAR', 'Saudi riyal'], ['QAR', 'Qatari riyal'],
  ['KWD', 'Kuwaiti dinar'], ['BHD', 'Bahraini dinar'], ['OMR', 'Omani rial'], ['JOD', 'Jordanian dinar'],
  ['ILS', 'Israeli new shekel'], ['BRL', 'Brazilian real'], ['MXN', 'Mexican peso'], ['ARS', 'Argentine peso'],
  ['ZAR', 'South African rand'], ['SGD', 'Singapore dollar'], ['HKD', 'Hong Kong dollar'], ['TWD', 'New Taiwan dollar'],
  ['KRW', 'South Korean won'], ['IDR', 'Indonesian rupiah'], ['VND', 'Vietnamese đồng'], ['THB', 'Thai baht'],
  ['PHP', 'Philippine peso'], ['MYR', 'Malaysian ringgit'], ['PKR', 'Pakistani rupee'], ['EGP', 'Egyptian pound'],
  ['NGN', 'Nigerian naira'], ['KES', 'Kenyan shilling'], ['MAD', 'Moroccan dirham'], ['COP', 'Colombian peso'],
  ['CLP', 'Chilean peso'], ['TND', 'Tunisian dinar'], ['LYD', 'Libyan dinar'],
].map(([value, name]) => ({ value, label: `${value} — ${name}` }));

export const LEGACY_CURRENCY_OPTIONS: CurrencyOption[] = [
  { value: 'IRT', label: 'IRT — legacy custom label (use IRR for Iranian rial)' },
];

const ZERO_DIGIT_CURRENCIES = new Set(['JPY', 'KRW', 'VND', 'CLP']);
const THREE_DIGIT_CURRENCIES = new Set(['KWD', 'BHD', 'OMR', 'JOD', 'TND', 'LYD']);

export function currencyFractionDigits(code?: string): number {
  const normalized = (code || 'USD').trim().toUpperCase();
  if (ZERO_DIGIT_CURRENCIES.has(normalized)) return 0;
  if (THREE_DIGIT_CURRENCIES.has(normalized)) return 3;
  return 2;
}

export function roundCurrencyAmount(value: number, code?: string): number {
  const factor = 10 ** currencyFractionDigits(code);
  return Math.round((value + Number.EPSILON) * factor) / factor;
}
