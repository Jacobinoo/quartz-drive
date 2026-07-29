import { RefreshSessionResponse } from "@/RefreshSessionResponse";
import { createDpopProof, getDpopKeyPair } from "./lib/dpop";
import { getCsrfToken, setAccountPrivateKeys, setAuthState } from "./lib/authStore";
import { getSodium } from "./lib/crypto/sodium";
import { loadSessionKeys } from "./DeviceKeyStore";

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
    setAuthState(resData.token || "", resData.csrfToken || "", resData.sessionPrivateKey || "")
    // --- NEW: Split-Key Session Unwrapping from IndexedDB ---
        try {
          if (resData.sessionPrivateKey) {
            const stored = await loadSessionKeys();
            if (stored) {
              const sodium = await getSodium();
              const sessionKeyBytes = sodium.from_base64(resData.sessionPrivateKey);
              const combinedKeys = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                  null,
                  stored.wrappedPrivateKeys,
                  sodium.from_string("SessionPersistence"),
                  stored.sessionNonce,
                  sessionKeyBytes
              );
              if (combinedKeys && combinedKeys.length === 96) {
                const signingKey = combinedKeys.slice(0, 64);
                const encryptionKey = combinedKeys.slice(64, 96);
                setAccountPrivateKeys(encryptionKey, signingKey);
                console.log("Successfully restored zero-knowledge account keys from IndexedDB!");
              }
            }
          }
        } catch (err) {
          console.warn("Could not unwrap session private keys from IndexedDB during boot:", err);
        }
    } else {
        throw new Error("Could not refresh session:" + refreshRes.body);
    }

    console.log("New access token:", resData.token);
    console.log("New CSRF token:", resData.csrfToken);

    return resData;
}
