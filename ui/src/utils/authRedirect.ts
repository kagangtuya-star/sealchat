// Only canonical, allowlisted in-app paths may survive authentication.
export const getInviteRedirect = (value: unknown): string | undefined => (
  typeof value === 'string' && value === value.trim() && /^\/invite\/[a-z0-9]{1,64}$/.test(value) ? value : undefined
);

export const getSignInRedirect = (value: unknown): string | undefined => {
  if (typeof value === 'string' && value === value.trim() && /^\/log\/layout\/[a-z0-9]{4,64}$/.test(value)) {
    return value;
  }
  return getInviteRedirect(value);
};
