import { reactive } from 'vue';
import { api } from '@/services/api';

export const authStore = reactive({
    user: JSON.parse(localStorage.getItem('paperless_user') || 'null'),
    token: localStorage.getItem('paperless_token'),

    isAuthenticated() {
        return Boolean(this.token);
    },

    async login(username, password) {
        const result = await api.login(username, password);
        this.token = result.token;
        this.user = result.user;
        localStorage.setItem('paperless_token', result.token);
        localStorage.setItem('paperless_user', JSON.stringify(result.user));
        return result;
    },

    async loadMe() {
        const result = await api.me();
        this.user = result.user;
        localStorage.setItem('paperless_user', JSON.stringify(result.user));
        return result.user;
    },

    async logout() {
        try {
            await api.logout();
        } finally {
            this.clear();
        }
    },

    clear() {
        this.user = null;
        this.token = null;
        localStorage.removeItem('paperless_token');
        localStorage.removeItem('paperless_user');
    }
});

