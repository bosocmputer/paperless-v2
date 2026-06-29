const API_BASE = '';

async function request(path, options = {}) {
    const headers = new Headers(options.headers || {});
    const isFormData = options.body instanceof FormData;
    if (!isFormData) headers.set('Content-Type', 'application/json');

    const token = localStorage.getItem('paperless_token');
    if (token && !headers.has('Authorization')) headers.set('Authorization', `Bearer ${token}`);

    const response = await fetch(`${API_BASE}${path}`, {
        ...options,
        headers
    });

    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
        const message = payload.message || 'Cannot connect to PaperLess API.';
        const error = new Error(message);
        error.status = response.status;
        error.payload = payload;
        throw error;
    }

    return payload;
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

function splitIdempotencyPayload(payload = {}) {
    const { idempotencyKey, ...body } = payload;
    const headers = idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {};
    return { body, headers };
}

export const api = {
    login(username, password) {
        return request('/api/auth/login', {
            method: 'POST',
            body: JSON.stringify({ username, password })
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
    listSigningDocuments() {
        return request('/api/signing-documents');
    },
    createSigningDocument(payload) {
        const form = new FormData();
        form.set('docFormatCode', payload.docFormatCode);
        form.set('docNo', payload.docNo);
        if (payload.confirmLocked) form.set('confirmLocked', '1');
        form.set('file', payload.file);
        return request('/api/signing-documents', {
            method: 'POST',
            body: form
        });
    },
    getSigningDocument(id) {
        return request(`/api/signing-documents/${id}`);
    },
    signingDocumentPDFUrl(id, version = 'current') {
        return withQuery(`/api/signing-documents/${id}/pdf`, { version });
    },
    retrySigningDocumentLock(id) {
        return request(`/api/signing-documents/${id}/retry-sml-lock`, { method: 'POST' });
    },
    retrySigningDocumentFinalPDF(id) {
        return request(`/api/signing-documents/${id}/retry-final-pdf`, { method: 'POST' });
    },
    createSigningDocumentPrintCopy(id, payload) {
        return request(`/api/signing-documents/${id}/print-copies`, {
            method: 'POST',
            body: JSON.stringify(payload)
        });
    },
    signingDocumentPrintCopyPDFUrl(id, printCopyId) {
        return `/api/signing-documents/${id}/print-copies/${printCopyId}/pdf`;
    },
    regenerateExternalToken(signerId) {
        return request(`/api/signing-documents/external-token/${signerId}/regenerate`, { method: 'POST' });
    },
    listMySigningTasks() {
        return request('/api/my/signing-tasks');
    },
    getMySigningTask(taskId) {
        return request(`/api/my/signing-tasks/${taskId}`);
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
    publicSigningPDFUrl(token) {
        return `/api/public/signing/${token}/pdf`;
    },
    authHeaders() {
        const token = localStorage.getItem('paperless_token');
        return token ? { Authorization: `Bearer ${token}` } : {};
    }
};
