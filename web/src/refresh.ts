import { config } from "@/config/env";
import { RefreshSessionResponse } from "@/RefreshSessionResponse";
import { createDpopProof, getDpopKeyPair } from "./lib/dpop";
import { getCsrfToken, setAccountPrivateKeys, setAuthState } from "./lib/authStore";
import { getSodium } from "./lib/crypto/sodium";
import { loadDevicePrivateKey } from "./DeviceKeyStore";

export async function refreshSession(): Promise<RefreshSessionResponse> {

  let dpopProof: string | null = null;
  try {
    const { privateKey, publicKey } = await getDpopKeyPair();
    dpopProof = await createDpopProof(privateKey, publicKey, "POST", `${config.apiUrl}/v1/refresh`);
  } catch (e) {
    console.warn("Could not generate DPoP proof (user might not be logged in yet):", e);
  }

    let refreshRes: Response;
    try {
        refreshRes = await fetch(`${config.apiUrl}/v1/refresh`, {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              "DPoP": dpopProof || "",
              "X-CSRF-Token": getCsrfToken() || "",
            },
            credentials: "include"
        });
    } catch (e) {
        throw new Error("NETWORK_ERROR");
    }

    if (refreshRes.status === 429) {
        throw new Error("RATE_LIMIT_ERROR");
    }
    
    if (refreshRes.status >= 500) {
        throw new Error("NETWORK_ERROR");
    }

    // 401/403 = server explicitly rejected the session (expired, revoked, reuse detection)
    // We should force a full sign-out.
    if (refreshRes.status === 401 || refreshRes.status === 403) {
        throw new Error("SESSION_EXPIRED");
    }

    const resData: RefreshSessionResponse = await refreshRes.json();

  if (resData.status === "ok") {
    setAuthState(resData.token || "", resData.csrfToken || "")
    // Split-Key Session Unwrapping from IndexedDB
    try {
        if (resData.wrappedAccountKeys) {
            const devicePrivateKey = await loadDevicePrivateKey();
            if (devicePrivateKey) {

                // 1. Convert the Base64 ciphertext string back into an ArrayBuffer
                const wrappedKeysBuffer = Uint8Array.from(atob(resData.wrappedAccountKeys), c => c.charCodeAt(0));

                // 2. Decrypt it using the Non-Extractable Private Key!
                const combinedKeysBuffer = await crypto.subtle.decrypt(
                    { name: "RSA-OAEP" },
                    devicePrivateKey,
                    wrappedKeysBuffer
                );

                // 3. Convert back to Uint8Array for Libsodium
                const combinedKeys = new Uint8Array(combinedKeysBuffer);

                if (combinedKeys && combinedKeys.length === 96) {
                    const signingKey = combinedKeys.slice(0, 64);
                    const encryptionKey = combinedKeys.slice(64, 96);

                    setAccountPrivateKeys(encryptionKey, signingKey);
                    console.log("Successfully restored zero-knowledge account keys from Non-Extractable Device Key!");
                } else {
                    throw new Error("Unwrapped keys are invalid length.");
                }
            } else {
                console.warn("Device Private Key not found in IndexedDB! User is logged out on this specific device.");
                throw new Error("SESSION_EXPIRED"); // Force re-login so device keys are regenerated
            }
        }
    } catch (err) {
        console.error("Could not unwrap Root Keys using Device Key:", err);
        throw new Error("SESSION_EXPIRED"); // Force re-login
    }
    } else {
        throw new Error("Could not refresh session:" + refreshRes.body);
    }

    console.log("New access token:", resData.token);
    console.log("New CSRF token:", resData.csrfToken);

    return resData;
}
