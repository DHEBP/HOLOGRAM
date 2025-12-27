import VillagerIdenticon from './villager-identicon.js';
import { CallXSWD } from '../../../wailsjs/go/main/App.js';

// Villager Smart Contract ID
// TODO: Update with actual mainnet SCID when deployed
let VILLAGER_SCID = '0000000000000000000000000000000000000000000000000000000000000000';

// Empty avatar string (576 'z' characters = all transparent pixels)
const EMPTY_AVATAR = 'z'.repeat(576);

// Cache for avatar URLs (address -> URL)
const avatarUrlCache = new Map();

// Cache for avatar pixel strings (address -> pixel string)
const avatarPixelCache = new Map();

/**
 * Hex string to 576-character pixel string
 */
function hexToString(hex) {
    if (hex.length !== 1152 || !/^[0-9a-fA-F]{1152}$/.test(hex)) {
        throw new Error("Invalid hex string – must be exactly 1152 hex chars");
    }
    let str = '';
    for (let i = 0; i < hex.length; i += 2) {
        str += String.fromCharCode(parseInt(hex.substr(i, 2), 16));
    }
    return str;
}

/**
 * Fetch avatar pixels from smart contract
 * @param {string} address - Wallet address
 * @returns {Promise<string|null>} - 576-character pixel string or null if not found
 */
async function fetchAvatarPixels(address) {
    if (!address) return null;
    
    // Check cache first
    if (avatarPixelCache.has(address)) {
        return avatarPixelCache.get(address);
    }
    
    try {
        // Call smart contract to get avatar
        const response = await CallXSWD(JSON.stringify({
            jsonrpc: "2.0",
            id: 1,
            method: "DERO.GetSC",
            params: {
                scid: VILLAGER_SCID,
                keysstring: [`avatar_${address}`]
            }
        }));
        
        if (response?.result?.valuesstring?.[0]) {
            const avatarHex = response.result.valuesstring[0];
            // Decode hex to 576-char string
            const avatarStr = hexToString(avatarHex);
            
            // Cache the result
            avatarPixelCache.set(address, avatarStr);
            return avatarStr;
        }
        
        // No avatar found, return null
        return null;
    } catch (error) {
        console.error('Failed to fetch avatar from SC:', error);
        return null;
    }
}

/**
 * Get avatar URL for an address
 * Renders frame instantly, fetches custom pixels in background
 * @param {string} address - Wallet address
 * @param {number} size - Requested size in pixels (default: 40)
 * @returns {Promise<string>} - Object URL for the avatar image
 */
export async function getAvatarUrl(address, size = 40) {
    if (!address) {
        throw new Error('Address is required');
    }
    
    // Check if we have a cached URL for this address and size
    const cacheKey = `${address}_${size}`;
    if (avatarUrlCache.has(cacheKey)) {
        return avatarUrlCache.get(cacheKey);
    }
    
    // Start with empty avatar (frame only) - renders instantly
    let avatarStr = EMPTY_AVATAR;
    
    // Try to fetch custom pixels (non-blocking)
    fetchAvatarPixels(address).then(pixels => {
        if (pixels && pixels.length === 576) {
            // Update cache with custom pixels
            avatarPixelCache.set(address, pixels);
            
            // Re-render with custom pixels
            // Clear old cache entry
            avatarUrlCache.delete(cacheKey);
            
            // Generate new avatar with custom pixels
            VillagerIdenticon.render(address, pixels, size).then(url => {
                avatarUrlCache.set(cacheKey, url);
            }).catch(err => {
                console.error('Failed to render avatar with custom pixels:', err);
            });
        }
    }).catch(err => {
        console.error('Failed to fetch avatar pixels:', err);
    });
    
    // Render immediately with empty pixels (frame only)
    try {
        const url = await VillagerIdenticon.render(address, avatarStr, size);
        avatarUrlCache.set(cacheKey, url);
        return url;
    } catch (error) {
        console.error('Failed to render avatar:', error);
        throw error;
    }
}

/**
 * Preload avatar for an address (useful for prefetching)
 * @param {string} address - Wallet address
 * @param {number} size - Size to preload (default: 40)
 */
export async function preloadAvatar(address, size = 40) {
    if (!address) return;
    try {
        await getAvatarUrl(address, size);
    } catch (error) {
        console.error('Failed to preload avatar:', error);
    }
}

/**
 * Clear avatar cache for an address
 * @param {string} address - Wallet address (optional, clears all if not provided)
 */
export function clearAvatarCache(address = null) {
    if (address) {
        // Clear specific address
        const keysToDelete = [];
        avatarUrlCache.forEach((value, key) => {
            if (key.startsWith(`${address}_`)) {
                keysToDelete.push(key);
            }
        });
        keysToDelete.forEach(key => {
            const url = avatarUrlCache.get(key);
            if (url) URL.revokeObjectURL(url);
            avatarUrlCache.delete(key);
        });
        avatarPixelCache.delete(address);
    } else {
        // Clear all
        avatarUrlCache.forEach(url => URL.revokeObjectURL(url));
        avatarUrlCache.clear();
        avatarPixelCache.clear();
        VillagerIdenticon.clearCache();
    }
}

/**
 * Set the Villager smart contract ID
 * @param {string} scid - Smart contract ID
 */
export function setVillagerSCID(scid) {
    if (typeof scid === 'string' && scid.length === 64) {
        VILLAGER_SCID = scid;
    } else {
        console.warn('Invalid Villager SCID:', scid);
    }
}

/**
 * Get the current Villager smart contract ID
 * @returns {string} - Smart contract ID
 */
export function getVillagerSCID() {
    return VILLAGER_SCID;
}

