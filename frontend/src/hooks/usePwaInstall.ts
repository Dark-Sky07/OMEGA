import { useCallback, useEffect, useState } from 'react';

interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>;
}

let deferredPromptGlobal: BeforeInstallPromptEvent | null = null;
let swRegistered = false;

export function registerPwaServiceWorker() {
  if (swRegistered || typeof window === 'undefined' || !('serviceWorker' in navigator)) {
    return;
  }
  const isSecure =
    window.location.protocol === 'https:' ||
    window.location.hostname === 'localhost' ||
    window.location.hostname === '127.0.0.1';
  if (!isSecure) return;
  swRegistered = true;
  const basePath = (window as { X_UI_BASE_PATH?: string }).X_UI_BASE_PATH || '/';
  const normalizedBase = basePath.endsWith('/') ? basePath : `${basePath}/`;
  navigator.serviceWorker
    .register(`${normalizedBase}sw.js`, { scope: normalizedBase })
    .catch(() => {});
}

export function usePwaInstall() {
  const [installable, setInstallable] = useState<boolean>(() => !!deferredPromptGlobal);

  useEffect(() => {
    registerPwaServiceWorker();

    const onBeforeInstallPrompt = (e: Event) => {
      e.preventDefault();
      deferredPromptGlobal = e as BeforeInstallPromptEvent;
      setInstallable(true);
    };
    const onAppInstalled = () => {
      deferredPromptGlobal = null;
      setInstallable(false);
    };

    window.addEventListener('beforeinstallprompt', onBeforeInstallPrompt);
    window.addEventListener('appinstalled', onAppInstalled);
    return () => {
      window.removeEventListener('beforeinstallprompt', onBeforeInstallPrompt);
      window.removeEventListener('appinstalled', onAppInstalled);
    };
  }, []);

  const promptInstall = useCallback(async () => {
    if (!deferredPromptGlobal) return false;
    await deferredPromptGlobal.prompt();
    const choice = await deferredPromptGlobal.userChoice;
    if (choice.outcome === 'accepted') {
      deferredPromptGlobal = null;
      setInstallable(false);
      return true;
    }
    return false;
  }, []);

  return {
    installable,
    promptInstall,
  };
}
