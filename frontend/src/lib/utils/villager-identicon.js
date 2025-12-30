/*
villager-identicon.js
Official Villager identicon renderer for HOLOGRAM.

Mainnet SCID: f0b29081c1ed35fe942cb3402cd9d7bf0cf27639201bbc96223bdc99c4c6aa9f

How developers can fetch the avatar data:
──────────────────────────────────────────────────────────────
// 1. Connect to any DERO daemon WebSocket (public or private node)
const socket = new WebSocket("http://ip:10102/ws"); // or your own node endpoint acquired from XSWD
──────────────────────────────────────────────────────────────
// Call example (get all avatars)
method: "DERO.GetSC",
params: {
scid: "f0b29081c1ed35fe942cb3402cd9d7bf0cf27639201bbc96223bdc99c4c6aa9f",
variables: true,
code: false
}

// Response handler example (for all avatars/identicons)
for (const key in stringKeys) {
	if (key.startsWith("avatar_") && typeof stringKeys[key] === "string" && stringKeys[key].length > 0) {
		let avatarStr = stringKeys[key];
		// Avatars are stored as hex strings in the SC, decode to 576-char string
		avatarStr = hexToString(avatarStr);
		const address = key.substring(7);  // After "avatar_"
		window.storedAvatars[address] = avatarStr;
		// Block heights are stored as hex strings, decode to number
		const blockHeightHex = stringKeys[`timestamp_${address}`] || '0';
		window.avatarTimestamps[address] = parseInt(blockHeightHex, 16);
	}
}
──────────────────────────────────────────────────────────────
// Alternative: Get single avatar/identicon string
method: "DERO.GetSC",
params: {
scid: "f0b29081c1ed35fe942cb3402cd9d7bf0cf27639201bbc96223bdc99c4c6aa9f",
keysstring: ["avatar_dero1qyre7td6x9r88y4cavdgpv6k7lvx6j39lfsx420hpvh3ydpcrtxrxqga4mp52"] // Replace with target address
}

// Alternative: Handle single avatar response
const avatarStr = data.result.valuesstring[0];
if (avatarStr) {
    // Avatars are stored as hex strings in the SC, decode to 576-char string
    const decodedAvatar = hexToString(avatarStr);
    window.storedAvatars[targetAddress] = decodedAvatar;
    // Block heights are stored as hex strings, decode to number
    const blockHeightHex = data.result.valuesstring[1] || '0'; // Assuming second value is block height
    window.avatarTimestamps[targetAddress] = parseInt(blockHeightHex, 16);
}
──────────────────────────────────────────────────────────────

Library usage:
──────────────────────────────────────────────────────────────
  <script src="villager-identicon.js"></script>

  // Common sizes:
  await VillagerIdenticon.render(addr, hex, 180);  // thumbnail
  await VillagerIdenticon.render(addr, hex, 512);  // profile
  await VillagerIdenticon.render(addr, hex, 800);  // full view

  // Always revoke when done:
  URL.revokeObjectURL(url);

  // Clear all cached images as often as possible, when they aren't in use.
  // Remember to call URL.revokeObjectURL(url) when the image is no longer needed, for memory safety.
  VillagerIdenticon.clearCache();
──────────────────────────────────────────────────────────────
*/

const VillagerIdenticon = (function () {
	const avatarCache = new Map();
    // ──────────────────────────────────────────────────────────────
    // 1. Official Villager palette (must never change)
    // ──────────────────────────────────────────────────────────────
	const Char_To_Color = {
		'0': 0xFFFF9999, '1': 0xFFFF6666, '2': 0xFFFF0000, '3': 0xFF800000,
		'4': 0xFFFFA899, '5': 0xFFFF8C66, '6': 0xFFFF4500, '7': 0xFF802200,
		'8': 0xFFFFC799, '9': 0xFFFFB266, 'A': 0xFFFF8C00, 'B': 0xFF804600,
		'C': 0xFFFFE099, 'D': 0xFFFFD866, 'E': 0xFFFFAA00, 'F': 0xFF5C4033,
		'G': 0xFFFFFF99, 'H': 0xFFFFFF66, 'I': 0xFFFFFF00, 'J': 0xFFFFD700,
		'K': 0xFFCFFF99, 'L': 0xFFBFFF66, 'M': 0xFF80FF00, 'N': 0xFF408000,
		'O': 0xFF99FF99, 'P': 0xFF66FF66, 'Q': 0xFF00FF00, 'R': 0xFF008000,
		'S': 0xFF99FFCF, 'T': 0xFF66FFBF, 'U': 0xFF00FF80, 'V': 0xFF008040,
		'W': 0xFF99FFFF, 'X': 0xFF66FFFF, 'Y': 0xFF00FFFF, 'Z': 0xFF008080,
		'a': 0xFF99CFFF, 'b': 0xFF66BFFF, 'c': 0xFF0080FF, 'd': 0xFF004080,
		'e': 0xFF9999FF, 'f': 0xFF6666FF, 'g': 0xFF0000FF, 'h': 0xFF000080,
		'i': 0xFFCF99FF, 'j': 0xFFBF66FF, 'k': 0xFF8000FF, 'l': 0xFF400080,
		'm': 0xFFFF99FF, 'n': 0xFFFF66FF, 'o': 0xFFFF00FF, 'p': 0xFF800080,
		'q': 0xFFFF99C7, 'r': 0xFFFF66B2, 's': 0xFFFF0080, 't': 0xFF800040,
		'u': 0xFFFFFFFF, 'v': 0xFFB4B4B4, 'w': 0xFF848484, 'x': 0xFF434343,
		'y': 0xFF000000, 'z': 0x00000000
	};

    // ──────────────────────────────────────────────────────────────
    // 2. Fast deterministic 32-bit hash
    // ──────────────────────────────────────────────────────────────
    function simpleHash(str) {
        let h = 1779033703 ^ str.length;
        for (let i = 0; i < str.length; i++) {
            h = Math.imul(h ^ str.charCodeAt(i), 3432918353);
            h = h << 13 | h >>> 19;
        }
        return h >>> 0;
    }

    // ──────────────────────────────────────────────────────────────
    // 3. Hex → 576-char string
    // ──────────────────────────────────────────────────────────────
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

	// ──────────────────────────────────────────────────────────────
    // 4. Render Controller
    // ──────────────────────────────────────────────────────────────
    async function renderSmart(address, rawHexOrString, requestedSize = 180) {
        let avatarStr = rawHexOrString;
        if (typeof avatarStr === 'string' && avatarStr.length === 1152 && /^[0-9a-fA-F]{1152}$/.test(avatarStr)) {
            avatarStr = hexToString(avatarStr);
        }

        if (avatarStr.length !== 576) {
            throw new Error("Avatar must be 576 characters after decoding");
        }

        const cacheKey = address;

        // Cache the full 800px version once
        if (!avatarCache.has(cacheKey)) {
            const fullUrl = await generateAvatarWithFrame(address, avatarStr, 800);
            avatarCache.set(cacheKey, fullUrl);
        }

        const fullUrl = avatarCache.get(cacheKey);

        // Return full size if requested
        if (requestedSize >= 800) {
            return fullUrl;
        }

        // Otherwise, scale down
        return new Promise(resolve => {
            const img = new Image();
            img.onload = () => {
                const canvas = document.createElement('canvas');
                canvas.width = requestedSize;
                canvas.height = requestedSize;
                const ctx = canvas.getContext('2d');
                ctx.imageSmoothingEnabled = true;
                ctx.imageSmoothingQuality = 'high';
                ctx.drawImage(img, 0, 0, requestedSize, requestedSize);
                canvas.toBlob(blob => resolve(URL.createObjectURL(blob)), 'image/png');
            };
            img.src = fullUrl;
        });
    }

    // ──────────────────────────────────────────────────────────────
    // 4. Core renderer
    // ──────────────────────────────────────────────────────────────
	async function generateAvatarWithFrame(address, avatarStr, size = 180) {
		if (avatarStr.length !== 576) return Promise.reject("Invalid avatar string");

		const uniquePart = address.startsWith('dero1') ? address.slice(5) : address;
		const frameSeed = simpleHash(uniquePart + "FRAME");
		const bgSeed   = simpleHash(uniquePart + "BACKGROUND");

		const canvas = document.createElement('canvas');
		const ctx = canvas.getContext('2d');
		canvas.width = size;
		canvas.height = size;

		const border = Math.floor(size * 0.13);
		const inner = size - 2 * border;

		// Varied gradient background
		const gradType = bgSeed % 4;
		const cx = size / 2 + (simpleHash(uniquePart + "CX") % 50 - 25);
		const cy = size / 2 + (simpleHash(uniquePart + "CY") % 50 - 25);

		let grad;
		if (gradType === 0) {
			grad = ctx.createRadialGradient(cx, cy, 0, cx, cy, size * 1.2);
			grad.addColorStop(0, `hsl(${bgSeed % 360}, 95%, 65%)`);
			grad.addColorStop(0.4, `hsl(${(bgSeed + 90) % 360}, 85%, 40%)`);
			grad.addColorStop(1, `hsl(${(bgSeed + 200) % 360}, 70%, 10%)`);
		} else if (gradType === 1) {
			grad = ctx.createLinearGradient(0, 0, size, size);
			grad.addColorStop(0, `hsl(${bgSeed % 360}, 100%, 60%)`);
			grad.addColorStop(1, `hsl(${(bgSeed + 150) % 360}, 80%, 15%)`);
		} else if (gradType === 2) {
			grad = ctx.createRadialGradient(cx, cy, size * 0.05, cx, cy, size);
			grad.addColorStop(0, `hsl(${(bgSeed + 130) % 360}, 100%, 75%)`);
			grad.addColorStop(0.5, `hsl(${bgSeed % 360}, 90%, 35%)`);
			grad.addColorStop(1, `hsl(${(bgSeed + 220) % 360}, 65%, 8%)`);
		} else {
			grad = ctx.createConicGradient(bgSeed * 0.008, size/2, size/2);
			grad.addColorStop(0, `hsl(${bgSeed % 360}, 95%, 70%)`);
			grad.addColorStop(0.33, `hsl(${(bgSeed + 120) % 360}, 90%, 50%)`);
			grad.addColorStop(0.66, `hsl(${(bgSeed + 240) % 360}, 85%, 40%)`);
			grad.addColorStop(1, `hsl(${bgSeed % 360}, 95%, 70%)`);
		}
		ctx.fillStyle = grad;
		ctx.fillRect(0, 0, size, size);

		// Tiny starfield
		for (let i = 0; i < size/4; i++) {
			const x = simpleHash(uniquePart + i) % size;
			const y = simpleHash(uniquePart + i + 7777) % size;
			const b = 60 + (simpleHash(uniquePart + i + 99999) % 40);
			ctx.fillStyle = `hsl(70, 40%, ${b}%)`;
			ctx.fillRect(x, y, 1, 1);
		}

		// Frame styles
		const hueBase = frameSeed % 360;
		const shapeType = frameSeed % 5;
		const rotation = (frameSeed % 91) - 45;

		ctx.save();
		ctx.translate(size / 2, size / 2);
		ctx.rotate(rotation * Math.PI / 180);

		if (shapeType === 0) { // Polygon shards
			const sides = 6 + (frameSeed >> 8) % 8;
			for (let l = 4; l >= 1; l--) {
				ctx.strokeStyle = `hsla(${(hueBase + l*72)%360},90%,66%,0.82)`;
				ctx.lineWidth = border * 0.25;
				ctx.beginPath();
				for (let i = 0; i <= sides; i++) {
					const a = i / sides * Math.PI * 2 + l*0.25;
					const r = inner/2 + border*0.75*(l/4) + Math.sin(a*7 + l)*border*0.18;
					const x = Math.cos(a) * r;
					const y = Math.sin(a) * r;
					i===0 ? ctx.moveTo(x,y) : ctx.lineTo(x,y);
				}
				ctx.closePath();
				ctx.stroke();
			}
		} else if (shapeType === 1) { // Starburst spikes
			ctx.lineCap = 'round';
			for (let i = 0; i < 30; i++) {
				const a = i / 30 * Math.PI * 2;
				const h = (hueBase + i*13) % 360;
				const len = inner/2 + border * (0.4 + (simpleHash(uniquePart + i) % 70)/100);
				ctx.strokeStyle = `hsla(${h},95%,72%,0.9)`;
				ctx.lineWidth = 1.8 + (i % 7);
				ctx.beginPath();
				ctx.moveTo(0,0);
				ctx.lineTo(Math.cos(a)*len, Math.sin(a)*len);
				ctx.stroke();
			}
		} else if (shapeType === 2) { // Glitch rings
			for (let i = 9; i >= 1; i--) {
				const r = inner/2 + border*0.9*(i/9);
				const off = (frameSeed >> i) % 60 - 30;
				ctx.strokeStyle = `hsla(${(hueBase + i*45)%360},95%,74%,0.7)`;
				ctx.lineWidth = 3 + i%5;
				ctx.setLineDash([12, 6 + i*4]);
				ctx.lineDashOffset = off;
				ctx.beginPath();
				ctx.arc(0,0,r,0,Math.PI*2);
				ctx.stroke();
			}
			ctx.setLineDash([]);
		} else if (shapeType === 3) { // Crystal grid
			const count = 5 + (frameSeed % 8);
			for (let i = 0; i < count; i++) {
				const a = i / count * Math.PI * 2;
				const h = (hueBase + i*58) % 360;
				ctx.fillStyle = `hsla(${h},92%,68%,0.48)`;
				ctx.beginPath();
				for (let j = 0; j < 6; j++) {
					const r = inner/2 + border*0.7;
					const a2 = a + j/6*Math.PI*2 + (i%2 ? Math.PI/6 : 0);
					const x = Math.cos(a2) * r;
					const y = Math.sin(a2) * r * 0.7;
					j===0 ? ctx.moveTo(x,y) : ctx.lineTo(x,y);
				}
				ctx.closePath();
				ctx.fill();
			}
		} else { // Nebula rings
			for (let i = 7; i >= 1; i--) {
				const r = inner/2 + border * i / 7;
				ctx.strokeStyle = `hsla(${(hueBase + i*55)%360},88%,65%,0.5)`;
				ctx.lineWidth = border * 0.22;
				ctx.globalAlpha = i/11;
				ctx.beginPath();
				ctx.arc(0,0,r + Math.sin(i*2.2)*border*0.3,0,Math.PI*2);
				ctx.stroke();
			}
			ctx.globalAlpha = 1;
		}

		// Varied multi-layer glow ring
		const glowCount = 1 + (frameSeed % 3);
		const glowHue = (hueBase + 100 + (frameSeed >> 5) % 140) % 360;
		for (let g = 0; g < glowCount; g++) {
			const offset = g * border * 0.09;
			const blur = border * (0.32 + g * 0.16 + (frameSeed % 50)/120);
			const width = border * (0.08 + g * 0.06);
			ctx.shadowBlur = blur;
			ctx.shadowColor = `hsla(${glowHue + g*45},100%,78%,0.92)`;
			ctx.strokeStyle = `hsla(${glowHue + g*55},100%,82%,1)`;
			ctx.lineWidth = width;
			ctx.beginPath();
			ctx.arc(0, 0, inner/2 + border*0.26 + offset, 0, Math.PI*2);
			ctx.stroke();
		}

		ctx.restore();

		// RENDER AVATAR
		const avatarCanvas = document.createElement('canvas');
		avatarCanvas.width = inner;
		avatarCanvas.height = inner;
		const actx = avatarCanvas.getContext('2d');
		actx.imageSmoothingEnabled = false;

		const CELL = inner / 24;
		const OVER = 1.0;

		let idx = 0;
		for (let x = 0; x < 24; x++) {
			for (let y = 0; y < 24; y++) {
				const ch = avatarStr[idx++];
				const argb = Char_To_Color[ch] || 0x00000000;
				const a = (argb >> 24) & 0xFF;
				if (a === 0) continue;

				const r = (argb >> 16) & 0xFF;
				const g = (argb >> 8)  & 0xFF;
				const b =  argb        & 0xFF;

				actx.fillStyle = `rgba(${r},${g},${b},${a/255})`;
				actx.fillRect(
					x * CELL - OVER/2,
					y * CELL - OVER/2,
					CELL + OVER,
					CELL + OVER
				);
			}
		}

		// Soft shadow under avatar
		ctx.shadowColor = 'rgba(0,0,0,0.7)';
		ctx.shadowBlur = border * 0.4;
		ctx.shadowOffsetY = border * 0.1;
		ctx.drawImage(avatarCanvas, border, border, inner, inner);

		ctx.shadowColor = 'transparent';

		return new Promise(resolve => {
			canvas.toBlob(blob => resolve(URL.createObjectURL(blob)), 'image/png');
		});
	}
    return {
        render: renderSmart,
        clearCache: () => {
            avatarCache.forEach(url => URL.revokeObjectURL(url));
            avatarCache.clear();
        }
    };
})();

// ES6 export for module systems
export default VillagerIdenticon;

// Also make available globally for compatibility
if (typeof window !== 'undefined') {
    window.VillagerIdenticon = VillagerIdenticon;
}

