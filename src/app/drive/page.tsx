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
import { useDriveStore, FolderKey } from "@/lib/driveStore";
import { getSodium } from "@/lib/crypto/sodium";
import { customFetch } from "@/lib/api";
import { SearchBar } from "@/components/search-bar";

export default function DriveHomePage() {
  const router = useRouter();

  const handleBreadcrumbDrop = async (e: any, targetFolder: FolderKey) => {
      e.preventDefault();
      const draggedItem = useDriveStore.getState().draggedItem;
      if (!draggedItem || draggedItem.nodeId === targetFolder.nodeId) return;

      try {
          const currentFolder = useDriveStore.getState().getCurrentFolder();
          if (!currentFolder) return;
          const sodium = await getSodium();

          // 1. Unwrap the moving item's Passphrase using CURRENT folder
          const fileKey = sodium.crypto_box_seal_open(
              sodium.from_base64(draggedItem.encryptedNodePassphrase),
              currentFolder.publicKey, currentFolder.privateKey
          );

          // 2. We ALREADY HAVE the target folder's keys from the Breadcrumb!
          const targetPublicKey = targetFolder.publicKey;
          const targetPrivateKey = targetFolder.privateKey;

          // 3. Re-wrap
          const newEncryptedNodePassphrase = sodium.crypto_box_seal(fileKey, targetPublicKey);

          // 4. Re-encrypt name
          const nameNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
          const newEncryptedName = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
              sodium.from_string(draggedItem.plaintextName), null, null, nameNonce, targetPrivateKey
          );

          // 5. Send API
          const res = await customFetch("https://localhost:3100/v1/files/move", {
              method: "PATCH",
              body: JSON.stringify({
                  nodeId: draggedItem.nodeId,
                  oldParentFolderId: currentFolder.nodeId,
                  newParentFolderId: targetFolder.nodeId,
                  newEncryptedName: sodium.to_base64(newEncryptedName),
                  newNameNonce: sodium.to_base64(nameNonce),
                  newEncryptedNodePassphrase: sodium.to_base64(newEncryptedNodePassphrase),
                  newSignedEncryptedNodePassphrase: "TODO"
              })
          });

          if (res.ok) {
              useDriveStore.getState().setDraggedItem(null);
              window.dispatchEvent(new Event('refreshFiles'));
          } else {
              alert("Failed to move item.");
          }
      } catch (err) {
          console.error("Failed to move via breadcrumb drop", err);
      }
  };

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className="flex h-16 shrink-0 items-center gap-2 justify-between border-b px-4">
          <SearchBar />
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
                    <BreadcrumbItem
                        className="hidden md:block transition-all"
                        onDragOver={(e) => {
                            const draggedItem = useDriveStore.getState().draggedItem;
                            if (draggedItem && draggedItem.nodeId !== folder.nodeId) {
                                e.preventDefault();
                                e.dataTransfer.dropEffect = "move";
                                e.currentTarget.classList.add("bg-blue-100", "dark:bg-blue-900/40", "rounded", "px-2", "py-1");
                            }
                        }}
                        onDragLeave={(e) => {
                            e.currentTarget.classList.remove("bg-blue-100", "dark:bg-blue-900/40", "rounded", "px-2", "py-1");
                        }}
                        onDrop={(e) => {
                            e.currentTarget.classList.remove("bg-blue-100", "dark:bg-blue-900/40", "rounded", "px-2", "py-1");
                            handleBreadcrumbDrop(e, folder);
                        }}
                    >
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
