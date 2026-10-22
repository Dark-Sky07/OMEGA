import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { theme as antdTheme } from 'antd';
import type { ThemeConfig } from 'antd';

const STORAGE_DARK = 'dark-mode';
const STORAGE_ULTRA = 'isUltraDarkThemeEnabled';
const BRAND_ACCENTS: Record<string, string> = { ocean: '#1677FF', forest: '#1C9A68', sunset: '#D9485F' };

function validBrandTheme(value: string): string {
  return ['default', ...Object.keys(BRAND_ACCENTS)].includes(value) ? value : 'default';
}

function validBrandColor(value: string): string {
  return /^#[0-9a-fA-F]{6}$/.test(value) ? value.toUpperCase() : '';
}

function rgba(hex: string, alpha: number): string {
  const color = validBrandColor(hex);
  if (!color) return `rgba(232, 80, 2, ${alpha})`;
  const r = Number.parseInt(color.slice(1, 3), 16);
  const g = Number.parseInt(color.slice(3, 5), 16);
  const b = Number.parseInt(color.slice(5, 7), 16);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

function readBool(key: string, fallback: boolean): boolean {
  const raw = localStorage.getItem(key);
  if (raw === null) return fallback;
  return raw === 'true';
}

function applyDom(isDark: boolean, isUltra: boolean) {
  document.body.setAttribute('class', isDark ? 'dark' : 'light');
  if (isUltra) {
    document.documentElement.setAttribute('data-theme', 'ultra-dark');
  } else {
    document.documentElement.removeAttribute('data-theme');
  }
  const msg = document.getElementById('message');
  if (msg) msg.className = isDark ? 'dark' : 'light';
}

// module load so the document is in the right theme before React mounts.
const initialDark = readBool(STORAGE_DARK, true);
const initialUltra = readBool(STORAGE_ULTRA, false);
applyDom(initialDark, initialUltra);

// ---------------------------------------------------------------------------
// PHOENIX "Solar Flame & Obsidian Ash" (ققنوس) theme — antd token layer.
// The CSS side of the same palette lives in styles/theme.css; keep the two in
// sync when touching colours. Fonts are self-hosted (see main.tsx) and reach
// antd through the CSS variables declared there.
// ---------------------------------------------------------------------------

const FONT_SANS = 'var(--font-sans)';
const FONT_MONO = 'var(--font-mono)';

const SHARED_TOKENS = {
  fontFamily: FONT_SANS,
  fontFamilyCode: FONT_MONO,
  fontSize: 14,
  fontSizeHeading1: 34,
  fontSizeHeading2: 26,
  fontSizeHeading3: 21,
  fontSizeHeading4: 17,
  fontSizeHeading5: 15,
  lineHeight: 1.55,
  borderRadius: 12,
  borderRadiusLG: 16,
  borderRadiusSM: 9,
  borderRadiusXS: 7,
  controlHeight: 36,
  controlHeightLG: 44,
  controlHeightSM: 28,
  controlOutlineWidth: 3,
  motionDurationMid: '0.18s',
  motionDurationSlow: '0.26s',
  wireframe: false,
};

const LIGHT_TOKENS = {
  ...SHARED_TOKENS,
  colorPrimary: '#e85002',
  colorInfo: '#d97706',
  colorSuccess: '#16a34a',
  colorWarning: '#d97706',
  colorError: '#dc2626',
  colorLink: '#c73e00',
  colorBgBase: '#faf5f0',
  colorBgLayout: 'transparent',
  colorBgContainer: 'rgba(255, 252, 248, 0.82)',
  colorBgElevated: '#ffffff',
  colorBgSpotlight: '#1a0f0a',
  colorBorder: 'rgba(67, 20, 7, 0.14)',
  colorBorderSecondary: 'rgba(67, 20, 7, 0.08)',
  colorText: '#1c1008',
  colorTextSecondary: 'rgba(28, 16, 8, 0.7)',
  colorTextTertiary: 'rgba(28, 16, 8, 0.48)',
  colorTextQuaternary: 'rgba(28, 16, 8, 0.3)',
  colorFill: 'rgba(232, 80, 2, 0.08)',
  colorFillSecondary: 'rgba(232, 80, 2, 0.06)',
  colorFillTertiary: 'rgba(232, 80, 2, 0.04)',
  colorFillQuaternary: 'rgba(232, 80, 2, 0.025)',
  controlOutline: 'rgba(232, 80, 2, 0.22)',
  boxShadow: '0 1px 2px rgba(67, 20, 7, 0.05), 0 18px 40px -24px rgba(180, 50, 0, 0.22)',
  boxShadowSecondary: '0 2px 4px rgba(67, 20, 7, 0.06), 0 24px 48px -24px rgba(180, 50, 0, 0.28)',
};

const DARK_TOKENS = {
  ...SHARED_TOKENS,
  colorPrimary: '#ff6a00',
  colorInfo: '#fbbf24',
  colorSuccess: '#34d399',
  colorWarning: '#fbbf24',
  colorError: '#fb7185',
  colorLink: '#ff9e2c',
  colorBgBase: '#0b0708',
  colorBgLayout: 'transparent',
  colorBgContainer: 'rgba(24, 14, 17, 0.68)',
  colorBgElevated: '#170d10',
  colorBgSpotlight: '#241317',
  colorBorder: 'rgba(255, 148, 92, 0.24)',
  colorBorderSecondary: 'rgba(255, 148, 92, 0.14)',
  colorText: '#f9f1ec',
  colorTextSecondary: 'rgba(249, 241, 236, 0.72)',
  colorTextTertiary: 'rgba(249, 241, 236, 0.48)',
  colorTextQuaternary: 'rgba(249, 241, 236, 0.28)',
  colorFill: 'rgba(255, 140, 66, 0.12)',
  colorFillSecondary: 'rgba(255, 140, 66, 0.08)',
  colorFillTertiary: 'rgba(255, 140, 66, 0.05)',
  colorFillQuaternary: 'rgba(255, 140, 66, 0.03)',
  controlOutline: 'rgba(255, 106, 0, 0.3)',
  boxShadow: '0 1px 2px rgba(0, 0, 0, 0.45), 0 24px 48px -28px rgba(0, 0, 0, 0.78)',
  boxShadowSecondary: '0 2px 4px rgba(0, 0, 0, 0.5), 0 32px 56px -28px rgba(0, 0, 0, 0.85)',
};

const ULTRA_DARK_TOKENS = {
  ...DARK_TOKENS,
  colorBgBase: '#030203',
  colorBgContainer: 'rgba(14, 9, 11, 0.78)',
  colorBgElevated: '#0d080a',
  colorBgSpotlight: '#1a0e12',
  colorBorderSecondary: 'rgba(255, 140, 66, 0.12)',
};

/** Opaque container colour per theme — fixed table columns must not ghost. */
const SOLID_SURFACE = {
  light: '#fffcf8',
  dark: '#140c0f',
  ultra: '#0b0708',
};

const STATISTIC_TOKENS = {
  contentFontSize: 20,
  titleFontSize: 12,
};

function buildComponents(mode: 'light' | 'dark' | 'ultra', brandPrimary?: string): ThemeConfig['components'] {
  const isDark = mode !== 'light';
  const solid = SOLID_SURFACE[mode];
  const primary = brandPrimary || (isDark ? '#ff6a00' : '#e85002');
  const accentHover = rgba(primary, isDark ? 0.08 : 0.06);
  const accentSelected = rgba(primary, isDark ? 0.2 : 0.12);
  const muted = isDark ? 'rgba(249, 241, 236, 0.74)' : 'rgba(28, 16, 8, 0.7)';
  return {
    Layout: {
      bodyBg: 'transparent',
      headerBg: 'transparent',
      footerBg: 'transparent',
      siderBg: 'transparent',
      triggerBg: 'transparent',
      triggerColor: muted,
      lightSiderBg: 'transparent',
      lightTriggerBg: 'transparent',
      lightTriggerColor: muted,
    },
    Menu: {
      itemBorderRadius: 12,
      itemMarginInline: 10,
      itemMarginBlock: 3,
      itemHeight: 42,
      iconSize: 17,
      collapsedIconSize: 19,
      iconMarginInlineEnd: 12,
      subMenuItemBorderRadius: 10,
      itemBg: 'transparent',
      subMenuItemBg: 'transparent',
      popupBg: isDark ? '#170d10' : '#ffffff',
      itemColor: muted,
      itemHoverBg: accentHover,
      itemSelectedBg: accentSelected,
      itemSelectedColor: isDark ? '#ffffff' : '#b83700',
      darkItemBg: 'transparent',
      darkSubMenuItemBg: 'transparent',
      darkPopupBg: '#170d10',
      darkItemColor: muted,
      darkItemHoverBg: accentHover,
      darkItemSelectedBg: accentSelected,
      darkItemSelectedColor: '#ffffff',
      groupTitleColor: isDark ? 'rgba(249, 241, 236, 0.42)' : 'rgba(28, 16, 8, 0.44)',
      groupTitleFontSize: 11,
    },
    Card: {
      headerBg: 'transparent',
      headerFontSize: 15,
      headerHeight: 52,
      paddingLG: 20,
      colorBorderSecondary: isDark ? 'rgba(255, 148, 92, 0.14)' : 'rgba(67, 20, 7, 0.09)',
    },
    Table: {
      colorBgContainer: solid,
      headerBg: rgba(primary, 0.04),
      headerColor: isDark ? 'rgba(249, 241, 236, 0.54)' : 'rgba(28, 16, 8, 0.54)',
      headerSplitColor: 'transparent',
      rowHoverBg: rgba(primary, isDark ? 0.09 : 0.06),
      rowSelectedBg: rgba(primary, isDark ? 0.14 : 0.1),
      rowSelectedHoverBg: rgba(primary, isDark ? 0.18 : 0.14),
      borderColor: isDark ? 'rgba(255, 148, 92, 0.13)' : 'rgba(67, 20, 7, 0.08)',
      cellPaddingBlock: 12,
      cellPaddingInline: 14,
    },
    Button: {
      fontWeight: 600,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      defaultBg: isDark ? 'rgba(255, 255, 255, 0.04)' : 'rgba(255, 255, 255, 0.78)',
      defaultHoverBg: isDark ? 'rgba(255, 122, 24, 0.1)' : '#ffffff',
    },
    Input: {
      activeShadow: `0 0 0 3px ${rgba(primary, isDark ? 0.28 : 0.2)}`,
      hoverBorderColor: primary,
      activeBorderColor: primary,
    },
    InputNumber: {
      activeShadow: `0 0 0 3px ${rgba(primary, isDark ? 0.28 : 0.2)}`,
    },
    Select: {
      optionSelectedBg: accentSelected,
      optionActiveBg: accentHover,
    },
    Modal: {
      contentBg: isDark ? (mode === 'ultra' ? 'rgba(12, 7, 9, 0.96)' : 'rgba(20, 12, 15, 0.95)') : 'rgba(255, 252, 248, 0.97)',
      headerBg: 'transparent',
      footerBg: 'transparent',
      titleFontSize: 18,
      titleLineHeight: 1.3,
    },
    Drawer: {
      colorBgElevated: isDark ? (mode === 'ultra' ? '#080506' : '#120a0d') : '#faf5f0',
    },
    Tabs: {
      itemColor: muted,
      itemSelectedColor: isDark ? '#ffffff' : '#1c1008',
      itemHoverColor: isDark ? '#ffffff' : '#1c1008',
      inkBarColor: primary,
      titleFontSize: 14,
      horizontalItemGutter: 24,
    },
    Tag: {
      defaultBg: isDark ? 'rgba(255, 140, 66, 0.08)' : 'rgba(67, 20, 7, 0.05)',
    },
    Tooltip: {
      colorBgSpotlight: isDark ? '#241317' : '#1a0f0a',
    },
    Popover: {
      titleMinWidth: 160,
    },
    Statistic: STATISTIC_TOKENS,
    Progress: {
      remainingColor: isDark ? 'rgba(255, 255, 255, 0.08)' : 'rgba(67, 20, 7, 0.08)',
    },
    Segmented: {
      itemSelectedBg: isDark ? 'rgba(255, 106, 0, 0.26)' : '#ffffff',
      itemSelectedColor: isDark ? '#ffffff' : '#1c1008',
      trackBg: isDark ? 'rgba(255, 255, 255, 0.05)' : 'rgba(67, 20, 7, 0.05)',
    },
    Switch: {
      trackHeight: 22,
      handleSize: 18,
    },
    Pagination: {
      itemActiveBg: 'transparent',
    },
    Alert: {
      withDescriptionPadding: '14px 16px',
    },
    Form: {
      labelColor: muted,
      labelFontSize: 13,
      verticalLabelPadding: '0 0 6px',
    },
    Descriptions: {
      labelBg: isDark ? 'rgba(255, 255, 255, 0.03)' : 'rgba(15, 23, 42, 0.03)',
    },
  };
}

const LIGHT_CONFIG: ThemeConfig = {
  algorithm: antdTheme.defaultAlgorithm,
  token: LIGHT_TOKENS,
  components: buildComponents('light'),
};

const DARK_CONFIG: ThemeConfig = {
  algorithm: antdTheme.darkAlgorithm,
  token: DARK_TOKENS,
  components: buildComponents('dark'),
};

const ULTRA_DARK_CONFIG: ThemeConfig = {
  algorithm: antdTheme.darkAlgorithm,
  token: ULTRA_DARK_TOKENS,
  components: buildComponents('ultra'),
};

export function buildAntdThemeConfig(isDark: boolean, isUltra: boolean, brandPrimary?: string): ThemeConfig {
  const mode = !isDark ? 'light' : isUltra ? 'ultra' : 'dark';
  const base = mode === 'light' ? LIGHT_CONFIG : mode === 'ultra' ? ULTRA_DARK_CONFIG : DARK_CONFIG;
  if (!brandPrimary) return base;
  return {
    ...base,
    token: { ...base.token, colorPrimary: brandPrimary, colorLink: brandPrimary },
    components: buildComponents(mode, brandPrimary),
  };
}

export function pauseAnimationsUntilLeave(elementId: string): void {
  document.documentElement.setAttribute('data-theme-animations', 'off');
  const el = document.getElementById(elementId);
  if (!el) return;
  const restore = () => {
    document.documentElement.removeAttribute('data-theme-animations');
    el.removeEventListener('mouseleave', restore);
    el.removeEventListener('touchend', restore);
  };
  el.addEventListener('mouseleave', restore);
  el.addEventListener('touchend', restore);
}

interface ThemeContextValue {
  isDark: boolean;
  isUltra: boolean;
  toggleTheme: () => void;
  toggleUltra: () => void;
  antdThemeConfig: ThemeConfig;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [isDark, setIsDark] = useState<boolean>(initialDark);
  const [isUltra, setIsUltra] = useState<boolean>(initialUltra);
  const [brandTheme, setBrandTheme] = useState('default');
  const [brandColor, setBrandColor] = useState('');

  useEffect(() => {
    const onBrandingChange = (event: Event) => {
      const detail = (event as CustomEvent<{ theme?: string; color?: string }>).detail || {};
      const theme = validBrandTheme(detail.theme || 'default');
      const color = validBrandColor(detail.color || '');
      setBrandTheme(theme);
      setBrandColor(color);
    };
    window.addEventListener('phoenix-branding-change', onBrandingChange);
    return () => window.removeEventListener('phoenix-branding-change', onBrandingChange);
  }, []);

  useEffect(() => {
    applyDom(isDark, isUltra);
    localStorage.setItem(STORAGE_DARK, String(isDark));
    localStorage.setItem(STORAGE_ULTRA, String(isUltra));
  }, [isDark, isUltra]);

  const toggleTheme = useCallback(() => setIsDark((v) => !v), []);
  const toggleUltra = useCallback(() => setIsUltra((v) => !v), []);

  const brandPrimary = brandColor || BRAND_ACCENTS[brandTheme];
  const antdThemeConfig = useMemo(() => buildAntdThemeConfig(isDark, isUltra, brandPrimary), [isDark, isUltra, brandPrimary]);

  const value = useMemo<ThemeContextValue>(
    () => ({ isDark, isUltra, toggleTheme, toggleUltra, antdThemeConfig }),
    [isDark, isUltra, toggleTheme, toggleUltra, antdThemeConfig],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used inside <ThemeProvider>');
  return ctx;
}
