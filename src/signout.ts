// src/signout.ts
import { getCsrfToken, clearAuthState } from "./lib/authStore";
import { deleteDpopDatabase } from "./lib/dpop";

export async function signOut() {
  try {
    // 1. Tell Go server to destroy the session in DB and wipe HttpOnly cookies
    await fetch("https://localhost:3100/v1/signout", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": getCsrfToken() || "",
      },
      credentials: "include",
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
