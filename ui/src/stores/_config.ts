import axiosFactory, { Axios } from "axios"
import Cookies from "js-cookie"
import { getStoredAccessToken, persistAccessToken } from "@/utils/authToken";
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
  ? '//' + window.location.host
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

export function buildAuthorizedHeaders(headers: Record<string, any> = {}) {
  const nextHeaders = { ...headers };
  const existingAuth = nextHeaders['Authorization'] || nextHeaders['authorization'];
  if (!existingAuth) {
    const token = getStoredAccessToken();
    if (token && token !== 'null' && token !== 'undefined') {
      nextHeaders['Authorization'] = token;
    } else {
      delete nextHeaders['Authorization'];
      delete nextHeaders['authorization'];
    }
  }
  return nextHeaders;
}

export function buildAuthorizedJsonRequestInit(init: RequestInit = {}): RequestInit {
  const headers = buildAuthorizedHeaders((init.headers || {}) as Record<string, any>);
  if (!headers['Content-Type'] && !headers['content-type']) {
    headers['Content-Type'] = 'application/json';
  }
  return {
    credentials: 'include',
    ...init,
    headers,
  };
}

api.interceptors.request.use(config => {
  const headers = (config.headers || {}) as Record<string, any>;
  config.headers = buildAuthorizedHeaders(headers);
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
