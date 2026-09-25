import type { KeyboardEvent } from 'react';

/** Make a non-button interactive element keyboard-activatable. */
export function activateOnKey(handler: () => void) {
  return (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    handler();
  };
}
