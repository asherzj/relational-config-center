// Only the current-identity capability supplies these in-memory credentials.
// Account IDs, cookies and CSRF values never enter browser persistent storage.
let credentials: { accountID: string; csrf: string } | null = null;
let generation = 0;
export const businessSessionInvalid = "rcc:business-session-invalid";
export function setBusinessSession(value: typeof credentials) {
  if (credentials?.accountID !== value?.accountID || credentials?.csrf !== value?.csrf) generation++;
  credentials = value;
}
export function businessSession() { return { credentials, generation }; }
