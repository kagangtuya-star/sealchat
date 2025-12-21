import axiosFactory, { Axios } from "axios"
import Cookies from "js-cookie"
const axios = axiosFactory.create()
axios.defaults.withCredentials = true;

// export const urlBase = '//' + window.location.hostname + ":" + 3212;
// export const urlBase = '//' + window.location.host + '/';

export const urlBase = import.meta.env.MODE === 'development'
  ? '//' + window.location.hostname + ":" + 3212
  : '//' + window.location.host;

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
