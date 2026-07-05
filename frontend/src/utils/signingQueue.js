export const SIGNING_DOCUMENT_QUEUES = Object.freeze({
    draft: 'draft',
    active: 'active',
    history: 'history'
});

export const SIGNING_DOCUMENT_MENU_KEYS = Object.freeze({
    draft: 'signing-document-drafts',
    active: 'signing-documents',
    history: 'signing-document-history'
});

export const ADMIN_SIGNER_MENU_KEYS = Object.freeze({
    tasks: 'admin-my-signing-tasks',
    history: 'admin-my-signing-history'
});

export function normalizeSigningDocumentQueue(value) {
    const queue = String(value || '').trim().toLowerCase();
    return Object.prototype.hasOwnProperty.call(SIGNING_DOCUMENT_QUEUES, queue) ? queue : '';
}

export function signingDocumentQueueForStatus(status) {
    const value = String(status || '').trim().toLowerCase();
    if (value === 'draft') return SIGNING_DOCUMENT_QUEUES.draft;
    if (value === 'completed') return SIGNING_DOCUMENT_QUEUES.history;
    return SIGNING_DOCUMENT_QUEUES.active;
}

export function signingDocumentMenuKeyForQueue(queue) {
    return SIGNING_DOCUMENT_MENU_KEYS[normalizeSigningDocumentQueue(queue)] || SIGNING_DOCUMENT_MENU_KEYS.active;
}

export function isSigningDocumentMenuKey(value) {
    return Object.values(SIGNING_DOCUMENT_MENU_KEYS).includes(value);
}
