<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { walletState, toast, handleBackendError } from '../stores/appState.js';
  import { Loader2, RotateCcw, Save, Download, Upload, Paintbrush, Eraser } from 'lucide-svelte';
  import VillagerIdenticon from '../utils/villager-identicon.js';
  import { getVillagerSCID } from '../utils/avatarService.js';
  import { CallXSWD, InternalWalletCall } from '../../../wailsjs/go/main/App.js';
  
  const dispatch = createEventDispatcher();
  
  // ============================================
  // CONSTANTS
  // ============================================
  
  const GRID_SIZE = 24;
  const TOTAL_PIXELS = 576;
  const EMPTY_CHAR = 'z'; // Transparent
  
  // Official Villager 52-color palette (char -> ARGB)
  const PALETTE = {
    '0': { hex: '#FF9999', name: 'Light Red' },
    '1': { hex: '#FF6666', name: 'Red' },
    '2': { hex: '#FF0000', name: 'Pure Red' },
    '3': { hex: '#800000', name: 'Dark Red' },
    '4': { hex: '#FFA899', name: 'Light Orange' },
    '5': { hex: '#FF8C66', name: 'Orange' },
    '6': { hex: '#FF4500', name: 'OrangeRed' },
    '7': { hex: '#802200', name: 'Dark Orange' },
    '8': { hex: '#FFC799', name: 'Light Peach' },
    '9': { hex: '#FFB266', name: 'Peach' },
    'A': { hex: '#FF8C00', name: 'DarkOrange' },
    'B': { hex: '#804600', name: 'Brown' },
    'C': { hex: '#FFE099', name: 'Light Gold' },
    'D': { hex: '#FFD866', name: 'Gold' },
    'E': { hex: '#FFAA00', name: 'Amber' },
    'F': { hex: '#5C4033', name: 'Dark Brown' },
    'G': { hex: '#FFFF99', name: 'Light Yellow' },
    'H': { hex: '#FFFF66', name: 'Yellow' },
    'I': { hex: '#FFFF00', name: 'Pure Yellow' },
    'J': { hex: '#FFD700', name: 'Golden' },
    'K': { hex: '#CFFF99', name: 'Light Lime' },
    'L': { hex: '#BFFF66', name: 'Lime' },
    'M': { hex: '#80FF00', name: 'Chartreuse' },
    'N': { hex: '#408000', name: 'Dark Green' },
    'O': { hex: '#99FF99', name: 'Light Green' },
    'P': { hex: '#66FF66', name: 'Green' },
    'Q': { hex: '#00FF00', name: 'Pure Green' },
    'R': { hex: '#008000', name: 'Forest' },
    'S': { hex: '#99FFCF', name: 'Light Mint' },
    'T': { hex: '#66FFBF', name: 'Mint' },
    'U': { hex: '#00FF80', name: 'Spring' },
    'V': { hex: '#008040', name: 'Dark Spring' },
    'W': { hex: '#99FFFF', name: 'Light Cyan' },
    'X': { hex: '#66FFFF', name: 'Cyan' },
    'Y': { hex: '#00FFFF', name: 'Pure Cyan' },
    'Z': { hex: '#008080', name: 'Teal' },
    'a': { hex: '#99CFFF', name: 'Light Blue' },
    'b': { hex: '#66BFFF', name: 'Sky Blue' },
    'c': { hex: '#0080FF', name: 'Azure' },
    'd': { hex: '#004080', name: 'Dark Blue' },
    'e': { hex: '#9999FF', name: 'Light Indigo' },
    'f': { hex: '#6666FF', name: 'Indigo' },
    'g': { hex: '#0000FF', name: 'Pure Blue' },
    'h': { hex: '#000080', name: 'Navy' },
    'i': { hex: '#CF99FF', name: 'Light Violet' },
    'j': { hex: '#BF66FF', name: 'Violet' },
    'k': { hex: '#8000FF', name: 'Purple' },
    'l': { hex: '#400080', name: 'Dark Purple' },
    'm': { hex: '#FF99FF', name: 'Light Magenta' },
    'n': { hex: '#FF66FF', name: 'Magenta' },
    'o': { hex: '#FF00FF', name: 'Pure Magenta' },
    'p': { hex: '#800080', name: 'Dark Magenta' },
    'q': { hex: '#FF99C7', name: 'Light Pink' },
    'r': { hex: '#FF66B2', name: 'Pink' },
    's': { hex: '#FF0080', name: 'Hot Pink' },
    't': { hex: '#800040', name: 'Dark Pink' },
    'u': { hex: '#FFFFFF', name: 'White' },
    'v': { hex: '#B4B4B4', name: 'Light Gray' },
    'w': { hex: '#848484', name: 'Gray' },
    'x': { hex: '#434343', name: 'Dark Gray' },
    'y': { hex: '#000000', name: 'Black' },
    'z': { hex: 'transparent', name: 'Transparent' }
  };
  
  // Palette groups for organized display
  const PALETTE_GROUPS = [
    { name: 'Reds', chars: ['0', '1', '2', '3'] },
    { name: 'Oranges', chars: ['4', '5', '6', '7'] },
    { name: 'Peach/Brown', chars: ['8', '9', 'A', 'B'] },
    { name: 'Gold/Brown', chars: ['C', 'D', 'E', 'F'] },
    { name: 'Yellows', chars: ['G', 'H', 'I', 'J'] },
    { name: 'Lime/Green', chars: ['K', 'L', 'M', 'N'] },
    { name: 'Greens', chars: ['O', 'P', 'Q', 'R'] },
    { name: 'Mint/Spring', chars: ['S', 'T', 'U', 'V'] },
    { name: 'Cyans', chars: ['W', 'X', 'Y', 'Z'] },
    { name: 'Blues', chars: ['a', 'b', 'c', 'd'] },
    { name: 'Indigo', chars: ['e', 'f', 'g', 'h'] },
    { name: 'Violets', chars: ['i', 'j', 'k', 'l'] },
    { name: 'Magentas', chars: ['m', 'n', 'o', 'p'] },
    { name: 'Pinks', chars: ['q', 'r', 's', 't'] },
    { name: 'Grays', chars: ['u', 'v', 'w', 'x', 'y', 'z'] }
  ];
  
  // ============================================
  // STATE
  // ============================================
  
  let pixels = Array(TOTAL_PIXELS).fill(EMPTY_CHAR);
  let selectedColor = 'y'; // Default to black
  let tool = 'paint'; // 'paint' | 'erase'
  let isDrawing = false;
  
  let previewUrl = null;
  let previewLoading = false;
  
  let loading = false;
  let saving = false;
  let hasChanges = false;
  let originalPixels = null;
  
  // Developer donation fee (fetched from SC, in atomic units)
  let devFee = 10000; // Default: 10000 DERI = 0.0001 DERO
  const DEV_DONATE_ADDRESS = "dero1qyqqtsvggrfxtsz6p3yn49n26k83nnr50jmpnyqylykzju4wgl9yvqqdxdvse";
  
  // ============================================
  // LIFECYCLE
  // ============================================
  
  onMount(async () => {
    await loadFromSC();
  });
  
  // ============================================
  // PIXEL GRID FUNCTIONS
  // ============================================
  
  function getPixelIndex(x, y) {
    return x * GRID_SIZE + y;
  }
  
  function setPixel(x, y) {
    const idx = getPixelIndex(x, y);
    const newChar = tool === 'erase' ? EMPTY_CHAR : selectedColor;
    
    if (pixels[idx] !== newChar) {
      pixels[idx] = newChar;
      pixels = [...pixels]; // Trigger reactivity
      hasChanges = true;
      updatePreview();
    }
  }
  
  function handlePixelMouseDown(x, y, event) {
    event.preventDefault();
    isDrawing = true;
    setPixel(x, y);
  }
  
  function handlePixelMouseEnter(x, y) {
    if (isDrawing) {
      setPixel(x, y);
    }
  }
  
  function handleMouseUp() {
    isDrawing = false;
  }
  
  function clearCanvas() {
    pixels = Array(TOTAL_PIXELS).fill(EMPTY_CHAR);
    hasChanges = true;
    updatePreview();
  }
  
  function resetToOriginal() {
    if (originalPixels) {
      pixels = [...originalPixels];
      hasChanges = false;
      updatePreview();
    }
  }
  
  // ============================================
  // PREVIEW FUNCTIONS
  // ============================================
  
  async function updatePreview() {
    if (!$walletState.address) return;
    
    previewLoading = true;
    try {
      const artString = pixels.join('');
      
      // Revoke old URL
      if (previewUrl) {
        URL.revokeObjectURL(previewUrl);
      }
      
      previewUrl = await VillagerIdenticon.render($walletState.address, artString, 256);
    } catch (err) {
      console.error('Failed to render preview:', err);
    } finally {
      previewLoading = false;
    }
  }
  
  // ============================================
  // SC INTEGRATION
  // ============================================
  
  function hexToString(hex) {
    if (hex.length !== 1152 || !/^[0-9a-fA-F]{1152}$/.test(hex)) {
      return null;
    }
    let str = '';
    for (let i = 0; i < hex.length; i += 2) {
      str += String.fromCharCode(parseInt(hex.substr(i, 2), 16));
    }
    return str;
  }
  
  function stringToHex(str) {
    let hex = '';
    for (let i = 0; i < str.length; i++) {
      hex += str.charCodeAt(i).toString(16).padStart(2, '0');
    }
    return hex;
  }
  
  async function loadFromSC() {
    if (!$walletState.address) return;
    
    loading = true;
    try {
      const scid = getVillagerSCID();
      
      // Fetch avatar AND devFee in one call
      const response = await CallXSWD(JSON.stringify({
        jsonrpc: "2.0",
        id: 1,
        method: "DERO.GetSC",
        params: {
          scid: scid,
          keysstring: [`avatar_${$walletState.address}`, 'devFee']
        }
      }));
      
      // Extract devFee if available
      if (response?.result?.valuesuint64) {
        // devFee is the second key we requested
        const feeValues = response.result.valuesuint64;
        if (feeValues.length > 1 && feeValues[1] > 0) {
          devFee = feeValues[1];
        } else if (feeValues.length > 0 && feeValues[0] > 0) {
          // Might be in first position if avatar doesn't exist
          devFee = feeValues[0];
        }
      }
      
      if (response?.result?.valuesstring?.[0]) {
        let avatarStr = response.result.valuesstring[0];
        
        // SC stores raw 576-char string, but DERO returns strings as hex
        // Try to decode if it looks like hex
        if (avatarStr.length === 1152 && /^[0-9a-fA-F]+$/.test(avatarStr)) {
          avatarStr = hexToString(avatarStr);
        }
        
        if (avatarStr && avatarStr.length === 576) {
          pixels = avatarStr.split('');
          originalPixels = [...pixels];
          hasChanges = false;
          await updatePreview();
          toast.success('Avatar loaded from blockchain');
        } else {
          // Invalid avatar, use empty
          pixels = Array(TOTAL_PIXELS).fill(EMPTY_CHAR);
          originalPixels = [...pixels];
          hasChanges = false;
          await updatePreview();
        }
      } else {
        // No avatar stored, use empty
        pixels = Array(TOTAL_PIXELS).fill(EMPTY_CHAR);
        originalPixels = [...pixels];
        hasChanges = false;
        await updatePreview();
      }
    } catch (err) {
      console.error('Failed to load avatar from SC:', err);
      // Still update preview with empty canvas
      pixels = Array(TOTAL_PIXELS).fill(EMPTY_CHAR);
      originalPixels = [...pixels];
      await updatePreview();
    } finally {
      loading = false;
    }
  }
  
  async function checkRegistration() {
    try {
      const scid = getVillagerSCID();
      const response = await CallXSWD(JSON.stringify({
        jsonrpc: "2.0",
        id: 1,
        method: "DERO.GetSC",
        params: {
          scid: scid,
          keysstring: [`registered_${$walletState.address}`]
        }
      }));
      
      // If we get a value, user is registered
      return response?.result?.valuesstring?.[0] === "1" || 
             response?.result?.valuesuint64?.[0] === 1;
    } catch (err) {
      console.error('Failed to check registration:', err);
      return false;
    }
  }
  
  async function registerAccount() {
    const scid = getVillagerSCID();
    
    const response = await InternalWalletCall(
      "scinvoke",
      {
        scid: scid,
        entrypoint: "RegisterAccount",
        sc_rpc: []
      },
      ""
    );
    
    if (!response?.success) {
      throw new Error(response?.error || 'Registration failed');
    }
    
    return response;
  }
  
  async function saveToSC() {
    if (!$walletState.address || saving) return;
    
    saving = true;
    try {
      // Check if user is registered
      const isRegistered = await checkRegistration();
      
      if (!isRegistered) {
        toast.info('Registering your account first...');
        await registerAccount();
        // Wait a moment for the transaction to propagate
        await new Promise(resolve => setTimeout(resolve, 2000));
      }
      
      const artString = pixels.join('');
      const scid = getVillagerSCID();
      
      // The SC stores the raw 576-char string, NOT hex encoded
      // Based on villager2.bas: STRLEN(avatar) == 576
      // Include developer donation as burn transfer (SC forwards to dev address)
      const response = await InternalWalletCall(
        "scinvoke",
        {
          scid: scid,
          entrypoint: "StoreAvatar",
          transfers: [{
            destination: DEV_DONATE_ADDRESS,
            amount: 0,
            burn: devFee
          }],
          sc_rpc: [
            { name: "avatar", datatype: "S", value: artString }
          ]
        },
        ""
      );
      
      if (!response?.success) {
        throw new Error(response?.error || 'Transaction failed');
      }
      
      originalPixels = [...pixels];
      hasChanges = false;
      toast.success('Avatar saved to blockchain!');
      dispatch('saved', { txid: response?.txid });
      
    } catch (err) {
      console.error('Failed to save avatar:', err);
      handleBackendError(err, 'Failed to save avatar');
    } finally {
      saving = false;
    }
  }
  
  // ============================================
  // EXPORT/IMPORT
  // ============================================
  
  function exportArtString() {
    const artString = pixels.join('');
    navigator.clipboard.writeText(artString);
    toast.success('Art string copied to clipboard');
  }
  
  function importArtString() {
    const input = prompt('Paste your 576-character art string:');
    if (input && input.length === 576) {
      // Validate all characters are in palette
      const valid = input.split('').every(c => c in PALETTE);
      if (valid) {
        pixels = input.split('');
        hasChanges = true;
        updatePreview();
        toast.success('Art string imported');
      } else {
        toast.error('Invalid art string - contains unknown characters');
      }
    } else if (input) {
      toast.error('Art string must be exactly 576 characters');
    }
  }
  
  // ============================================
  // REACTIVE
  // ============================================
  
  $: if ($walletState.address && !previewUrl) {
    updatePreview();
  }
</script>

<svelte:window on:mouseup={handleMouseUp} />

<div class="avatar-editor">
  {#if loading}
    <div class="editor-loading">
      <Loader2 size={32} class="spin" />
      <span>Loading your avatar...</span>
    </div>
  {:else}
    <div class="editor-layout">
      <!-- Left: Canvas Grid -->
      <div class="editor-canvas-section">
        <div class="editor-toolbar">
          <div class="tool-group">
            <button 
              class="tool-btn" 
              class:active={tool === 'paint'}
              on:click={() => tool = 'paint'}
              title="Paint"
            >
              <Paintbrush size={16} />
            </button>
            <button 
              class="tool-btn" 
              class:active={tool === 'erase'}
              on:click={() => tool = 'erase'}
              title="Eraser"
            >
              <Eraser size={16} />
            </button>
          </div>
          
          <div class="tool-group">
            <button class="tool-btn" on:click={clearCanvas} title="Clear All">
              <RotateCcw size={16} />
            </button>
          </div>
          
          <div class="tool-group">
            <button class="tool-btn" on:click={exportArtString} title="Copy Art String">
              <Download size={16} />
            </button>
            <button class="tool-btn" on:click={importArtString} title="Paste Art String">
              <Upload size={16} />
            </button>
          </div>
        </div>
        
        <div class="pixel-grid" role="grid">
          {#each Array(GRID_SIZE) as _, x}
            {#each Array(GRID_SIZE) as _, y}
              {@const idx = getPixelIndex(x, y)}
              {@const char = pixels[idx]}
              {@const color = PALETTE[char]}
              <div
                class="pixel"
                class:transparent={char === 'z'}
                style="background-color: {color?.hex || 'transparent'}"
                role="gridcell"
                on:mousedown={(e) => handlePixelMouseDown(x, y, e)}
                on:mouseenter={() => handlePixelMouseEnter(x, y)}
              ></div>
            {/each}
          {/each}
        </div>
      </div>
      
      <!-- Middle: Preview -->
      <div class="editor-preview-section">
        <div class="preview-label">Preview</div>
        <div class="preview-container">
          {#if previewUrl}
            <img src={previewUrl} alt="Avatar Preview" class="preview-image" />
          {:else if previewLoading}
            <div class="preview-loading">
              <Loader2 size={24} class="spin" />
            </div>
          {:else}
            <div class="preview-empty">No preview</div>
          {/if}
        </div>
        
        <div class="preview-actions">
          <button 
            class="btn btn-primary"
            on:click={saveToSC}
            disabled={saving || !hasChanges}
          >
            {#if saving}
              <Loader2 size={16} class="spin" />
              Saving...
            {:else}
              <Save size={16} />
              Save to Blockchain
            {/if}
          </button>
          
          <div class="dev-donation-info">
            <span class="donation-label">Developer support:</span>
            <span class="donation-amount">{(devFee / 100000).toFixed(5)} DERO</span>
          </div>
          
          {#if hasChanges && originalPixels}
            <button class="btn btn-ghost" on:click={resetToOriginal}>
              Reset Changes
            </button>
          {/if}
        </div>
      </div>
      
      <!-- Right: Color Palette -->
      <div class="editor-palette-section">
        <div class="palette-label">Colors</div>
        <div class="palette-grid">
          {#each PALETTE_GROUPS as group}
            <div class="palette-group">
              {#each group.chars as char}
                {@const color = PALETTE[char]}
                <button
                  class="palette-swatch"
                  class:selected={selectedColor === char}
                  class:transparent={char === 'z'}
                  style="background-color: {color.hex}"
                  title="{color.name} ({char})"
                  on:click={() => { selectedColor = char; tool = 'paint'; }}
                ></button>
              {/each}
            </div>
          {/each}
        </div>
        
        <div class="selected-color-info">
          <div 
            class="selected-color-preview"
            class:transparent={selectedColor === 'z'}
            style="background-color: {PALETTE[selectedColor]?.hex}"
          ></div>
          <span class="selected-color-name">{PALETTE[selectedColor]?.name}</span>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .avatar-editor {
    width: 100%;
  }
  
  .editor-loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--s-3);
    padding: var(--s-8);
    color: var(--text-3);
  }
  
  .editor-layout {
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: var(--s-6);
    align-items: start;
  }
  
  /* Canvas Section */
  .editor-canvas-section {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  
  .editor-toolbar {
    display: flex;
    gap: var(--s-3);
    padding: var(--s-2);
    background: var(--void-deep);
    border-radius: var(--r-md);
    border: 1px solid var(--border-subtle);
  }
  
  .tool-group {
    display: flex;
    gap: var(--s-1);
  }
  
  .tool-group:not(:last-child) {
    padding-right: var(--s-3);
    border-right: 1px solid var(--border-subtle);
  }
  
  .tool-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--r-sm);
    color: var(--text-3);
    cursor: pointer;
    transition: all 0.15s ease;
  }
  
  .tool-btn:hover {
    background: var(--void-up);
    color: var(--text-1);
  }
  
  .tool-btn.active {
    background: var(--cyan-400);
    color: var(--void-pure);
    border-color: var(--cyan-400);
  }
  
  .pixel-grid {
    display: grid;
    grid-template-columns: repeat(24, 1fr);
    gap: 1px;
    background: var(--void-deep);
    border: 2px solid var(--border-default);
    border-radius: var(--r-md);
    padding: 2px;
    width: 360px;
    height: 360px;
    user-select: none;
  }
  
  .pixel {
    aspect-ratio: 1;
    cursor: crosshair;
    transition: opacity 0.1s;
  }
  
  .pixel:hover {
    opacity: 0.8;
  }
  
  .pixel.transparent {
    background-image: 
      linear-gradient(45deg, var(--void-up) 25%, transparent 25%),
      linear-gradient(-45deg, var(--void-up) 25%, transparent 25%),
      linear-gradient(45deg, transparent 75%, var(--void-up) 75%),
      linear-gradient(-45deg, transparent 75%, var(--void-up) 75%);
    background-size: 8px 8px;
    background-position: 0 0, 0 4px, 4px -4px, -4px 0px;
    background-color: var(--void-mid);
  }
  
  /* Preview Section */
  .editor-preview-section {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--s-4);
    padding: var(--s-4);
    background: var(--void-deep);
    border-radius: var(--r-lg);
    border: 1px solid var(--border-subtle);
  }
  
  .preview-label,
  .palette-label {
    font-family: var(--font-mono);
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-4);
  }
  
  .preview-container {
    width: 256px;
    height: 256px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--void-mid);
    border-radius: var(--r-md);
    overflow: hidden;
  }
  
  .preview-image {
    width: 100%;
    height: 100%;
    object-fit: contain;
  }
  
  .preview-loading,
  .preview-empty {
    color: var(--text-4);
    font-size: 12px;
  }
  
  .preview-actions {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    width: 100%;
  }
  
  .preview-actions .btn {
    width: 100%;
    justify-content: center;
  }
  
  .dev-donation-info {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--s-2) var(--s-3);
    background: var(--void-mid);
    border-radius: var(--r-sm);
    font-size: 11px;
  }
  
  .donation-label {
    color: var(--text-4);
  }
  
  .donation-amount {
    font-family: var(--font-mono);
    color: var(--emerald-400);
  }
  
  /* Palette Section */
  .editor-palette-section {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
    padding: var(--s-3);
    background: var(--void-deep);
    border-radius: var(--r-lg);
    border: 1px solid var(--border-subtle);
  }
  
  .palette-grid {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  
  .palette-group {
    display: flex;
    gap: 2px;
  }
  
  .palette-swatch {
    width: 24px;
    height: 24px;
    border: 2px solid transparent;
    border-radius: var(--r-xs);
    cursor: pointer;
    transition: all 0.15s ease;
  }
  
  .palette-swatch:hover {
    transform: scale(1.1);
    z-index: 1;
  }
  
  .palette-swatch.selected {
    border-color: var(--cyan-400);
    box-shadow: 0 0 0 2px var(--void-deep), 0 0 8px var(--cyan-400);
  }
  
  .palette-swatch.transparent {
    background-image: 
      linear-gradient(45deg, var(--void-up) 25%, transparent 25%),
      linear-gradient(-45deg, var(--void-up) 25%, transparent 25%),
      linear-gradient(45deg, transparent 75%, var(--void-up) 75%),
      linear-gradient(-45deg, transparent 75%, var(--void-up) 75%);
    background-size: 8px 8px;
    background-position: 0 0, 0 4px, 4px -4px, -4px 0px;
    background-color: var(--void-mid);
  }
  
  .selected-color-info {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding-top: var(--s-3);
    border-top: 1px solid var(--border-subtle);
  }
  
  .selected-color-preview {
    width: 32px;
    height: 32px;
    border-radius: var(--r-sm);
    border: 2px solid var(--cyan-400);
  }
  
  .selected-color-preview.transparent {
    background-image: 
      linear-gradient(45deg, var(--void-up) 25%, transparent 25%),
      linear-gradient(-45deg, var(--void-up) 25%, transparent 25%),
      linear-gradient(45deg, transparent 75%, var(--void-up) 75%),
      linear-gradient(-45deg, transparent 75%, var(--void-up) 75%);
    background-size: 8px 8px;
    background-position: 0 0, 0 4px, 4px -4px, -4px 0px;
    background-color: var(--void-mid);
  }
  
  .selected-color-name {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-2);
  }
  
  /* Responsive */
  @media (max-width: 900px) {
    .editor-layout {
      grid-template-columns: 1fr;
      justify-items: center;
    }
    
    .pixel-grid {
      width: 288px;
      height: 288px;
    }
    
    .preview-container {
      width: 200px;
      height: 200px;
    }
  }
  
  /* Spinner animation */
  :global(.spin) {
    animation: spin 1s linear infinite;
  }
  
  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
</style>

