import Cookies from 'js-cookie';

export const ACCESS_TOKEN_COOKIE_EXPIRES_DAYS = 15;
export const ACCESS_TOKEN_COOKIE_OPTIONS = {
  path: '/',
  sameSite: 'Lax' as const,
};

function resolveAccessTokenExpireDate(accessToken: string): Date | undefined {
  const parts = accessToken.trim().split('-');
  if (parts.length !== 3) {
    return undefined;
  }
  const tokenExpireMinutes = Number.parseInt(parts[1], 35);
  if (!Number.isFinite(tokenExpireMinutes) || tokenExpireMinutes <= 0) {
    return undefined;
  }
  return new Date(tokenExpireMinutes * 60 * 1000);
}

export function getStoredAccessToken(): string {
  const storedToken = localStorage.getItem('accessToken') || '';
  const cookieToken = Cookies.get('Authorization') || '';
  const latestToken = (storedToken || cookieToken).trim();
  return latestToken;
}

export function persistAccessToken(accessToken: string): string {
  const normalizedToken = accessToken.trim();
  if (!normalizedToken) {
    return '';
  }
  localStorage.setItem('accessToken', normalizedToken);

  const expireAt = resolveAccessTokenExpireDate(normalizedToken);
  Cookies.set('Authorization', normalizedToken, {
    ...ACCESS_TOKEN_COOKIE_OPTIONS,
    expires: expireAt || ACCESS_TOKEN_COOKIE_EXPIRES_DAYS,
  });
  return normalizedToken;
}

export function clearPersistedAccessToken() {
  localStorage.removeItem('accessToken');

  // 兼容历史 cookie 属性，双删确保可清理
  Cookies.remove('Authorization');
  Cookies.remove('Authorization', { path: '/' });
  Cookies.remove('accessToken');
  Cookies.remove('accessToken', { path: '/' });
}
