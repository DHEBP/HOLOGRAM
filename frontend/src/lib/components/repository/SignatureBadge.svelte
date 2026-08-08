<script context="module">
  // The single source of truth for how a TELA author signature is described.
  //
  // Two viewers show these states (the repository file tree and the version
  // history panel). Two copies of this vocabulary would drift, and the drift
  // would be an honesty bug rather than a cosmetic one, so both import from
  // here.
  export const SIG_LABEL = {
    verified: 'signed',
    anonymous: 'author private',
    unsigned: 'unsigned',
    unverified: 'did not verify',
    index: 'nested index',
    unreadable: 'unreadable'
  };

  export const SIG_TITLE = {
    verified: 'The recorded author signed exactly the file this contract carries.',
    anonymous: 'Signed, but published with a ring size above 2, so the chain recorded no author address to check against. A privacy choice, not a fault.',
    unsigned: 'No signature was stored for this file.',
    unverified: 'A signature is stored and does not match the file. This can mean the file changed after signing, or that its original bytes cannot be recovered.',
    index: 'A nested INDEX rather than a single document — this is how TELA shards a large file. Signatures live on its own entries.',
    unreadable: 'This contract parses as neither a TELA INDEX nor a DOC, so nothing can be said about it.'
  };

  export function shortSigner(addr) {
    if (!addr) return '';
    return addr.length > 20 ? `${addr.slice(0, 10)}…${addr.slice(-6)}` : addr;
  }
</script>

<script>
  export let state = '';
  export let signer = '';
  export let pending = false;
  export let showSigner = false;

  $: label = SIG_LABEL[state] || state;
  $: title = SIG_TITLE[state] || '';
</script>

{#if pending}
  <span class="sig-chip sig-pending">checking…</span>
{:else if state}
  <span class="sig-chip sig-{state}" {title}>{label}</span>
  {#if showSigner && signer}
    <span class="sig-signer" title={signer}>{shortSigner(signer)}</span>
  {/if}
{/if}

<style>
  .sig-chip {
    flex-shrink: 0;
    font-family: var(--font-mono);
    font-size: 10px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    padding: 1px var(--s-2);
    border-radius: var(--r-sm);
    border: 1px solid transparent;
    white-space: nowrap;
    cursor: help;
  }

  .sig-verified {
    color: var(--status-ok);
    border-color: var(--status-ok);
  }

  /* Anonymous is a privacy choice, not a fault - it reads neutral, never as a
     warning. Rendering it like a failure would punish the stronger option.
     A nested index and an unreadable contract are not signature outcomes at
     all, so they stay neutral for the same reason. */
  .sig-anonymous,
  .sig-unsigned,
  .sig-index,
  .sig-unreadable {
    color: var(--text-4);
    border-color: var(--border-dim);
  }

  /* Warn, not error. A stored signature that does not match can also mean the
     original bytes are unrecoverable, so this must not read as proven forgery.
     Red stays reserved for true alarms. */
  .sig-unverified {
    color: var(--status-warn);
    border-color: var(--status-warn);
  }

  .sig-pending {
    color: var(--text-4);
    border-color: var(--border-dim);
    opacity: 0.6;
  }

  .sig-signer {
    flex-shrink: 0;
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-4);
  }
</style>
