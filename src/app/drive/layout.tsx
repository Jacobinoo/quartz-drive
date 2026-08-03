"use client";

import AppSidebar from "@/components/app-sidebar";
import { SidebarProvider } from "@/components/ui/sidebar";
import { useEffect } from "react";
import { initializeDriveKeys } from "@/crypto/drive";

export default function DriveLayout({ children }: { children: React.ReactNode }) {
    useEffect(() => {
        initializeDriveKeys().catch(console.error);
    }, []);

    return (
        <SidebarProvider>
            <AppSidebar />
            {children}
        </SidebarProvider>
    );
}
