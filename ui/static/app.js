document.addEventListener('DOMContentLoaded', () => {
    // Local Pane Elements
    const fileListBody = document.getElementById('file-list-body');
    const refreshBtn = document.getElementById('refresh-btn');
    const upBtn = document.getElementById('up-btn');
    const mkdirBtn = document.getElementById('mkdir-btn');
    const deleteBtn = document.getElementById('delete-btn');
    const localBreadcrumb = document.getElementById('local-breadcrumb');
    const selectAllCheckbox = document.getElementById('select-all-checkbox');

    // Remote Pane Elements
    const remoteFileListBody = document.getElementById('remote-file-list-body');
    const remoteRefreshBtn = document.getElementById('remote-refresh-btn');
    const remoteUpBtn = document.getElementById('remote-up-btn');
    const remoteMkdirBtn = document.getElementById('remote-mkdir-btn');
    const remoteDeleteBtn = document.getElementById('remote-delete-btn');
    const remoteBreadcrumb = document.getElementById('remote-breadcrumb');
    const remoteDropZone = document.getElementById('remote-drop-zone');

    // Queue Elements
    const queueBody = document.getElementById('queue-body');
    const metricsOverlay = document.getElementById('metrics-overlay');

    // Shared Elements
    const testBtn = document.getElementById('test-btn');
    const updateThrottleBtn = document.getElementById('update-throttle-btn');
    const uploadBtn = document.getElementById('upload-btn');
    const logoutBtn = document.getElementById('logout-btn');
    const statusMsg = document.getElementById('status-message');
    const logContainer = document.getElementById('log-container');
    const sftpForm = document.getElementById('sftp-form');
    
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

    // Rate tracking (module-scoped instead of window globals)
    let globalTotalRate = 0;
    let lastTotalRate = 0;

    // --- Secure Storage Wrappers ---
    let masterKey = null;

    const getSecureItem = async (key) => {
        if (!masterKey) masterKey = await SecureStorage.getKey();
        const raw = localStorage.getItem(key);
        if (!raw || !masterKey) return raw;
        try {
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

    // --- Compact View State (persisted) ---
    const loadCompactState = async () => {
        const saved = await getSecureItem('uplarr_compact');
        if (saved !== null) return JSON.parse(saved);
        return { local: false, remote: false };
    };

    const saveCompactState = async (state) => {
        await setSecureItem('uplarr_compact', JSON.stringify(state));
    };

    let compactState = { local: false, remote: false };

    const applyCompact = () => {
        localPane.classList.toggle('compact', compactState.local);
        remotePane.classList.toggle('compact', compactState.remote);
        compactToggle.classList.toggle('active', compactState.local);
        remoteCompactToggle.classList.toggle('active', compactState.remote);
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

    applyCompact();

    // --- Sort Logic ---

    const sortFiles = (files, sortKey, sortDir) => {
        const sorted = [...files];
        const dirMul = sortDir === 'asc' ? 1 : -1;

        sorted.sort((a, b) => {
            // Directories always first
            if (a.is_dir && !b.is_dir) return -1;
            if (!a.is_dir && b.is_dir) return 1;

            switch (sortKey) {
                case 'name':
                    return dirMul * a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' });
                case 'size':
                    return dirMul * ((a.size || 0) - (b.size || 0));
                case 'type': {
                    const extA = a.name.includes('.') ? a.name.split('.').pop().toLowerCase() : '';
                    const extB = b.name.includes('.') ? b.name.split('.').pop().toLowerCase() : '';
                    return dirMul * extA.localeCompare(extB);
                }
                default:
                    return 0;
            }
        });
        return sorted;
    };

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

    // --- Helpers ---

    const formatSize = (bytes) => {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    };

    const addLog = (message, type = 'info') => {
        const entry = document.createElement('div');
        entry.className = `log-entry log-${type}`;
        const timeSpan = document.createElement('span');
        timeSpan.className = 'log-time';
        timeSpan.textContent = `[${new Date().toLocaleTimeString()}] `;
        const msgSpan = document.createElement('span');
        msgSpan.textContent = message;
        entry.appendChild(timeSpan);
        entry.appendChild(msgSpan);
        logContainer.appendChild(entry);
        logContainer.scrollTop = logContainer.scrollHeight;
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
        } catch (e) { console.error('Failed to restore form data', e); }
    };

    sftpForm.addEventListener('input', saveFormData);

    const showStatus = (msg, type) => {
        statusMsg.textContent = msg;
        statusMsg.className = `status-msg ${type}`;
        statusMsg.classList.remove('hidden');
    };

    const updateUploadButtonText = () => {
        uploadBtn.textContent = queuedFiles.size > 0 ? `Queue ${queuedFiles.size} Files` : "Queue All Files";
    };

    // --- Local Files ---

    const renderLocalFiles = () => {
        const sorted = sortFiles(localFilesList, localSort.key, localSort.dir);
        updateSortHeaders('file-table', localSort);
        fileListBody.innerHTML = '';

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
            });
            tdCheck.appendChild(cb);

            // Name Cell
            const tdName = document.createElement('td');
            tdName.className = 'col-name';
            const icon = document.createElement('span');
            icon.className = file.is_dir ? 'icon-folder' : 'icon-file';
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
            fileListBody.appendChild(row);
        });

        lastCheckedIndex = -1;
        selectAllCheckbox.checked = false;
    };

    const fetchFiles = async (path = '') => {
        try {
            const response = await fetch(`/api/files?path=${encodeURIComponent(path)}`);
            if (response.status === 401) return window.location.href = '/';
            const data = await response.json();
            currentPath = data.current_path;
            await setSecureItem('uplarr_local_path', currentPath);
            localBreadcrumb.textContent = '/' + currentPath;
            localFilesList = data.files || [];
            renderLocalFiles();
        } catch (err) { addLog(`Local fetch error: ${err.message}`, 'error'); }
    };

    refreshBtn.addEventListener('click', () => fetchFiles(currentPath));
    upBtn.addEventListener('click', () => {
        if (!currentPath || currentPath === '.') return;
        const parts = currentPath.split(/[\\/]/);
        parts.pop();
        fetchFiles(parts.join('/'));
    });

    mkdirBtn.addEventListener('click', async () => {
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

    deleteBtn.addEventListener('click', async () => {
        if (queuedFiles.size === 0) return alert("Select files to delete first.");
        if (!confirm(`Delete ${queuedFiles.size} items?`)) return;
        const failedPaths = [];
        for (const path of Array.from(queuedFiles.keys())) {
            try {
                const res = await fetch('/api/files/action', { 
                    method: 'POST', 
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ action: 'delete', path }) 
                });
                if (res.ok) {
                    queuedFiles.delete(path);
                } else {
                    failedPaths.push(path);
                    addLog(`Failed to delete ${path}: ${res.statusText}`, 'error');
                }
            } catch (err) {
                failedPaths.push(path);
                addLog(`Failed to delete ${path}: ${err.message}`, 'error');
            }
        }
        if (failedPaths.length > 0) {
            addLog(`${failedPaths.length} file(s) could not be deleted`, 'warn');
        }
        updateUploadButtonText();
        fetchFiles(currentPath);
    });

    selectAllCheckbox.addEventListener('change', (e) => {
        const checked = e.target.checked;
        document.querySelectorAll('#file-list-body .file-checkbox:not(:disabled)').forEach(cb => {
            cb.checked = checked;
            const path = cb.dataset.path;
            const name = cb.dataset.name;
            if (checked) queuedFiles.set(path, { name });
            else queuedFiles.delete(path);
        });
        updateUploadButtonText();
    });

    // --- Remote Files ---

    const renderRemoteFiles = () => {
        const sorted = sortFiles(remoteFilesList, remoteSort.key, remoteSort.dir);
        updateSortHeaders('remote-file-table', remoteSort);
        remoteFileListBody.innerHTML = '';

        if (sorted.length === 0) {
            const row = document.createElement('tr');
            const td = document.createElement('td');
            td.colSpan = 3;
            td.className = 'empty-msg';
            td.textContent = 'Empty directory';
            row.appendChild(td);
            remoteFileListBody.appendChild(row);
            return;
        }

        sorted.forEach(file => {
            const row = document.createElement('tr');
            if (file.is_dir) {
                row.className = 'clickable-row folder-row';
                row.addEventListener('click', () => {
                    const newPath = remoteCurrentPath.endsWith('/') ? remoteCurrentPath + file.name : remoteCurrentPath + '/' + file.name;
                    fetchRemoteFiles(newPath);
                });
            }

            const tdName = document.createElement('td');
            tdName.className = 'col-name';
            const icon = document.createElement('span');
            icon.className = file.is_dir ? 'icon-folder' : 'icon-file';
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
            remoteFileListBody.appendChild(row);
        });
    };

    const fetchRemoteFiles = async (path = null) => {
        if (!sftpForm.checkValidity()) return sftpForm.reportValidity();
        const config = getFormData();
        const targetPath = path !== null ? path : (remoteCurrentPath || config.remote_dir);
        try {
            const response = await fetch(`/api/remote/files?path=${encodeURIComponent(targetPath)}`, {
                method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config)
            });
            if (response.status === 401) return window.location.href = '/';
            const data = await response.json();
            if (!response.ok) throw new Error(data.error || 'Failed to fetch remote files');
            
            remoteCurrentPath = data.current_path;
            
            // Sync with configuration form input for consistency
            const remoteDirInput = document.getElementById('remote_dir');
            if (remoteDirInput) {
                remoteDirInput.value = remoteCurrentPath;
            }
            
            await setSecureItem('uplarr_remote_path', remoteCurrentPath);
            remoteBreadcrumb.textContent = remoteCurrentPath;
            remoteFilesList = data.files || [];
            renderRemoteFiles();
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

    remoteRefreshBtn.addEventListener('click', () => fetchRemoteFiles(remoteCurrentPath));
    remoteUpBtn.addEventListener('click', () => {
        if (!remoteCurrentPath || remoteCurrentPath === '/') return;
        const parts = remoteCurrentPath.split('/');
        parts.pop();
        let parent = parts.join('/');
        if (parent === '') parent = '/';
        fetchRemoteFiles(parent);
    });

    remoteMkdirBtn.addEventListener('click', async () => {
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

    remoteDeleteBtn.addEventListener('click', async () => {
        alert("Remote multi-delete not implemented yet. Use drag-and-drop or single actions if available.");
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
            const tasks = await res.json();
            
            // Calculate aggregate speed for active hosts
            const activeSpeeds = new Map(); // host -> totalRate
            
            queueBody.innerHTML = '';
            tasks.reverse().forEach(task => {
                const row = document.createElement('tr');
                
                const tdFile = document.createElement('td');
                tdFile.textContent = task.file_name;
                tdFile.title = task.file_name;
                tdFile.style.maxWidth = '200px';
                tdFile.style.overflow = 'hidden';
                tdFile.style.textOverflow = 'ellipsis';
                tdFile.style.whiteSpace = 'nowrap';
                
                const tdDest = document.createElement('td');
                tdDest.textContent = task.remote_dir || '';
                tdDest.title = task.remote_dir || '';
                tdDest.style.maxWidth = '200px';
                tdDest.style.overflow = 'hidden';
                tdDest.style.textOverflow = 'ellipsis';
                tdDest.style.whiteSpace = 'nowrap';
                
                const tdStatus = document.createElement('td');
                tdStatus.className = `status-${task.status}`;
                tdStatus.textContent = task.status + (task.error ? ` (${task.error})` : '');

                // Progress column
                const tdProgress = document.createElement('td');
                tdProgress.style.minWidth = '120px';
                if (task.status === 'Running' && task.total_bytes > 0) {
                    const pct = Math.min(100, task.progress || 0);
                    const barContainer = document.createElement('div');
                    barContainer.className = 'progress-bar-container';
                    const barFill = document.createElement('div');
                    barFill.className = 'progress-bar-fill';
                    barFill.style.width = pct + '%';
                    barContainer.appendChild(barFill);
                    const label = document.createElement('span');
                    label.className = 'progress-label';
                    label.textContent = `${pct}% (${formatSize(task.bytes_uploaded)} / ${formatSize(task.total_bytes)})`;
                    tdProgress.appendChild(barContainer);
                    tdProgress.appendChild(label);
                } else if (task.status === 'Completed') {
                    tdProgress.textContent = task.total_bytes > 0 ? formatSize(task.total_bytes) : '100%';
                } else if (task.status === 'Failed') {
                    tdProgress.textContent = task.bytes_uploaded > 0 ? formatSize(task.bytes_uploaded) : '-';
                } else {
                    tdProgress.textContent = '-';
                }

                // Rate column
                const tdRate = document.createElement('td');
                tdRate.style.whiteSpace = 'nowrap';
                if (task.status === 'Running' && task.started_at && task.bytes_uploaded > 0) {
                    const elapsed = (Date.now() - new Date(task.started_at).getTime()) / 1000;
                    if (elapsed > 0) {
                        const rate = task.bytes_uploaded / elapsed;
                        const remaining = task.total_bytes - task.bytes_uploaded;
                        const eta = remaining > 0 && rate > 0 ? remaining / rate : 0;
                        tdRate.textContent = formatRate(rate);
                        if (eta > 0) {
                            tdRate.textContent += ` (${formatETA(eta)})`;
                        }
                    } else {
                        tdRate.textContent = '-';
                    }
                } else {
                    tdRate.textContent = '-';
                }
                
                const tdCreated = document.createElement('td');
                tdCreated.textContent = new Date(task.created_at).toLocaleTimeString();
                
                const tdActions = document.createElement('td');
                if (task.status === 'Pending') {
                    const btn = document.createElement('button');
                    btn.textContent = 'Pause';
                    btn.addEventListener('click', () => controlTask(task.id, 'pause'));
                    tdActions.appendChild(btn);
                } else if (task.status === 'Paused') {
                    const btn = document.createElement('button');
                    btn.textContent = 'Resume';
                    btn.addEventListener('click', () => controlTask(task.id, 'resume'));
                    tdActions.appendChild(btn);
                }
                const remBtn = document.createElement('button');
                remBtn.textContent = 'Remove';
                remBtn.addEventListener('click', () => controlTask(task.id, 'remove'));
                tdActions.appendChild(remBtn);

                row.appendChild(tdFile);
                row.appendChild(tdDest);
                row.appendChild(tdStatus);
                row.appendChild(tdProgress);
                row.appendChild(tdRate);
                row.appendChild(tdCreated);
                row.appendChild(tdActions);
                queueBody.appendChild(row);

                // Aggregate calculation (we use the rate we displayed)
                if (task.status === 'Running' && task.started_at && task.bytes_uploaded > 0) {
                    const elapsed = (Date.now() - new Date(task.started_at).getTime()) / 1000;
                    if (elapsed > 0) {
                        const rate = task.bytes_uploaded / elapsed;
                        globalTotalRate += rate;
                    }
                }
            });
            lastTotalRate = globalTotalRate;
            globalTotalRate = 0;
        } catch (e) {
            console.error('Failed to fetch queue:', e);
            queueBody.innerHTML = '';
            const errRow = document.createElement('tr');
            const errTd = document.createElement('td');
            errTd.colSpan = 7;
            errTd.className = 'log-error';
            errTd.textContent = 'Failed to load queue';
            errRow.appendChild(errTd);
            queueBody.appendChild(errRow);
        }
    };

    const fetchStats = async () => {
        try {
            const res = await fetch('/api/stats');
            if (res.status === 401) {
                window.location.href = '/';
                return;
            }
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
        if (!stats || stats.length === 0) {
            metricsOverlay.innerHTML = '';
            return;
        }

        // Build DOM safely to prevent XSS from user-controlled host values
        metricsOverlay.innerHTML = '';
        stats.forEach(s => {
            const card = document.createElement('div');
            card.className = 'metric-card';

            const hostEl = document.createElement('div');
            hostEl.className = 'metric-host';
            hostEl.textContent = s.host;
            card.appendChild(hostEl);

            // Latency row
            const latRow = document.createElement('div');
            latRow.className = 'metric-row';
            const latLabel = document.createElement('span');
            latLabel.className = 'metric-label';
            latLabel.textContent = 'Latency';
            const latValue = document.createElement('span');
            latValue.className = 'metric-value latency' + (s.last_latency_ms > 100 ? ' high-latency' : '');
            latValue.textContent = s.last_latency_ms + 'ms';
            latRow.appendChild(latLabel);
            latRow.appendChild(latValue);
            card.appendChild(latRow);

            // Rate Limit row
            const rlRow = document.createElement('div');
            rlRow.className = 'metric-row';
            const rlLabel = document.createElement('span');
            rlLabel.className = 'metric-label';
            rlLabel.textContent = 'Rate Limit';
            const rlValue = document.createElement('span');
            rlValue.className = 'metric-value';
            rlValue.textContent = (s.current_limit_kb / 1024).toFixed(1) + ' MB/s';
            rlRow.appendChild(rlLabel);
            rlRow.appendChild(rlValue);
            card.appendChild(rlRow);

            // Current Speed row — uses per-host speed from backend
            const spRow = document.createElement('div');
            spRow.className = 'metric-row';
            const spLabel = document.createElement('span');
            spLabel.className = 'metric-label';
            spLabel.textContent = 'Current Speed';
            const spValue = document.createElement('span');
            spValue.className = 'metric-value speed';
            // total_speed_kbps is KB/s from backend; formatRate expects bytes/s
            const hostSpeed = (s.total_speed_kbps || 0) * 1024;
            spValue.textContent = formatRate(hostSpeed);
            spRow.appendChild(spLabel);
            spRow.appendChild(spValue);
            card.appendChild(spRow);

            metricsOverlay.appendChild(card);
        });
    };

    const controlTask = async (id, action) => {
        const res = await fetch('/api/queue', { 
            method: 'POST', 
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id, action }) 
        });
        if (!res.ok) {
            const data = await res.json();
            alert(`Action failed: ${data.error || res.statusText}`);
        }
        fetchQueue();
    };

    // --- Actions ---

    testBtn.addEventListener('click', async () => {
        const config = getFormData();
        testBtn.disabled = true;
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
    });

    updateThrottleBtn.addEventListener('click', async () => {
        const config = getFormData();
        updateThrottleBtn.disabled = true;
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
    });

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
                fetchQueue();
            } else {
                const data = await res.json().catch(() => ({}));
                showStatus(`Failed to queue: ${data.error || "Unknown error"}`, "error");
            }
        } catch (e) { showStatus("Upload request failed", "error"); }
        uploadBtn.disabled = false;
    });

    // --- Init ---
    const init = async () => {
        // Check for secure context
        if (!SecureStorage.isAvailable) {
            const banner = document.getElementById('insecure-banner');
            if (banner) banner.style.display = 'flex';
        }

        masterKey = await SecureStorage.getKey();
        if (!masterKey) {
            // Check if backend actually requires auth
            const res = await fetch('/api/queue');
            if (res.status === 401) {
                window.location.href = '/';
                return;
            }
        }

        // Restore state
        currentPath = await getSecureItem('uplarr_local_path') || '';
        remoteCurrentPath = await getSecureItem('uplarr_remote_path') || '';
        localSort = await loadSortState('local');
        remoteSort = await loadSortState('remote');
        compactState = await loadCompactState();
        
        await restoreFormData();
        applyCompact();
        
        fetchFiles(currentPath);
        if (remoteCurrentPath) {
            fetchRemoteFiles(remoteCurrentPath);
        }
        setInterval(fetchQueue, 1000);
        setInterval(fetchStats, 1000);
    };

    logoutBtn.addEventListener('click', async () => {
        await fetch('/api/logout', { method: 'POST' });
        sessionStorage.removeItem('uplarr_master_key');
        window.location.href = '/';
    });

    init();
    
    // SSE
    const connectSSE = () => {
        const es = new EventSource('/api/logs');
        es.onmessage = (e) => { 
            try { 
                const d = JSON.parse(e.data); 
                addLog(d.msg, d.level); 
            } catch(err) {
                addLog(e.data, 'info');
            } 
        };
        es.onerror = () => { es.close(); setTimeout(connectSSE, 5000); };
    };
    connectSSE();
});
