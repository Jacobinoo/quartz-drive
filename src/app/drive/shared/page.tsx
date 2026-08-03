"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { AppSidebar } from "@/components/app-sidebar";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { customFetch } from "@/lib/api";
import { getSodium } from "@/lib/crypto/sodium";
import { getAccessToken, getAccountEncryptionPrivateKey } from "@/lib/authStore";
import { useDriveStore } from "@/lib/driveStore";
import { FolderIcon } from "lucide-react";

export default function SharedWithMePage() {
    const [volumes, setVolumes] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const router = useRouter();

    useEffect(() => {
        async function fetchShared() {
            try {
                const res = await customFetch("https://localhost:3100/v1/files/shared", { method: "GET" });
                if (!res.ok) throw new Error("Failed to fetch");
                const rawVolumes = await res.json();
                if (!rawVolumes) return setVolumes([]);

                const sodium = await getSodium();

                // 1. Get Account Keys
                const accountPrivKey = getAccountEncryptionPrivateKey();
                if (!accountPrivKey) throw new Error("Not authenticated");
                const accountPubKey = sodium.crypto_scalarmult_base(accountPrivKey);

                // 2. Decode user email for AAD verification
                const token = getAccessToken();
                const userEmail = JSON.parse(atob(token!.split('.')[1])).email;

                const decryptedVolumes = [];

                for (const vol of rawVolumes) {
                    try {
                        // LAYER 1: Unwrap Share Passphrase
                        const sharePassphrase = sodium.crypto_box_seal_open(
                            sodium.from_base64(vol.encryptedSharePassphraseForOwner),
                            accountPubKey,
                            accountPrivKey
                        );

                        // LAYER 2: Decrypt Share Private Key
                        const sharePrivKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                            null, sodium.from_base64(vol.wrappedSharePrivateKey),
                            sodium.from_string("SharedVolume:" + userEmail), // AAD MATCHES SHARE SERVICE!
                            sodium.from_base64(vol.sharePrivNonce),
                            sharePassphrase
                        );
                        const sharePubKey = sodium.crypto_scalarmult_base(sharePrivKey);

                        // LAYER 3: Unwrap Target Folder Passphrase
                        const rootNodePassphrase = sodium.crypto_box_seal_open(
                            sodium.from_base64(vol.encryptedRootNodePassphrase),
                            sharePubKey,
                            sharePrivKey
                        );

                        // LAYER 4: Decrypt Target Folder Private Key
                        const rootNodePrivKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                            null, sodium.from_base64(vol.wrappedNodePrivateKey),
                            sodium.from_string("FolderNode"),
                            sodium.from_base64(vol.nodePrivNonce),
                            rootNodePassphrase
                        );

                        // BONUS: Decrypt the Name!
                        const decryptedNameBytes = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                            null, sodium.from_base64(vol.encryptedName),
                            null, sodium.from_base64(vol.nameNonce),
                            sharePrivKey // The name was encrypted with the Share Priv Key in shareService.ts
                        );

                        decryptedVolumes.push({
                            nodeId: vol.nodeId,
                            name: sodium.to_string(decryptedNameBytes),
                            privateKey: rootNodePrivKey,
                            publicKey: sodium.from_base64(vol.nodePublicKey),
                            isShared: true
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
        <SidebarProvider>
            <AppSidebar />
            <SidebarInset>
                <header className="flex h-16 items-center border-b px-4">
                    <SidebarTrigger />
                    <h1 className="ml-4 font-semibold">Shared with Me</h1>
                </header>
                <div className="p-8">
                    {loading ? (
                        <p className="text-gray-500">Decrypting shared volumes...</p>
                    ) : volumes.length === 0 ? (
                        <div className="text-center text-gray-500 mt-20">
                            <FolderIcon className="w-16 h-16 mx-auto mb-4 opacity-20" />
                            No folders have been shared with you yet.
                        </div>
                    ) : (
                        <div className="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-4 gap-4">
                            {volumes.map((vol) => (
                                <div
                                    key={vol.nodeId}
                                    onClick={() => openVolume(vol)}
                                    className="p-4 border rounded-xl flex items-center cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-900 transition-colors shadow-sm"
                                >
                                    <FolderIcon className="w-10 h-10 text-blue-500 fill-blue-500/20 mr-4" />
                                    <span className="font-medium truncate">{vol.name}</span>
                                </div>
                            ))}
                        </div>
                    )}
                </div>
            </SidebarInset>
        </SidebarProvider>
    );
}
