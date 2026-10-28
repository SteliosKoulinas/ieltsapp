/** A random UUID v4. crypto.randomUUID only exists in secure contexts (https or
 *  localhost), so fall back to getRandomValues when the app is opened over plain
 *  http on a LAN address. */
export function newId(): string {
	const c: Crypto & { randomUUID?: () => string } = crypto;
	if (typeof c.randomUUID === 'function') return c.randomUUID();
	const b = c.getRandomValues(new Uint8Array(16));
	b[6] = (b[6] & 0x0f) | 0x40;
	b[8] = (b[8] & 0x3f) | 0x80;
	const h = [...b].map((x) => x.toString(16).padStart(2, '0'));
	return `${h.slice(0, 4).join('')}-${h.slice(4, 6).join('')}-${h.slice(6, 8).join('')}-${h.slice(8, 10).join('')}-${h.slice(10).join('')}`;
}
