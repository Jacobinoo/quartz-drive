import { deleteDeviceKeys } from "@/DeviceKeyStore";

import posthog from "posthog-js";

let memoryAccessToken: string | null = null;
let memoryCsrfToken: string | null = null;

let memoryAccountEncryptionPrivateKey: Uint8Array | null = null;
let memoryAccountSigningPrivateKey: Uint8Array | null = null;

let memoryOpaqueExportKey: string | Uint8Array | null = null;
let memoryUserEmail: string | null = null;

export function setOpaqueInitData(exportKey: string | Uint8Array | null, email: string | null) {
  memoryOpaqueExportKey = exportKey;
  memoryUserEmail = email;
}

export function getOpaqueExportKey() { return memoryOpaqueExportKey; }
export function getUserEmail() { return memoryUserEmail; }


export function identifyCurrentUser() {
  const token = memoryAccessToken;
  if (token && posthog.get_explicit_consent_status() === 'granted') {
    try {
      const payload = JSON.parse(atob(token.split('.')[1]));
      if (payload.sub) {
        posthog.identify(payload.sub, {
          email: payload.email
        });
      }
    } catch (e) {
      console.warn("Failed to parse token for PostHog identify", e);
    }
  }
}

export function setAuthState(token: string | null, csrf: string | null) {
  memoryAccessToken = token;
  memoryCsrfToken = csrf;

  if (csrf) {
    localStorage.setItem("cf", csrf);
  }

  identifyCurrentUser();
}

export function setAccountPrivateKeys(encryptionKey: Uint8Array | null, signingKey: Uint8Array | null) {
  memoryAccountEncryptionPrivateKey = encryptionKey;
  memoryAccountSigningPrivateKey = signingKey;

  console.log(`MemoryAccountEncryptionPrivateKey: ${encryptionKey}`);
  console.log(`MemoryAccountEncryptionPrivateKey: ${signingKey}`);
}

export function getAccountEncryptionPrivateKey(): Uint8Array | null {
  return memoryAccountEncryptionPrivateKey;
}
export function getAccountSigningPrivateKey(): Uint8Array | null {
  return memoryAccountSigningPrivateKey;
}

export function getAccessToken(): string | null {
  return memoryAccessToken;
}

export function getCsrfToken(): string | null {
  return memoryCsrfToken || localStorage.getItem("cf");
}

export function clearAuthState() {
  memoryAccessToken = null;
  memoryCsrfToken = null;

  memoryAccountEncryptionPrivateKey = null;
  memoryAccountSigningPrivateKey = null;

  localStorage.removeItem("cf");
  deleteDeviceKeys().catch((e) => console.warn("Failed to delete device keys DB:", e));
}
