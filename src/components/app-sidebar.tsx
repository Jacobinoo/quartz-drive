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

    const formatBytes = (bytes: number) => {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    };

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

  // Grab the Root Folder from the store (it's always the first item in the breadcrumbs)
  const rootFolder = useDriveStore(s => s.breadcrumbs.length > 0 ? s.breadcrumbs[0] : null);

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
          <SidebarGroupLabel>My Files</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {/* Kick off the recursive tree starting at the Root Folder! */}
              {rootFolder && <DynamicFolderTree pathStack={[rootFolder]} />}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup>
          <SidebarGroupLabel>Other</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <StaticTree item={"Devices"} icon={<LucideMonitorSmartphone />} />
              <StaticTree item={"Shared With Me"} icon={<Users />} />
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
    }
  }, [isOpen, hasFetched, folder]);

  const handleNavigate = () => {
     // Magic! Overwrite the entire breadcrumb stack so the main view instantly updates
     // and your breadcrumb component at the top of the page perfectly reflects the new path!
     useDriveStore.setState({ breadcrumbs: pathStack });
     setIsOpen(true); // Automatically expand when clicked!
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

// --- STATIC TREE (For Trash, Shared, etc) ---
function StaticTree({ item, icon }: { item: string, icon?: JSX.Element }) {
    const router = useRouter();
  return (
    <SidebarMenuItem>
      <SidebarMenuButton onClick={() => {
        if(item == "Trash") router.push('/drive/trash')
      }}>
        {icon == undefined ? <File /> : icon}
        {item}
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}
