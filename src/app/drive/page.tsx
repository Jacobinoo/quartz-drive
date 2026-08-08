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
import { SidebarTrigger } from "@/components/ui/sidebar";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TransferList } from "@/components/transfer-list";
import { UploadManager } from "@/components/upload-manager";
import { Button } from "@/components/ui/button";
import { FormEvent, useEffect, useState } from "react";
import { redirect, useRouter } from "next/navigation";
import { FileList } from "@/components/file-list";
import { NewDriveItemButton } from "@/components/new-drive-item-button";
import { FolderKey, useDriveStore } from "@/lib/driveStore";
import { getAccountSigningPrivateKey } from "@/lib/authStore";
import { getSodium } from "@/lib/crypto/sodium";
import { customFetch } from "@/lib/api";
import { SearchBar } from "@/components/search-bar";
import { getAccessToken } from "@/lib/authStore";
import { Users } from "lucide-react";

export default function DriveHomePage() {
  const router = useRouter();

  const [userEmail, setUserEmail] = useState("Loading...");

  useEffect(() => {
        const token = getAccessToken();
        if (token) {
            try {
                const payload = JSON.parse(atob(token.split('.')[1]));
                if (payload.email) setUserEmail(payload.email);
            } catch (e) {
                console.warn("Failed to decode token");
            }
        }
    }, []);

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

          const accountSigningPrivKey = getAccountSigningPrivateKey();
          if (!accountSigningPrivKey) throw new Error("Missing signing key");
          const signature = sodium.crypto_sign_detached(newEncryptedNodePassphrase, accountSigningPrivKey);
          const newSignedEncryptedNodePassphrase = sodium.to_base64(signature);

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
                  newSignedEncryptedNodePassphrase: newSignedEncryptedNodePassphrase
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
      <>
        <div className="flex h-10 shrink-0 items-center gap-2 border-b px-4 bg-background">
          <SidebarTrigger className="hidden max-md:block -ml-1" />
          <Separator
            orientation="vertical"
            className="mr-2 data-[orientation=vertical]:h-4 hidden max-md:block"
          />
          <Breadcrumb>
            <BreadcrumbList>

              {useDriveStore(s => s.breadcrumbs)[0]?.isShared && (
                  <>
                      <BreadcrumbItem className="hidden md:block cursor-pointer" onClick={() => router.push('/drive/shared')}>
                          <BreadcrumbPage className="text-gray-500 flex items-center">
                              <Users className="w-4 h-4 mr-2" /> Shared with Me
                          </BreadcrumbPage>
                      </BreadcrumbItem>
                      <BreadcrumbSeparator className="hidden md:block" />
                  </>
              )}

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
      </>
  );
}
