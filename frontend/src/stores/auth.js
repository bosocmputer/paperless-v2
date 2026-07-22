import { reactive } from 'vue';
import { api } from '@/services/api';

export const authStore = reactive({
    user: JSON.parse(localStorage.getItem('paperless_user') || 'null'),
    token: localStorage.getItem('paperless_token'),
    session: JSON.parse(localStorage.getItem('paperless_session') || 'null'),
    features: JSON.parse(localStorage.getItem('paperless_features') || '{}'),
    sessionChecked: false,

    isAuthenticated() {
        return Boolean(this.token);
    },

    async login(username, password, databaseName = '', authSource = '') {
        const result = await api.login(username, password, databaseName, authSource);
        if (result.databaseRequired) return result;
        this.token = result.token;
        this.user = result.user;
        this.session = result.session || null;
        this.features = result.features || {};
        this.sessionChecked = true;
        localStorage.setItem('paperless_token', result.token);
        localStorage.setItem('paperless_user', JSON.stringify(result.user));
        localStorage.setItem('paperless_session', JSON.stringify(this.session));
        localStorage.setItem('paperless_features', JSON.stringify(this.features));
        return result;
    },

    async loadMe() {
        const result = await api.me();
        this.user = result.user;
        this.session = result.session || null;
        this.features = result.features || {};
        this.sessionChecked = true;
        localStorage.setItem('paperless_user', JSON.stringify(result.user));
        localStorage.setItem('paperless_session', JSON.stringify(this.session));
        localStorage.setItem('paperless_features', JSON.stringify(this.features));
        return result.user;
    },

    async logout() {
        try {
            await api.logout();
        } catch {
            // Logging out should still clear local state if the server session is already gone.
        } finally {
            this.clear();
        }
    },

    clear() {
        this.user = null;
        this.token = null;
        this.session = null;
        this.features = {};
        this.sessionChecked = false;
        localStorage.removeItem('paperless_token');
        localStorage.removeItem('paperless_user');
        localStorage.removeItem('paperless_session');
        localStorage.removeItem('paperless_features');
    }
});

window.addEventListener('paperless:session-expired', () => {
    authStore.clear();
});
