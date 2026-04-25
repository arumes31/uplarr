document.addEventListener('DOMContentLoaded', () => {
    // Local Pane Elements
    const fileListBody = document.getElementById('file-list-body');
    const refreshBtn = document.getElementById('refresh-btn');
    const upBtn = document.getElementById('up-btn');
    const mkdirBtn = document.getElementById('mkdir-btn');
    const localBreadcrumb = document.getElementById('local-breadcrumb');
    const selectAllCheckbox = document.getElementById('select-all-checkbox');

    // Remote Pane Elements
    const remoteFileListBody = document.getElementById('remote-file-list-body');
    const remoteRefreshBtn = document.getElementById('remote-refresh-btn');
    const remoteUpBtn = document.getElementById('remote-up-btn');
    const remoteMkdirBtn = document.getElementById('remote-mkdir-btn');
    const remoteBreadcrumb = document.getElementById('remote-breadcrumb');
    const remoteDropZone = document.getElementById('remote-drop-zone');

    // Queue Elements
    const queueBody = document.getElementById('queue-body');
    const headerHostMetrics = document.getElementById('header-host-metrics');

    // Shared Elements
    const testBtn = document.getElementById('test-btn');
    const updateThrottleBtn = document.getElementById('update-throttle-btn');
    const uploadBtn = document.getElementById('upload-btn');
    const logoutBtn = document.getElementById('logout-btn');
    const toastContainer = document.getElementById('toast-container');
    const logSearchInput = document.getElementById('log-search');
    const dropOverlay = document.getElementById('drop-overlay');
    const statusMsg = document.getElementById('status-message');
    const logContainer = document.getElementById('log-container');
    const sftpForm = document.getElementById('sftp-form');
    
    // Search Elements
    const localSearchInput = document.getElementById('local-search');
    const remoteSearchInput = document.getElementById('remote-search');
    
    // UI state
    let localFilter = '';
    let remoteFilter = '';
    
    // Modal Elements
    const dropModal = document.getElementById('drop-modal');
    const modalFileInfo = document.getElementById('modal-file-info');
    const modalDeleteLocal = document.getElementById('modal-delete-local');
    const modalOverwriteRemote = document.getElementById('modal-overwrite-remote');
    const modalCancelBtn = document.getElementById('modal-cancel-btn');
    const modalConfirmBtn = document.getElementById('modal-confirm-btn');
    
    // Compact toggle elements
    const compactToggle = document.getElementById('compact-toggle');
    const remoteCompactToggle = document.getElementById('remote-compact-toggle');
    const localPane = document.querySelector('.local-pane');
    const remotePane = document.querySelector('.remote-pane');
    const toggleGlobalCompact = document.getElementById('toggle-global-compact');
    
    // Phase 3 Elements
    const selectionBar = document.getElementById('selection-bar');
    const selectionCount = document.getElementById('selection-count');
    const selectionQueueBtn = document.getElementById('selection-queue-btn');
    const selectionDeleteBtn = document.getElementById('selection-delete-btn');
    const selectionClearBtn = document.getElementById('selection-clear-btn');
    // Phase 5 Elements
    const sessionStats = document.getElementById('session-stats');
    const statTotalData = document.getElementById('stat-total-data');
    const statAvgSpeed = document.getElementById('stat-avg-speed');
    const viewToggleBtn = document.getElementById('view-toggle-btn');
    const viewMenu = document.getElementById('view-menu');
    const renameModal = document.getElementById('rename-modal');
    const renameSearch = document.getElementById('rename-search');
    const renameReplace = document.getElementById('rename-replace');
    const renamePreview = document.getElementById('rename-preview');
    const renameCancelBtn = document.getElementById('rename-cancel-btn');
    const renameConfirmBtn = document.getElementById('rename-confirm-btn');
    const selectionRenameBtn = document.getElementById('selection-rename-btn');
    const renameCountBadge = document.getElementById('rename-count-badge');
    const concurrentFilesInput = document.getElementById('concurrent_files');

    const contextMenu = document.getElementById('context-menu');
    const sidebarTree = document.getElementById('sidebar-tree');

    // Rate tracking
    let globalTotalRate = 0;
    let lastTotalRate = 0;
    let sessionTotalBytes = 0;
    let sessionStart = Date.now();
    const speedHistory = new Map(); // host -> [rates...] for sparklines
    const folderTreeHistory = { local: new Set(), remote: new Set() };
    const countedTaskIds = new Set(); // To prevent double-counting analytics
    let currentLiveBytes = 0; // sum of bytes_uploaded for non-completed tasks

    // --- Centralized Auth Failure Handler ---
    // Prevents multiple simultaneous 401 redirects from creating a reload loop.
    let authRedirectPending = false;
    const handleAuthFailure = (source = 'unknown', status = 0) => {
        if (authRedirectPending) return;
        authRedirectPending = true;
        console.warn(`Session expired/invalid (Source: ${source}, Status: ${status}). Redirecting to login...`);
        window.location.href = '/';
    };

    // --- Secure Storage Wrappers ---
    let masterKey = null;

    const getSecureItem = async (key) => {
        if (!masterKey) masterKey = await SecureStorage.getKey();
        const raw = localStorage.getItem(key);
        if (!raw || !masterKey) return raw;
        try {
            // Decrypt using masterKey (V2). Plaintext password is no longer stored in sessionStorage.
            return await SecureStorage.decrypt(raw, masterKey);
        } catch (e) {
            console.error(`Failed to decrypt ${key}`, e);
            return null;
        }
    };

    const setSecureItem = async (key, value) => {
        if (!masterKey) masterKey = await SecureStorage.getKey();
        if (!masterKey) {
            localStorage.setItem(key, value);
            return;
        }
        try {
            const encrypted = await SecureStorage.encrypt(value, masterKey);
            localStorage.setItem(key, encrypted);
        } catch (e) {
            console.error(`Failed to encrypt ${key}`, e);
            localStorage.setItem(key, value);
        }
    };

    let currentPath = '';
    let remoteCurrentPath = '';
    let queuedFiles = new Map(); // path -> fileInfo
    let dropData = null;
    let localFilesList = [];
    let remoteFilesList = [];

    // --- Sort State (persisted) ---
    const loadSortState = async (key) => {
        const saved = await getSecureItem(`uplarr_sort_${key}`);
        if (saved) return JSON.parse(saved);
        return { key: 'name', dir: 'asc' };
    };

    const saveSortState = async (key, state) => {
        await setSecureItem(`uplarr_sort_${key}`, JSON.stringify(state));
    };

    let localSort = { key: 'name', dir: 'asc' };
    let remoteSort = { key: 'name', dir: 'asc' };
    let lastCheckedIndex = -1; // for shift-click bulk select
    let compactState = { local: false, remote: false, global: false };

    const applyCompact = () => {
        localPane.classList.toggle('compact', compactState.local);
        remotePane.classList.toggle('compact', compactState.remote);
        document.body.classList.toggle('global-compact', compactState.global);
        compactToggle.classList.toggle('active', compactState.local);
        compactToggle.setAttribute('aria-pressed', compactState.local.toString());
        remoteCompactToggle.classList.toggle('active', compactState.remote);
        remoteCompactToggle.setAttribute('aria-pressed', compactState.remote.toString());
        if (toggleGlobalCompact) toggleGlobalCompact.checked = !!compactState.global;
    };
    
    const saveCompactState = async (state) => {
        await setSecureItem('uplarr_compact', JSON.stringify(state));
    };

    const loadCompactState = async () => {
        const saved = await getSecureItem('uplarr_compact');
        if (saved !== null) {
            compactState = JSON.parse(saved);
            applyCompact();
        }
    };

    compactToggle.addEventListener('click', async () => {
        compactState.local = !compactState.local;
        applyCompact();
        await saveCompactState(compactState);
    });

    remoteCompactToggle.addEventListener('click', async () => {
        compactState.remote = !compactState.remote;
        applyCompact();
        await saveCompactState(compactState);
    });

    if (toggleGlobalCompact) {
        toggleGlobalCompact.addEventListener('change', async (e) => {
            compactState.global = e.target.checked;
            applyCompact();
            await saveCompactState(compactState);
        });
    }

    loadCompactState();

    // --- Sort Logic ---
    // --- Navigation & Search Logic ---

    // Instant Search Functionalify

    // ⚡ Bolt: Debounce search inputs to prevent jank when typing quickly.
    // 📊 Impact: Reduces UI render cycle executions on every keystroke by combining them.
    const debounce = (func, delay) => {
        let timeoutId;
        return (...args) => {
            clearTimeout(timeoutId);
            timeoutId = setTimeout(() => func(...args), delay);
        };
    };

    localSearchInput.addEventListener('input', debounce((e) => {
        localFilter = e.target.value.toLowerCase();
        renderLocalFiles();
    }, 250));

    remoteSearchInput.addEventListener('input', debounce((e) => {
        remoteFilter = e.target.value.toLowerCase();
        renderRemoteFiles();
    }, 250));

    // Interactive Breadcrumb Generator
    const renderPath = (container, path, isRemote = false) => {
        container.innerHTML = '';
        const rootIcon = document.createElement('span');
        rootIcon.className = 'breadcrumb-segment root';
        rootIcon.textContent = '/';
        rootIcon.title = 'Go to root';
        rootIcon.addEventListener('click', () => isRemote ? fetchRemoteFiles('/') : fetchFiles(''));
        container.appendChild(rootIcon);

        const cleanPath = path.replace(/^[\\/]+|[\\/]+$/g, '');
        if (!cleanPath) return;

        const parts = cleanPath.split(/[\\/]/);
        let accumulated = '';
        parts.forEach((part, idx) => {
            const separator = document.createElement('span');
            separator.className = 'breadcrumb-separator';
            separator.textContent = '/';
            container.appendChild(separator);

            accumulated += (idx === 0 ? part : '/' + part);
            const currentAcc = accumulated; // closure for the click handler
            const segment = document.createElement('span');
            segment.className = 'breadcrumb-segment';
            segment.textContent = part;
            segment.title = `Jump to ${part}`;
            segment.addEventListener('click', () => isRemote ? fetchRemoteFiles(currentAcc) : fetchFiles(currentAcc));
            container.appendChild(segment);
        });
    };

    // Skeleton Loader for smooth transitions
    const showSkeleton = (container, count = 5, isLocal = true) => {
        container.innerHTML = '';
        for (let i = 0; i < count; i++) {
            const row = document.createElement('tr');
            row.className = 'skeleton-row';
            row.innerHTML = `
                ${isLocal ? `<td class="col-check"></td>` : ''}
                <td class="col-name">...</td>
                <td class="col-size">-</td>
                <td class="col-type">-</td>
            `;
            container.appendChild(row);
        }
    };

    // --- Sort Logic ---

    // ⚡ Bolt: Initialize Intl.Collator once instead of calling localeCompare in a tight loop
    // 📊 Impact: ~40x faster string sorting for large file lists
    const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });
    const basicCollator = new Intl.Collator();

    const sortFiles = (files, sortKey, sortDir) => {
        const sorted = [...files];
        const dirMul = sortDir === 'asc' ? 1 : -1;

        sorted.sort((a, b) => {
            // Directories always first
            if (a.is_dir && !b.is_dir) return -1;
            if (!a.is_dir && b.is_dir) return 1;

            switch (sortKey) {
                case 'name':
                    return dirMul * collator.compare(a.name, b.name);
                case 'size':
                    return dirMul * ((a.size || 0) - (b.size || 0));
                case 'type': {
                    const extA = a.name.includes('.') ? a.name.split('.').pop().toLowerCase() : '';
                    const extB = b.name.includes('.') ? b.name.split('.').pop().toLowerCase() : '';
                    return dirMul * basicCollator.compare(extA, extB);
                }
                default:
                    return 0;
            }
        });
        return sorted;
    };


    // --- Selection Bar & Shortcut Logic ---

    const updateSelectionBar = () => {
        const count = queuedFiles.size;
        selectionCount.textContent = count;
        if (count > 0) {
            selectionBar.classList.remove('hidden');
        } else {
            selectionBar.classList.add('hidden');
        }
    };

    selectionClearBtn.addEventListener('click', () => {
        queuedFiles.clear();
        renderLocalFiles();
        updateUploadButtonText();
        updateSelectionBar();
    });

    selectionQueueBtn.addEventListener('click', () => {
        uploadBtn.click();
    });

    selectionDeleteBtn.addEventListener('click', () => {
        deleteBtn.click();
    });

    // Keyboard Shortcuts
    document.addEventListener('keydown', (e) => {
        // Prevent shortcuts when typing in inputs
        if (['INPUT', 'TEXTAREA'].includes(document.activeElement.tagName)) {
            if (e.key === 'Escape') document.activeElement.blur();
            return;
        }

        switch (e.key) {
            case '/':
                e.preventDefault();
                localSearchInput.focus();
                break;
            case 'Backspace':
                e.preventDefault();
                upBtn.click();
                break;
            case 'Delete':
                if (selectionDeleteBtn) selectionDeleteBtn.click();
                break;
            case 'Enter':
                if (e.ctrlKey) uploadBtn.click();
                break;
            case 'Escape':
                selectionClearBtn.click();
                break;
        }
    });

    // Progress Ring Helper
    const createProgressRing = (id, radius = 8) => {
        const circumference = 2 * Math.PI * radius;
        return `
            <svg class="progress-ring" height="${radius * 2.5}" width="${radius * 2.5}">
                <circle class="progress-bg" stroke="currentColor" stroke-width="2" fill="transparent" r="${radius}" cx="${radius * 1.25}" cy="${radius * 1.25}" />
                <circle id="ring-${id}" class="progress-ring__circle" stroke-width="2" stroke-dasharray="${circumference} ${circumference}" stroke-dashoffset="${circumference}" fill="transparent" r="${radius}" cx="${radius * 1.25}" cy="${radius * 1.25}" />
            </svg>
        `;
    };

    const setProgress = (id, percent) => {
        const ring = document.getElementById(`ring-${id}`);
        if (!ring) return;
        const radius = ring.r.baseVal.value;
        const circumference = 2 * Math.PI * radius;
        const offset = circumference - (percent / 100 * circumference);
        ring.style.strokeDashoffset = offset;
    };

    // Global Drag & Drop Feedback
    let dragCounter = 0;
    window.addEventListener('dragenter', (e) => {
        e.preventDefault();
        dragCounter++;
        dropOverlay.classList.remove('hidden');
    });

    window.addEventListener('dragleave', (e) => {
        e.preventDefault();
        dragCounter--;
        if (dragCounter === 0) dropOverlay.classList.add('hidden');
    });

    window.addEventListener('dragover', (e) => e.preventDefault());
    window.addEventListener('drop', (e) => {
        e.preventDefault();
        dragCounter = 0;
        dropOverlay.classList.add('hidden');
    });

    const updateSortHeaders = (tableId, sortState) => {
        const table = document.getElementById(tableId);
        table.querySelectorAll('th.sortable').forEach(th => {
            th.classList.remove('sort-asc', 'sort-desc');
            const arrow = th.querySelector('.sort-arrow');
            if (arrow) arrow.textContent = '\u25B2';
            if (th.dataset.sort === sortState.key) {
                th.classList.add(sortState.dir === 'asc' ? 'sort-asc' : 'sort-desc');
                if (arrow) arrow.textContent = sortState.dir === 'asc' ? '\u25B2' : '\u25BC';
            }
        });
    };


    // Bind sort clicks for local file table
    document.querySelectorAll('#file-table th.sortable').forEach(th => {
        th.addEventListener('click', async () => {
            const key = th.dataset.sort;
            if (localSort.key === key) {
                localSort.dir = localSort.dir === 'asc' ? 'desc' : 'asc';
            } else {
                localSort.key = key;
                localSort.dir = 'asc';
            }
            await saveSortState('local', localSort);
            renderLocalFiles();
        });
    });

    // Bind sort clicks for remote file table
    document.querySelectorAll('#remote-file-table th.sortable').forEach(th => {
        th.addEventListener('click', async () => {
            const key = th.dataset.sort;
            if (remoteSort.key === key) {
                remoteSort.dir = remoteSort.dir === 'asc' ? 'desc' : 'asc';
            } else {
                remoteSort.key = key;
                remoteSort.dir = 'asc';
            }
            await saveSortState('remote', remoteSort);
            renderRemoteFiles();
        });
    });

    // --- Phase 4: Toast System ---
    const showToast = (message, type = 'info', duration = 4000) => {
        const toast = document.createElement('div');
        toast.className = `toast toast-${type}`;
        
        let iconName = 'icon-info';
        if (type === 'success') iconName = 'icon-check';
        if (type === 'error') iconName = 'icon-alert';
        if (type === 'warn') iconName = 'icon-alert';

        toast.innerHTML = `
            <div class="toast-icon">
                <svg width="20" height="20"><use href="#${iconName}"></use></svg>
            </div>
            <div class="toast-content">${escapeHTML(message)}</div>
        `;

        toastContainer.appendChild(toast);

        const remove = () => {
            toast.classList.add('removing');
            setTimeout(() => toast.remove(), 300);
        };

        if (duration > 0) {
            setTimeout(remove, duration);
        }

        toast.addEventListener('click', remove);
    };

    // Replace showStatus with showToast
    const showStatus = (msg, type) => {
        showToast(msg, type);
    };

    // Log Filtering
    let currentLogFilter = '';
    logSearchInput.addEventListener('input', (e) => {
        currentLogFilter = e.target.value.toLowerCase();
        const logs = logContainer.querySelectorAll('.log-entry');
        logs.forEach(log => {
            const visible = !currentLogFilter || log.textContent.toLowerCase().includes(currentLogFilter);
            log.style.display = visible ? 'block' : 'none';
        });
    });

    const addLog = (msg, level = 'info') => {
        const isNearBottom = logContainer.scrollHeight - logContainer.clientHeight <= logContainer.scrollTop + 30;
        
        const entry = document.createElement('div');
        entry.className = `log-entry log-${level}`;
        const time = new Date().toLocaleTimeString();
        entry.textContent = `[${time}] ${msg}`;
        
        if (currentLogFilter && !msg.toLowerCase().includes(currentLogFilter)) {
            entry.style.display = 'none';
        }
        
        logContainer.appendChild(entry);
        
        if (isNearBottom) {
            logContainer.scrollTop = logContainer.scrollHeight;
        }
        
        // Limit logs to 100 as requested
        while (logContainer.children.length > 100) {
            logContainer.removeChild(logContainer.firstChild);
        }
    };

    // Sparkline Generator
    const generateSparklinePath = (data, width, height) => {
        if (data.length < 2) return '';
        const max = Math.max(...data, 1024); // min scale to 1KB/s
        const points = data.map((val, i) => {
            const x = (i / (data.length - 1)) * width;
            const y = height - (val / max) * height;
            return `${x.toFixed(1)},${y.toFixed(1)}`;
        });
        return `M ${points.join(' L ')}`;
    };

    // --- Phase 5: Command Center Logic ---

    // Theme Customizer
    const hexToRgba = (hex, alpha) => {
        let r = 0, g = 0, b = 0;
        if (hex.length === 4) {
            r = parseInt(hex[1] + hex[1], 16);
            g = parseInt(hex[2] + hex[2], 16);
            b = parseInt(hex[3] + hex[3], 16);
        } else if (hex.length === 7) {
            r = parseInt(hex.substring(1, 3), 16);
            g = parseInt(hex.substring(3, 5), 16);
            b = parseInt(hex.substring(5, 7), 16);
        }
        return `rgba(${r}, ${g}, ${b}, ${alpha})`;
    };

    document.querySelectorAll('.theme-swatch').forEach(swatch => {
        swatch.addEventListener('click', () => {
            const color = swatch.dataset.color;
            const root = document.documentElement;
            root.style.setProperty('--accent-primary', color);
            root.style.setProperty('--accent-secondary', hexToRgba(color, 0.8));
            root.style.setProperty('--accent-primary-alpha', hexToRgba(color, 0.15));
            root.style.setProperty('--accent-subtle', hexToRgba(color, 0.05));
            setSecureItem('uplarr_theme_accent', color);
            showToast(`Theme updated`, 'success', 2000);
        });
    });

    const initTheme = async () => {
        const saved = await getSecureItem('uplarr_theme_accent');
        if (saved) {
            const root = document.documentElement;
            root.style.setProperty('--accent-primary', saved);
            root.style.setProperty('--accent-secondary', hexToRgba(saved, 0.8));
            root.style.setProperty('--accent-primary-alpha', hexToRgba(saved, 0.15));
            root.style.setProperty('--accent-subtle', hexToRgba(saved, 0.05));
        }
    };

    // Layout Toggles
    viewToggleBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const isHidden = viewMenu.classList.toggle('hidden');
        viewToggleBtn.setAttribute('aria-expanded', (!isHidden).toString());
    });

    document.addEventListener('click', (e) => {
        if (!viewMenu.contains(e.target)) {
            viewMenu.classList.add('hidden');
            viewToggleBtn.setAttribute('aria-expanded', 'false');
        }
    });

    ['metrics', 'queue', 'logs'].forEach(id => {
        const cb = document.getElementById(`toggle-${id}`);
        const section = id === 'metrics' ? headerHostMetrics : document.querySelector(`.section-${id}`);
        
        cb.addEventListener('change', async () => {
            if (section) section.style.display = cb.checked ? '' : 'none';
            await setSecureItem(`uplarr_view_${id}`, cb.checked);
        });

        // Restore state
        getSecureItem(`uplarr_view_${id}`).then(val => {
            if (val === 'false') {
                cb.checked = false;
                if (section) section.style.display = 'none';
            }
        });
    });

    // Bulk Renaming
    const updateRenamePreview = () => {
        const search = renameSearch.value;
        const replace = renameReplace.value;
        const files = Array.from(queuedFiles.keys());
        renamePreview.innerHTML = '';
        
        files.forEach((oldPath, idx) => {
            const oldName = oldPath.split('/').pop();
            let newName = oldName;
            
            try {
                if (search) {
                    const re = new RegExp(search, 'g');
                    newName = oldName.replace(re, replace.replace(/\$idx/g, (idx + 1).toString()));
                }
            } catch (e) {}

            const item = document.createElement('div');
            item.className = 'rename-preview-item';
            item.innerHTML = `
                <span class="rename-from">${escapeHTML(oldName)}</span>
                <span class="rename-arrow">&rarr;</span>
                <span class="rename-to">${escapeHTML(newName)}</span>
            `;
            renamePreview.appendChild(item);
        });
    };

    if (selectionRenameBtn) {
        selectionRenameBtn.addEventListener('click', () => {
            if (queuedFiles.size === 0) return showToast("Select files to rename", "warn");
            if (renameCountBadge) renameCountBadge.textContent = queuedFiles.size;
            if (renameSearch) renameSearch.value = '';
            if (renameReplace) renameReplace.value = '';
            updateRenamePreview();
            if (renameModal) renameModal.classList.remove('hidden');
        });
    }

    if (renameSearch) renameSearch.addEventListener('input', updateRenamePreview);
    if (renameReplace) renameReplace.addEventListener('input', updateRenamePreview);
    if (renameCancelBtn) renameCancelBtn.addEventListener('click', () => renameModal.classList.add('hidden'));

    if (renameConfirmBtn) {
        renameConfirmBtn.addEventListener('click', async () => {
            if (!renameSearch || !renameReplace) return;
            const search = renameSearch.value;
            const replace = renameReplace.value;
            if (!search) return showToast("Enter a search pattern", "warn");

            const items = Array.from(queuedFiles.entries());
            if (items.length === 0) return;

            renameConfirmBtn.disabled = true;
            renameConfirmBtn.textContent = 'Renaming...';
            
            try {
                const operations = items.map(([path, info], idx) => {
                    const lastSlash = path.lastIndexOf('/');
                    const dir = path.substring(0, lastSlash + 1);
                    const oldName = path.substring(lastSlash + 1);
                    const re = new RegExp(search, 'g');
                    const newName = oldName.replace(re, replace.replace(/\$idx/g, (idx + 1).toString()));
                    return { old: path, new: dir + newName };
                });

                const res = await fetch('/api/local/rename-bulk', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ operations })
                });

                if (res.ok) {
                    showToast("Rename successful", "success");
                    queuedFiles.clear();
                    updateSelectionBar();
                    if (renameModal) renameModal.classList.add('hidden');
                    refreshLocal();
                } else {
                    const data = await res.json();
                    showToast(`Rename failed: ${data.error}`, "error");
                }
            } catch (e) {
                showToast("Request failed", "error");
            } finally {
                if (renameConfirmBtn) {
                    renameConfirmBtn.disabled = false;
                    renameConfirmBtn.textContent = 'Apply Rename';
                }
            }
        });
    }

    // Session Analytics
    const updateSessionStats = () => {
        const total = sessionTotalBytes + currentLiveBytes;
        statTotalData.textContent = formatSize(total);
        const elapsed = (Date.now() - sessionStart) / 1000;
        const avg = total / (elapsed || 1);
        statAvgSpeed.textContent = formatRate(avg);
    };

    // --- Phase 6: Pro Polish Logic ---

    // Custom Context Menu
    const showContextMenu = (e, file, isRemote) => {
        e.preventDefault();
        const { clientX: x, clientY: y } = e;
        contextMenu.style.left = `${x}px`;
        contextMenu.style.top = `${y}px`;
        contextMenu.classList.remove('hidden');

        // Store target file data on the menu
        contextMenu.dataset.path = file.path;
        contextMenu.dataset.isRemote = isRemote;
        contextMenu.dataset.name = file.name;
    };

    document.addEventListener('click', () => contextMenu.classList.add('hidden'));
    document.addEventListener('contextmenu', (e) => {
        if (!e.target.closest('.file-row')) contextMenu.classList.add('hidden');
    });

    contextMenu.querySelectorAll('.ctx-item').forEach(item => {
        item.addEventListener('click', async () => {
            const action = item.dataset.action;
            const path = contextMenu.dataset.path;
            const name = contextMenu.dataset.name;
            const isRemote = contextMenu.dataset.isRemote === 'true';

            if (action === 'rename') {
                // Reuse existing rename modal logic
                renameSearch.value = name;
                renameReplace.value = name;
                queuedFiles.clear();
                queuedFiles.set(path, { name });
                updateRenamePreview();
                renameModal.classList.remove('hidden');
            } else if (action === 'delete') {
                if (confirm(`Delete ${name}?`)) {
                    const endpoint = isRemote ? '/api/remote/files/action' : '/api/files/action';
                    const res = await fetch(endpoint, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ action: 'delete', path })
                    });
                    if (res.ok) {
                        showToast(`Deleted ${name}`, 'success');
                        isRemote ? fetchRemoteFiles(remoteCurrentPath) : fetchFiles(currentPath);
                    } else {
                        showToast(`Delete failed`, 'error');
                    }
                }
            } else if (action === 'download') {
                if (isRemote) return showToast("Direct remote download not yet implemented", "info");
                window.open(`/api/files/download?path=${encodeURIComponent(path)}`, '_blank');
            }
        });
    });

    // Folder Tree Logic
    const updateFolderTree = (path, isRemote) => {
        const type = isRemote ? 'remote' : 'local';
        const history = folderTreeHistory[type];
        
        // Ensure path is in history
        history.add(path || '/');
        
        // Limit history to last 15 unique entries to keep sidebar clean
        if (history.size > 15) {
            const arr = Array.from(history);
            // Keep '/' if it exists, otherwise just shift
            if (arr[0] === '/' && arr.length > 15) {
                const newSet = new Set(['/']);
                arr.slice(-14).forEach(item => newSet.add(item));
                folderTreeHistory[type] = newSet;
            } else if (arr.length > 15) {
                folderTreeHistory[type] = new Set(arr.slice(-15));
            }
        }
        
        renderFolderTree(isRemote);
    };

    const renderFolderTree = (isRemote) => {
        const type = isRemote ? 'remote' : 'local';
        const container = document.getElementById(`${type}-tree-container`);
        if (!container) return;

        container.innerHTML = '';
        // Sort alphabetically but root always first
        const sortedItems = Array.from(folderTreeHistory[type]).sort((a, b) => {
            if (a === '/') return -1;
            if (b === '/') return 1;
            return basicCollator.compare(a, b);
        });

        sortedItems.forEach(p => {
            const item = document.createElement('div');
            item.className = 'tree-item' + ((isRemote ? remoteCurrentPath : currentPath) === p ? ' active' : '');
            let name = p === '/' ? 'Root' : p.split(/[\\/]/).pop() || p;
            
            item.innerHTML = `
                <svg class="icon-inline"><use href="#icon-folder"></use></svg>
                <span>${escapeHTML(name)}</span>
            `;
            item.title = p;
            item.onclick = () => isRemote ? fetchRemoteFiles(p) : fetchFiles(p);
            container.appendChild(item);
        });
    };

    // PWA Service Worker Registration
    if ('serviceWorker' in navigator) {
        window.addEventListener('load', () => {
            navigator.serviceWorker.register('/static/sw.js').catch(() => {});
        });
    }

    // --- Helpers ---




    const escapeHTML = (str) => {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;');
    };

    const formatSize = (bytes) => {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    };


    const getFormData = () => {
        const formData = new FormData(sftpForm);
        return {
            host: formData.get('host'),
            port: parseInt(formData.get('port')),
            user: formData.get('user'),
            password: formData.get('password'),
            key_path: formData.get('key_path'),
            known_hosts_path: formData.get('known_hosts_path'),
            remote_dir: formData.get('remote_dir'),
            delete_after_verify: formData.get('delete_after_verify') === 'on',
            overwrite: formData.get('overwrite') === 'on',
            max_retries: parseInt(formData.get('max_retries')),
            skip_host_key_verification: formData.get('skip_host_key_verification') === 'on',
            rate_limit_kbps: parseInt(formData.get('rate_limit_kbps')) || 0,
            max_latency_ms: parseInt(formData.get('max_latency_ms')) || 0,
            min_limit_kbps: parseInt(formData.get('min_limit_kbps')) || 0,
            concurrent_files: parseInt(formData.get('concurrent_files')) || 1,
            files: Array.from(queuedFiles.keys())
        };
    };

    const saveFormData = async () => {
        const data = getFormData();
        // Remove transient/large data
        delete data.files;
        await setSecureItem('uplarr_form_data', JSON.stringify(data));
    };

    const restoreFormData = async () => {
        const saved = await getSecureItem('uplarr_form_data');
        if (!saved) return;
        try {
            const data = JSON.parse(saved);
            for (const key in data) {
                const el = sftpForm.elements[key];
                if (!el) continue;
                if (el.type === 'checkbox') {
                    el.checked = data[key];
                } else {
                    el.value = data[key];
                }
            }
        } catch (e) { 
            console.error('Failed to restore form data', e);
            localStorage.removeItem('uplarr_form_data');
        }
    };

    sftpForm.addEventListener('input', saveFormData);


    const updateUploadButtonText = () => {
        const textSpan = uploadBtn.querySelector('.btn-text');
        if (textSpan) {
            textSpan.textContent = queuedFiles.size > 0 ? `Queue ${queuedFiles.size} Files` : "Queue All Files";
        } else {
            uploadBtn.textContent = queuedFiles.size > 0 ? `Queue ${queuedFiles.size} Files` : "Queue All Files";
        }
    };

    const toggleButtonLoading = (btn, isLoading, loadingText = "Loading...", restoreFn = null) => {
        if (!btn) return;
        const icon = btn.querySelector('.btn-icon');
        const spinner = btn.querySelector('.btn-spinner');
        const textSpan = btn.querySelector('.btn-text');

        if (isLoading) {
            if (icon) icon.classList.add('hidden');
            if (spinner) spinner.classList.remove('hidden');
            if (textSpan) {
                btn.dataset.originalText = textSpan.textContent;
                textSpan.textContent = loadingText;
            }
        } else {
            if (icon) icon.classList.remove('hidden');
            if (spinner) spinner.classList.add('hidden');
            if (restoreFn) {
                restoreFn();
            } else if (textSpan && btn.dataset.originalText) {
                textSpan.textContent = btn.dataset.originalText;
            }
        }
    };

    // --- Local Files ---

    const renderLocalFiles = () => {
        // ⚡ Bolt: Filter files before sorting to improve performance.
        // 📊 Impact: O(n log n) sorting now only runs on the matching files, not the entire list.
        const filtered = localFilesList.filter(f => f.name.toLowerCase().includes(localFilter));
        const sorted = sortFiles(filtered, localSort.key, localSort.dir);
        updateSortHeaders('file-table', localSort);
        fileListBody.innerHTML = '';

        // ⚡ Bolt: Batch DOM inserts using DocumentFragment to prevent layout thrashing
        // 📊 Impact: O(1) reflow instead of O(n) for large directories, making rendering significantly smoother
        const fragment = document.createDocumentFragment();

        if (sorted.length === 0) {
            const row = document.createElement('tr');
            const td = document.createElement('td');
            td.colSpan = 4;
            td.className = 'empty-msg';
            td.textContent = localFilter ? `No files matching "${localFilter}"` : 'This folder is empty';
            row.appendChild(td);
            fileListBody.appendChild(row);
            return;
        }

        sorted.forEach(file => {
            const fullRelPath = currentPath ? `${currentPath}/${file.name}` : file.name;
            const row = document.createElement('tr');

            if (file.is_dir) {
                row.className = 'clickable-row folder-row';
                row.addEventListener('click', (e) => {
                    if (e.target.type !== 'checkbox') fetchFiles(fullRelPath);
                });
            } else {
                row.draggable = true;
                row.addEventListener('dragstart', (e) => {
                    e.dataTransfer.setData('text/plain', JSON.stringify({ path: fullRelPath, name: file.name, size: file.size }));
                    row.classList.add('dragging');
                });
                row.addEventListener('dragend', () => row.classList.remove('dragging'));
            }

            row.addEventListener('contextmenu', (e) => showContextMenu(e, { path: fullRelPath, name: file.name }, false));


            // Checkbox Cell
            const tdCheck = document.createElement('td');
            tdCheck.className = 'col-check';
            const cb = document.createElement('input');
            cb.type = 'checkbox';
            cb.className = 'file-checkbox';
            cb.dataset.path = fullRelPath;
            cb.dataset.name = file.name;
            if (file.is_dir) cb.disabled = true;
            if (queuedFiles.has(fullRelPath)) cb.checked = true;

            cb.addEventListener('click', (e) => {
                const allCheckboxes = Array.from(fileListBody.querySelectorAll('.file-checkbox:not(:disabled)'));
                const currentIndex = allCheckboxes.indexOf(cb);

                if (e.shiftKey && lastCheckedIndex >= 0 && lastCheckedIndex !== currentIndex) {
                    const start = Math.min(lastCheckedIndex, currentIndex);
                    const end = Math.max(lastCheckedIndex, currentIndex);
                    const newState = cb.checked;
                    for (let i = start; i <= end; i++) {
                        const c = allCheckboxes[i];
                        c.checked = newState;
                        const p = c.dataset.path;
                        const n = c.dataset.name;
                        if (newState) queuedFiles.set(p, { name: n });
                        else queuedFiles.delete(p);
                    }
                } else {
                    if (cb.checked) queuedFiles.set(fullRelPath, { name: file.name });
                    else queuedFiles.delete(fullRelPath);
                }
                lastCheckedIndex = currentIndex;
                updateUploadButtonText();
                updateSelectionBar();
            });
            tdCheck.appendChild(cb);

            // Name Cell
            const tdName = document.createElement('td');
            tdName.className = 'col-name';
            const icon = document.createElement('span');
            icon.innerHTML = `<svg class="icon-inline" width="16" height="16"><use href="#${file.is_dir ? 'icon-folder' : 'icon-file'}"></use></svg> `;
            tdName.appendChild(icon);
            tdName.appendChild(document.createTextNode(file.name));
            tdName.title = file.name;

            // Size Cell
            const tdSize = document.createElement('td');
            tdSize.className = 'col-size';
            tdSize.textContent = file.is_dir ? '-' : formatSize(file.size);

            // Type Cell
            const tdType = document.createElement('td');
            tdType.className = 'col-type';
            tdType.textContent = file.is_dir ? 'Dir' : 'File';

            row.appendChild(tdCheck);
            row.appendChild(tdName);
            row.appendChild(tdSize);
            row.appendChild(tdType);
            fragment.appendChild(row);
        });

        fileListBody.appendChild(fragment);
        lastCheckedIndex = -1;
        selectAllCheckbox.checked = false;
    };

    const fetchFiles = async (path = '') => {
        showSkeleton(fileListBody);
        try {
            const response = await fetch(`/api/files?path=${encodeURIComponent(path)}`);
            if (response.status === 401) return handleAuthFailure('fetchFiles', 401);
            const data = await response.json();
            currentPath = data.current_path;
            await setSecureItem('uplarr_local_path', currentPath);
            renderPath(localBreadcrumb, currentPath, false);
            localFilesList = data.files || [];
            renderLocalFiles();
            updateFolderTree(currentPath, false);
        } catch (err) { addLog(`Local fetch error: ${err.message}`, 'error'); }

    };

    if (refreshBtn) refreshBtn.addEventListener('click', () => fetchFiles(currentPath));
    if (upBtn) upBtn.addEventListener('click', () => {
        if (!currentPath || currentPath === '.') return;
        const parts = currentPath.split(/[\\/]/);
        parts.pop();
        fetchFiles(parts.join('/'));
    });

    if (mkdirBtn) mkdirBtn.addEventListener('click', async () => {
        const name = prompt("Enter folder name:");
        if (!name) return;
        const res = await fetch('/api/files/action', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ action: 'mkdir', path: currentPath ? `${currentPath}/${name}` : name })
        });
        if (res.ok) fetchFiles(currentPath);
        else addLog(`Mkdir failed: ${res.statusText}`, 'error');
    });

    if (selectionDeleteBtn) selectionDeleteBtn.addEventListener('click', async () => {
        if (queuedFiles.size === 0) return alert("Select files to delete first.");
        if (!confirm(`Delete ${queuedFiles.size} items?`)) return;
        
        try {
            const operations = Array.from(queuedFiles.keys()).map(path => ({ action: 'delete', path }));
            // We'll just run them sequentially or use a bulk endpoint if available
            // Assuming current local action endpoint supports single delete
            for (const op of operations) {
                 await fetch('/api/files/action', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ action: 'delete', path: op.path })
                });
            }
            showToast("Selection deleted", "success");
            queuedFiles.clear();
            updateSelectionBar();
            fetchFiles(currentPath);
        } catch (e) {
            showToast("Delete failed", "error");
        }
    });

    if (selectAllCheckbox) selectAllCheckbox.addEventListener('change', (e) => {
        const checked = e.target.checked;
        document.querySelectorAll('#file-list-body .file-checkbox:not(:disabled)').forEach(cb => {
            cb.checked = checked;
            const path = cb.dataset.path;
            const name = cb.dataset.name;
            if (checked) queuedFiles.set(path, { name });
            else queuedFiles.delete(path);
        });
        updateUploadButtonText();
        updateSelectionBar();
    });

    // --- Remote Files ---

    const renderRemoteFiles = () => {
        const filtered = remoteFilesList.filter(f => f.name.toLowerCase().includes(remoteFilter));
        const sorted = sortFiles(filtered, remoteSort.key, remoteSort.dir);
        updateSortHeaders('remote-file-table', remoteSort);
        remoteFileListBody.innerHTML = '';

        if (sorted.length === 0) {
            const row = document.createElement('tr');
            const td = document.createElement('td');
            td.colSpan = 3;
            td.className = 'empty-msg';
            td.textContent = remoteFilter ? `No files matching "${remoteFilter}"` : 'Empty directory';
            row.appendChild(td);
            remoteFileListBody.appendChild(row);
            return;
        }

        // ⚡ Bolt: Batch DOM inserts using DocumentFragment to prevent layout thrashing
        // 📊 Impact: O(1) reflow instead of O(n) for remote directory rendering
        const fragment = document.createDocumentFragment();

        sorted.forEach(file => {
            const row = document.createElement('tr');
            if (file.is_dir) {
                row.className = 'clickable-row folder-row';
                row.addEventListener('click', () => {
                    const newPath = remoteCurrentPath.endsWith('/') ? remoteCurrentPath + file.name : remoteCurrentPath + '/' + file.name;
                    fetchRemoteFiles(newPath);
                });
            }

            const rFullRelPath = remoteCurrentPath.endsWith('/') ? remoteCurrentPath + file.name : remoteCurrentPath + '/' + file.name;
            row.addEventListener('contextmenu', (e) => showContextMenu(e, { path: rFullRelPath, name: file.name }, true));


            const tdName = document.createElement('td');
            tdName.className = 'col-name';
            const icon = document.createElement('span');
            icon.innerHTML = `<svg class="icon-inline" width="16" height="16"><use href="#${file.is_dir ? 'icon-folder' : 'icon-file'}"></use></svg> `;
            tdName.appendChild(icon);
            tdName.appendChild(document.createTextNode(file.name));
            tdName.title = file.name;

            const tdSize = document.createElement('td');
            tdSize.className = 'col-size';
            tdSize.textContent = file.is_dir ? '-' : formatSize(file.size);

            const tdType = document.createElement('td');
            tdType.className = 'col-type';
            tdType.textContent = file.is_dir ? 'Dir' : 'File';

            row.appendChild(tdName);
            row.appendChild(tdSize);
            row.appendChild(tdType);
            fragment.appendChild(row);
        });

        remoteFileListBody.appendChild(fragment);
    };

    const fetchRemoteFiles = async (path = null) => {
        if (!sftpForm.checkValidity()) return sftpForm.reportValidity();
        const config = getFormData();
        const targetPath = path !== null ? path : (remoteCurrentPath || config.remote_dir);
        showSkeleton(remoteFileListBody, 3);
        try {
            const response = await fetch(`/api/remote/files?path=${encodeURIComponent(targetPath)}`, {
                method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config)
            });
            if (response.status === 401) return handleAuthFailure('fetchRemoteFiles', 401);
            const data = await response.json();
            if (!response.ok) throw new Error(data.error || 'Failed to fetch remote files');
            
            remoteCurrentPath = data.current_path;
            
            // Sync with configuration form input for consistency
            const remoteDirInput = document.getElementById('remote_dir');
            if (remoteDirInput) {
                remoteDirInput.value = remoteCurrentPath;
            }
            
            await setSecureItem('uplarr_remote_path', remoteCurrentPath);
            renderPath(remoteBreadcrumb, remoteCurrentPath, true);
            remoteFilesList = data.files || [];
            renderRemoteFiles();
            updateFolderTree(remoteCurrentPath, true);
        } catch (err) {
            addLog(`Remote fetch error: ${err.message}`, 'error');
            remoteFileListBody.innerHTML = '';
            const row = document.createElement('tr');
            const td = document.createElement('td');
            td.colSpan = 3;
            td.className = 'log-error';
            td.textContent = `Error: ${err.message}`;
            row.appendChild(td);
            remoteFileListBody.appendChild(row);
        }
    };

    // Manual navigation via remote directory input
    const remoteDirInputManual = document.getElementById('remote_dir');
    if (remoteDirInputManual) {
        remoteDirInputManual.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                fetchRemoteFiles(remoteDirInputManual.value);
            }
        });
    }

    if (remoteRefreshBtn) remoteRefreshBtn.addEventListener('click', () => fetchRemoteFiles(remoteCurrentPath));
    if (remoteUpBtn) remoteUpBtn.addEventListener('click', () => {
        if (!remoteCurrentPath || remoteCurrentPath === '/') return;
        const parts = remoteCurrentPath.split('/');
        parts.pop();
        let parent = parts.join('/');
        if (parent === '') parent = '/';
        fetchRemoteFiles(parent);
    });

    if (remoteMkdirBtn) remoteMkdirBtn.addEventListener('click', async () => {
        const name = prompt("Enter remote folder name:");
        if (!name) return;
        const config = getFormData();
        const basePath = remoteCurrentPath || config.remote_dir;
        const path = basePath.endsWith('/') ? basePath + name : basePath + '/' + name;
        const res = await fetch('/api/remote/files/action', {
            method: 'POST', 
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ action: 'mkdir', path, config })
        });
        if (res.ok) fetchRemoteFiles(remoteCurrentPath || config.remote_dir);
        else addLog(`Remote mkdir failed`, 'error');
    });

    // --- Drag & Drop ---

    remoteDropZone.addEventListener('dragover', (e) => { e.preventDefault(); remoteDropZone.classList.add('drop-target'); });
    remoteDropZone.addEventListener('dragleave', () => remoteDropZone.classList.remove('drop-target'));
    remoteDropZone.addEventListener('drop', (e) => {
        e.preventDefault();
        remoteDropZone.classList.remove('drop-target');
        const rawData = e.dataTransfer.getData('text/plain');
        try {
            dropData = JSON.parse(rawData);
            modalFileInfo.textContent = `Upload ${dropData.name} to ${remoteCurrentPath}?`;
            dropModal.classList.remove('hidden');
        } catch (err) {
            console.error('Failed to parse drag data:', err, rawData);
            addLog('Invalid drag data: could not parse file info', 'error');
        }
    });

    modalConfirmBtn.addEventListener('click', async () => {
        if (!dropData) return;
        const config = getFormData();
        config.files = [dropData.path];
        config.remote_dir = remoteCurrentPath;
        config.delete_after_verify = modalDeleteLocal.checked;
        config.overwrite = modalOverwriteRemote.checked;
        dropModal.classList.add('hidden');
        try {
            const res = await fetch('/api/upload', { 
                method: 'POST', 
                headers: { 'Content-Type': 'application/json' }, 
                body: JSON.stringify(config) 
            });
            if (res.ok) {
                showStatus("Task queued", "success");
                fetchQueue();
            } else {
                const data = await res.json().catch(() => ({}));
                addLog(`Failed to queue task: ${data.error || res.statusText}`, 'error');
            }
        } catch (err) {
            addLog(`Failed to queue task: ${err.message}`, 'error');
            showStatus("Upload request failed", "error");
        }
    });

    modalCancelBtn.addEventListener('click', () => dropModal.classList.add('hidden'));

    const formatRate = (bytesPerSec) => {
        if (bytesPerSec <= 0) return '-';
        if (bytesPerSec >= 1024 * 1024 * 1024) return (bytesPerSec / (1024 * 1024 * 1024)).toFixed(1) + ' GB/s';
        if (bytesPerSec >= 1024 * 1024) return (bytesPerSec / (1024 * 1024)).toFixed(1) + ' MB/s';
        if (bytesPerSec >= 1024) return (bytesPerSec / 1024).toFixed(0) + ' KB/s';
        return bytesPerSec.toFixed(0) + ' B/s';
    };

    const formatETA = (seconds) => {
        if (!seconds || seconds <= 0 || !isFinite(seconds)) return '';
        if (seconds < 60) return `${Math.ceil(seconds)}s`;
        if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${Math.ceil(seconds % 60)}s`;
        return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
    };

    const fetchQueue = async () => {
        try {
            const res = await fetch('/api/queue');
            if (res.status === 401) return handleAuthFailure('fetchQueue', 401);
            const tasks = await res.json();
            
            let activeBytesSum = 0;
            queueBody.innerHTML = '';
            // ⚡ Bolt: Batch DOM inserts using DocumentFragment to prevent layout thrashing
            // 📊 Impact: O(1) reflow instead of O(n) for queue rendering
            const fragment = document.createDocumentFragment();

            if (tasks.length === 0) {
                const row = document.createElement('tr');
                const td = document.createElement('td');
                td.colSpan = 6;
                td.className = 'empty-msg';
                td.textContent = 'Queue is empty. Select files to start transferring.';
                row.appendChild(td);
                fragment.appendChild(row);
            }

            tasks.reverse().forEach(task => {
                if (task.status !== 'Completed') {
                    activeBytesSum += task.bytes_uploaded;
                }
                const id = task.id;
                const row = document.createElement('tr');
                row.id = `queue-row-${id}`;
                
                const tdFile = document.createElement('td');
                tdFile.textContent = task.file_name;
                tdFile.title = task.file_name;
                tdFile.className = 'col-file';
                
                const tdDest = document.createElement('td');
                tdDest.textContent = task.remote_dir || '';
                tdDest.title = task.remote_dir || '';
                tdDest.className = 'col-dest';
                
                const tdStatus = document.createElement('td');
                tdStatus.className = `col-status status-${task.status.toLowerCase()}`;
                
                if (task.status === 'Running') {
                    tdStatus.innerHTML = createProgressRing(id);
                    // Defer setting progress until after row is in DOM
                    setTimeout(() => {
                        const pct = Math.round((task.bytes_uploaded / (task.total_bytes || 1)) * 100);
                        setProgress(id, pct);
                    }, 0);
                } else if (task.status === 'Completed') {
                    tdStatus.innerHTML = `<svg class="icon-success" width="16" height="16"><use href="#icon-check"></use></svg>`;
                } else if (task.status === 'Failed') {
                    tdStatus.innerHTML = `<span class="status-badge status-failed">Failed</span>`;
                    tdStatus.title = task.error || 'Unknown error';
                } else {
                    tdStatus.textContent = task.status;
                }

                const tdProgress = document.createElement('td');
                tdProgress.className = 'col-progress';
                if (task.status === 'Running' || task.status === 'Completed' || task.status === 'Failed') {
                    const pct = Math.round((task.bytes_uploaded / (task.total_bytes || 1)) * 100);
                    tdProgress.textContent = `${pct}% (${formatSize(task.bytes_uploaded)} / ${formatSize(task.total_bytes)})`;
                } else {
                    tdProgress.textContent = '-';
                }

                // Rate tracking
                const tdRate = document.createElement('td');
                tdRate.className = 'col-rate';
                if (task.status === 'Running' && task.started_at && task.bytes_uploaded > 0) {
                    const elapsed = (Date.now() - new Date(task.started_at).getTime()) / 1000;
                    if (elapsed > 0) {
                        const rate = task.bytes_uploaded / elapsed;
                        const remaining = task.total_bytes - task.bytes_uploaded;
                        const eta = remaining > 0 && rate > 0 ? remaining / rate : 0;
                        tdRate.textContent = formatRate(rate);
                        if (eta > 0) tdRate.textContent += ` (${formatETA(eta)})`;
                        globalTotalRate += rate;
                    }
                } else if (task.status === 'Completed') {
                    // Update session total for analytics (once per task)
                    if (!countedTaskIds.has(id)) {
                        sessionTotalBytes += task.total_bytes || 0;
                        countedTaskIds.add(id);
                    }
                    tdRate.textContent = '-';
                } else {
                    tdRate.textContent = '-';
                }

                
                const tdCreated = document.createElement('td');
                tdCreated.textContent = new Date(task.created_at).toLocaleTimeString();
                
                const tdActions = document.createElement('td');
                tdActions.className = 'col-actions';
                if (task.status === 'Pending' || task.status === 'Paused') {
                    const controlBtn = document.createElement('button');
                    controlBtn.className = 'action-btn';
                    controlBtn.textContent = task.status === 'Paused' ? 'Resume' : 'Pause';
                    controlBtn.addEventListener('click', () => controlTask(task.id, task.status === 'Paused' ? 'resume' : 'pause'));
                    tdActions.appendChild(controlBtn);
                } else if (task.status === 'Failed' || task.status === 'Completed') {
                    if (task.local_file_exists) {
                        const retryBtn = document.createElement('button');
                        retryBtn.className = 'action-btn';
                        retryBtn.textContent = 'Retry';
                        retryBtn.addEventListener('click', () => controlTask(task.id, 'retry'));
                        tdActions.appendChild(retryBtn);
                    }
                }
                const remBtn = document.createElement('button');
                remBtn.className = 'action-btn btn-danger-text';
                remBtn.textContent = 'Remove';
                remBtn.addEventListener('click', () => controlTask(task.id, 'remove'));
                tdActions.appendChild(remBtn);

                row.appendChild(tdStatus);
                row.appendChild(tdFile);
                row.appendChild(tdDest);
                row.appendChild(tdProgress);
                row.appendChild(tdRate);
                row.appendChild(tdCreated);
                row.appendChild(tdActions);
                fragment.appendChild(row);
            });

            queueBody.appendChild(fragment);
            lastTotalRate = globalTotalRate;
            globalTotalRate = 0;
            currentLiveBytes = activeBytesSum;
        } catch (e) {
            console.error('Failed to fetch queue:', e);
        }
    };


    const fetchStats = async () => {
        try {
            const res = await fetch('/api/stats');
            if (res.status === 401) return handleAuthFailure('fetchStats', 401);
            if (res.ok) {
                const stats = await res.json();
                renderStats(stats);
            } else {
                // Non-OK response — clear stale metrics so the overlay doesn't lie
                renderStats([]);
            }
        } catch (e) {
            // Network error — clear stale metrics
            console.error("Failed to fetch stats", e);
            renderStats([]);
        }
    };

    const renderStats = (stats) => {
        if (!stats || stats.length === 0 || !headerHostMetrics) {
            if (headerHostMetrics) headerHostMetrics.innerHTML = '';
            return;
        }

        headerHostMetrics.innerHTML = '';
        stats.forEach(s => {
            const item = document.createElement('div');
            item.className = 'host-metric-item';

            const hostName = document.createElement('span');
            hostName.className = 'host-name';
            hostName.textContent = s.host;

            const latencyVal = document.createElement('span');
            latencyVal.className = `latency ${s.last_latency_ms > 100 ? 'high' : ''}`;
            latencyVal.textContent = `${s.last_latency_ms}ms`;

            const speedVal = document.createElement('span');
            speedVal.className = 'speed';
            const hostSpeed = (s.total_speed_kbps || 0) * 1024;
            speedVal.textContent = formatRate(hostSpeed);

            item.appendChild(hostName);
            item.appendChild(latencyVal);
            item.appendChild(speedVal);
            headerHostMetrics.appendChild(item);
        });
    };


    const controlTask = async (id, action) => {
        const res = await fetch('/api/queue', { 
            method: 'POST', 
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id, action }) 
        });
        if (!res.ok) {
            const data = await res.json().catch(() => ({}));
            showToast(`Action failed: ${data.error || res.statusText}`, 'error');
        }

        fetchQueue();
    };

    const clearQueueBtn = document.getElementById('clear-queue-btn');
    if (clearQueueBtn) {
        clearQueueBtn.addEventListener('click', () => controlTask('', 'clear_finished'));
    }

    const retryAllFailedBtn = document.getElementById('retry-all-failed-btn');
    if (retryAllFailedBtn) {
        retryAllFailedBtn.addEventListener('click', () => controlTask('', 'retry_all_failed'));
    }

    // --- Actions ---
    if (testBtn) {
        testBtn.addEventListener('click', async () => {
            const config = getFormData();
            testBtn.disabled = true;
            toggleButtonLoading(testBtn, true, "Connecting...");
            showStatus("Connecting...", "info");
            try {
                const res = await fetch('/api/test-connection', { 
                    method: 'POST', 
                    headers: { 'Content-Type': 'application/json' }, 
                    body: JSON.stringify(config) 
                });
                const data = await res.json();
                if (res.ok) { showStatus("Connected", "success"); fetchRemoteFiles(); }
                else showStatus(`Failed: ${data.error || "Unknown error"}`, "error");
            } catch (e) { showStatus("Request failed", "error"); }
            testBtn.disabled = false;
            toggleButtonLoading(testBtn, false);
        });
    }

    if (updateThrottleBtn) {
        updateThrottleBtn.addEventListener('click', async () => {
            const config = getFormData();
            updateThrottleBtn.disabled = true;
            toggleButtonLoading(updateThrottleBtn, true, "Applying...");
            try {
                const res = await fetch('/api/throttle/update', { 
                    method: 'POST', 
                    headers: { 'Content-Type': 'application/json' }, 
                    body: JSON.stringify(config) 
                });
                const data = await res.json();
                if (res.ok) { 
                    showStatus(`Throttling updated for ${config.host}`, "success"); 
                    addLog(`Updated throttling for ${config.host}: ${config.rate_limit_kbps} KB/s`, 'info');
                } else {
                    showStatus(`Update failed: ${data.error || "Unknown error"}`, "error");
                }
            } catch (e) { showStatus("Request failed", "error"); }
            updateThrottleBtn.disabled = false;
            toggleButtonLoading(updateThrottleBtn, false);
        });
    }

    if (uploadBtn) {
        uploadBtn.addEventListener('click', async () => {
            // Build the file list without mutating global queuedFiles until success
            let filesToUpload;
            const wasImplicit = queuedFiles.size === 0;
        if (wasImplicit) {
            // Implicitly queue all files in current local directory
            filesToUpload = [];
            localFilesList.forEach(file => {
                if (!file.is_dir) {
                    const fullPath = currentPath ? `${currentPath}/${file.name}` : file.name;
                    filesToUpload.push(fullPath);
                }
            });
        } else {
            filesToUpload = Array.from(queuedFiles.keys());
        }
        if (filesToUpload.length === 0) return alert("No files to upload.");

        const config = getFormData();
        config.files = filesToUpload;
        
        // Final destination enforcement: always use the most recent remoteCurrentPath
        // This ensures the queue reflects what the user sees in the browser pane
        if (remoteCurrentPath) {
            config.remote_dir = remoteCurrentPath;
        }
        
        addLog(`Queueing ${filesToUpload.length} files to: ${config.remote_dir}`, 'info');
        
        uploadBtn.disabled = true;
        toggleButtonLoading(uploadBtn, true, "Queueing...");
        try {
            const res = await fetch('/api/upload', { 
                method: 'POST', 
                headers: { 'Content-Type': 'application/json' }, 
                body: JSON.stringify(config) 
            });
            if (res.ok) {
                showStatus("Tasks added to background queue", "success");
                queuedFiles.clear();
                updateUploadButtonText();
                updateSelectionBar();
                fetchQueue();
            } else {
                const data = await res.json().catch(() => ({}));
                showStatus(`Failed to queue: ${data.error || "Unknown error"}`, "error");
            }
        } catch (e) { showStatus("Upload request failed", "error"); }
        uploadBtn.disabled = false;
        toggleButtonLoading(uploadBtn, false, "", updateUploadButtonText);
    });
}

    // --- Init ---
    const init = async () => {
        // Check for secure context
        if (!SecureStorage.isAvailable) {
            const banner = document.getElementById('insecure-banner');
            if (banner) banner.style.display = 'flex';
        }

        masterKey = await SecureStorage.getKey();
        if (!masterKey) {
            console.warn('Uplarr init: masterKey is null.',
                'SecureStorage.isAvailable:', SecureStorage.isAvailable,
                'sessionStorage has key:', !!sessionStorage.getItem('uplarr_master_key'));
            try {
                const authRes = await fetch('/api/auth/status');
                const authData = await authRes.json();
                if (authData.auth_required) {
                    // Backend requires auth but we have no masterKey (e.g. opened in a new tab
                    // or sessionStorage was cleared). We need the user to re-enter their password
                    // to derive the encryption key.
                    // Invalidate the session so '/' serves the login page, then redirect.
                    await fetch('/api/logout', { method: 'POST' });
                    handleAuthFailure('init-masterKey-missing', 401);
                    return;
                }
            } catch (e) {
                console.error("Failed to check auth status", e);
            }
        }

        // Restore state
        currentPath = await getSecureItem('uplarr_local_path') || '';
        remoteCurrentPath = await getSecureItem('uplarr_remote_path') || '';
        localSort = await loadSortState('local');
        remoteSort = await loadSortState('remote');
        await loadCompactState();
        
        await restoreFormData();
        applyCompact();
        
        fetchFiles(currentPath);
        if (remoteCurrentPath) {
            fetchRemoteFiles(remoteCurrentPath);
        }
        setInterval(fetchQueue, 1000);
        setInterval(fetchStats, 1000);
        setInterval(updateSessionStats, 1000);
        await initTheme();
    };


    logoutBtn.addEventListener('click', async () => {
        await fetch('/api/logout', { method: 'POST' });
        sessionStorage.removeItem('uplarr_master_key');
        window.location.href = '/';
    });

    // SSE with exponential backoff
    let sseRetryDelay = 2000;
    const MAX_SSE_RETRY = 30000;
    const connectSSE = () => {
        if (authRedirectPending) return; // Don't reconnect if auth failed
        console.log('SSE: Attempting to connect to /api/logs...');
        const es = new EventSource('/api/logs');
        
        es.onopen = (e) => { 
            console.log('SSE: Connection established');
            sseRetryDelay = 2000; 
        };
        
        es.onmessage = (e) => { 
            try { 
                const d = JSON.parse(e.data); 
                addLog(d.msg, d.level); 
            } catch(err) {
                if (e.data) addLog(e.data, 'info');
            } 
        };
        
        es.onerror = (err) => {
            console.error('SSE: Connection error', err);
            es.close();
            if (authRedirectPending) return;
            console.log(`SSE: Retrying in ${sseRetryDelay}ms...`);
            setTimeout(connectSSE, sseRetryDelay);
            sseRetryDelay = Math.min(sseRetryDelay * 1.5, MAX_SSE_RETRY);
        };
    };

    // Start init, connect SSE only after successful auth check
    init().then(() => {
        if (!authRedirectPending) connectSSE();
    });
});
