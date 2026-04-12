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

    let currentPath = '';
    let remoteCurrentPath = '';
    let queuedFiles = new Map(); // path -> fileInfo
    let dropData = null;

    const formatSize = (bytes) => {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
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

    // --- Local Files ---

    const fetchFiles = async (path = '') => {
        try {
            const response = await fetch(`/api/files?path=${encodeURIComponent(path)}`);
            const data = await response.json();
            currentPath = data.current_path;
            localBreadcrumb.textContent = '/' + currentPath;
            fileListBody.innerHTML = '';
            
            data.files.forEach(file => {
                const fullRelPath = currentPath ? `${currentPath}/${file.name}` : file.name;
                const row = document.createElement('tr');
                row.draggable = !file.is_dir;
                if (file.is_dir) row.className = 'clickable-row folder-row';
                const isChecked = queuedFiles.has(fullRelPath) ? 'checked' : '';
                const iconClass = file.is_dir ? 'icon-folder' : 'icon-file';

                row.innerHTML = `
                    <td class="col-check"><input type="checkbox" class="file-checkbox" data-path="${fullRelPath}" data-name="${file.name}" data-isdir="${file.is_dir}" ${isChecked} ${file.is_dir ? 'disabled' : ''}></td>
                    <td class="col-name"><span class="${iconClass}"></span>${file.name}</td>
                    <td class="col-size">${file.is_dir ? '-' : formatSize(file.size)}</td>
                    <td class="col-type">${file.is_dir ? 'Directory' : 'File'}</td>
                `;

                if (file.is_dir) {
                    row.addEventListener('click', (e) => {
                        if (e.target.type !== 'checkbox') fetchFiles(fullRelPath);
                    });
                } else {
                    row.addEventListener('dragstart', (e) => {
                        e.dataTransfer.setData('text/plain', JSON.stringify({ path: fullRelPath, name: file.name, size: file.size }));
                        row.classList.add('dragging');
                    });
                    row.addEventListener('dragend', () => row.classList.remove('dragging'));
                }
                fileListBody.appendChild(row);
            });

            document.querySelectorAll('.file-checkbox').forEach(cb => {
                cb.addEventListener('change', (e) => {
                    const path = e.target.getAttribute('data-path');
                    if (e.target.checked) queuedFiles.set(path, { name: e.target.getAttribute('data-name') });
                    else queuedFiles.delete(path);
                    updateUploadButtonText();
                });
            });
            selectAllCheckbox.checked = false;
        } catch (err) { addLog(`Local fetch error: ${err.message}`, 'error'); }
    };

    mkdirBtn.addEventListener('click', async () => {
        const name = prompt("Enter folder name:");
        if (!name) return;
        const res = await fetch('/api/files/action', {
            method: 'POST',
            body: JSON.stringify({ action: 'mkdir', path: currentPath ? `${currentPath}/${name}` : name })
        });
        if (res.ok) fetchFiles(currentPath);
    });

    deleteBtn.addEventListener('click', async () => {
        if (queuedFiles.size === 0) return alert("Select files to delete first.");
        if (!confirm(`Delete ${queuedFiles.size} items?`)) return;
        for (const path of queuedFiles.keys()) {
            await fetch('/api/files/action', { method: 'POST', body: JSON.stringify({ action: 'delete', path }) });
        }
        queuedFiles.clear();
        fetchFiles(currentPath);
    });

    // --- Remote Files ---

    const fetchRemoteFiles = async (path = null) => {
        if (!sftpForm.checkValidity()) return sftpForm.reportValidity();
        const config = getFormData();
        const targetPath = path !== null ? path : (remoteCurrentPath || config.remote_dir);
        try {
            const response = await fetch(`/api/remote/files?path=${encodeURIComponent(targetPath)}`, {
                method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config)
            });
            if (!response.ok) throw new Error((await response.json()).error || 'Failed to fetch remote files');
            const data = await response.json();
            remoteCurrentPath = data.current_path;
            remoteBreadcrumb.textContent = remoteCurrentPath;
            remoteFileListBody.innerHTML = '';
            
            data.files.forEach(file => {
                const row = document.createElement('tr');
                if (file.is_dir) row.className = 'clickable-row folder-row';
                const iconClass = file.is_dir ? 'icon-folder' : 'icon-file';
                row.innerHTML = `
                    <td class="col-name"><span class="${iconClass}"></span>${file.name}</td>
                    <td class="col-size">${file.is_dir ? '-' : formatSize(file.size)}</td>
                    <td class="col-type">${file.is_dir ? 'Directory' : 'File'}</td>
                `;
                if (file.is_dir) {
                    row.addEventListener('click', () => {
                        const newPath = remoteCurrentPath.endsWith('/') ? remoteCurrentPath + file.name : remoteCurrentPath + '/' + file.name;
                        fetchRemoteFiles(newPath);
                    });
                }
                remoteFileListBody.appendChild(row);
            });
        } catch (err) {
            addLog(`Remote fetch error: ${err.message}`, 'error');
            remoteFileListBody.innerHTML = `<tr><td colspan="3" class="log-error">Error: ${err.message}</td></tr>`;
        }
    };

    remoteMkdirBtn.addEventListener('click', async () => {
        const name = prompt("Enter remote folder name:");
        if (!name) return;
        const config = getFormData();
        const path = remoteCurrentPath.endsWith('/') ? remoteCurrentPath + name : remoteCurrentPath + '/' + name;
        const res = await fetch('/api/remote/files/action', {
            method: 'POST', body: JSON.stringify({ action: 'mkdir', path, config })
        });
        if (res.ok) fetchRemoteFiles(remoteCurrentPath);
    });

    // --- Drag & Drop ---

    remoteDropZone.addEventListener('dragover', (e) => { e.preventDefault(); remoteDropZone.classList.add('drop-target'); });
    remoteDropZone.addEventListener('dragleave', () => remoteDropZone.classList.remove('drop-target'));
    remoteDropZone.addEventListener('drop', (e) => {
        e.preventDefault();
        remoteDropZone.classList.remove('drop-target');
        try {
            dropData = JSON.parse(e.dataTransfer.getData('text/plain'));
            modalFileInfo.textContent = `Upload ${dropData.name} to ${remoteCurrentPath}?`;
            dropModal.classList.remove('hidden');
        } catch (err) {}
    });

    modalConfirmBtn.addEventListener('click', async () => {
        const config = getFormData();
        config.files = [dropData.path];
        config.remote_dir = remoteCurrentPath;
        config.delete_after_verify = modalDeleteLocal.checked;
        config.overwrite = modalOverwriteRemote.checked;
        dropModal.classList.add('hidden');
        const res = await fetch('/api/upload', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config) });
        if (res.ok) { showStatus("Task queued", "success"); fetchQueue(); }
    });

    modalCancelBtn.addEventListener('click', () => dropModal.classList.add('hidden'));

    // --- Queue ---

    const fetchQueue = async () => {
        const res = await fetch('/api/queue');
        const tasks = await res.json();
        queueBody.innerHTML = '';
        tasks.reverse().forEach(task => {
            const row = document.createElement('tr');
            row.innerHTML = `
                <td>${task.file_name}</td>
                <td class="status-${task.status}">${task.status} ${task.error ? `(${task.error})` : ''}</td>
                <td>${new Date(task.created_at).toLocaleTimeString()}</td>
                <td>
                    ${task.status === 'Pending' ? `<button onclick="controlTask('${task.id}', 'pause')">Pause</button>` : ''}
                    ${task.status === 'Paused' ? `<button onclick="controlTask('${task.id}', 'resume')">Resume</button>` : ''}
                </td>
            `;
            queueBody.appendChild(row);
        });
    };

    window.controlTask = async (id, action) => {
        await fetch('/api/queue', { method: 'POST', body: JSON.stringify({ id, action }) });
        fetchQueue();
    };

    // --- Init ---

    testBtn.addEventListener('click', async () => {
        const config = getFormData();
        const res = await fetch('/api/test-connection', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config) });
        if (res.ok) { showStatus("Connected", "success"); fetchRemoteFiles(); }
        else showStatus("Failed", "error");
    });

    uploadBtn.addEventListener('click', async () => {
        const config = getFormData();
        if (queuedFiles.size === 0) {
            // Queue all in current dir logic can be added here
        }
        await fetch('/api/upload', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config) });
        queuedFiles.clear();
        updateUploadButtonText();
        fetchQueue();
    });

    refreshBtn.addEventListener('click', () => fetchFiles(currentPath));
    remoteRefreshBtn.addEventListener('click', () => fetchRemoteFiles(remoteCurrentPath));
    
    const updateUploadButtonText = () => {
        uploadBtn.textContent = queuedFiles.size > 0 ? `Queue ${queuedFiles.size} Files` : "Queue All Files";
    };

    const showStatus = (msg, type) => {
        statusMsg.textContent = msg;
        statusMsg.className = `status-msg ${type}`;
        statusMsg.classList.remove('hidden');
    };

    fetchFiles();
    setInterval(fetchQueue, 2000);
    
    // SSE
    const connectSSE = () => {
        const es = new EventSource('/api/logs');
        es.onmessage = (e) => { try { const d = JSON.parse(e.data); addLog(d.msg, d.level); } catch(err) {} };
        es.onerror = () => { es.close(); setTimeout(connectSSE, 3000); };
    };
    connectSSE();
});
