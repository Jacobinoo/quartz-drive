import { RefreshSessionResponse } from "@/RefreshSessionResponse";
import { createDpopProof, getDpopKeyPair } from "./lib/dpop";
import { getCsrfToken, setAccountPrivateKeys, setAuthState } from "./lib/authStore";
import { getSodium } from "./lib/crypto/sodium";
import { loadDevicePrivateKey } from "./DeviceKeyStore";

export async function refreshSession(): Promise<RefreshSessionResponse> {

  let dpopProof: string | null = null;
  try {
    const { privateKey, publicKey } = await getDpopKeyPair();
    dpopProof = await createDpopProof(privateKey, publicKey, "POST", "https://localhost:3100/v1/refresh");
  } catch (e) {
    console.warn("Could not generate DPoP proof (user might not be logged in yet):", e);
  }

    const refreshRes = await fetch("https://localhost:3100/v1/refresh", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "DPoP": dpopProof || "",
          "X-CSRF-Token": getCsrfToken() || "",
        },
        credentials: "include"
    });
    const resData: RefreshSessionResponse = await refreshRes.json();

  if (resData.status === "ok") {
    setAuthState(resData.token || "", resData.csrfToken || "")
    // --- NEW: Split-Key Session Unwrapping from IndexedDB ---
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
                }
            } else {
                console.warn("Device Private Key not found in IndexedDB! User is logged out on this specific device.");
            }
        }
    } catch (err) {
        console.warn("Could not unwrap Root Keys using Device Key:", err);
    }
    } else {
        throw new Error("Could not refresh session:" + refreshRes.body);
    }

    console.log("New access token:", resData.token);
    console.log("New CSRF token:", resData.csrfToken);

    return resData;
}
