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
  const publicPaths = ["/signin", "/signup", "/forgot-password", "/reset-password"];

  const [isBooting, setIsBooting] = useState(true);

  const hasBooted = useRef(false);

  const [isOffline, setIsOffline] = useState(false);
  const [isRateLimited, setIsRateLimited] = useState(false);

  useEffect(() => {
    if (hasBooted.current) return;
    hasBooted.current = true;

    async function boot() {
      try {
        console.log("App booting... restoring session from secure cookie...");
        // This hits /v1/refresh and automatically populates your authStore in memory!
        await refreshSession();
        
        if (publicPaths.includes(pathname)) {
           router.push("/drive");
           return;
        }
        setIsBooting(false);
      } catch (err: any) {
        if (err.message === "NETWORK_ERROR") {
          console.warn("Backend is offline during boot. Halting boot sequence without logging out.");
          setIsOffline(true);
          setIsBooting(false);
          return;
        }
        if (err.message === "RATE_LIMIT_ERROR") {
          console.warn("Backend rate limit exceeded during boot.");
          setIsRateLimited(true);
          setIsBooting(false);
          return;
        }
        
        console.warn("Silent boot failed (cookie missing/expired).");
        
        if (publicPaths.includes(pathname)) {
            clearAuthState();
            deleteDpopDatabase().catch(() => {});
            deleteDeviceKeys().catch(() => {});
            setIsBooting(false);
        } else {
            await signOut();
            router.push("/signin");
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
