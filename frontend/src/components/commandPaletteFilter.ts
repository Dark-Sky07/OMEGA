export interface PaletteItem {
  id: string;
  group: 'pages' | 'clients' | 'inbounds' | 'actions';
  title: string;
  subtitle?: string;
  badge?: string;
  keywords?: string;
  onSelect: () => void;
}

/**
 * Pure filter helper: case-insensitive multi-token substring match across
 * title, subtitle, badge, and keywords.
 */
export function filterPaletteItems(items: PaletteItem[], rawQuery: string): PaletteItem[] {
  const q = rawQuery.trim().toLowerCase();
  if (!q) return items;
  const tokens = q.split(/\s+/).filter(Boolean);
  return items.filter((item) => {
    const hay = `${item.title} ${item.subtitle || ''} ${item.badge || ''} ${item.keywords || ''}`.toLowerCase();
    return tokens.every((tok) => hay.includes(tok));
  });
}
