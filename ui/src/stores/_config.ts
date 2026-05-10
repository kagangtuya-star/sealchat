import axiosFactory, { Axios } from "axios"
import Cookies from "js-cookie"
import { persistAccessToken } from "@/utils/authToken";
const axios = axiosFactory.create()
axios.defaults.withCredentials = true;

// export const urlBase = '//' + window.location.hostname + ":" + 3212;
// export const urlBase = '//' + window.location.host + '/';

const _appBase: string = typeof window !== 'undefined'
  ? ((window as any).__SEALCHAT_BASE__ ?? '')
  : '';

function detectBasePathFromURL(): string {
  if (typeof window === 'undefined') return '';
  let base = window.location.pathname;
  // Remove /index.html suffix if present
  base = base.replace(/\/index\.html$/, '');
  // Remove trailing slashes
  base = base.replace(/\/+$/, '');
  // If root path, return empty (no subdirectory)
  if (base === '' || base === '/') return '';
  return base;
}

const _effectiveBase = _appBase || detectBasePathFromURL();

export const urlBase = import.meta.env.MODE === 'development'
  ? '//' + window.location.hostname + ":" + 3212
  : '//' + window.location.host + _effectiveBase;

console.log('mode', import.meta.env.MODE)

export const api = axiosFactory.create({
  baseURL: urlBase + '/',
  withCredentials: true,
  timeout: 10000,
  maxRedirects: 3,
  transitional: {
    silentJSONParsing: false
  },
  responseType: 'json',
});

api.interceptors.request.use(config => {
  const headers = (config.headers || {}) as Record<string, any>;
  const existingAuth = headers['Authorization'] || headers['authorization'];
  if (!existingAuth) {
    const token = localStorage.getItem('accessToken') || Cookies.get('Authorization') || '';
    if (token && token !== 'null' && token !== 'undefined') {
      headers['Authorization'] = token;
    } else {
      delete headers['Authorization'];
      delete headers['authorization'];
    }
  }
  config.headers = headers;
  return config;
});

api.interceptors.response.use(resp => {
  const headers = (resp.headers || {}) as Record<string, any>;
  const refreshedToken = headers['x-access-token-refresh'] || headers['X-Access-Token-Refresh'];
  if (typeof refreshedToken === 'string' && refreshedToken.trim() !== '') {
    persistAccessToken(refreshedToken);
  }
  return resp;
});
