"use client";

import * as React from "react"
import { ChevronRight, File, Folder, LucideMonitorSmartphone, Users, Trash2 } from "lucide-react"

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarRail,
} from "@/components/ui/sidebar"
import { useRouter } from "next/navigation";
import { JSX, useState, useEffect } from "react";
import { NewDriveItemButton } from "@/components/new-drive-item-button";

import { useDriveStore, FolderKey } from "@/lib/driveStore";
import { fetchFiles } from "@/crypto/files";
import { getSodium } from "@/lib/crypto/sodium";
import { routerServerGlobal } from "next/dist/server/lib/router-utils/router-server-context";
import Router from "next/router";
import { customFetch } from "@/lib/api";
import { formatBytes } from "@/lib/utils/size";
import { getAccessToken, getAccountEncryptionPrivateKey } from "@/lib/authStore";
import Link from 'next/link'

function StorageQuota() {
    const [used, setUsed] = useState(0);
    const [max, setMax] = useState(15 * 1024 * 1024 * 1024);

    useEffect(() => {
        async function fetchQuota() {
            try {
                // In production, wire this to use your customFetch with DPoP!
                const res = await customFetch("https://localhost:3100/v1/files/quota");
                if (res.ok) {
                    const data = await res.json();
                    setUsed(data.usedBytes);
                    setMax(data.maxBytes);
                }
            } catch (e) {
                console.error("Failed to fetch quota:", e);
            }
        }
        fetchQuota();
    }, []);

    const percentage = Math.min(100, Math.round((used / max) * 100));

    return (
        <SidebarGroup className="mt-auto mb-2">
            <SidebarGroupLabel>Storage</SidebarGroupLabel>
            <SidebarGroupContent className="px-4 py-2">
                <div className="w-full bg-gray-200 dark:bg-gray-800 rounded-full h-2 mb-2 overflow-hidden">
                    <div
                        className="bg-blue-600 h-2 rounded-full transition-all duration-1000 ease-out"
                        style={{ width: `${percentage}%` }}
                    />
                </div>
                <div className="text-xs text-gray-500 flex justify-between font-medium">
                    <span>{formatBytes(used)} used</span>
                    <span>{formatBytes(max)} total</span>
                </div>
            </SidebarGroupContent>
        </SidebarGroup>
    );
}

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const router = useRouter();

  // Grab the Root Folder from the store (it's persistent even if we navigate to shared folders)
  const rootFolder = useDriveStore(s => s.myDriveRoot);

  return (
    <Sidebar {...props}>
      <SidebarContent>
        <SidebarGroup className="mb-0 gap-5">
          <a href="#" className="flex gap-2 font-semibold px-2 pt-2" onClick={(e) => {
            e.preventDefault();
            router.push('/drive')
          }}>
            <div className="bg-primary text-primary-foreground flex size-6 items-center justify-center rounded-md">
              <Folder className="size-4" />
            </div>
            Quartz Drive
          </a>
          <NewDriveItemButton />
        </SidebarGroup>

        <SidebarGroup>
          <SidebarGroupLabel>Cloud</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenu>
                {rootFolder && <DynamicFolderTree pathStack={[rootFolder]} />}
                <SharedVolumesTree />
              </SidebarMenu>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup>
          <SidebarGroupLabel>Other</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <StaticTree item={"Devices"} icon={<LucideMonitorSmartphone />} />
              <StaticTree item={"Trash"} icon={<Trash2 />} />
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <StorageQuota />

      </SidebarContent>
      <SidebarRail />
    </Sidebar>
  )
}

export default AppSidebar;

// --- DYNAMIC FOLDER TREE COMPONENT ---
function DynamicFolderTree({ pathStack }: { pathStack: FolderKey[] }) {
    const router = useRouter()
  const folder = pathStack[pathStack.length - 1]; // The current folder we are rendering
  const [children, setChildren] = useState<FolderKey[]>([]);
  const [isOpen, setIsOpen] = useState(false);
  const [hasFetched, setHasFetched] = useState(false);

  // Lazy-load and decrypt children ONLY when the folder is expanded!
  useEffect(() => {
    if (isOpen && !hasFetched) {
      async function loadFolders() {
        const rawFiles = await fetchFiles(folder.nodeId);
        if (!rawFiles) {
           setHasFetched(true);
           return;
        }

        const sodium = await getSodium();
        const subFolders: FolderKey[] = [];

        for (const file of rawFiles) {
          if (file.type !== 'FOLDER') continue; // Only show folders in the sidebar tree!

          try {
              // 1. Decrypt Name
              const nameBytes = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                  null, sodium.from_base64(file.encryptedName), null,
                  sodium.from_base64(file.nameNonce), folder.privateKey
              );

              // 2. Decrypt Node Passphrase
              const nodePassphrase = sodium.crypto_box_seal_open(
                  sodium.from_base64(file.encryptedNodePassphrase),
                  folder.publicKey, folder.privateKey
              );

              // 3. Decrypt Node Private Key
              const childPrivKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                  null, sodium.from_base64(file.wrappedNodeKey),
                  sodium.from_string("FolderNode"), sodium.from_base64(file.nodePrivNonce),
                  nodePassphrase
              );

              subFolders.push({
                  nodeId: file.nodeId,
                  name: sodium.to_string(nameBytes),
                  privateKey: childPrivKey,
                  publicKey: sodium.from_base64(file.nodePublicKey),
                  hasChildren: file.hasChildren
              });
          } catch (e) {
              console.error("Failed to decrypt sidebar folder", e);
          }
        }

        setChildren(subFolders);
        setHasFetched(true);
      }
      loadFolders();

      const handleRefresh = () => {
          setHasFetched(false); // Force a re-fetch of the tree node
      };
      window.addEventListener('refreshFiles', handleRefresh);
      return () => window.removeEventListener('refreshFiles', handleRefresh);
    }
  }, [isOpen, hasFetched, folder]);

  const handleNavigate = () => {
     // Magic! Overwrite the entire breadcrumb stack so the main view instantly updates
     // and your breadcrumb component at the top of the page perfectly reflects the new path!
     useDriveStore.setState({ breadcrumbs: pathStack });
    setIsOpen(true); // Automatically expand when clicked!
    router.push("/drive");
  };

  return (
    <SidebarMenuItem>
      <Collapsible
        className="group/collapsible [&[data-state=open]>div>button>svg]:rotate-90"
        open={isOpen}
        onOpenChange={setIsOpen}
      >
        {/* We use asChild so the wrapper div gets the beautiful SidebarMenuButton hover styling */}
        <SidebarMenuButton asChild>
            <div className="flex items-center w-full cursor-pointer" onClick={handleNavigate}>
                {/* Show Arrow if we explicitly know it has children, or if we haven't fetched and it's undefined (root) */}
                {folder.hasChildren !== false ? (
                    <CollapsibleTrigger asChild>
                        <button
                            onClick={(e) => e.stopPropagation()}
                            className="p-1 -ml-1 mr-1 hover:bg-black/5 dark:hover:bg-white/10 rounded-sm transition-colors text-gray-500"
                        >
                            <ChevronRight className="w-4 h-4 transition-transform" />
                        </button>
                    </CollapsibleTrigger>
                ) : (
                    <div className="w-6 h-6 mr-1 -ml-1" /> /* Spacer to keep alignment for empty folders */
                )}

                {/* The Folder Icon and Name */}
                <Folder className="w-4 h-4 mr-2" />
                <span className="flex-1 truncate">{folder.name}</span>
            </div>
        </SidebarMenuButton>

        <CollapsibleContent>
          {children.length > 0 && (
            <SidebarMenuSub className="ml-4 border-l pl-2">
              {children.map((childFolder) => (
                <DynamicFolderTree key={childFolder.nodeId} pathStack={[...pathStack, childFolder]} />
              ))}
            </SidebarMenuSub>
          )}
        </CollapsibleContent>
      </Collapsible>
    </SidebarMenuItem>
  )
}

// --- SHARED VOLUMES TREE ---
function SharedVolumesTree() {
    const [volumes, setVolumes] = useState<FolderKey[]>([]);
    const [isOpen, setIsOpen] = useState(false);
    const [hasFetched, setHasFetched] = useState(false);

    useEffect(() => {
        if (isOpen && !hasFetched) {
            async function fetchShared() {
                try {
                    const res = await customFetch("https://localhost:3100/v1/files/shared", { method: "GET" });
                    if (!res.ok) throw new Error("Failed to fetch shared volumes");
                    const rawVolumes = await res.json();
                    if (!rawVolumes) {
                        setHasFetched(true);
                        return;
                    }

                    const sodium = await getSodium();
                    const accountPrivKey = getAccountEncryptionPrivateKey();
                    if (!accountPrivKey) throw new Error("Not authenticated");
                    const accountPubKey = sodium.crypto_scalarmult_base(accountPrivKey);

                    const token = getAccessToken();
                    const userEmail = JSON.parse(atob(token!.split('.')[1])).email;

                    const decryptedVolumes: FolderKey[] = [];

                    for (const vol of rawVolumes) {
                        try {
                            const sharePassphrase = sodium.crypto_box_seal_open(
                                sodium.from_base64(vol.encryptedSharePassphraseForOwner),
                                accountPubKey, accountPrivKey
                            );

                            const sharePrivKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                                null, sodium.from_base64(vol.wrappedSharePrivateKey),
                                sodium.from_string("SharedVolume:" + userEmail),
                                sodium.from_base64(vol.sharePrivNonce), sharePassphrase
                            );
                            const sharePubKey = sodium.crypto_scalarmult_base(sharePrivKey);

                            const rootNodePassphrase = sodium.crypto_box_seal_open(
                                sodium.from_base64(vol.encryptedRootNodePassphrase),
                                sharePubKey, sharePrivKey
                            );

                            const rootNodePrivKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                                null, sodium.from_base64(vol.wrappedNodePrivateKey),
                                sodium.from_string("FolderNode"), sodium.from_base64(vol.nodePrivNonce),
                                rootNodePassphrase
                            );

                            const decryptedNameBytes = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                                null, sodium.from_base64(vol.encryptedName),
                                null, sodium.from_base64(vol.nameNonce), sharePrivKey
                            );

                            decryptedVolumes.push({
                                nodeId: vol.nodeId,
                                name: sodium.to_string(decryptedNameBytes),
                                privateKey: rootNodePrivKey,
                                publicKey: sodium.from_base64(vol.nodePublicKey),
                              hasChildren: undefined,
                              isShared: true
                            });
                        } catch (e) {
                            console.error("Failed to decrypt a shared volume", e);
                        }
                    }
                    setVolumes(decryptedVolumes);
                    setHasFetched(true);
                } catch (err) {
                    console.error(err);
                }
            }
            fetchShared();
        }
    }, [isOpen, hasFetched]);

    return (
        <SidebarMenuItem>
            <Collapsible
                className="group/collapsible [&[data-state=open]>div>button>svg]:rotate-90"
                open={isOpen}
                onOpenChange={setIsOpen}
            >
                <SidebarMenuButton asChild>
                    <div className="flex items-center w-full">
                        <CollapsibleTrigger asChild>
                            <button
                  onClick={(e) => {
                    e.stopPropagation();
                    setIsOpen(!isOpen);
                  }}
                                className="p-1 -ml-1 mr-1 hover:bg-black/5 dark:hover:bg-white/10 rounded-sm transition-colors text-gray-500"
                            >
                                <ChevronRight className="w-4 h-4 transition-transform" />
                            </button>
                        </CollapsibleTrigger>
                        <Link href="/drive/shared" className="flex flex-1 items-center cursor-pointer">
                          <Users className="w-4 h-4 mr-2" />
                          <span className="truncate">Shared with Me</span>
                        </Link>
                    </div>
                </SidebarMenuButton>

                <CollapsibleContent>
                    {volumes.length > 0 ? (
                        <SidebarMenuSub className="ml-4 border-l pl-2">
                            {volumes.map((vol) => (
                                <DynamicFolderTree key={vol.nodeId} pathStack={[vol]} />
                            ))}
                        </SidebarMenuSub>
                    ) : hasFetched ? (
                         <div className="pl-8 py-2 text-xs text-gray-400">No shared folders</div>
                    ) : null}
                </CollapsibleContent>
            </Collapsible>
        </SidebarMenuItem>
    );
}

// --- STATIC TREE (For Trash, Shared, etc) ---
function StaticTree({ item, icon }: { item: string, icon?: JSX.Element }) {
    const router = useRouter();
  return (
    <SidebarMenuItem>
      <SidebarMenuButton className="cursor-pointer"  onClick={() => {
        if(item == "Trash") router.push('/drive/trash')
        if(item == "Devices") router.push('/drive/devices')
      }}>
        {icon == undefined ? <File /> : icon}
        {item}
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}
