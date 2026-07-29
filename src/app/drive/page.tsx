"use client";

import AppSidebar from "@/components/app-sidebar";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { Separator } from "@/components/ui/separator";
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TransferList } from "@/components/transfer-list";
import { UploadManager } from "@/components/upload-manager";
import { Button } from "@/components/ui/button";
import { FormEvent } from "react";
import { redirect, useRouter } from "next/navigation";
import { signOut } from "@/signout";
import { FileList } from "@/components/file-list";
import { useDriveStore } from "@/lib/driveStore";

export default function DriveHomePage() {
  const router = useRouter();

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className="flex h-16 shrink-0 items-center gap-2 justify-between border-b px-4">
          <Input placeholder="Search" />
          <Separator
            orientation="vertical"
            className="mr- data-[orientation=vertical]:h-4 mx-5"
          />
          <div
            className="flex flex-col justify-center items-end cursor-pointer"
            onClick={async () => {
              await signOut();
              console.log('signing out')
              window.location.href = '/signin';
            }}
          >
            <Label className="font-normal text-xs">john.novak</Label>
            <Label className="font-normal text-zinc-400 text-xs">
              john_novak@example.com
            </Label>
          </div>
        </header>
        <div className="flex h-10 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator
            orientation="vertical"
            className="mr-2 data-[orientation=vertical]:h-4"
          />
          <Breadcrumb>
            <BreadcrumbList>
              {useDriveStore(s => s.breadcrumbs).map((folder, index, array) => {
                const isLast = index === array.length - 1;

                return (
                  <div key={folder.nodeId} className="flex items-center">
                    <BreadcrumbItem className="hidden md:block">
                      {isLast ? (
                        <BreadcrumbPage>{folder.name}</BreadcrumbPage>
                      ) : (
                        <BreadcrumbLink
                          href="#"
                          onClick={(e) => {
                              e.preventDefault();
                              useDriveStore.getState().jumpToFolder(index);
                          }}
                        >
                          {folder.name}
                        </BreadcrumbLink>
                      )}
                    </BreadcrumbItem>

                    {/* Add a separator after every item EXCEPT the last one */}
                    {!isLast && <BreadcrumbSeparator className="hidden md:block ml-2" />}
                  </div>
                );
              })}
            </BreadcrumbList>
          </Breadcrumb>
        </div>
        <div className="flex flex-1 flex-col gap-4 p-4">
          <FileList />
          {/*<div className="bg-muted/50 min-h-[100vh] flex-1 rounded-xl md:min-h-min">*/}
          {/*</div>*/}
          <UploadManager />
          <TransferList />
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}
