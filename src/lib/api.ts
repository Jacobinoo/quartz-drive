import { refreshSession } from "@/refresh";
import { createDpopProof, getDpopKeyPair } from "./dpop";
import { getAccessToken, getCsrfToken } from "./authStore";
import { signOut } from "@/signout";

// --- CONCURRENCY LOCK STATE ---
let isRefreshing = false;
let refreshQueue: Array<(token: string) => void> = [];

const processQueue = (newToken: string) => {
  refreshQueue.forEach((callback) => callback(newToken));
  refreshQueue = [];
};

async function getFreshConfig(
  init: RequestInit = {},
  token: string,
  method: string,
  url: string
): Promise<RequestInit> {
  const cf = getCsrfToken();
  let dpopProof: string | null = null;

  try {
    const { privateKey, publicKey } = await getDpopKeyPair();
    // Generate a BRAND NEW proof signed specifically for this new token!
    dpopProof = await createDpopProof(privateKey, publicKey, method, url, token);
  } catch (e) {
    console.warn("Could not generate DPoP proof for retry:", e);
  }
  return attachHeaders(init, token, dpopProof, cf);
}

// --- HELPER TO INJECT AUTH HEADERS ---
function attachHeaders(init: RequestInit = {}, token: string | null, dpopProof: string | null, cf: string | null): RequestInit {
  const headers = new Headers(init.headers || {});

  if (token) {
    headers.set("Authorization", `DPoP ${token}`);
  }
  if (dpopProof) {
    headers.set("DPoP", dpopProof);
  }
  if (cf) {
    headers.set("X-CSRF-Token", cf);
  }

  // Always include cookies for session/refresh mechanics
  return {
    ...init,
    headers,
    credentials: "include",
  };
}

// --- THE SMART FETCH WRAPPER ---
export async function customFetch(
  input: RequestInfo | URL,
  init?: RequestInit
): Promise<Response> {
  // 1. Get the current token and make the initial request
  let token = getAccessToken();
  let cf = getCsrfToken();

  const method = (init?.method || "GET").toUpperCase();
  const url = typeof input === "string" ? input : input.toString();


  let dpopProof: string | null = null;
  try {
    const { privateKey, publicKey } = await getDpopKeyPair();
    dpopProof = await createDpopProof(privateKey, publicKey, method, url, token || undefined);
  } catch (e) {
    console.warn("Could not generate DPoP proof (user might not be logged in yet):", e);
  }

  console.log(`Attaching headers: token:${token}, dpopProof:${dpopProof}, csrf:${cf}`)

  let config = attachHeaders(init, token, dpopProof, cf);
  let response = await fetch(input, config);

  // 2. If the request succeeded (or failed with something other than 401), return immediately!
  if (response.status !== 401) {
    return response;
  }

  // 3. We hit a 401 Unauthorized! Check if someone else is already refreshing.
  if (isRefreshing) {
    console.log("401 encountered, but refresh is already in progress. Waiting in queue...");
    // Return a promise that pauses until the ongoing refresh finishes
    return new Promise<Response>((resolve) => {
      refreshQueue.push(async (newToken: string) => {
        // When woken up, retry the original request with the new token!
          const freshConfig = await getFreshConfig(init, newToken, method, url);
          resolve(await fetch(input, freshConfig));
      });
    });
  }

  // 4. We are the first request to hit a 401! Lock the gate.
  console.log("Token expired! Locking gate and calling refreshSession()...");
  isRefreshing = true;

  try {
    const resData = await refreshSession();
    const newToken = resData.token || "";

    // Unlock the gate and wake up anyone waiting in the queue!
    isRefreshing = false;
    processQueue(newToken);

    // 5. Retry OUR original request with the brand new token
    config = await getFreshConfig(init, newToken, method, url);
    return await fetch(input, config);

  } catch (err) {
    // Refresh completely failed (token expired or reuse detection triggered!)
    console.error("Silent refresh failed. Logging out.", err);
    isRefreshing = false;
    refreshQueue = []; // Clear queue

    await signOut();
    window.location.href = "/signout";
    throw err;
  }
}
