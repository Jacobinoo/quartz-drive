import { refreshSession } from "@/refresh";
import { createDpopProof, getDpopKeyPair } from "./dpop";
import { getAccessToken, getCsrfToken } from "./authStore";
import { signOut } from "@/signout";

// --- CONCURRENCY LOCK STATE ---
let isRefreshing = false;
let refreshQueue: Array<(token: string | Error) => void> = [];

const processQueue = (newToken: string | Error) => {
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
    dpopProof = await createDpopProof(privateKey, publicKey, method, url, token);
  } catch (e) {
    console.warn("Could not generate DPoP proof for retry:", e);
  }
  return attachHeaders(init, token, dpopProof, cf);
}

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

  return {
    ...init,
    headers,
    credentials: "include",
  };
}

export async function customFetch(
  input: RequestInfo | URL,
  init?: RequestInit
): Promise<Response> {
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

  if (response.status !== 401) {
    return response;
  }

  if (isRefreshing) {
    console.log("401 encountered, but refresh is already in progress. Waiting in queue...");
    return new Promise<Response>((resolve, reject) => {
      refreshQueue.push(async (newToken: string | Error) => {
        if (newToken instanceof Error) {
            reject(newToken);
            return;
        }
          const freshConfig = await getFreshConfig(init, newToken, method, url);
          resolve(await fetch(input, freshConfig));
      });
    });
  }

  console.log("Token expired! Locking gate and calling refreshSession()...");
  isRefreshing = true;

  try {
    const resData = await refreshSession();
    const newToken = resData.token || "";

    isRefreshing = false;
    processQueue(newToken);

    config = await getFreshConfig(init, newToken, method, url);
    return await fetch(input, config);

  } catch (err: any) {
    isRefreshing = false;

    if (err.message === "NETWORK_ERROR") {
      console.warn("Backend is offline. Pausing requests.");
      const queued = refreshQueue;
      refreshQueue = [];
      queued.forEach((callback) => callback(err));
      throw err;
    }

    if (err.message === "SESSION_EXPIRED") {
      console.error("Session expired (server returned 401/403). Logging out.");
      refreshQueue = [];
      await signOut();
      window.location.href = "/signin";
      throw err;
    }

    // Any other error (DPoP failure, JSON parse, etc.) is transient.
    // Do NOT destroy the session — just propagate the error to the caller.
    console.warn("Refresh failed with non-fatal error. Not logging out.", err.message);
    const queued = refreshQueue;
    refreshQueue = [];
    queued.forEach((callback) => callback(err));
    throw err;
  }
}
