document.addEventListener('DOMContentLoaded', () => {
    const fileListBody = document.getElementById('file-list-body');
    const refreshBtn = document.getElementById('refresh-btn');
    const uploadBtn = document.getElementById('upload-btn');
    const statusMsg = document.getElementById('status-message');
    const logContainer = document.getElementById('log-container');

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
        const time = new Date().toLocaleTimeString();
        entry.innerHTML = `<span class="log-time">[${time}]</span> ${message}`;
        logContainer.appendChild(entry);
        logContainer.scrollTop = logContainer.scrollHeight;
    };

    const fetchFiles = async () => {
        try {
            const response = await fetch('/api/files');
            const files = await response.json();
            
            fileListBody.innerHTML = '';
            if (files.length === 0) {
                fileListBody.innerHTML = '<tr><td colspan="3" style="text-align:center;">No files found in local directory</td></tr>';
                return;
            }

            files.forEach(file => {
                const row = document.createElement('tr');
                row.innerHTML = `
                    <td>${file.name}</td>
                    <td>${formatSize(file.size)}</td>
                    <td>${file.is_dir ? 'Directory' : 'File'}</td>
                `;
                fileListBody.appendChild(row);
            });
        } catch (err) {
            addLog(`Error fetching files: ${err.message}`, 'error');
        }
    };

    const triggerUpload = async () => {
        const form = document.getElementById('sftp-form');
        if (!form.checkValidity()) {
            form.reportValidity();
            return;
        }

        const formData = new FormData(form);
        const config = {
            host: formData.get('host'),
            port: parseInt(formData.get('port')),
            user: formData.get('user'),
            password: formData.get('password'),
            key_path: formData.get('key_path'),
            remote_dir: formData.get('remote_dir'),
            delete_after_verify: formData.get('delete_after_verify') === 'on',
            max_retries: parseInt(formData.get('max_retries'))
        };

        uploadBtn.disabled = true;
        statusMsg.className = 'status-msg hidden';
        addLog('Starting SFTP upload process...', 'info');

        try {
            const response = await fetch('/api/upload', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(config)
            });
            const result = await response.json();

            if (response.ok) {
                addLog(`Upload process completed: ${result.message}`, 'success');
                statusMsg.textContent = result.message;
                statusMsg.className = 'status-msg success';
                
                // Refresh list after a delay if delete_after_verify was true
                setTimeout(fetchFiles, 2000);
            } else {
                throw new Error(result.error || 'Upload failed');
            }
        } catch (err) {
            addLog(`Upload Error: ${err.message}`, 'error');
            statusMsg.textContent = `Error: ${err.message}`;
            statusMsg.className = 'status-msg error';
        } finally {
            uploadBtn.disabled = false;
        }
    };

    refreshBtn.addEventListener('click', fetchFiles);
    uploadBtn.addEventListener('click', triggerUpload);

    // Initial load
    fetchFiles();
});
