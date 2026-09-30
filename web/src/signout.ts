import { config } from "@/config/env";
// src/signout.ts
import {getCsrfToken, clearAuthState, getAccessToken} from "./lib/authStore";
import { deleteDpopDatabase } from "./lib/dpop";
import {customFetch} from "@/lib/api";

export async function signOut() {
  try {
    let endpoint = "/v1/signout";
    
    // Check if user is a demo user by reading the JWT
    const token = getAccessToken();
    if (token) {
      try {
        const payload = JSON.parse(atob(token.split('.')[1]));
        if (payload.isDemo === true) {
          endpoint = "/v1/demo/finish";
        }
      } catch (e) {
        // ignore decode errors
      }
    }

    // 1. Tell Go server to destroy the session in DB and wipe HttpOnly cookies
    await customFetch(`${config.apiUrl}${endpoint}`, {
      method: "POST",
    });
  } catch (err) {
    console.error("Failed to reach server during signout:", err);
  } finally {
    // 2. Regardless of network success, wipe JS memory & localStorage immediately!
    clearAuthState();
    try {
      await deleteDpopDatabase();
    } catch (e) {
      console.warn("Could not wipe DPoP database:", e);
    }
  }
}
