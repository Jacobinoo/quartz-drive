"use client";

import { useEffect, useState, useRef } from "react";
import { refreshSession } from "@/refresh";
import { useRouter, usePathname } from "next/navigation";
import { signOut } from "@/signout";
import { clearAuthState } from "@/lib/authStore";
import { deleteDpopDatabase } from "@/lib/dpop";
import { deleteDeviceKeys } from "@/DeviceKeyStore";

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const publicPaths = ["/signin", "/signup", "/forgot-password", "/reset-password", "/verify-email", "/onboarding", "/terms", "/privacy"];

  const [isBooting, setIsBooting] = useState(true);

  const hasBooted = useRef(false);

  const [isOffline, setIsOffline] = useState(false);
  const [isRateLimited, setIsRateLimited] = useState(false);

  if (process.env.NODE_ENV != "development") {
    useEffect(() => {
      const timeout = setTimeout(() => {
        displayConsoleScamWarning()
        displayConsoleScamWarning()
      }, 2000);

      return () => clearTimeout(timeout);
    }, [pathname]);
  }

  useEffect(() => {
    if (hasBooted.current) return;
    hasBooted.current = true;

    async function boot() {
      try {
        console.log("App booting... restoring session from secure cookie...");
        // This hits /v1/refresh and automatically populates your authStore in memory!
        await refreshSession();

        // User is authenticated. If they're on a public auth page, redirect to drive.
        if (publicPaths.includes(pathname)) {
           router.push("/drive");
        }
        setIsBooting(false);
      } catch (err: any) {
        if (err.message === "NETWORK_ERROR") {
          console.warn("Backend is offline during boot. Halting boot sequence without logging out.");
          if (publicPaths.includes(pathname)) {
            setIsBooting(false);
          } else {
            setIsOffline(true);
            setIsBooting(false);
          }
          return;
        }
        if (err.message === "RATE_LIMIT_ERROR") {
          console.warn("Backend rate limit exceeded during boot.");
          if (publicPaths.includes(pathname)) {
            setIsBooting(false);
          } else {
            setIsRateLimited(true);
            setIsBooting(false);
          }
          return;
        }

        // SESSION_EXPIRED = server explicitly rejected the session (401/403).
        // Force a full sign-out. Use hard navigation to guarantee redirect
        // and keep the loading screen up (don't setIsBooting(false)) so the
        // protected page never renders.
        if (err.message === "SESSION_EXPIRED") {
          console.warn("Session expired (server returned 401/403). Signing out.");
          if (publicPaths.includes(pathname)) {
            clearAuthState();
            deleteDpopDatabase().catch(() => {});
            deleteDeviceKeys().catch(() => {});
            setIsBooting(false);
          } else {
            await signOut();
            window.location.href = "/signin";
          }
          return;
        }

        // Any other error (DPoP failure, IndexedDB hiccup, JSON parse error, etc.)
        // is transient — do NOT destroy the session.
        console.warn("Boot failed with non-fatal error:", err.message);
        if (publicPaths.includes(pathname)) {
          // On public pages, just show the page (e.g., the login form)
          setIsBooting(false);
        } else {
          // On protected pages, show offline screen so user can retry
          setIsOffline(true);
          setIsBooting(false);
        }
      }
    }

    boot();
  }, [pathname, router]);

  // 3. Show a sleek loading screen while we verify cookies and unwrap keys
  if (isBooting) {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-background text-muted-foreground">
        <p className="animate-pulse text-sm font-medium">Loading Quartz Drive...</p>
      </div>
    );
  }

  if (isOffline) {
    return (
      <div className="flex flex-col h-screen w-screen items-center justify-center bg-background text-muted-foreground gap-4">
        <p className="text-sm font-medium text-destructive">Cannot reach Quartz servers. Are you offline?</p>
        <button
          onClick={() => window.location.reload()}
          className="px-4 py-2 bg-primary text-primary-foreground font-medium rounded-md hover:opacity-90 transition-opacity"
        >
          Try Again
        </button>
      </div>
    );
  }

  if (isRateLimited) {
    return (
      <div className="flex flex-col h-screen w-screen items-center justify-center bg-background text-muted-foreground gap-4">
        <p className="text-sm font-medium text-destructive">Rate limited. Try again in a minute.</p>
        <button
          onClick={() => window.location.reload()}
          className="px-4 py-2 bg-primary text-primary-foreground font-medium rounded-md hover:opacity-90 transition-opacity"
        >
          Try Again
        </button>
      </div>
    );
  }

  return <>{children}</>;
}

function displayConsoleScamWarning(): void {
    console.log(
      "\n\n\n\n%cHold Up! WARNING!",
      "color: red; font-size: 50px; font-weight: bold; text-shadow: 2px 2px 0 #000;"
    );

    console.log(
      "%cIf someone told you to copy/paste something here you have an 11/10 chance %cyou're being scammed.",
      "font-size: 20px;", "font-size: 20px; color: red;"
    );

    console.log(
      "%cPasting anything in here could give attackers access to your encrypted files. Don't paste anything. %cCLOSE this window RIGHT NOW to stay safe!",
      "font-size: 16px; font-weight: bold;", "font-weight: bold; font-size: 20px; color: red;"
    );

    console.log(
      "%cPlease report any suspicious behavior to customer support!\nOur customer support will NEVER ask you to paste ANYTHING in here. We will NEVER ask for your password, recovery keys, or private keys. If someone asked you to give them your credentials, you are being scammed.\n",
      "font-size: 14px;"
    );
}
