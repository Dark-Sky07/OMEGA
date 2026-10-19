import { describe, expect, it } from 'vitest';
import { filterPaletteItems, type PaletteItem } from '../components/commandPaletteFilter';

const noop = () => {};

const SAMPLE: PaletteItem[] = [
  { id: '1', group: 'pages', title: 'Dashboard', keywords: 'home status', onSelect: noop },
  { id: '2', group: 'pages', title: 'Settings → Security', keywords: '2fa token password', onSelect: noop },
  { id: '3', group: 'inbounds', title: 'vless-reality', subtitle: ':443', badge: 'VLESS', onSelect: noop },
];

describe('filterPaletteItems', () => {
  it('returns all items when query is empty or whitespace', () => {
    expect(filterPaletteItems(SAMPLE, '')).toHaveLength(3);
    expect(filterPaletteItems(SAMPLE, '   ')).toHaveLength(3);
  });

  it('matches case-insensitively across title, subtitle, badge, and keywords', () => {
    expect(filterPaletteItems(SAMPLE, '2FA').map((i) => i.id)).toEqual(['2']);
    expect(filterPaletteItems(SAMPLE, '443 vless').map((i) => i.id)).toEqual(['3']);
    expect(filterPaletteItems(SAMPLE, 'nonexistent')).toEqual([]);
  });
});
