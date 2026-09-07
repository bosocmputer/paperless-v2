import { ref, watch } from 'vue';

const DEFAULT_OPTIONS = [10, 20, 50, 100];
const STORAGE_PREFIX = 'paperless_rows_per_page:';

// Per-viewer table preference (how many rows per page), remembered per table
// via localStorage - not filter/search state, so it does not belong in the
// URL query string like the search/filter sync in SigningDocuments.vue.
export function useTableRowsPerPage(tableKey, { options = DEFAULT_OPTIONS, defaultRows = 10 } = {}) {
    const storageKey = `${STORAGE_PREFIX}${tableKey}`;
    let stored = null;
    try {
        stored = Number(localStorage.getItem(storageKey));
    } catch {
        stored = null;
    }
    const initial = options.includes(stored) ? stored : defaultRows;
    const rowsPerPage = ref(initial);

    watch(rowsPerPage, (value) => {
        try {
            localStorage.setItem(storageKey, String(value));
        } catch {
            // ignore storage failures (private mode, quota, etc.) - preference just won't persist
        }
    });

    function onRowsPerPageChange(value) {
        rowsPerPage.value = value;
    }

    return { rowsPerPage, rowsPerPageOptions: options, onRowsPerPageChange };
}
