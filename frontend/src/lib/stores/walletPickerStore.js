import { writable, derived, get } from 'svelte/store';
import { settingsState, saveSetting } from './appState.js';

/**
 * walletPickerStore — visibility + interaction state for the launch wallet picker.
 *
 * Persistence piggybacks on the first-class `walletPickerOnLaunch` setting
 * (settingsKeyMap: 'wallet_picker_on_launch'), so it loads with loadSettings()
 * and saves via saveSetting() like every other preference. Default is true:
 * the picker appears on every launch until the user explicitly opts out.
 */
function createWalletPicker() {
  const openStore = writable(false);
  const busyStore = writable(false);
  const errorStore = writable('');

  const api = {
    isOpen: openStore,
    busy: busyStore,
    error: errorStore,

    /** Call once after waitForWails() + loadSettings() so the setting is live. */
    init() { api.maybeShow(); },

    /**
     * Show on every launch until the user explicitly opts out.
     * Deliberately NOT gated on wallet-open state: the picker should offer the
     * existing wallet(s) to select/switch even when one is already connected.
     */
    maybeShow() {
      const enabled = get(settingsState).walletPickerOnLaunch !== false;
      openStore.set(enabled);
    },

    /** Re-evaluate (e.g. when a wallet opens/closes elsewhere). */
    sync() { api.maybeShow(); },

    /** Persist "Don't show again" (wallet_picker_on_launch = false). */
    async dismissForever() {
      openStore.set(false);
      try { await saveSetting('walletPickerOnLaunch', false); } catch { /* non-fatal */ }
    },

    dismissOnce() { openStore.set(false); },
    open() { openStore.set(true); },
    close() { openStore.set(false); },

    setBusy(v) { busyStore.set(!!v); },
    setError(msg) { errorStore.set(msg || ''); },
  };

  return api;
}

export const walletPicker = createWalletPicker();

// Raw stores for direct `$store` subscription in components.
// (walletPicker is a plain API object — subscribing to it with `$walletPicker`
//  would crash with "subscribe is not a function".)
export const walletPickerBusy = walletPicker.busy;
export const walletPickerError = walletPicker.error;

/** True only while the modal should render. */
export const walletPickerVisible = derived(
  [walletPicker.isOpen, settingsState],
  ([$open, $settings]) => $open && $settings.walletPickerOnLaunch !== false
);
