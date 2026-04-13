const SecureStorage = (() => {
    const isAvailable = !!(window.crypto && window.crypto.subtle && window.isSecureContext);
    
    if (!isAvailable) {
        console.warn("Uplarr: Web Crypto API not available. Secure contexts (HTTPS or localhost) are required for local encryption.");
        return {
            isAvailable: false,
            deriveKey: async () => [],
            getKey: async () => null,
            encrypt: async (text) => text,
            decrypt: async (text) => text
        };
    }

    const ALGO = 'AES-GCM';
    const SALT = new TextEncoder().encode('uplarr-salt-v1'); // Static salt for key derivation
    const PBKDF2_ITERATIONS = 100000;

    const deriveKey = async (password) => {
        const passwordKey = await crypto.subtle.importKey(
            'raw',
            new TextEncoder().encode(password),
            { name: 'PBKDF2' },
            false,
            ['deriveKey']
        );

        const key = await crypto.subtle.deriveKey(
            {
                name: 'PBKDF2',
                salt: SALT,
                iterations: PBKDF2_ITERATIONS,
                hash: 'SHA-256'
            },
            passwordKey,
            { name: ALGO, length: 256 },
            true,
            ['encrypt', 'decrypt']
        );

        // Export to store in sessionStorage (as raw bytes)
        const raw = await crypto.subtle.exportKey('raw', key);
        return Array.from(new Uint8Array(raw));
    };

    const getKey = async () => {
        const saved = sessionStorage.getItem('uplarr_master_key');
        if (!saved) return null;
        try {
            const raw = new Uint8Array(JSON.parse(saved));
            return crypto.subtle.importKey(
                'raw',
                raw,
                ALGO,
                false,
                ['encrypt', 'decrypt']
            );
        } catch (e) {
            console.error("Failed to import key from session storage", e);
            return null;
        }
    };

    const encrypt = async (text, key) => {
        if (!key) return text;
        const iv = crypto.getRandomValues(new Uint8Array(12));
        const encoded = new TextEncoder().encode(text);
        const ciphertext = await crypto.subtle.encrypt(
            { name: ALGO, iv },
            key,
            encoded
        );

        // Combine IV + Ciphertext
        const combined = new Uint8Array(iv.length + ciphertext.byteLength);
        combined.set(iv);
        combined.set(new Uint8Array(ciphertext), iv.length);

        return btoa(String.fromCharCode(...combined));
    };

    const decrypt = async (base64, key) => {
        if (!key) return base64;
        const combined = new Uint8Array(atob(base64).split('').map(c => c.charCodeAt(0)));
        const iv = combined.slice(0, 12);
        const ciphertext = combined.slice(12);

        const decrypted = await crypto.subtle.decrypt(
            { name: ALGO, iv },
            key,
            ciphertext
        );

        return new TextDecoder().decode(decrypted);
    };

    return { isAvailable: true, deriveKey, getKey, encrypt, decrypt };
})();
