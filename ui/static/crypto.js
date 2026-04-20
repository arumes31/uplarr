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
    const LEGACY_SALT = new TextEncoder().encode('uplarr-salt-v1');
    const LEGACY_ITERATIONS = 100000;
    const PBKDF2_ITERATIONS = 600000; 

    const deriveKey = async (password, salt = null, iterations = PBKDF2_ITERATIONS) => {
        const useSalt = salt || crypto.getRandomValues(new Uint8Array(16));
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
                salt: useSalt,
                iterations: iterations,
                hash: 'SHA-256'
            },
            passwordKey,
            { name: ALGO, length: 256 },
            true,
            ['encrypt', 'decrypt']
        );

        const raw = await crypto.subtle.exportKey('raw', key);
        return {
            key: Array.from(new Uint8Array(raw)),
            salt: Array.from(useSalt),
            iterations: iterations
        };
    };

    const getKey = async () => {
        const saved = sessionStorage.getItem('uplarr_master_key');
        if (!saved) return null;
        try {
            const raw = new Uint8Array(JSON.parse(saved));
            return crypto.subtle.importKey(
                'raw', raw, ALGO, false, ['encrypt', 'decrypt']
            );
        } catch (e) {
            console.error("Failed to import key", e);
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

        const combined = new Uint8Array(iv.length + ciphertext.byteLength);
        combined.set(iv);
        combined.set(new Uint8Array(ciphertext), iv.length);

        return 'v2:' + btoa(String.fromCharCode(...combined));
    };

    const decrypt = async (encoded, key, password = null) => {
        if (!key || !encoded) return encoded;
        
        let data = encoded;
        let isV2 = false;
        if (encoded.startsWith('v2:')) {
            data = encoded.substring(3);
            isV2 = true;
        }

        try {
            const combined = new Uint8Array(atob(data).split('').map(c => c.charCodeAt(0)));
            const iv = combined.slice(0, 12);
            const ciphertext = combined.slice(12);

            const decrypted = await crypto.subtle.decrypt(
                { name: ALGO, iv },
                key,
                ciphertext
            );
            return new TextDecoder().decode(decrypted);
        } catch (e) {
            // If V2 failed or it was V1 and we have a password, try legacy fallback
            if (!isV2 && password) {
                try {
                    const legacyKeyObj = await deriveKey(password, LEGACY_SALT, LEGACY_ITERATIONS);
                    const legacyKey = await crypto.subtle.importKey(
                        'raw', new Uint8Array(legacyKeyObj.key), ALGO, false, ['decrypt']
                    );
                    return await decrypt(encoded, legacyKey); // Recursion with legacy key
                } catch (err) {
                    console.error("Legacy decryption failed", err);
                }
            }
            throw e;
        }
    };

    return { isAvailable: true, deriveKey, getKey, encrypt, decrypt };
})();
