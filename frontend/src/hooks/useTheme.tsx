import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { theme as antdTheme } from 'antd';
import type { ThemeConfig } from 'antd';

const STORAGE_DARK = 'dark-mode';
const STORAGE_ULTRA = 'isUltraDarkThemeEnabled';

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
// OMEGA "Aurora" theme — antd token layer.
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
  colorPrimary: '#6a4df6',
  colorInfo: '#2563eb',
  colorSuccess: '#16a34a',
  colorWarning: '#d97706',
  colorError: '#e11d48',
  colorLink: '#5a3ee6',
  colorBgBase: '#f7f8fc',
  colorBgLayout: 'transparent',
  colorBgContainer: 'rgba(255, 255, 255, 0.78)',
  colorBgElevated: '#ffffff',
  colorBgSpotlight: '#111827',
  colorBorder: 'rgba(15, 23, 42, 0.14)',
  colorBorderSecondary: 'rgba(15, 23, 42, 0.08)',
  colorText: '#0f172a',
  colorTextSecondary: 'rgba(15, 23, 42, 0.68)',
  colorTextTertiary: 'rgba(15, 23, 42, 0.46)',
  colorTextQuaternary: 'rgba(15, 23, 42, 0.3)',
  colorFill: 'rgba(15, 23, 42, 0.08)',
  colorFillSecondary: 'rgba(15, 23, 42, 0.06)',
  colorFillTertiary: 'rgba(15, 23, 42, 0.04)',
  colorFillQuaternary: 'rgba(15, 23, 42, 0.025)',
  controlOutline: 'rgba(106, 77, 246, 0.22)',
  boxShadow: '0 1px 2px rgba(15, 23, 42, 0.04), 0 18px 40px -24px rgba(15, 23, 42, 0.25)',
  boxShadowSecondary: '0 2px 4px rgba(15, 23, 42, 0.05), 0 24px 48px -24px rgba(15, 23, 42, 0.32)',
};

const DARK_TOKENS = {
  ...SHARED_TOKENS,
  colorPrimary: '#8b6cff',
  colorInfo: '#60a5fa',
  colorSuccess: '#34d399',
  colorWarning: '#fbbf24',
  colorError: '#fb7185',
  colorLink: '#a78bfa',
  colorBgBase: '#0b1020',
  colorBgLayout: 'transparent',
  colorBgContainer: 'rgba(17, 24, 39, 0.62)',
  colorBgElevated: '#111a2e',
  colorBgSpotlight: '#1b2440',
  colorBorder: 'rgba(148, 163, 184, 0.22)',
  colorBorderSecondary: 'rgba(148, 163, 184, 0.12)',
  colorText: '#e7ebf3',
  colorTextSecondary: 'rgba(231, 235, 243, 0.7)',
  colorTextTertiary: 'rgba(231, 235, 243, 0.46)',
  colorTextQuaternary: 'rgba(231, 235, 243, 0.28)',
  colorFill: 'rgba(255, 255, 255, 0.1)',
  colorFillSecondary: 'rgba(255, 255, 255, 0.07)',
  colorFillTertiary: 'rgba(255, 255, 255, 0.045)',
  colorFillQuaternary: 'rgba(255, 255, 255, 0.03)',
  controlOutline: 'rgba(139, 108, 255, 0.28)',
  boxShadow: '0 1px 2px rgba(0, 0, 0, 0.35), 0 24px 48px -28px rgba(0, 0, 0, 0.7)',
  boxShadowSecondary: '0 2px 4px rgba(0, 0, 0, 0.4), 0 32px 56px -28px rgba(0, 0, 0, 0.8)',
};

const ULTRA_DARK_TOKENS = {
  ...DARK_TOKENS,
  colorBgBase: '#000000',
  colorBgContainer: 'rgba(10, 12, 20, 0.72)',
  colorBgElevated: '#0a0d16',
  colorBgSpotlight: '#141a2b',
  colorBorderSecondary: 'rgba(148, 163, 184, 0.1)',
};

/** Opaque container colour per theme — fixed table columns must not ghost. */
const SOLID_SURFACE = {
  light: '#ffffff',
  dark: '#0f172a',
  ultra: '#0a0d16',
};

const STATISTIC_TOKENS = {
  contentFontSize: 20,
  titleFontSize: 12,
};

function buildComponents(mode: 'light' | 'dark' | 'ultra'): ThemeConfig['components'] {
  const isDark = mode !== 'light';
  const solid = SOLID_SURFACE[mode];
  const primary = isDark ? '#8b6cff' : '#6a4df6';
  const muted = isDark ? 'rgba(231, 235, 243, 0.72)' : 'rgba(15, 23, 42, 0.68)';
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
      popupBg: isDark ? '#111a2e' : '#ffffff',
      itemColor: muted,
      itemHoverBg: isDark ? 'rgba(255, 255, 255, 0.05)' : 'rgba(15, 23, 42, 0.05)',
      itemSelectedBg: isDark ? 'rgba(139, 108, 255, 0.18)' : 'rgba(106, 77, 246, 0.12)',
      itemSelectedColor: isDark ? '#ffffff' : '#4c33d6',
      darkItemBg: 'transparent',
      darkSubMenuItemBg: 'transparent',
      darkPopupBg: '#111a2e',
      darkItemColor: muted,
      darkItemHoverBg: 'rgba(255, 255, 255, 0.05)',
      darkItemSelectedBg: 'rgba(139, 108, 255, 0.18)',
      darkItemSelectedColor: '#ffffff',
      groupTitleColor: isDark ? 'rgba(231, 235, 243, 0.4)' : 'rgba(15, 23, 42, 0.42)',
      groupTitleFontSize: 11,
    },
    Card: {
      headerBg: 'transparent',
      headerFontSize: 15,
      headerHeight: 52,
      paddingLG: 20,
      colorBorderSecondary: isDark ? 'rgba(148, 163, 184, 0.12)' : 'rgba(15, 23, 42, 0.08)',
    },
    Table: {
      colorBgContainer: solid,
      headerBg: isDark ? 'rgba(255, 255, 255, 0.03)' : 'rgba(15, 23, 42, 0.03)',
      headerColor: isDark ? 'rgba(231, 235, 243, 0.5)' : 'rgba(15, 23, 42, 0.5)',
      headerSplitColor: 'transparent',
      rowHoverBg: isDark ? 'rgba(139, 108, 255, 0.08)' : 'rgba(106, 77, 246, 0.06)',
      rowSelectedBg: isDark ? 'rgba(139, 108, 255, 0.12)' : 'rgba(106, 77, 246, 0.1)',
      rowSelectedHoverBg: isDark ? 'rgba(139, 108, 255, 0.16)' : 'rgba(106, 77, 246, 0.14)',
      borderColor: isDark ? 'rgba(148, 163, 184, 0.12)' : 'rgba(15, 23, 42, 0.08)',
      cellPaddingBlock: 12,
      cellPaddingInline: 14,
    },
    Button: {
      fontWeight: 600,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      defaultBg: isDark ? 'rgba(255, 255, 255, 0.04)' : 'rgba(255, 255, 255, 0.7)',
      defaultHoverBg: isDark ? 'rgba(255, 255, 255, 0.08)' : '#ffffff',
    },
    Input: {
      activeShadow: `0 0 0 3px ${isDark ? 'rgba(139, 108, 255, 0.26)' : 'rgba(106, 77, 246, 0.2)'}`,
      hoverBorderColor: primary,
      activeBorderColor: primary,
    },
    InputNumber: {
      activeShadow: `0 0 0 3px ${isDark ? 'rgba(139, 108, 255, 0.26)' : 'rgba(106, 77, 246, 0.2)'}`,
    },
    Select: {
      optionSelectedBg: isDark ? 'rgba(139, 108, 255, 0.18)' : 'rgba(106, 77, 246, 0.12)',
      optionActiveBg: isDark ? 'rgba(255, 255, 255, 0.05)' : 'rgba(15, 23, 42, 0.05)',
    },
    Modal: {
      contentBg: isDark ? (mode === 'ultra' ? 'rgba(10, 13, 22, 0.94)' : 'rgba(15, 23, 42, 0.94)') : 'rgba(255, 255, 255, 0.96)',
      headerBg: 'transparent',
      footerBg: 'transparent',
      titleFontSize: 18,
      titleLineHeight: 1.3,
    },
    Drawer: {
      colorBgElevated: isDark ? (mode === 'ultra' ? '#070a12' : '#0c1324') : '#f7f8fc',
    },
    Tabs: {
      itemColor: muted,
      itemSelectedColor: isDark ? '#ffffff' : '#0f172a',
      itemHoverColor: isDark ? '#ffffff' : '#0f172a',
      inkBarColor: primary,
      titleFontSize: 14,
      horizontalItemGutter: 24,
    },
    Tag: {
      defaultBg: isDark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(15, 23, 42, 0.05)',
    },
    Tooltip: {
      colorBgSpotlight: isDark ? '#1b2440' : '#111827',
    },
    Popover: {
      titleMinWidth: 160,
    },
    Statistic: STATISTIC_TOKENS,
    Progress: {
      remainingColor: isDark ? 'rgba(255, 255, 255, 0.08)' : 'rgba(15, 23, 42, 0.07)',
    },
    Segmented: {
      itemSelectedBg: isDark ? 'rgba(139, 108, 255, 0.24)' : '#ffffff',
      itemSelectedColor: isDark ? '#ffffff' : '#0f172a',
      trackBg: isDark ? 'rgba(255, 255, 255, 0.05)' : 'rgba(15, 23, 42, 0.05)',
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

export function buildAntdThemeConfig(isDark: boolean, isUltra: boolean): ThemeConfig {
  if (!isDark) return LIGHT_CONFIG;
  return isUltra ? ULTRA_DARK_CONFIG : DARK_CONFIG;
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

  useEffect(() => {
    applyDom(isDark, isUltra);
    localStorage.setItem(STORAGE_DARK, String(isDark));
    localStorage.setItem(STORAGE_ULTRA, String(isUltra));
  }, [isDark, isUltra]);

  const toggleTheme = useCallback(() => setIsDark((v) => !v), []);
  const toggleUltra = useCallback(() => setIsUltra((v) => !v), []);

  const antdThemeConfig = useMemo(() => buildAntdThemeConfig(isDark, isUltra), [isDark, isUltra]);

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
