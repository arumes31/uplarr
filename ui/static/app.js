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

    // Shared Elements
    const testBtn = document.getElementById('test-btn');
    const uploadBtn = document.getElementById('upload-btn');
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

    let currentPath = '';
    let remoteCurrentPath = '';
    let queuedFiles = new Map(); // path -> fileInfo
    let dropData = null;
    let localFilesList = [];
    let remoteFilesList = [];

    // --- Sort State ---
    let localSort = { key: 'name', dir: 'asc' };
    let remoteSort = { key: 'name', dir: 'asc' };

    // --- Compact View State (persisted) ---
    const loadCompactState = () => {
        const saved = localStorage.getItem('uplarr_compact');
        if (saved !== null) return JSON.parse(saved);
        return { local: false, remote: false };
    };

    const saveCompactState = (state) => {
        localStorage.setItem('uplarr_compact', JSON.stringify(state));
    };

    const compactState = loadCompactState();

    const applyCompact = () => {
        localPane.classList.toggle('compact', compactState.local);
        remotePane.classList.toggle('compact', compactState.remote);
        compactToggle.classList.toggle('active', compactState.local);
        remoteCompactToggle.classList.toggle('active', compactState.remote);
    };

    compactToggle.addEventListener('click', () => {
        compactState.local = !compactState.local;
        applyCompact();
        saveCompactState(compactState);
    });

    remoteCompactToggle.addEventListener('click', () => {
        compactState.remote = !compactState.remote;
        applyCompact();
        saveCompactState(compactState);
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
        th.addEventListener('click', () => {
            const key = th.dataset.sort;
            if (localSort.key === key) {
                localSort.dir = localSort.dir === 'asc' ? 'desc' : 'asc';
            } else {
                localSort.key = key;
                localSort.dir = 'asc';
            }
            renderLocalFiles();
        });
    });

    // Bind sort clicks for remote file table
    document.querySelectorAll('#remote-file-table th.sortable').forEach(th => {
        th.addEventListener('click', () => {
            const key = th.dataset.sort;
            if (remoteSort.key === key) {
                remoteSort.dir = remoteSort.dir === 'asc' ? 'desc' : 'asc';
            } else {
                remoteSort.key = key;
                remoteSort.dir = 'asc';
            }
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
            files: Array.from(queuedFiles.keys())
        };
    };

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

            cb.addEventListener('change', (e) => {
                if (e.target.checked) queuedFiles.set(fullRelPath, { name: file.name });
                else queuedFiles.delete(fullRelPath);
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

        selectAllCheckbox.checked = false;
    };

    const fetchFiles = async (path = '') => {
        try {
            const response = await fetch(`/api/files?path=${encodeURIComponent(path)}`);
            const data = await response.json();
            currentPath = data.current_path;
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
            const data = await response.json();
            if (!response.ok) throw new Error(data.error || 'Failed to fetch remote files');
            
            remoteCurrentPath = data.current_path;
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

    // --- Queue ---

    const fetchQueue = async () => {
        try {
            const res = await fetch('/api/queue');
            const tasks = await res.json();
            queueBody.innerHTML = '';
            tasks.reverse().forEach(task => {
                const row = document.createElement('tr');
                
                const tdFile = document.createElement('td');
                tdFile.textContent = task.file_name;
                
                const tdStatus = document.createElement('td');
                tdStatus.className = `status-${task.status}`;
                tdStatus.textContent = task.status + (task.error ? ` (${task.error})` : '');
                
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
                remBtn.disabled = (task.status === 'Running');
                remBtn.addEventListener('click', () => controlTask(task.id, 'remove'));
                tdActions.appendChild(remBtn);

                row.appendChild(tdFile);
                row.appendChild(tdStatus);
                row.appendChild(tdCreated);
                row.appendChild(tdActions);
                queueBody.appendChild(row);
            });
        } catch (e) {
            console.error('Failed to fetch queue:', e);
            queueBody.innerHTML = '';
            const errRow = document.createElement('tr');
            const errTd = document.createElement('td');
            errTd.colSpan = 4;
            errTd.className = 'log-error';
            errTd.textContent = 'Failed to load queue';
            errRow.appendChild(errTd);
            queueBody.appendChild(errRow);
        }
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
    fetchFiles();
    setInterval(fetchQueue, 3000);
    
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
