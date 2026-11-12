"use client";

import AppSidebar from "@/components/app-sidebar";
import { SidebarProvider, SidebarInset } from "@/components/ui/sidebar";
import { useEffect } from "react";
import { initializeDriveKeys } from "@/crypto/drive";
import { AppHeader } from "@/components/app-header";
import { DemoBanner } from "@/components/demo-banner";
import { DownloadManager } from "@/components/download-manager";
import { TransferList } from "@/components/transfer-list";
import { UploadManager } from "@/components/upload-manager";

export default function DriveLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    useEffect(() => {
        initializeDriveKeys().catch(console.error);
    }, []);

    return (
        <SidebarProvider>
            <AppSidebar />
            <SidebarInset>
                <DemoBanner />
                <AppHeader />
                {children}

                <UploadManager />
                <DownloadManager />
                <TransferList />
            </SidebarInset>
        </SidebarProvider>
    );
}
