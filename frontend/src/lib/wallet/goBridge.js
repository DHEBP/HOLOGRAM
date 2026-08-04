/**
 * goBridge.js — thin adapter over the generated Wails App bindings for wallet ops.
 * Signatures verified against wallet.go. Secrets are pass-through only; nothing
 * is logged or persisted here.
 */
import * as App from '../../../wailsjs/go/main/App.js';

function ok(res) {
  if (res == null) return { ok: false, message: 'No response from backend' };
  if (typeof res === 'object') {
    if (res.error) return { ok: false, message: String(res.error) };
    if (res.success === false) return { ok: false, message: res.message || 'Operation failed' };
  }
  return { ok: true, ...(typeof res === 'object' ? res : {}) };
}

export const goBridge = {
  available: () => typeof App?.OpenWallet === 'function',

  async isWalletOpen() {
    try { return !!(await App.IsWalletOpen()); } catch { return false; }
  },

  async getAddress() {
    try {
      const r = await App.GetAddress();
      return (r && (r.address ?? r.Address)) || null;
    } catch { return null; }
  },

  async selectWalletFile() {
    try { return (await App.SelectWalletFile()) || null; } catch { return null; }
  },

  async openWallet(filePath, password) {
    try {
      const r = ok(await App.OpenWallet(filePath, password ?? ''));
      if (r.ok && !r.address) r.address = await this.getAddress();
      return r;
    } catch (e) { return { ok: false, message: e?.message || 'Open failed' }; }
  },

  async createWallet(filePath, password) {
    try {
      const r = ok(await App.CreateWallet(filePath, password ?? ''));
      if (r.ok && !r.address) r.address = await this.getAddress();
      return r;
    } catch (e) { return { ok: false, message: e?.message || 'Create failed' }; }
  },

  async restoreWallet(filePath, password, seed) {
    try {
      const r = ok(await App.RestoreWallet(filePath, password ?? '', seed ?? ''));
      if (r.ok && !r.address) r.address = await this.getAddress();
      return r;
    } catch (e) { return { ok: false, message: e?.message || 'Restore failed' }; }
  },

  async connectExternal() {
    try {
      const r = ok(await App.ConnectXSWD());
      if (r.ok && !r.address) r.address = await this.getAddress();
      return r;
    } catch (e) { return { ok: false, message: e?.message || 'XSWD connect failed' }; }
  },

  async getCurrentWalletPath() {
    try { return (await App.GetCurrentWalletPath()) || null; } catch { return null; }
  },
};
