document.addEventListener('DOMContentLoaded', () => {
    const fileListBody = document.getElementById('file-list-body');
    const refreshBtn = document.getElementById('refresh-btn');
    const testBtn = document.getElementById('test-btn');
    const uploadBtn = document.getElementById('upload-btn');
    const statusMsg = document.getElementById('status-message');
    const logContainer = document.getElementById('log-container');
    const sftpForm = document.getElementById('sftp-form');

    let queuedFiles = new Set();

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

    // SSE Connection for live logs
    const connectSSE = () => {
        const eventSource = new EventSource('/api/logs');
        
        const cleanup = () => {
            eventSource.close();
            window.removeEventListener('beforeunload', cleanup);
        };

        window.addEventListener('beforeunload', cleanup);

        eventSource.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                addLog(data.msg, data.level);
            } catch (e) {
                addLog(event.data, 'info');
            }
        };

        eventSource.onerror = (err) => {
            console.error("SSE Error:", err);
            cleanup();
            setTimeout(connectSSE, 3000); // Reconnect after 3s
        };
    };

    const fetchFiles = async () => {
        try {
            const response = await fetch('/api/files');
            const files = await response.json();
            
            fileListBody.innerHTML = '';
            if (files.length === 0) {
                fileListBody.innerHTML = '<tr><td colspan="4" style="text-align:center;">No files found in local directory</td></tr>';
                return;
            }

            files.forEach(file => {
                const row = document.createElement('tr');
                const isChecked = queuedFiles.has(file.name) ? 'checked' : '';
                row.innerHTML = `
                    <td><input type="checkbox" class="file-checkbox" data-name="${file.name}" ${isChecked}></td>
                    <td>${file.name}</td>
                    <td>${formatSize(file.size)}</td>
                    <td>${file.is_dir ? 'Directory' : 'File'}</td>
                `;
                fileListBody.appendChild(row);
            });

            document.querySelectorAll('.file-checkbox').forEach(cb => {
                cb.addEventListener('change', (e) => {
                    const name = e.target.getAttribute('data-name');
                    if (e.target.checked) queuedFiles.add(name);
                    else queuedFiles.delete(name);
                    updateUploadButtonText();
                });
            });
            updateUploadButtonText();
        } catch (err) {
            addLog(`Error fetching files: ${err.message}`, 'error');
        }
    };

    const updateUploadButtonText = () => {
        if (queuedFiles.size > 0) {
            uploadBtn.textContent = `Upload ${queuedFiles.size} Selected Files`;
        } else {
            uploadBtn.textContent = "Upload All Files";
        }
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
            max_retries: parseInt(formData.get('max_retries')),
            skip_host_key_verification: formData.get('skip_host_key_verification') === 'on',
            files: Array.from(queuedFiles)
        };
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
            } else {
                throw new Error(result.error || 'Connection failed');
            }
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
                if (result.errors) {
                    result.errors.forEach(err => addLog(err, 'error'));
                }
                setTimeout(fetchFiles, 2000);
            } else if (response.ok) {
                showStatus(result.message, 'success');
                queuedFiles.clear();
                setTimeout(fetchFiles, 2000);
            } else {
                throw new Error(result.error || 'Upload failed');
            }
        } catch (err) {
            addLog(`Upload Error: ${err.message}`, 'error');
            showStatus(`Error: ${err.message}`, 'error');
        } finally {
            uploadBtn.disabled = false;
        }
    };

    refreshBtn.addEventListener('click', fetchFiles);
    testBtn.addEventListener('click', testConnection);
    uploadBtn.addEventListener('click', triggerUpload);

    // Initial load
    fetchFiles();
    connectSSE();
});
