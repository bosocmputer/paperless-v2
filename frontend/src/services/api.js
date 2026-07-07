const API_BASE = '';
let authRedirecting = false;

async function request(path, options = {}) {
    const headers = new Headers(options.headers || {});
    const isFormData = options.body instanceof FormData;
    if (!isFormData) headers.set('Content-Type', 'application/json');

    const token = localStorage.getItem('paperless_token');
    if (token && !headers.has('Authorization')) headers.set('Authorization', `Bearer ${token}`);

    const response = await fetch(`${API_BASE}${path}`, {
        cache: 'no-store',
        ...options,
        headers
    });

    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
        const message = payload.message || 'Cannot connect to PaperLess API.';
        const error = new Error(message);
        error.status = response.status;
        error.payload = payload;
        if (response.status === 401) handleUnauthorized(path);
        throw error;
    }

    return payload;
}

function handleUnauthorized(path) {
    localStorage.removeItem('paperless_token');
    localStorage.removeItem('paperless_user');
    localStorage.removeItem('paperless_session');
    window.dispatchEvent(new CustomEvent('paperless:session-expired'));

    if (authRedirecting || shouldSkipUnauthorizedRedirect(path)) return;
    authRedirecting = true;

    const current = `${window.location.pathname}${window.location.search}${window.location.hash}`;
    const params = new URLSearchParams({ session: 'expired' });
    if (!current.startsWith('/auth/login')) params.set('redirect', current);
    window.location.replace(`/auth/login?${params.toString()}`);
}

function shouldSkipUnauthorizedRedirect(path) {
    return path.startsWith('/api/auth/login') || path.startsWith('/api/auth/logout') || path.startsWith('/api/public/');
}

function withQuery(path, params = {}) {
    const query = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
        if (value !== undefined && value !== null && value !== '') {
            query.set(key, value);
        }
    });
    const qs = query.toString();
    return qs ? `${path}?${qs}` : path;
}

function withPDFCacheKey(url, cacheKey = '') {
    if (!url || !cacheKey) return url;
    if (/[?&]v=/.test(url)) return url;
    const separator = url.includes('?') ? '&' : '?';
    return `${url}${separator}v=${encodeURIComponent(cacheKey)}`;
}

function signingDocumentPDFCacheKey(document = {}, version = 'current') {
    if (!document) return '';
    const final = version === 'final';
    const file = final ? document.finalFile : document.currentFile;
    return [
        version || 'current',
        final ? document.finalFileId : document.currentFileId,
        file?.id,
        file?.sha256,
        document.currentVersion,
        document.updatedAt,
        file?.createdAt
    ]
        .filter(Boolean)
        .join('-');
}

function signingTaskPDFCacheKey(task = {}, document = {}, version = 'current') {
    return [signingDocumentPDFCacheKey(document, version), task?.id, task?.status, task?.signedAt, task?.rejectedAt].filter(Boolean).join('-');
}

function splitIdempotencyPayload(payload = {}) {
    const { idempotencyKey, ...body } = payload;
    const headers = idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {};
    return { body, headers };
}

export const api = {
    login(username, password, databaseName = '', authSource = '') {
        return request('/api/auth/login', {
            method: 'POST',
            body: JSON.stringify({ username, password, databaseName, authSource })
        });
    },
    provisionSMLImageDatabase(username, password, databaseName = '', authSource = '') {
        return request('/api/auth/sml/provision-image-db', {
            method: 'POST',
            body: JSON.stringify({ username, password, databaseName, authSource })
        });
    },
    me() {
        return request('/api/auth/me');
    },
    logout() {
        return request('/api/auth/logout', { method: 'POST' });
    },
    listUsers() {
        return request('/api/users');
    },
    createUser(payload) {
        return request('/api/users', {
            method: 'POST',
            body: JSON.stringify(payload)
        });
    },
    updateUser(id, payload) {
        return request(`/api/users/${id}`, {
            method: 'PUT',
            body: JSON.stringify(payload)
        });
    },
    deactivateUser(id) {
        return request(`/api/users/${id}`, { method: 'DELETE' });
    },
    listSMLScreenCodes() {
        return request('/api/sml/screen-codes');
    },
    listSMLDocFormats(screenCode) {
        return request(withQuery('/api/sml/doc-formats', { screen_code: screenCode }));
    },
    getSMLDocFormat(docFormatCode) {
        return request(withQuery('/api/sml/doc-format', { doc_format_code: docFormatCode }));
    },
    listSMLDocumentCandidates(params = {}) {
        return request(
            withQuery('/api/sml/document-candidates', {
                doc_format_code: params.docFormatCode,
                search: params.search,
                page: params.page,
                size: params.size
            })
        );
    },
    getSMLDocumentCandidate(docFormatCode, docNo) {
        return request(withQuery(`/api/sml/document-candidates/${encodeURIComponent(docNo)}`, { doc_format_code: docFormatCode }));
    },
    listDocumentConfigs(params) {
        return request(
            withQuery('/api/document-configs', {
                screen_code: params?.screenCode,
                doc_format_code: params?.docFormatCode
            })
        );
    },
    createDocumentConfig(payload) {
        return request('/api/document-configs', {
            method: 'POST',
            body: JSON.stringify(payload)
        });
    },
    updateDocumentConfig(id, payload) {
        return request(`/api/document-configs/${id}`, {
            method: 'PUT',
            body: JSON.stringify(payload)
        });
    },
    deleteDocumentConfig(id) {
        return request(`/api/document-configs/${id}`, { method: 'DELETE' });
    },
    listDocumentConfigWorkflows() {
        return request('/api/document-config-workflows');
    },
    getDocumentConfigWorkflow(docFormatCode) {
        return request(`/api/document-config-workflows/${encodeURIComponent(docFormatCode)}`);
    },
    saveDocumentConfigWorkflow(docFormatCode, payload) {
        return request(`/api/document-config-workflows/${encodeURIComponent(docFormatCode)}`, {
            method: 'PUT',
            body: JSON.stringify(payload)
        });
    },
    copyDocumentConfigWorkflow(docFormatCode, payload) {
        return request(`/api/document-config-workflows/${encodeURIComponent(docFormatCode)}/copy`, {
            method: 'POST',
            body: JSON.stringify(payload)
        });
    },
    recordDocumentConfigWorkflowEvent(docFormatCode, payload) {
        return request(`/api/document-config-workflows/${encodeURIComponent(docFormatCode)}/events`, {
            method: 'POST',
            body: JSON.stringify(payload)
        });
    },
    getSignatureTemplateState(docFormatCode) {
        return request(withQuery('/api/signature-templates', { doc_format_code: docFormatCode }));
    },
    uploadSignatureTemplateSamplePDF(docFormatCode, file) {
        const form = new FormData();
        form.set('file', file);
        return request(withQuery('/api/signature-templates/sample-pdf', { doc_format_code: docFormatCode }), {
            method: 'POST',
            body: form
        });
    },
    saveSignatureTemplateBoxes(id, payload) {
        return request(`/api/signature-templates/${id}/boxes`, {
            method: 'PUT',
            body: JSON.stringify(payload)
        });
    },
    recordSignatureDesignerEvent(id, payload) {
        return request(`/api/signature-templates/${id}/designer-events`, {
            method: 'POST',
            body: JSON.stringify(payload)
        });
    },
    publishSignatureTemplate(id) {
        return request(`/api/signature-templates/${id}/publish`, { method: 'POST' });
    },
    signatureTemplateSamplePDFUrl(id) {
        return `/api/signature-templates/${id}/sample-pdf`;
    },
    listSigningDocuments(params = {}) {
        return request(
            withQuery('/api/signing-documents', {
                queue: params.queue,
                search: params.search,
                page: params.page,
                size: params.size
            })
        );
    },
    checkSigningDocumentDuplicate(params = {}) {
        return request(
            withQuery('/api/signing-documents/duplicate-check', {
                doc_format_code: params.docFormatCode,
                doc_no: params.docNo
            })
        );
    },
    getAdminDashboard() {
        return request('/api/admin/dashboard');
    },
    getAdminDocumentFlow(params = {}) {
        return request(
            withQuery('/api/admin/document-flow', {
                doc_no: params.docNo,
                doc_format_code: params.docFormatCode,
                depth: params.depth
            })
        );
    },
    recordDocumentFlowEvent(payload) {
        return request('/api/admin/document-flow/events', {
            method: 'POST',
            body: JSON.stringify(payload)
        });
    },
    uploadSigningDocumentPDF(file) {
        const form = new FormData();
        form.set('file', file);
        return request('/api/signing-documents/uploads', {
            method: 'POST',
            body: form
        });
    },
    signingDocumentUploadPDFUrl(fileId) {
        return `/api/signing-documents/uploads/${fileId}/pdf`;
    },
    recordSigningDocumentCreateEvent(payload) {
        return request('/api/signing-documents/create-events', {
            method: 'POST',
            body: JSON.stringify(payload)
        });
    },
    createSigningDocument(payload) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request('/api/signing-documents', {
            method: 'POST',
            headers,
            body: JSON.stringify(body)
        });
    },
    sendSigningDocument(id, payload = {}) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request(`/api/signing-documents/${id}/send`, {
            method: 'POST',
            headers,
            body: JSON.stringify(body)
        });
    },
    confirmSigningDocument(id, payload = {}) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request(`/api/signing-documents/${id}/confirm`, {
            method: 'POST',
            headers,
            body: JSON.stringify(body)
        });
    },
    cancelSigningDocument(id, payload = {}) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request(`/api/signing-documents/${id}/cancel`, {
            method: 'POST',
            headers,
            body: JSON.stringify(body)
        });
    },
    getSigningDocument(id) {
        return request(`/api/signing-documents/${id}`);
    },
    getSigningDocumentRelatedDocuments(id, depth = 3) {
        return request(withQuery(`/api/signing-documents/${id}/related-documents`, { depth }));
    },
    getSigningDocumentReferenceCheck(id) {
        return request(`/api/signing-documents/${id}/reference-check`);
    },
    signingDocumentPDFCacheKey(document, version = 'current') {
        return signingDocumentPDFCacheKey(document, version);
    },
    signingTaskPDFCacheKey(task, document, version = 'current') {
        return signingTaskPDFCacheKey(task, document, version);
    },
    withPDFCacheKey(url, cacheKey) {
        return withPDFCacheKey(url, cacheKey);
    },
    signingDocumentPDFUrl(id, version = 'current', cacheKey = '') {
        return withQuery(`/api/signing-documents/${id}/pdf`, { version, v: cacheKey });
    },
    signingDocumentPDFUrlForDocument(document, version = 'current') {
        if (!document?.id) return '';
        return this.signingDocumentPDFUrl(document.id, version, signingDocumentPDFCacheKey(document, version));
    },
    retrySigningDocumentLock(id, payload = {}) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request(`/api/signing-documents/${id}/retry-sml-lock`, {
            method: 'POST',
            headers,
            body: JSON.stringify(body)
        });
    },
    retrySigningDocumentFinalPDF(id, payload = {}) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request(`/api/signing-documents/${id}/retry-final-pdf`, {
            method: 'POST',
            headers,
            body: JSON.stringify(body)
        });
    },
    retrySigningDocumentImages(id, payload = {}) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request(`/api/signing-documents/${id}/retry-sml-images`, {
            method: 'POST',
            headers,
            body: JSON.stringify(body)
        });
    },
    createSigningDocumentPrintCopy(id, payload = {}) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request(`/api/signing-documents/${id}/print-copies`, {
            method: 'POST',
            headers,
            body: JSON.stringify(body)
        });
    },
    signingDocumentPrintCopyPDFUrl(id, printCopyId) {
        return `/api/signing-documents/${id}/print-copies/${printCopyId}/pdf`;
    },
    regenerateExternalToken(signerId) {
        return request(`/api/signing-documents/external-token/${signerId}/regenerate`, { method: 'POST' });
    },
    listMySigningTasks(params = {}) {
        return request(
            withQuery('/api/my/signing-tasks', {
                readyPage: params.readyPage,
                waitingPage: params.waitingPage,
                size: params.size
            })
        );
    },
    getMySigningTask(taskId) {
        return request(`/api/my/signing-tasks/${taskId}`);
    },
    listMySigningHistory(params = {}) {
        return request(
            withQuery('/api/my/signing-history', {
                page: params.page,
                size: params.size,
                search: params.search
            })
        );
    },
    getMySigningHistory(taskId) {
        return request(`/api/my/signing-history/${taskId}`);
    },
    mySigningHistoryPDFUrl(taskId, version = '', cacheKey = '') {
        return withQuery(`/api/my/signing-history/${taskId}/pdf`, { version, v: cacheKey });
    },
    mySigningHistoryPDFUrlForTask(task, document, version = '') {
        if (!task?.id) return '';
        return this.mySigningHistoryPDFUrl(task.id, version, signingTaskPDFCacheKey(task, document, version || 'current'));
    },
    getMySigningTaskRelatedDocuments(taskId, depth = 3) {
        return request(withQuery(`/api/my/signing-tasks/${taskId}/related-documents`, { depth }));
    },
    getMySigningTaskReferenceStatus(taskId) {
        return request(`/api/my/signing-tasks/${taskId}/reference-status`);
    },
    signMyTask(taskId, payload) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request(`/api/my/signing-tasks/${taskId}/sign`, {
            method: 'POST',
            headers,
            body: JSON.stringify(body)
        });
    },
    rejectMyTask(taskId, payload) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request(`/api/my/signing-tasks/${taskId}/reject`, {
            method: 'POST',
            headers,
            body: JSON.stringify(body)
        });
    },
    recordMySigningTaskEvent(taskId, payload) {
        return request(`/api/my/signing-tasks/${taskId}/events`, {
            method: 'POST',
            body: JSON.stringify(payload)
        });
    },
    uploadMyTaskAttachment(taskId, file, note = '') {
        const form = new FormData();
        form.set('file', file);
        if (note) form.set('note', note);
        return request(`/api/my/signing-tasks/${taskId}/attachments`, {
            method: 'POST',
            body: form
        });
    },
    verifyExternalOTP(token, otp) {
        return request(`/api/public/signing/${token}/verify-otp`, {
            method: 'POST',
            body: JSON.stringify({ otp })
        });
    },
    getPublicSigningDocument(token, sessionToken) {
        return request(`/api/public/signing/${token}`, {
            headers: { Authorization: `Bearer ${sessionToken}` }
        });
    },
    getPublicSigningRelatedDocuments(token, sessionToken, depth = 3) {
        return request(withQuery(`/api/public/signing/${token}/related-documents`, { depth }), {
            headers: { Authorization: `Bearer ${sessionToken}` }
        });
    },
    signPublicTask(token, sessionToken, payload) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request(`/api/public/signing/${token}/sign`, {
            method: 'POST',
            headers: { Authorization: `Bearer ${sessionToken}`, ...headers },
            body: JSON.stringify(body)
        });
    },
    rejectPublicTask(token, sessionToken, payload) {
        const { body, headers } = splitIdempotencyPayload(payload);
        return request(`/api/public/signing/${token}/reject`, {
            method: 'POST',
            headers: { Authorization: `Bearer ${sessionToken}`, ...headers },
            body: JSON.stringify(body)
        });
    },
    recordPublicSigningTaskEvent(token, sessionToken, payload) {
        return request(`/api/public/signing/${token}/events`, {
            method: 'POST',
            headers: { Authorization: `Bearer ${sessionToken}` },
            body: JSON.stringify(payload)
        });
    },
    uploadPublicTaskAttachment(token, sessionToken, file, note = '') {
        const form = new FormData();
        form.set('file', file);
        if (note) form.set('note', note);
        return request(`/api/public/signing/${token}/attachments`, {
            method: 'POST',
            headers: { Authorization: `Bearer ${sessionToken}` },
            body: form
        });
    },
    publicSigningPDFUrl(token, cacheKey = '') {
        return withPDFCacheKey(`/api/public/signing/${token}/pdf`, cacheKey);
    },
    authHeaders() {
        const token = localStorage.getItem('paperless_token');
        return token ? { Authorization: `Bearer ${token}` } : {};
    }
};
