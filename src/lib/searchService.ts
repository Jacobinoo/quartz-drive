import { getSodium } from "./crypto/sodium";
import { saveSearchIndex, loadSearchIndex, DecryptedSearchItem } from "./SearchIndexStore";
import { customFetch } from "./api";
import { getAccountEncryptionPrivateKey, getAccountSigningPrivateKey } from "./authStore";

import { useDriveStore } from "@/lib/driveStore";

export async function buildE2EESearchIndex(): Promise<void> {
    const res = await customFetch("https://localhost:3100/v1/files/all", {
        method: "GET",
    });
    if (!res.ok) throw new Error("Failed to fetch files for search index");

    const rawFiles = await res.json();
    if (!rawFiles || rawFiles.length === 0) return;

    const sodium = await getSodium();

    // 1. Seed our dictionary with the Root Folder(s) already unlocked by your Drive UI!
    // We store BOTH the Public and Private keys because crypto_box_seal_open requires both!
    const folderKeys: Record<string, { publicKey: Uint8Array, privateKey: Uint8Array }> = {};

    // Grab the root folder from your Zustand store (it's the first breadcrumb)
    const breadcrumbs = useDriveStore.getState().breadcrumbs;
    if (breadcrumbs.length > 0) {
        const rootFolder = breadcrumbs[0];
        folderKeys[rootFolder.nodeId] = {
            publicKey: rootFolder.publicKey,
            privateKey: rootFolder.privateKey
        };
    } else {
        throw new Error("Root folder keys not found in memory!");
    }

    // 2. Tree Walk: Unlock all sub-folders using their Parent's unlocked keys
    let keysAdded = true;
    while (keysAdded) {
        keysAdded = false;
        for (const f of rawFiles) {
            // If it's a folder, we haven't unlocked it yet, and we HAVE unlocked its parent
            if (f.type === "FOLDER" && !folderKeys[f.nodeId] && folderKeys[f.parentNodeId]) {
                try {
                    const parentKeys = folderKeys[f.parentNodeId];

                    // A. Decrypt the folder's Passphrase using the Parent's Public & Private key
                    const folderPassphrase = sodium.crypto_box_seal_open(
                        sodium.from_base64(f.encryptedNodePassphrase),
                        parentKeys.publicKey,
                        parentKeys.privateKey
                    );

                    // B. Decrypt the folder's Private Key using that Passphrase!
                    const folderPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                        null, sodium.from_base64(f.wrappedNodeKey),
                        sodium.from_string("FolderNode"), // AAD used in folder creation
                        sodium.from_base64(f.nodePrivNonce),
                        folderPassphrase
                    );

                    if (folderPrivateKey) {
                        folderKeys[f.nodeId] = {
                            publicKey: sodium.from_base64(f.nodePublicKey),
                            privateKey: folderPrivateKey
                        };
                        keysAdded = true; // We unlocked a new level of the tree, keep looping!
                    }
                } catch (e) {
                    console.warn(`Could not unlock subfolder ${f.nodeId}`);
                }
            }
        }
    }

    // 3. Finally, decrypt all File Names (and Folder Names) using their Parent's Unlocked Key!
    const decryptedItems: DecryptedSearchItem[] = [];

    for (const f of rawFiles) {
        const parentKeys = folderKeys[f.parentNodeId];
        if (!parentKeys) continue; // Skip if parent folder is permanently locked

        try {
            const ciphertext = sodium.from_base64(f.encryptedName);
            const nonce = sodium.from_base64(f.nameNonce);

            // Decrypt the name using the Parent Folder's Private Key!
            const decryptedBytes = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                null, ciphertext, null, nonce, parentKeys.privateKey
            );

            if (decryptedBytes) {
                decryptedItems.push({
                    id: f.nodeId,
                    name: sodium.to_string(decryptedBytes),
                    type: f.type,
                    sizeBytes: f.sizeBytes,
                    createdAt: f.createdAt
                });
            }
        } catch (e) {
            console.warn(`Could not decrypt name for ${f.nodeId}`);
        }
    }

    await saveSearchIndex(decryptedItems);
    console.log(`Successfully built Tree Search Index! Unlocked ${Object.keys(folderKeys).length} folders and ${decryptedItems.length} items!`);
}
// 4. The Lightning-Fast Filter Function
export async function searchFiles(query: string): Promise<DecryptedSearchItem[]> {
    if (!query || query.trim() === "") return [];

    // Load the index from our local DB (very fast)
    const index = await loadSearchIndex();

    const lowerQuery = query.toLowerCase();

    // Simple in-memory array filter
    return index.filter(item => item.name.toLowerCase().includes(lowerQuery));
}
