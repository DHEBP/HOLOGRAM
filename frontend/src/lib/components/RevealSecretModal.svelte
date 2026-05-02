<script>
  // RevealSecretModal: Engram-shaped reveal flow for the wallet's recovery seed
  // and secret/public keys.
  //
  // Security invariants (matches Engram's pattern + a few extras):
  //   1. Decrypted material lives ONLY inside this component's local lets.
  //      When the parent unmounts the modal (close, wallet change, wallet close,
  //      route change, ESC), the binding goes out of scope and is GC'd.
  //   2. Password is required on every reveal. There is no sticky "revealed"
  //      state that survives navigating away and back.
  //   3. The wallet handle is checked server-side on every call; we never trust
  //      a previous reveal.
  //   4. Auto-hide after AUTO_HIDE_MS so an unattended screen does not leak.
  //   5. Clipboard auto-clear after CLIPBOARD_CLEAR_MS on copy.
  //
  // The component is intentionally self-contained. The parent only sets `show`
  // and `kind`; everything else (password, secret, timers) lives here.

  import { createEventDispatcher, onDestroy, tick } from 'svelte';
  import { GetSeedPhrase, GetWalletKeys, ClipboardClearIf } from '../../../wailsjs/go/main/App.js';
  import { toast, handleBackendError } from '../stores/appState.js';
  import PasswordInput from './PasswordInput.svelte';
  import { AlertTriangle, Copy, Loader2, Lock, Shield, X, Eye } from 'lucide-svelte';

  export let show = false;
  /** @type {'seed' | 'keys'} */
  export let kind = 'seed';

  const dispatch = createEventDispatcher();

  const AUTO_HIDE_MS = 60_000;
  const CLIPBOARD_CLEAR_MS = 30_000;
  const TICK_MS = 1_000;

  // === LOCAL STATE — the only place decrypted material exists in the UI ===
  let password = '';
  let loading = false;
  let error = null;
  let revealed = false;
  let seed = '';
  let secretKey = '';
  let publicKey = '';

  // Auto-hide countdown
  let autoHideTimer = null;
  let autoHideTickTimer = null;
  let autoHideRemainingMs = AUTO_HIDE_MS;

  // Clipboard auto-clear scheduling
  /** @type {{ timer: any, value: string } | null} */
  let clipboardScrub = null;

  $: title = kind === 'seed' ? 'Recovery Seed' : 'Wallet Keys';
  $: lockSubtitle =
    kind === 'seed'
      ? 'Enter your wallet password to view your recovery seed phrase'
      : 'Enter your wallet password to view your secret and public keys';

  function clearAutoHide() {
    if (autoHideTimer) { clearTimeout(autoHideTimer); autoHideTimer = null; }
    if (autoHideTickTimer) { clearInterval(autoHideTickTimer); autoHideTickTimer = null; }
  }

  function startAutoHide() {
    clearAutoHide();
    autoHideRemainingMs = AUTO_HIDE_MS;
    const startedAt = Date.now();
    autoHideTickTimer = setInterval(() => {
      autoHideRemainingMs = Math.max(0, AUTO_HIDE_MS - (Date.now() - startedAt));
    }, TICK_MS);
    autoHideTimer = setTimeout(() => {
      toast.info('Auto-hidden after 60s of inactivity');
      close('auto-hide');
    }, AUTO_HIDE_MS);
  }

  function scrubLocalSecrets(reason) {
    seed = '';
    secretKey = '';
    publicKey = '';
    password = '';
    revealed = false;
    error = null;
    loading = false;
    clearAutoHide();
    autoHideRemainingMs = AUTO_HIDE_MS;
  }

  async function close(reason = 'user') {
    scrubLocalSecrets(reason);
    show = false;
    dispatch('close', { reason });
  }

  // Parent toggles `show`. When it goes false-from-true, scrub immediately.
  // When it goes true-from-false, ensure we start with a clean slate.
  let prevShow = false;
  $: if (show !== prevShow) {
    if (show) {
      scrubLocalSecrets('open');
      // Focus password on next tick so the field exists in the DOM
      tick().then(() => {
        const el = document.querySelector('.reveal-modal .password-input input');
        if (el) el.focus();
      });
    } else {
      scrubLocalSecrets('hidden-by-parent');
    }
    prevShow = show;
  }

  async function reveal() {
    if (!password.trim() || loading) return;
    loading = true;
    error = null;
    try {
      const result = kind === 'seed'
        ? await GetSeedPhrase(password)
        : await GetWalletKeys(password);

      // Scrub the password we just used regardless of outcome
      password = '';

      if (result?.success) {
        if (kind === 'seed') {
          seed = result.seed || '';
        } else {
          secretKey = result.secretKey || '';
          publicKey = result.publicKey || '';
        }
        revealed = true;
        startAutoHide();
      } else {
        error = handleBackendError(result, { showToast: false }) ||
          (kind === 'seed' ? 'Failed to retrieve seed phrase' : 'Failed to retrieve wallet keys');
      }
    } catch (err) {
      console.error('[RevealSecretModal] reveal error:', err);
      error = err.message || 'Unexpected error';
    } finally {
      loading = false;
    }
  }

  async function copyAndScheduleClear(text, label) {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      toast.success(`${label} (clipboard auto-clears in 30s)`, 2500);
    } catch (e) {
      toast.error('Failed to copy to clipboard');
      return;
    }
    if (clipboardScrub?.timer) clearTimeout(clipboardScrub.timer);
    const value = text;
    clipboardScrub = {
      value,
      timer: setTimeout(async () => {
        // Browser clipboard API requires a user-gesture context, which is
        // gone 30s after the click. Route the conditional clear through Go,
        // which talks to the native pasteboard directly with no such
        // restriction. The Go side reads first and only clears if the value
        // still matches what we placed, so we never clobber user content.
        try {
          const result = await ClipboardClearIf(value);
          if (result?.cleared) {
            toast.info('Clipboard cleared');
          }
        } catch (e) {
          console.error('[RevealSecretModal] clipboard scrub failed:', e);
        }
        clipboardScrub = null;
      }, CLIPBOARD_CLEAR_MS),
    };
  }

  function handleKeydown(e) {
    if (!show) return;
    if (e.key === 'Escape') close('escape');
    if (e.key === 'Enter' && !revealed && !loading && password.trim()) reveal();
  }

  onDestroy(() => {
    scrubLocalSecrets('destroy');
    if (clipboardScrub?.timer) {
      clearTimeout(clipboardScrub.timer);
      clipboardScrub = null;
    }
  });

  $: secondsRemaining = Math.ceil(autoHideRemainingMs / 1000);
</script>

<svelte:window on:keydown={handleKeydown} />

{#if show}
  <div class="modal-overlay reveal-modal" on:click|self={() => close('overlay-click')}>
    <div class="modal-content reveal-content">
      <div class="modal-header">
        <div class="modal-title">
          {#if kind === 'seed'}
            <Eye size={18} />
            <span>{title}</span>
          {:else}
            <Shield size={18} />
            <span>{title}</span>
          {/if}
        </div>
        <button class="modal-close" on:click={() => close('close-button')} title="Close">
          <X size={18} />
        </button>
      </div>

      <div class="modal-body">
        {#if !revealed}
          <div class="lock-banner">
            <Lock size={16} />
            <span>{lockSubtitle}</span>
          </div>

          {#if kind === 'keys'}
            <div class="critical-banner">
              <AlertTriangle size={16} />
              <div>
                <strong>CRITICAL:</strong> Your secret key provides full control over your wallet. Never share it with anyone.
              </div>
            </div>
          {/if}

          <div class="form-group">
            <label class="form-label">Wallet Password</label>
            <div class="password-input">
              <PasswordInput bind:value={password} placeholder="Enter wallet password" />
            </div>
          </div>

          {#if error}
            <div class="alert alert-error">
              <AlertTriangle size={14} />
              <span>{error}</span>
            </div>
          {/if}
        {:else}
          <div class="auto-hide-pill" title="Auto-hides for your protection">
            <Lock size={12} />
            <span>Auto-hides in {secondsRemaining}s</span>
          </div>

          {#if kind === 'seed'}
            <div class="seed-header">
              <AlertTriangle size={28} class="seed-warning-icon" />
              <h2 class="seed-title">Your Recovery Seed</h2>
              <p class="seed-subtitle">Write down these 25 words in order. This is the ONLY way to recover your wallet.</p>
            </div>

            <div class="seed-grid">
              {#each seed.split(' ') as word, i}
                <div class="seed-word">
                  <span class="seed-num">{i + 1}</span>
                  <span class="seed-text">{word}</span>
                </div>
              {/each}
            </div>

            <div class="seed-warnings">
              <div class="warning-item">
                <AlertTriangle size={14} />
                <span>NEVER share your seed with anyone</span>
              </div>
              <div class="warning-item">
                <AlertTriangle size={14} />
                <span>Hologram will NEVER ask for your seed</span>
              </div>
              <div class="warning-item">
                <AlertTriangle size={14} />
                <span>Store this offline in a safe place</span>
              </div>
            </div>
          {:else}
            <div class="key-section">
              <div class="key-header">
                <span class="key-label">SECRET KEY</span>
                <span class="key-warning-badge">CRITICAL</span>
              </div>
              <div class="key-value-box">
                <code class="key-value mono">{secretKey}</code>
              </div>
              <button class="btn btn-secondary btn-sm" on:click={() => copyAndScheduleClear(secretKey, 'Secret key copied')}>
                <Copy size={14} />
                Copy Secret Key
              </button>
              <div class="key-warning-text">
                <AlertTriangle size={14} />
                <span>This key provides full wallet control. Keep it secure and never share it.</span>
              </div>
            </div>

            <div class="key-separator"></div>

            <div class="key-section">
              <div class="key-header">
                <span class="key-label">PUBLIC KEY</span>
              </div>
              <div class="key-value-box">
                <code class="key-value mono">{publicKey}</code>
              </div>
              <button class="btn btn-secondary btn-sm" on:click={() => copyAndScheduleClear(publicKey, 'Public key copied')}>
                <Copy size={14} />
                Copy Public Key
              </button>
              <div class="key-info-text">
                <span>Public key can be shared safely. It's used to verify signatures.</span>
              </div>
            </div>
          {/if}
        {/if}
      </div>

      <div class="modal-footer">
        {#if !revealed}
          <button class="btn btn-ghost" on:click={() => close('cancel')}>Cancel</button>
          <button class="btn btn-primary" disabled={loading || !password.trim()} on:click={reveal}>
            {#if loading}
              <Loader2 size={14} class="spin" />
              Verifying...
            {:else}
              View {kind === 'seed' ? 'Seed Phrase' : 'Keys'}
            {/if}
          </button>
        {:else}
          {#if kind === 'seed'}
            <button class="btn btn-secondary" on:click={() => copyAndScheduleClear(seed, 'Seed phrase copied')}>
              <Copy size={14} />
              Copy Seed Phrase
            </button>
          {/if}
          <button class="btn btn-primary" on:click={() => close('hide')}>
            <Lock size={14} />
            Hide
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .reveal-content {
    max-width: 560px;
    width: calc(100vw - 48px);
  }

  .lock-banner {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    border: 1px solid var(--border-warning, #b8860b);
    background: rgba(184, 134, 11, 0.08);
    color: var(--text-warning, #ffb84d);
    border-radius: 6px;
    font-size: 13px;
    margin-bottom: 12px;
  }

  .critical-banner {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 10px 12px;
    border: 1px solid var(--border-danger, #b22222);
    background: rgba(178, 34, 34, 0.08);
    color: var(--text-danger, #ff6666);
    border-radius: 6px;
    font-size: 13px;
    margin-bottom: 12px;
    line-height: 1.4;
  }

  .auto-hide-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    align-self: flex-end;
    padding: 4px 10px;
    border: 1px solid var(--border-subtle, #2b3340);
    border-radius: 999px;
    font-size: 11px;
    color: var(--text-muted, #8a96a8);
    margin-bottom: 8px;
    margin-left: auto;
  }

  .seed-header {
    text-align: center;
    margin-bottom: 12px;
  }

  .seed-title {
    margin: 6px 0 4px;
    font-size: 18px;
    font-weight: 700;
  }

  .seed-subtitle {
    margin: 0;
    font-size: 12px;
    color: var(--text-muted, #8a96a8);
  }

  .seed-grid {
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    gap: 6px;
    margin-bottom: 12px;
  }

  .seed-word {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 8px;
    background: rgba(19, 25, 34, 0.85);
    border: 1px solid var(--border-subtle, #2b3340);
    border-radius: 4px;
    font-size: 12px;
  }

  .seed-num {
    color: var(--text-muted, #8a96a8);
    font-size: 10px;
    min-width: 18px;
    text-align: right;
    font-variant-numeric: tabular-nums;
  }

  .seed-text {
    font-weight: 600;
    font-family: var(--font-mono);
  }

  .seed-warnings {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 8px;
  }

  .warning-item {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--text-muted, #8a96a8);
  }

  .key-section {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-bottom: 12px;
  }

  .key-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 11px;
    letter-spacing: 0.06em;
    color: var(--text-muted, #8a96a8);
  }

  .key-warning-badge {
    padding: 2px 6px;
    border-radius: 3px;
    background: rgba(178, 34, 34, 0.15);
    color: var(--text-danger, #ff6666);
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.08em;
  }

  .key-value-box {
    padding: 10px;
    border: 1px solid var(--border-subtle, #2b3340);
    border-radius: 4px;
    background: rgba(19, 25, 34, 0.85);
    word-break: break-all;
  }

  .key-value {
    font-family: var(--font-mono);
    font-size: 12px;
  }

  .key-warning-text,
  .key-info-text {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
    color: var(--text-muted, #8a96a8);
  }

  .key-separator {
    height: 1px;
    background: var(--border-subtle, #2b3340);
    margin: 4px 0 12px;
  }

  .mono {
    font-family: var(--font-mono);
    font-size: 12px;
  }

  :global(.reveal-modal .modal-body) {
    display: flex;
    flex-direction: column;
  }
</style>
