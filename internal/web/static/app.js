// =============================================================================
// News App - Shared JavaScript
// =============================================================================

// -----------------------------------------------------------------------------
// Toast Notifications
// -----------------------------------------------------------------------------

function getToastContainer() {
    let container = document.getElementById('toast-container');
    if (!container) {
        container = document.createElement('div');
        container.id = 'toast-container';
        container.className = 'toast-container';
        document.body.appendChild(container);
    }
    return container;
}

function showToast(type, title, message, duration = 10000) {
    const container = getToastContainer();
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    
    const icons = {
        success: '✓',
        error: '✕',
        info: 'ℹ'
    };
    
    toast.innerHTML = `
        <span class="toast-icon">${icons[type] || icons.info}</span>
        <div class="toast-content">
            <div class="toast-title">${title}</div>
            ${message ? `<div class="toast-message">${message}</div>` : ''}
        </div>
        <button class="toast-close" onclick="this.parentElement.remove()">&times;</button>
    `;
    
    container.appendChild(toast);
    
    // Auto-remove after duration
    if (duration > 0) {
        setTimeout(() => {
            toast.classList.add('toast-out');
            setTimeout(() => toast.remove(), 300);
        }, duration);
    }
    
    return toast;
}

function showSuccess(title, message) {
    return showToast('success', title, message);
}

function showError(title, message) {
    return showToast('error', title, message);
}

function showInfo(title, message) {
    return showToast('info', title, message);
}

// -----------------------------------------------------------------------------
// CSRF Token Helper
// -----------------------------------------------------------------------------

function getCsrfHeaders() {
    const headers = {'Content-Type': 'application/json'};
    if (window.CSRF_TOKEN) {
        headers['X-CSRF-Token'] = window.CSRF_TOKEN;
    }
    return headers;
}

// -----------------------------------------------------------------------------
// Job Actions
// -----------------------------------------------------------------------------

async function runJob(id) {
    if (!confirm('Run this job now?')) return;
    try {
        const res = await fetch(`/api/jobs/${id}/run`, { method: 'POST', headers: getCsrfHeaders() });
        if (res.ok) {
            showSuccess('Job Started', 'The job is now running.');
            setTimeout(() => location.reload(), 3000);
        } else {
            const err = await res.json();
            showError('Failed to Start Job', err.error);
        }
    } catch (err) {
        showError('Network Error', err.message);
    }
}

async function stopJob(id) {
    if (!confirm('Stop this running job?')) return;
    try {
        const res = await fetch(`/api/jobs/${id}/stop`, { method: 'POST', headers: getCsrfHeaders() });
        if (res.ok) {
            showSuccess('Job Stopped', 'The job has been stopped.');
            setTimeout(() => location.reload(), 1500);
        } else {
            const err = await res.json();
            showError('Failed to Stop Job', err.error);
        }
    } catch (err) {
        showError('Network Error', err.message);
    }
}

async function deleteJob(id) {
    if (!confirm('Delete this job? This cannot be undone.')) return;
    try {
        const res = await fetch(`/api/jobs/${id}`, { method: 'DELETE', headers: getCsrfHeaders() });
        if (res.ok) {
            showSuccess('Job Deleted', 'Redirecting to jobs list...');
            setTimeout(() => window.location.href = '/jobs', 1500);
        } else {
            const err = await res.json();
            showError('Failed to Delete Job', err.error);
        }
    } catch (err) {
        showError('Network Error', err.message);
    }
}

async function cancelRun(id) {
    if (!confirm('Cancel this run?')) return;
    try {
        const res = await fetch(`/api/runs/${id}/cancel`, { method: 'POST', headers: getCsrfHeaders() });
        if (res.ok) {
            showSuccess('Run Cancelled', 'The run has been cancelled.');
            setTimeout(() => location.reload(), 1500);
        } else {
            const err = await res.json();
            showError('Failed to Cancel Run', err.error);
        }
    } catch (err) {
        showError('Network Error', err.message);
    }
}

// -----------------------------------------------------------------------------
// Run Duration Timer
// -----------------------------------------------------------------------------

function updateDurations() {
    document.querySelectorAll('.run-duration').forEach(el => {
        const started = parseInt(el.dataset.started);
        const now = Math.floor(Date.now() / 1000);
        const duration = now - started;
        const minutes = Math.floor(duration / 60);
        const seconds = duration % 60;
        if (minutes > 0) {
            el.textContent = minutes + 'm ' + seconds + 's';
        } else {
            el.textContent = seconds + 's';
        }
    });
}

// -----------------------------------------------------------------------------
// Form Submission Helpers
// -----------------------------------------------------------------------------

async function submitJobForm(form, method, url, redirectUrl) {
    const submitBtn = form.querySelector('button[type="submit"]');
    const originalText = submitBtn.textContent;
    
    submitBtn.disabled = true;
    submitBtn.textContent = method === 'POST' ? 'Creating...' : 'Saving...';
    
    const data = {
        name: form.elements.name.value,
        prompt: form.elements.prompt.value,
        keywords: form.elements.keywords.value,
        sources: form.elements.sources.value,
        region: form.elements.region.value,
        frequency: form.elements.frequency.value,
    };
    
    // Add type-specific fields
    if (form.elements.isOneTime) {
        data.is_one_time = form.elements.isOneTime.checked;
    }
    if (form.elements.isActive) {
        data.is_active = form.elements.isActive.checked;
    }
    
    try {
        const res = await fetch(url, {
            method: method,
            headers: getCsrfHeaders(),
            body: JSON.stringify(data)
        });
        if (res.ok) {
            if (typeof redirectUrl === 'function') {
                redirectUrl(data);
            } else {
                window.location.href = redirectUrl;
            }
        } else {
            const err = await res.json();
            showError('Failed to Save', err.error);
            submitBtn.disabled = false;
            submitBtn.textContent = originalText;
        }
    } catch (err) {
        showError('Network Error', err.message);
        submitBtn.disabled = false;
        submitBtn.textContent = originalText;
    }
}

async function submitPreferencesForm(form) {
    const data = {
        system_prompt: form.systemPrompt.value,
        discord_webhook: form.discordWebhook.value,
        notify_success: form.notifySuccess.checked,
        notify_failure: form.notifyFailure.checked
    };
    
    try {
        const res = await fetch('/api/preferences', {
            method: 'POST',
            headers: getCsrfHeaders(),
            body: JSON.stringify(data)
        });
        if (res.ok) {
            showSuccess('Preferences Saved', 'Your settings have been updated.');
        } else {
            const err = await res.json();
            showError('Failed to Save Preferences', err.error);
        }
    } catch (err) {
        showError('Network Error', err.message);
    }
}

// -----------------------------------------------------------------------------
// Dashboard Helpers
// -----------------------------------------------------------------------------

function showCreatedMessage() {
    const params = new URLSearchParams(window.location.search);
    const created = params.get('created');
    if (created) {
        showSuccess('Job Created', `Job "${created}" created successfully!`);
        history.replaceState(null, '', '/');
    }
}

// -----------------------------------------------------------------------------
// Auto-initialization
// -----------------------------------------------------------------------------

document.addEventListener('DOMContentLoaded', function() {
    // Start duration timers if any exist
    if (document.querySelector('.run-duration')) {
        updateDurations();
        setInterval(updateDurations, 1000);
    }
    
    // Show created message on dashboard
    if (document.getElementById('successMessage')) {
        showCreatedMessage();
    }
});
