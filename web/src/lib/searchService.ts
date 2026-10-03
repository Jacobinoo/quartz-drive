import { config } from "@/config/env";
import { getSodium } from "./crypto/sodium";
import {
    saveSearchIndex,
    loadSearchIndex,
    DecryptedSearchItem,
} from "./SearchIndexStore";
import { customFetch } from "./api";
import {
    getAccountEncryptionPrivateKey,
    getAccountSigningPrivateKey,
} from "./authStore";

import { FolderKey, useDriveStore } from "@/lib/driveStore";
import { quantumSealOpen } from "@/crypto/kem";

export async function buildE2EESearchIndex(): Promise<void> {
    const res = await customFetch(`${config.apiUrl}/v1/files/all`, {
        method: "GET",
    });
    if (!res.ok) throw new Error("Failed to fetch files for search index");

    const rawFiles = await res.json();
    if (!rawFiles || rawFiles.length === 0) return;

    const sodium = await getSodium();

    // 1. Seed our dictionary with the Root Folder(s) already unlocked by your Drive UI!
    // We store BOTH the Public and Private keys because crypto_box_seal_open requires both!
    const folderKeys: Record<
        string,
        { publicKey: Uint8Array; privateKey: Uint8Array }
    > = {};

    // Grab the root folder from your Zustand store (it's the first breadcrumb)
    const breadcrumbs = useDriveStore.getState().breadcrumbs;
    if (breadcrumbs.length > 0) {
        const rootFolder = breadcrumbs[0];
        folderKeys[rootFolder.nodeId] = {
            publicKey: rootFolder.publicKey,
            privateKey: rootFolder.privateKey,
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
            if (
                f.type === "FOLDER" &&
                !folderKeys[f.nodeId] &&
                folderKeys[f.parentNodeId]
            ) {
                try {
                    const parentKeys = folderKeys[f.parentNodeId];

                    // A. Decrypt the folder's Passphrase using the Parent's Public & Private key
                    const folderPassphrase = await quantumSealOpen(
                        sodium.from_base64(f.encryptedNodePassphrase),
                        parentKeys.privateKey
                    );

                    // B. Decrypt the folder's Private Key using that Passphrase!
                    const folderPrivateKey =
                        sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                            null,
                            sodium.from_base64(f.wrappedNodeKey),
                            sodium.from_string("FolderNode"), // AAD used in folder creation
                            sodium.from_base64(f.nodePrivNonce),
                            folderPassphrase
                        );

                    if (folderPrivateKey) {
                        folderKeys[f.nodeId] = {
                            publicKey: sodium.from_base64(f.nodePublicKey),
                            privateKey: folderPrivateKey,
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
            const decryptedBytes =
                sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                    null,
                    ciphertext,
                    null,
                    nonce,
                    parentKeys.privateKey
                );

            if (decryptedBytes) {
                decryptedItems.push({
                    id: f.nodeId,
                    name: sodium.to_string(decryptedBytes),
                    type: f.type,
                    sizeBytes: f.sizeBytes,
                    createdAt: f.createdAt,
                });
            }
        } catch (e) {
            console.warn(`Could not decrypt name for ${f.nodeId}`);
        }
    }

    await saveSearchIndex(decryptedItems);
    console.log(
        `Successfully built Tree Search Index! Unlocked ${Object.keys(folderKeys).length} folders and ${decryptedItems.length} items!`
    );
}
// 4. The Lightning-Fast Filter Function
export async function searchFiles(
    query: string
): Promise<DecryptedSearchItem[]> {
    if (!query || query.trim() === "") return [];

    // Load the index from our local DB (very fast)
    const index = await loadSearchIndex();

    const lowerQuery = query.toLowerCase();

    // Simple in-memory array filter
    return index.filter((item) => item.name.toLowerCase().includes(lowerQuery));
}

export async function resolvePathAndNavigate(targetNodeId: string) {
    const res = await customFetch(
        `${config.apiUrl}/v1/files/path?nodeId=${targetNodeId}`,
        {
            method: "GET",
        }
    );
    if (!res.ok) throw new Error("Failed to fetch path");
    const pathNodes = await res.json();
    if (!pathNodes || pathNodes.length === 0) return;

    // 1. Grab the Root folder from the Drive Store
    const currentBreadcrumbs = useDriveStore.getState().breadcrumbs;
    if (currentBreadcrumbs.length === 0)
        throw new Error("Root folder not found in memory!");

    // We will build a brand new breadcrumbs array starting with the Root!
    const newBreadcrumbs: FolderKey[] = [currentBreadcrumbs[0]];
    const sodium = await getSodium();

    // 2. Loop downwards through the path returned by PostgreSQL
    // (We start at index 1 because the DB returns the Root at index 0, which we already have unlocked above!)
    for (let i = 1; i < pathNodes.length; i++) {
        const f = pathNodes[i];

        // If the target was a FILE, the last item in the path array will be that FILE.
        // We stop! We can't navigate INTO a file, we only navigate to its parent folder.
        if (f.type === "FILE") break;

        const parentKeys = newBreadcrumbs[i - 1]; // The parent we just unlocked!

        try {
            // A. Decrypt Folder Passphrase
            const folderPassphrase = await quantumSealOpen(
                sodium.from_base64(f.encryptedNodePassphrase),
                parentKeys.privateKey
            );
            // B. Decrypt Folder Private Key
            const folderPrivateKey =
                sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                    null,
                    sodium.from_base64(f.wrappedNodeKey),
                    sodium.from_string("FolderNode"),
                    sodium.from_base64(f.nodePrivNonce),
                    folderPassphrase
                );

            // C. Decrypt Folder Name
            const decryptedNameBytes =
                sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                    null,
                    sodium.from_base64(f.encryptedName),
                    null,
                    sodium.from_base64(f.nameNonce),
                    parentKeys.privateKey
                );
            if (folderPrivateKey && decryptedNameBytes) {
                newBreadcrumbs.push({
                    nodeId: f.nodeId,
                    name: sodium.to_string(decryptedNameBytes), // Store as plaintext for the UI!
                    publicKey: sodium.from_base64(f.nodePublicKey),
                    privateKey: folderPrivateKey,
                });
            } else {
                throw new Error("Decryption failed for subfolder in path");
            }
        } catch (e) {
            console.error("Path resolution failed at node", f.nodeId, e);
            break; // Stop navigating deeper if a folder is corrupted
        }
    }

    // 3. Inject the fully unlocked path into Zustand!
    useDriveStore.getState().setBreadcrumbs(newBreadcrumbs);
}
