"use client";

import AppSidebar from "@/components/app-sidebar";
import { SidebarProvider, SidebarInset } from "@/components/ui/sidebar";
import { useEffect } from "react";
import { initializeDriveKeys } from "@/crypto/drive";
import { AppHeader } from "@/components/app-header";

export default function DriveLayout({ children }: { children: React.ReactNode }) {
    useEffect(() => {
        initializeDriveKeys().catch(console.error);
    }, []);

    return (
        <SidebarProvider>
            <AppSidebar />
            <SidebarInset>
                <AppHeader />
                {children}
            </SidebarInset>
        </SidebarProvider>
    );
}
