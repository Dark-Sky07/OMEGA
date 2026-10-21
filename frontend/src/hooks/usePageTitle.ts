import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

import { useSession } from '@/api/queries/useSession';

const TITLE_KEYS: Record<string, string> = {
  '/': 'menu.dashboard',
  '/inbounds': 'menu.inbounds',
  '/clients': 'menu.clients',
  '/groups': 'menu.groups',
  '/nodes': 'menu.nodes',
  '/settings': 'menu.settings',
  '/xray': 'menu.xray',
  '/api-docs': 'menu.apiDocs',
  '/resellers': 'menu.resellers',
  '/reseller/report': 'menu.report',
  '/reseller/profile': 'menu.profile',
  '/reseller/renewals': 'resellers.renewalCenter',
};

function safeBrandLogo(value?: string): string | undefined {
  const logo = value?.trim();
  if (!logo || logo.length > 2048 || /[\\\r\n]/.test(logo)) return undefined;
  try {
    if (logo.startsWith('/')) {
      if (logo.startsWith('//')) return undefined;
      return new URL(logo, window.location.origin).origin === window.location.origin ? logo : undefined;
    }
    const parsed = new URL(logo);
    return parsed.protocol === 'https:' && parsed.hostname && !parsed.username && !parsed.password ? logo : undefined;
  } catch {
    return undefined;
  }
}

function makeBrandFavicon(name: string, color?: string): string {
  const accent = color && /^#[0-9a-fA-F]{6}$/.test(color) ? color : '#1677ff';
  const initial = Array.from(name.trim())[0]?.toUpperCase() || 'R';
  const escapedInitial = initial.replace(/[&<>"']/g, (character) => `&#${character.charCodeAt(0)};`);
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64"><rect width="64" height="64" rx="14" fill="${accent}"/><text x="32" y="44" text-anchor="middle" font-family="Arial,sans-serif" font-size="40" font-weight="700" fill="#fff">${escapedInitial}</text></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

export function usePageTitle() {
  const { pathname } = useLocation();
  const { t } = useTranslation();
  const { isReseller, session } = useSession();
  const brand = session?.stat?.reseller;
  const brandName = brand?.brandName?.trim() || '';
  const brandLogo = brand?.brandLogo?.trim() || '';
  const brandColor = brand?.brandColor?.trim() || '';
  const displayBrandName = brandName || session?.name?.trim() || '';
  const hasCustomBrand = isReseller && Boolean(brandName || brandLogo);

  useEffect(() => {
    const key = TITLE_KEYS[pathname];
    const pageTitle = key ? t(key) : 'PHOENIX';
    const title = hasCustomBrand && displayBrandName ? `${displayBrandName} - ${pageTitle}` : pageTitle;
    const host = window.location.hostname;
    document.title = host ? `${host} - ${title}` : title;

    const favicon = document.querySelector<HTMLLinkElement>('link[rel~="icon"]');
    if (!favicon) return;
    if (!favicon.dataset.defaultHref) {
      favicon.dataset.defaultHref = favicon.getAttribute('href') || '/phoenix-icon.png';
    }
    if (hasCustomBrand) {
      favicon.href = safeBrandLogo(brandLogo) || makeBrandFavicon(displayBrandName, brandColor);
    } else {
      favicon.href = favicon.dataset.defaultHref;
    }
  }, [
    pathname,
    t,
    hasCustomBrand,
    displayBrandName,
    brandLogo,
    brandColor,
  ]);
}
