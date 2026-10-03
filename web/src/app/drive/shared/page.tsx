"use client";

import { config } from "@/config/env";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { AppSidebar } from "@/components/app-sidebar";
import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { customFetch } from "@/lib/api";
import { getSodium } from "@/lib/crypto/sodium";
import {
    getAccessToken,
    getAccountEncryptionPrivateKey,
} from "@/lib/authStore";
import { useDriveStore } from "@/lib/driveStore";
import { FolderIcon } from "lucide-react";
import { quantumSealOpen } from "@/crypto/kem";

export default function SharedWithMePage() {
    const [volumes, setVolumes] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const router = useRouter();

    useEffect(() => {
        async function fetchShared() {
            try {
                const res = await customFetch(
                    `${config.apiUrl}/v1/files/shared`,
                    { method: "GET" }
                );
                if (!res.ok) throw new Error("Failed to fetch");
                const rawVolumes = await res.json();
                if (!rawVolumes) return setVolumes([]);

                const sodium = await getSodium();

                // 1. Get Account Keys
                const accountPrivKey = getAccountEncryptionPrivateKey();
                if (!accountPrivKey) throw new Error("Not authenticated");
                const accountPubKey =
                    sodium.crypto_scalarmult_base(accountPrivKey);

                // 2. Decode user email for AAD verification
                const token = getAccessToken();
                const userEmail = JSON.parse(atob(token!.split(".")[1])).email;

                const decryptedVolumes = [];

                for (const vol of rawVolumes) {
                    try {
                        // LAYER 1: Unwrap Share Passphrase
                        const sharePassphrase = await quantumSealOpen(
                            sodium.from_base64(
                                vol.encryptedSharePassphraseForOwner
                            ),
                            accountPrivKey
                        );

                        // LAYER 2: Decrypt Share Private Key
                        const sharePrivKey =
                            sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                                null,
                                sodium.from_base64(vol.wrappedSharePrivateKey),
                                sodium.from_string("SharedVolume:" + userEmail), // AAD MATCHES SHARE SERVICE!
                                sodium.from_base64(vol.sharePrivNonce),
                                sharePassphrase
                            );
                        const sharePubKey =
                            sodium.crypto_scalarmult_base(sharePrivKey);

                        // LAYER 3: Unwrap Target Folder Passphrase
                        const rootNodePassphrase = await quantumSealOpen(
                            sodium.from_base64(vol.encryptedRootNodePassphrase),
                            sharePrivKey
                        );

                        // LAYER 4: Decrypt Target Folder Private Key
                        const rootNodePrivKey =
                            sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                                null,
                                sodium.from_base64(vol.wrappedNodePrivateKey),
                                sodium.from_string("FolderNode"),
                                sodium.from_base64(vol.nodePrivNonce),
                                rootNodePassphrase
                            );

                        // BONUS: Decrypt the Name!
                        const decryptedNameBytes =
                            sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                                null,
                                sodium.from_base64(vol.encryptedName),
                                null,
                                sodium.from_base64(vol.nameNonce),
                                sharePrivKey // The name was encrypted with the Share Priv Key in shareService.ts
                            );

                        decryptedVolumes.push({
                            nodeId: vol.nodeId,
                            name: sodium.to_string(decryptedNameBytes),
                            privateKey: rootNodePrivKey,
                            publicKey: sodium.from_base64(vol.nodePublicKey),
                            isShared: true,
                        });
                    } catch (e) {
                        console.error("Failed to decrypt a shared volume", e);
                    }
                }

                setVolumes(decryptedVolumes);
            } catch (err) {
                console.error(err);
            } finally {
                setLoading(false);
            }
        }
        fetchShared();
    }, []);

    const openVolume = (volume: any) => {
        // Inject the folder's root keys into the Drive Store!
        useDriveStore.getState().setBreadcrumbs([volume]);
        // Route to the standard drive page which will render the files!
        router.push("/drive");
    };

    return (
        <div className="flex flex-col h-full">
            <header className="flex h-16 items-center border-b px-4">
                <SidebarTrigger />
                <h1 className="ml-4 font-semibold">Shared with Me</h1>
            </header>

            <div className="flex flex-col w-full p-4">
                {loading ? (
                    <div className="flex flex-col items-center justify-center py-24 text-muted-foreground gap-3">
                        <p className="text-sm">Decrypting shared volumes...</p>
                    </div>
                ) : volumes.length === 0 ? (
                    <div className="flex flex-col items-center justify-center py-24 text-muted-foreground gap-3">
                        <FolderIcon className="w-10 h-10 opacity-20" />
                        <span className="text-sm">
                            No folders have been shared with you yet.
                        </span>
                    </div>
                ) : (
                    <>
                        {/* Column header */}
                        <div className="grid grid-cols-[minmax(0,1fr)] items-center px-3 py-2 border-b border-border/60">
                            <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                                Name
                            </span>
                        </div>

                        <div className="flex flex-col">
                            {volumes.map((vol) => (
                                <div
                                    key={vol.nodeId}
                                    onClick={() => openVolume(vol)}
                                    className="group grid grid-cols-[minmax(0,1fr)] items-center px-3 py-1.5 rounded-md transition-colors cursor-pointer hover:bg-accent/60"
                                >
                                    <div className="flex items-center gap-2 min-w-0">
                                        <FolderIcon className="w-4 h-4 shrink-0 text-blue-500 fill-blue-500/20" />
                                        <span className="text-sm font-medium truncate">
                                            {vol.name}
                                        </span>
                                    </div>
                                </div>
                            ))}
                        </div>
                    </>
                )}
            </div>
        </div>
    );
}
