// =============================================================================
// News App - Shared JavaScript
// =============================================================================

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
            alert('Job started! Refresh the page in a moment to see results.');
            location.reload();
        } else {
            const err = await res.json();
            alert('Error: ' + err.error);
        }
    } catch (err) {
        alert('Error: ' + err.message);
    }
}

async function stopJob(id) {
    if (!confirm('Stop this running job?')) return;
    try {
        const res = await fetch(`/api/jobs/${id}/stop`, { method: 'POST', headers: getCsrfHeaders() });
        if (res.ok) {
            alert('Job stopped.');
            location.reload();
        } else {
            const err = await res.json();
            alert('Error: ' + err.error);
        }
    } catch (err) {
        alert('Error: ' + err.message);
    }
}

async function deleteJob(id) {
    if (!confirm('Delete this job? This cannot be undone.')) return;
    try {
        const res = await fetch(`/api/jobs/${id}`, { method: 'DELETE', headers: getCsrfHeaders() });
        if (res.ok) {
            window.location.href = '/jobs';
        } else {
            const err = await res.json();
            alert('Error: ' + err.error);
        }
    } catch (err) {
        alert('Error: ' + err.message);
    }
}

async function cancelRun(id) {
    if (!confirm('Cancel this run?')) return;
    try {
        const res = await fetch(`/api/runs/${id}/cancel`, { method: 'POST', headers: getCsrfHeaders() });
        if (res.ok) {
            location.reload();
        } else {
            const err = await res.json();
            alert('Error: ' + err.error);
        }
    } catch (err) {
        alert('Error: ' + err.message);
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
        name: form.name.value,
        prompt: form.prompt.value,
        keywords: form.keywords.value,
        sources: form.sources.value,
        region: form.region.value,
        frequency: form.frequency.value,
    };
    
    // Add type-specific fields
    if (form.isOneTime) {
        data.is_one_time = form.isOneTime.checked;
    }
    if (form.isActive) {
        data.is_active = form.isActive.checked;
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
            alert('Error: ' + err.error);
            submitBtn.disabled = false;
            submitBtn.textContent = originalText;
        }
    } catch (err) {
        alert('Error: ' + err.message);
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
            alert('Preferences saved!');
        } else {
            const err = await res.json();
            alert('Error: ' + err.error);
        }
    } catch (err) {
        alert('Error: ' + err.message);
    }
}

// -----------------------------------------------------------------------------
// Dashboard Helpers
// -----------------------------------------------------------------------------

function showCreatedMessage() {
    const params = new URLSearchParams(window.location.search);
    const created = params.get('created');
    if (created) {
        const msg = document.getElementById('successMessage');
        if (msg) {
            msg.textContent = 'Job "' + created + '" created successfully!';
            msg.style.display = 'block';
        }
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
