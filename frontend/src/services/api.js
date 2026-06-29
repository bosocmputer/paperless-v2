const API_BASE = '';

async function request(path, options = {}) {
    const headers = new Headers(options.headers || {});
    const isFormData = options.body instanceof FormData;
    if (!isFormData) headers.set('Content-Type', 'application/json');

    const token = localStorage.getItem('paperless_token');
    if (token) headers.set('Authorization', `Bearer ${token}`);

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
    authHeaders() {
        const token = localStorage.getItem('paperless_token');
        return token ? { Authorization: `Bearer ${token}` } : {};
    }
};
