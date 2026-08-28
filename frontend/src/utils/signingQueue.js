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

export const INTERNAL_DOCUMENT_MENU_KEYS = Object.freeze({
    create: 'internal-document-create'
});

// Mirrors StatusesForSigningDocumentQueue in backend/internal/store/signing_documents.go -
// keep both lists in sync if a status is ever added/removed from a queue.
export const STATUSES_BY_QUEUE = Object.freeze({
    draft: Object.freeze(['draft']),
    active: Object.freeze([
        'in_progress',
        'pending_confirm',
        'auto_confirming',
        'completed_evidence_failed',
        'completed_image_failed',
        'completed_lock_failed',
        'sml_source_changed',
        'sml_source_missing'
    ]),
    history: Object.freeze(['completed', 'rejected', 'cancelled'])
});

export function normalizeSigningDocumentQueue(value) {
    const queue = String(value || '').trim().toLowerCase();
    return Object.prototype.hasOwnProperty.call(SIGNING_DOCUMENT_QUEUES, queue) ? queue : '';
}

export function signingDocumentQueueForStatus(status) {
    const value = String(status || '').trim().toLowerCase();
    if (value === 'draft') return SIGNING_DOCUMENT_QUEUES.draft;
    if (value === 'completed' || value === 'rejected' || value === 'cancelled') return SIGNING_DOCUMENT_QUEUES.history;
    return SIGNING_DOCUMENT_QUEUES.active;
}

export function signingDocumentMenuKeyForQueue(queue) {
    return SIGNING_DOCUMENT_MENU_KEYS[normalizeSigningDocumentQueue(queue)] || SIGNING_DOCUMENT_MENU_KEYS.active;
}

export function isSigningDocumentMenuKey(value) {
    return Object.values(SIGNING_DOCUMENT_MENU_KEYS).includes(value);
}
