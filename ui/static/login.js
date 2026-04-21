/**
 * Uplarr Login Logic
 * Handles master password verification and master key derivation.
 */

document.addEventListener('DOMContentLoaded', () => {
    const loginForm = document.getElementById('login-form');
    const passwordInput = document.getElementById('password');
    const loginBtn = document.getElementById('login-btn');
    const errorEl = document.getElementById('error-msg');
    const insecureNotice = document.getElementById('insecure-notice');

    let loginSucceeded = false;

    // Check for Secure Context
    if (typeof SecureStorage !== 'undefined' && !SecureStorage.isAvailable) {
        if (insecureNotice) insecureNotice.style.display = 'flex';
    }

    if (!loginForm || !passwordInput || !loginBtn || !errorEl) return;

    loginForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        
        const password = passwordInput.value;
        if (!password) return;

        // Verify SecureStorage Availability
        if (typeof SecureStorage === 'undefined' || !SecureStorage.isAvailable) {
            errorEl.textContent = 'Hardware encryption unavailable. Check HTTPS/Localhost.';
            errorEl.classList.add('visible');
            return;
        }

        // Set Loading State
        loginBtn.disabled = true;
        loginBtn.classList.add('loading');
        const originalBtnText = loginBtn.innerHTML;
        loginBtn.innerHTML = `
            <svg class="spinner" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21 12a9 9 0 1 1-6.219-8.56"></path>
            </svg>
            Verifying...
        `;
        errorEl.textContent = '';
        errorEl.classList.remove('visible');

        try {
            const res = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password })
            });

            if (res.ok) {
                loginSucceeded = true;

                // Initialize crypto master key
                let saltStr = localStorage.getItem('uplarr_installation_salt');
                let salt = null;
                if (saltStr) {
                    try {
                        salt = new Uint8Array(JSON.parse(saltStr));
                    } catch (e) {
                        console.error('Failed to parse salt:', e);
                    }
                }
                
                const keyObj = await SecureStorage.deriveKey(password, salt);
                console.log('Login: Master key derived successfully');
                
                // Store raw key array for compatibility with app.js
                sessionStorage.setItem('uplarr_master_key', JSON.stringify(keyObj.key));
                console.log('Login: sessionStorage masterKey initialized');
                
                // Perform legacy migration immediately using in-memory password
                const masterKey = await SecureStorage.getKey();
                if (masterKey) {
                    console.log('Login: Verified masterKey retrieval works');
                    const keys = Object.keys(localStorage);
                    for (const key of keys) {
                        if (key.startsWith('uplarr_') && !key.endsWith('_salt')) {
                            const raw = localStorage.getItem(key);
                            if (raw && !raw.startsWith('v2:')) {
                                try {
                                    // Try to decrypt legacy V1 using the plaintext password
                                    const decrypted = await SecureStorage.decrypt(raw, masterKey, password);
                                    if (decrypted) {
                                        // Re-encrypt to V2
                                        const encrypted = await SecureStorage.encrypt(decrypted, masterKey);
                                        localStorage.setItem(key, encrypted);
                                    }
                                } catch (e) {
                                    // Silent failure for non-encrypted or incompatible items
                                }
                            }
                        }
                    }
                }

                if (!saltStr) {
                    localStorage.setItem('uplarr_installation_salt', JSON.stringify(keyObj.salt));
                    console.log('Login: Installation salt generated and stored');
                }
                
                // Success transition
                loginBtn.innerHTML = 'Success!';
                loginBtn.style.background = 'var(--success)';
                
                console.log('Login: Success. Redirecting to home...');
                setTimeout(() => {
                    window.location.href = '/';
                }, 500);
            } else {
                const data = await res.json().catch(() => ({}));
                errorEl.textContent = data.error || 'Invalid master password';
                errorEl.classList.add('visible');
                
                // Shake animation for error
                const card = document.querySelector('.login-card');
                if (card) {
                    card.classList.add('shake');
                    setTimeout(() => card.classList.remove('shake'), 500);
                }
            }
        } catch (err) {
            console.error('Login error:', err);
            errorEl.textContent = 'Connection failed. Please try again.';
            errorEl.classList.add('visible');
        } finally {
            if (!loginSucceeded && window.location.pathname !== '/') {
                loginBtn.disabled = false;
                loginBtn.classList.remove('loading');
                loginBtn.innerHTML = originalBtnText;
            }
        }
    });

    // Toggle Password Visibility (if icon added later)
    const toggleBtn = document.getElementById('toggle-password');
    if (toggleBtn) {
        toggleBtn.addEventListener('click', () => {
            if (!passwordInput) return;
            const type = passwordInput.getAttribute('type') === 'password' ? 'text' : 'password';
            passwordInput.setAttribute('type', type);
            toggleBtn.classList.toggle('fa-eye');
            toggleBtn.classList.toggle('fa-eye-slash');
        });
    }
});
