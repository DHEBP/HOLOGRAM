<script>
  import { createEventDispatcher, onDestroy } from 'svelte';
  import { PreviewTELAFork, ForkTELA } from '../../../../wailsjs/go/main/App.js';
  import { ClipboardSetText } from '../../../../wailsjs/runtime/runtime.js';
  import { toast } from '../../stores/appState.js';
  import { Icons } from '../holo';

  export let show = false;
  export let scid = '';
  export let source = null; // the GetINDEXInfo result the view already loaded

  const dispatch = createEventDispatcher();

  let durl = '';
  let name = '';
  let description = '';
  let iconUrl = '';

  // The preview is the SAME code path the install uses, so the cost shown is
  // measured over the arguments that would actually be broadcast.
  let preview = null;
  let previewError = '';
  let previewing = false;

  let forking = false;
  let result = null;

  let seeded = false;
  let previewTimer = null;
  let previewToken = 0;

  // Seeded once per opening. The dURL is left blank on purpose: the backend
  // fills it with its own suggestion, so the rule that a fork must not reuse the
  // original's dURL has exactly one definition.
  $: if (show && !seeded) {
    seeded = true;
    durl = '';
    name = source?.name || '';
    description = source?.description || '';
    iconUrl = source?.icon || '';
    result = null;
    previewError = '';
    preview = null;
    runPreview();
  }

  $: if (!show && seeded) {
    seeded = false;
    clearTimeout(previewTimer);
  }

  function request() {
    return JSON.stringify({
      sourceScid: scid,
      durl,
      name,
      description,
      iconUrl
    });
  }

  async function runPreview() {
    const token = ++previewToken;
    previewing = true;
    try {
      const res = await PreviewTELAFork(request());
      if (token !== previewToken) return;
      if (res?.success) {
        preview = res;
        previewError = '';
        // Only ever adopt the backend's dURL while the field is untouched, or
        // typing would fight the response of the request it triggered.
        if (!durl && res.durl) durl = res.durl;
      } else {
        preview = null;
        previewError = res?.error || 'This repository cannot be forked.';
      }
    } catch (error) {
      if (token !== previewToken) return;
      preview = null;
      previewError = 'Could not work out what this fork would install.';
    } finally {
      if (token === previewToken) previewing = false;
    }
  }

  function schedulePreview() {
    clearTimeout(previewTimer);
    previewTimer = setTimeout(runPreview, 350);
  }

  async function confirm() {
    if (forking || !canFork) return;
    forking = true;
    try {
      const res = await ForkTELA(request());
      if (res?.success) {
        result = res;
      } else {
        toast.error(res?.error || 'Fork failed');
      }
    } catch (error) {
      toast.error('Fork failed');
    } finally {
      forking = false;
    }
  }

  async function copyTxid() {
    try {
      await ClipboardSetText(result.txid);
      toast.success('SCID copied');
    } catch {
      toast.error('Could not copy');
    }
  }

  function close() {
    clearTimeout(previewTimer);
    dispatch('close');
  }

  function onBackdrop(event) {
    if (event.target === event.currentTarget) close();
  }

  function onKeydown(event) {
    if (show && event.key === 'Escape') close();
  }

  onDestroy(() => clearTimeout(previewTimer));

  $: docCount = preview?.docCount ?? source?.docs?.length ?? 0;
  $: walletReady = preview?.walletReady === true;
  $: canFork = !!preview && !preview.tooLarge && !previewError && !forking && walletReady;

  // "could not be measured" states only what is known. storageGasFor returns
  // no answer for a missing daemon connection, an RPC error, a result that is
  // not a map AND a reply with no gasstorage key — only one of those is the
  // node failing to answer. When the request never reached the node at all,
  // the warn note above already carries the real reason and this row says
  // nothing rather than inventing a second one.
  $: costLabel = previewError
    ? '—'
    : preview?.costMeasured
      ? `${preview.storageGas.toLocaleString()} storage gas · about ${preview.storageGasDero.toFixed(5)} DERO`
      : 'could not be measured';
</script>

<svelte:window on:keydown={onKeydown} />

{#if show}
  <div class="fork-backdrop" on:click={onBackdrop}>
    <div class="fork-panel">
      <div class="fork-head">
        <div class="fork-head-title">
          <Icons name="git-branch" size={15} />
          <span>{result ? 'Fork submitted' : 'Fork this repository'}</span>
        </div>
        <button type="button" class="fork-btn icon" title="Close" on:click={close}>
          <Icons name="close" size={14} />
        </button>
      </div>

      <div class="fork-body">
        {#if result}
          <p class="fork-result-lead">
            One new INDEX contract was broadcast. It exists once the transaction is
            mined.
          </p>
          <dl class="fork-facts">
            <div class="fork-fact">
              <dt>SCID</dt>
              <dd>
                <button type="button" class="fork-copy" title={`${result.txid} — click to copy`} on:click={copyTxid}>
                  {result.txid}
                </button>
              </dd>
            </div>
            <div class="fork-fact">
              <dt>dURL</dt>
              <dd class="mono">{result.durl}</dd>
            </div>
            <div class="fork-fact">
              <dt>Documents</dt>
              <dd>{result.docCount} — the same contracts the original lists</dd>
            </div>
          </dl>
        {:else}
          <!-- What a fork actually is. Every line here is a property of the
               mechanism, not a reassurance. -->
          <ul class="fork-truths">
            <li>
              <strong>{docCount} {docCount === 1 ? 'file stays' : 'files stay'} on the original contracts.</strong>
              Nothing is copied and nothing is redeployed — the fork is one new
              contract that lists the same documents.
            </li>
            <li>
              <strong>Author signatures survive.</strong>
              A signature is checked against the document's own contract, so every
              file in the fork still verifies to whoever wrote it, not to you.
            </li>
            <li>
              <strong>You become the owner.</strong>
              You can update the fork. You cannot update the original, and the
              original's owner cannot update the fork.
            </li>
            <li>
              <strong>Your wallet address is recorded, publicly.</strong>
              A fork installs at ring size {preview?.ringsize ?? 2}, which is what lets you
              own and update it — the contract stores the signing address as its
              owner and anyone can read it. A larger ring would hide you and store
              "anon" instead, but then the fork could never be updated by anyone.
            </li>
            <li>
              <strong>Ratings do not carry over.</strong>
              The fork starts at zero likes and zero dislikes, with no history.
            </li>
            <li>
              <strong>No link back is recorded.</strong>
              TELA has no field for "this INDEX forks that one", so nothing on
              chain will say where the fork came from. Say it in the description
              if you want it known.
            </li>
          </ul>

          <div class="fork-fields">
            <label class="fork-field">
              <span class="fork-label">dURL</span>
              <input
                type="text"
                bind:value={durl}
                on:input={schedulePreview}
                spellcheck="false"
                autocomplete="off"
                placeholder="the address this fork answers to"
              />
              <span class="fork-hint">
                Must differ from the original's
                <code>{source?.durl || '—'}</code>. Two INDEXes on one dURL compete
                for the same address and only one of them can be cloned.
              </span>
            </label>

            <label class="fork-field">
              <span class="fork-label">Name</span>
              <input type="text" bind:value={name} on:input={schedulePreview} spellcheck="false" />
            </label>

            <label class="fork-field">
              <span class="fork-label">Description</span>
              <textarea rows="3" bind:value={description} on:input={schedulePreview}></textarea>
            </label>

            <label class="fork-field">
              <span class="fork-label">Icon URL</span>
              <input type="text" bind:value={iconUrl} on:input={schedulePreview} spellcheck="false" />
            </label>
          </div>

          {#if previewError}
            <div class="fork-note warn">{previewError}</div>
          {:else if preview?.tooLarge}
            <div class="fork-note warn">{preview.costWarning}</div>
          {:else if preview && !walletReady}
            <!-- Stated here rather than as a toast after the click: the install
                 signs a transaction, so with no wallet open there is nothing to
                 sign with and the form cannot go anywhere. walletReady comes
                 from the same lookup the install performs, so it is right in
                 simulator mode too. -->
            <div class="fork-note warn">
              No wallet is open, so nothing can sign this install. Open one, then
              reopen this panel.
            </div>
          {/if}

          <div class="fork-cost">
            <span class="fork-cost-label">Installs 1 contract</span>
            <span class="fork-cost-value" class:pending={previewing}>{costLabel}</span>
          </div>
          {#if preview && !preview.costMeasured}
            <p class="fork-note">
              The cost could not be measured, so this will be funded from the
              estimate taken when the transaction is built.
            </p>
          {/if}
          {#if preview?.mods}
            <p class="fork-note">
              Carries the original's modules: <code>{preview.mods}</code>
            </p>
          {/if}
        {/if}
      </div>

      <div class="fork-foot">
        {#if result}
          <button type="button" class="fork-btn primary" on:click={close}>Done</button>
        {:else}
          <button type="button" class="fork-btn" on:click={close}>Cancel</button>
          <button type="button" class="fork-btn primary" on:click={confirm} disabled={!canFork}>
            {forking ? 'Broadcasting…' : 'Install fork'}
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  /* Below the toast layer (1000) on purpose: this panel reports its own failures
     through toasts, and at the same z-index DOM order would decide whether the
     message was visible. */
  .fork-backdrop {
    position: fixed;
    inset: 0;
    z-index: 900;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--s-6);
  }

  .fork-panel {
    width: 100%;
    max-width: 560px;
    max-height: 88vh;
    display: flex;
    flex-direction: column;
    background: var(--void-base);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-sm);
    overflow: hidden;
  }

  /* Same rectangular strip as the repository header. */
  .fork-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-4);
    padding: var(--s-3) var(--s-5);
    background: var(--void-mid);
    border-bottom: 1px solid var(--border-dim);
    flex-shrink: 0;
  }

  .fork-head-title {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    font-family: var(--font-mono);
    font-size: 13px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-1);
  }

  .fork-body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: var(--s-5);
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
  }

  .fork-truths {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
    font-size: 12.5px;
    line-height: 1.6;
    color: var(--text-3);
  }

  .fork-truths li {
    padding-left: var(--s-4);
    border-left: 1px solid var(--border-dim);
  }

  .fork-truths strong {
    display: block;
    color: var(--text-2);
    font-weight: 600;
  }

  .fork-fields {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
  }

  .fork-field {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }

  .fork-label {
    font-family: var(--font-mono);
    font-size: 10px;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--text-4);
  }

  .fork-field input,
  .fork-field textarea {
    width: 100%;
    padding: var(--s-2) var(--s-3);
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-1);
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-sm);
    resize: vertical;
  }

  .fork-field input:focus,
  .fork-field textarea:focus {
    outline: none;
    border-color: var(--border-accent);
  }

  .fork-hint {
    font-size: 11px;
    line-height: 1.5;
    color: var(--text-5);
  }

  code {
    font-family: var(--font-mono);
    color: var(--cyan-400);
  }

  .fork-cost {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--s-4);
    padding: var(--s-3);
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-sm);
  }

  .fork-cost-label {
    font-family: var(--font-mono);
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-4);
  }

  .fork-cost-value {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-2);
    text-align: right;
  }

  .fork-cost-value.pending {
    opacity: 0.5;
  }

  .fork-note {
    margin: 0;
    font-size: 11.5px;
    line-height: 1.5;
    color: var(--text-4);
  }

  /* Warn, not error. A refused fork is a size or naming problem, not an alarm. */
  .fork-note.warn {
    padding: var(--s-2) var(--s-3);
    color: var(--status-warn);
    border: 1px solid rgba(251, 191, 36, 0.4);
    border-radius: var(--r-sm);
  }

  .fork-result-lead {
    margin: 0;
    font-size: 12.5px;
    line-height: 1.6;
    color: var(--text-3);
  }

  .fork-facts {
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }

  .fork-fact {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }

  .fork-fact dt {
    font-family: var(--font-mono);
    font-size: 10px;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--text-4);
  }

  .fork-fact dd {
    margin: 0;
    font-size: 12px;
    color: var(--text-2);
    word-break: break-all;
  }

  .fork-fact dd.mono {
    font-family: var(--font-mono);
    color: var(--cyan-400);
  }

  .fork-copy {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-2);
    background: transparent;
    border: none;
    border-bottom: 1px dashed var(--border-default);
    padding: 0;
    cursor: pointer;
    word-break: break-all;
    text-align: left;
  }

  .fork-copy:hover {
    color: var(--cyan-400);
  }

  .fork-foot {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--s-2);
    padding: var(--s-3) var(--s-5);
    background: var(--void-mid);
    border-top: 1px solid var(--border-dim);
    flex-shrink: 0;
  }

  .fork-btn {
    display: inline-flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-4);
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-3);
    background: var(--void-up);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-sm);
    cursor: pointer;
    transition: all var(--dur-fast) ease;
    white-space: nowrap;
  }

  .fork-btn:hover:not(:disabled) {
    background: var(--void-surface);
    border-color: var(--border-accent);
    color: var(--text-1);
  }

  .fork-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .fork-btn.icon {
    padding: var(--s-2);
  }

  .fork-btn.primary {
    color: var(--cyan-400);
    border-color: var(--border-accent);
  }
</style>
