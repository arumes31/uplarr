document.addEventListener('DOMContentLoaded', () => {
    // Elements
    const fileListBody = document.getElementById('file-list-body');
    const remoteFileListBody = document.getElementById('remote-file-list-body');
    const refreshBtn = document.getElementById('refresh-btn');
    const upBtn = document.getElementById('up-btn');
    const remoteRefreshBtn = document.getElementById('remote-refresh-btn');
    const remoteUpBtn = document.getElementById('remote-up-btn');
    const testBtn = document.getElementById('test-btn');
    const uploadBtn = document.getElementById('upload-btn');
    const statusMsg = document.getElementById('status-message');
    const logContainer = document.getElementById('log-container');
    const sftpForm = document.getElementById('sftp-form');
    const localBreadcrumb = document.getElementById('local-breadcrumb');
    const remoteBreadcrumb = document.getElementById('remote-breadcrumb');
    const selectAllCheckbox = document.getElementById('select-all-checkbox');
    const remoteDropZone = document.getElementById('remote-drop-zone');

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
    let dropData = null; // Stores data about the current drag-and-drop operation

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

    const fetchFiles = async (path = '') => {
        try {
            const response = await fetch(`/api/files?path=${encodeURIComponent(path)}`);
            const data = await response.json();
            
            currentPath = data.current_path;
            localBreadcrumb.textContent = '/' + currentPath;
            
            fileListBody.innerHTML = '';
            if (data.files.length === 0) {
                fileListBody.innerHTML = '<tr><td colspan="4" style="text-align:center;">Empty directory</td></tr>';
                return;
            }

            data.files.forEach(file => {
                const fullRelPath = currentPath ? `${currentPath}/${file.name}` : file.name;
                const row = document.createElement('tr');
                row.draggable = !file.is_dir;
                if (file.is_dir) {
                    row.className = 'clickable-row folder-row';
                }
                
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
                        if (e.target.type !== 'checkbox') {
                            fetchFiles(fullRelPath);
                        }
                    });
                } else {
                    row.addEventListener('dragstart', (e) => {
                        e.dataTransfer.setData('text/plain', JSON.stringify({
                            path: fullRelPath,
                            name: file.name,
                            size: file.size
                        }));
                        row.classList.add('dragging');
                    });
                    row.addEventListener('dragend', () => row.classList.remove('dragging'));
                }

                fileListBody.appendChild(row);
            });

            document.querySelectorAll('.file-checkbox').forEach(cb => {
                cb.addEventListener('change', (e) => {
                    const path = e.target.getAttribute('data-path');
                    const name = e.target.getAttribute('data-name');
                    if (e.target.checked) {
                        queuedFiles.set(path, { name });
                    } else {
                        queuedFiles.delete(path);
                    }
                    updateUploadButtonText();
                });
            });
            
            selectAllCheckbox.checked = false;
        } catch (err) {
            addLog(`Error fetching local files: ${err.message}`, 'error');
        }
    };

    const fetchRemoteFiles = async (path = null) => {
        if (!sftpForm.checkValidity()) {
            sftpForm.reportValidity();
            return;
        }
        const config = getFormData();
        const targetPath = path !== null ? path : (remoteCurrentPath || config.remote_dir);
        
        try {
            const response = await fetch(`/api/remote/files?path=${encodeURIComponent(targetPath)}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(config)
            });
            
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || 'Failed to fetch remote files');
            }

            const data = await response.json();
            remoteCurrentPath = data.current_path;
            remoteBreadcrumb.textContent = remoteCurrentPath;
            
            remoteFileListBody.innerHTML = '';
            if (!data.files || data.files.length === 0) {
                remoteFileListBody.innerHTML = '<tr><td colspan="3" style="text-align:center;">Empty directory</td></tr>';
                return;
            }

            data.files.forEach(file => {
                const row = document.createElement('tr');
                if (file.is_dir) {
                    row.className = 'clickable-row folder-row';
                }
                
                const iconClass = file.is_dir ? 'icon-folder' : 'icon-file';
                row.innerHTML = `
                    <td class="col-name"><span class="${iconClass}"></span>${file.name}</td>
                    <td class="col-size">${file.is_dir ? '-' : formatSize(file.size)}</td>
                    <td class="col-type">${file.is_dir ? 'Directory' : 'File'}</td>
                `;

                if (file.is_dir) {
                    row.addEventListener('click', () => {
                        const newPath = remoteCurrentPath.endsWith('/') ? 
                            remoteCurrentPath + file.name : 
                            remoteCurrentPath + '/' + file.name;
                        fetchRemoteFiles(newPath);
                    });
                }

                remoteFileListBody.appendChild(row);
            });
        } catch (err) {
            addLog(`Error fetching remote files: ${err.message}`, 'error');
            remoteFileListBody.innerHTML = `<tr><td colspan="3" class="log-error">Error: ${err.message}</td></tr>`;
        }
    };

    // Drag and Drop Logic
    remoteDropZone.addEventListener('dragover', (e) => {
        e.preventDefault();
        remoteDropZone.classList.add('drop-target');
    });

    remoteDropZone.addEventListener('dragleave', () => {
        remoteDropZone.classList.remove('drop-target');
    });

    remoteDropZone.addEventListener('drop', (e) => {
        e.preventDefault();
        remoteDropZone.classList.remove('drop-target');
        
        try {
            const data = JSON.parse(e.dataTransfer.getData('text/plain'));
            dropData = data;
            modalFileInfo.textContent = `File: ${data.name} (${formatSize(data.size)}) to ${remoteCurrentPath}`;
            dropModal.classList.remove('hidden');
        } catch (err) {
            console.error('Invalid drop data', err);
        }
    });

    modalCancelBtn.addEventListener('click', () => {
        dropModal.classList.add('hidden');
        dropData = null;
    });

    modalConfirmBtn.addEventListener('click', async () => {
        if (!dropData) return;
        
        const config = getFormData();
        config.files = [dropData.path];
        config.remote_dir = remoteCurrentPath;
        config.delete_after_verify = modalDeleteLocal.checked;
        config.overwrite = modalOverwriteRemote.checked;
        
        dropModal.classList.add('hidden');
        showStatus(`Uploading ${dropData.name}...`, 'success');
        
        try {
            const response = await fetch('/api/upload', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(config)
            });
            const result = await response.json();
            if (response.ok) {
                showStatus(result.message, 'success');
                fetchRemoteFiles(remoteCurrentPath);
                fetchFiles(currentPath);
            } else {
                throw new Error(result.error || result.message || 'Upload failed');
            }
        } catch (err) {
            addLog(`Upload Error: ${err.message}`, 'error');
            showStatus(`Error: ${err.message}`, 'error');
        } finally {
            dropData = null;
        }
    });

    selectAllCheckbox.addEventListener('change', (e) => {
        const checkboxes = document.querySelectorAll('.file-checkbox:not(:disabled)');
        checkboxes.forEach(cb => {
            cb.checked = e.target.checked;
            const path = cb.getAttribute('data-path');
            const name = cb.getAttribute('data-name');
            if (e.target.checked) {
                queuedFiles.set(path, { name });
            } else {
                queuedFiles.delete(path);
            }
        });
        updateUploadButtonText();
    });

    const updateUploadButtonText = () => {
        if (queuedFiles.size > 0) {
            uploadBtn.textContent = `Upload ${queuedFiles.size} Selected Files`;
        } else {
            uploadBtn.textContent = "Upload All Files in Current Dir";
        }
    };

    const showStatus = (message, type) => {
        statusMsg.textContent = message;
        statusMsg.className = `status-msg ${type}`;
        statusMsg.classList.remove('hidden');
    };

    const testConnection = async () => {
        if (!sftpForm.checkValidity()) {
            sftpForm.reportValidity();
            return;
        }
        const config = getFormData();
        testBtn.disabled = true;
        try {
            const response = await fetch('/api/test-connection', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(config)
            });
            const result = await response.json();
            if (response.ok) {
                showStatus('Connection successful!', 'success');
                fetchRemoteFiles();
            } else throw new Error(result.error || 'Connection failed');
        } catch (err) {
            showStatus(`Connection Error: ${err.message}`, 'error');
        } finally {
            testBtn.disabled = false;
        }
    };

    const triggerUpload = async () => {
        if (!sftpForm.checkValidity()) {
            sftpForm.reportValidity();
            return;
        }
        const config = getFormData();
        uploadBtn.disabled = true;
        statusMsg.classList.add('hidden');
        try {
            const response = await fetch('/api/upload', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(config)
            });
            const result = await response.json();
            if (response.status === 207) {
                showStatus(result.message, 'error');
                if (result.errors) result.errors.forEach(err => addLog(err, 'error'));
            } else if (response.ok) {
                showStatus(result.message, 'success');
                queuedFiles.clear();
                updateUploadButtonText();
                fetchRemoteFiles(remoteCurrentPath);
            } else {
                throw new Error(result.error || 'Upload failed');
            }
            setTimeout(() => fetchFiles(currentPath), 2000);
        } catch (err) {
            addLog(`Upload Error: ${err.message}`, 'error');
            showStatus(`Error: ${err.message}`, 'error');
        } finally {
            uploadBtn.disabled = false;
        }
    };

    refreshBtn.addEventListener('click', () => fetchFiles(currentPath));
    upBtn.addEventListener('click', () => {
        if (!currentPath) return;
        const parts = currentPath.split('/');
        parts.pop();
        fetchFiles(parts.join('/'));
    });

    remoteRefreshBtn.addEventListener('click', () => fetchRemoteFiles(remoteCurrentPath));
    remoteUpBtn.addEventListener('click', () => {
        if (!remoteCurrentPath || remoteCurrentPath === '/') return;
        const parts = remoteCurrentPath.split('/');
        parts.pop();
        let parent = parts.join('/');
        if (parent === '') parent = '/';
        fetchRemoteFiles(parent);
    });

    testBtn.addEventListener('click', testConnection);
    uploadBtn.addEventListener('click', triggerUpload);

    const connectSSE = () => {
        const eventSource = new EventSource('/api/logs');
        eventSource.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                addLog(data.msg, data.level);
            } catch (e) {
                addLog(event.data, 'info');
            }
        };
        eventSource.onerror = () => {
            eventSource.close();
            setTimeout(connectSSE, 3000);
        };
    };

    fetchFiles();
    connectSSE();
});
