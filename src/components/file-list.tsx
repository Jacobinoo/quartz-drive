"use client";

import { useEffect, useState } from "react";
import { fetchFiles, getDownloadUrls } from "@/crypto/files";
import { getSodium } from "@/lib/crypto/sodium";
import { useDriveStore } from "@/lib/driveStore";
import { FileIcon, FolderIcon, MoreVertical, Pencil, Trash2 } from "lucide-react";
import { customFetch } from "@/lib/api"; // Added for our direct API calls
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { initializeDriveKeys } from "@/crypto/drive";

export function FileList() {
    const currentFolder = useDriveStore(s => s.getCurrentFolder());
  const [files, setFiles] = useState<any[]>([]);

      useEffect(() => {
          initializeDriveKeys().catch(console.error);
      }, []);

    useEffect(() => {
        if (!currentFolder) return;
        async function loadAndDecrypt() {
            try {
                const rawFiles = await fetchFiles(currentFolder!.nodeId);
                if (!rawFiles) {
                    setFiles([]);
                    return;
                }

                const sodium = await getSodium();
                const decryptedFiles = rawFiles.map((file: any) => {
                    try {
                        const ciphertext = sodium.from_base64(file.encryptedName);
                        const nonce = sodium.from_base64(file.nameNonce);
                        const decryptedBytes = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                            null, ciphertext, null, nonce, currentFolder!.privateKey
                        );
                        return { ...file, plaintextName: sodium.to_string(decryptedBytes) };
                    } catch (e) {
                        return { ...file, plaintextName: "Decryption Failed" };
                    }
                });
                setFiles(decryptedFiles);
            } catch (err) {
                console.error(err);
            }
        }
        loadAndDecrypt();
    }, [currentFolder]);

    // --- NEW: Rename Handler ---
    const handleRename = async (file: any) => {
        try {
            const newName = window.prompt("Enter new name:", file.plaintextName);
            if (!newName || newName === file.plaintextName) return;

            const sodium = await getSodium();
            if (!currentFolder) return;

            // 1. Generate new crypto payload
            const nameNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
            const encryptedName = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
                sodium.from_string(newName),
                null, null, nameNonce, currentFolder.privateKey
            );

            // 2. Send PATCH to Go Backend
            const res = await customFetch(`https://localhost:3100/v1/files/rename?nodeId=${file.nodeId}&parentFolderId=${currentFolder.nodeId}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    encryptedName: sodium.to_base64(encryptedName),
                    nameNonce: sodium.to_base64(nameNonce)
                })
            });

            // 3. Instantly update UI on success
            if (res.ok) {
                setFiles(files => files.map(f => f.nodeId === file.nodeId ? { ...f, plaintextName: newName } : f));
            }
        } catch (e) {
            console.error("Rename failed:", e);
        }
    };

    // --- NEW: Trash Handler ---
    const handleTrash = async (file: any) => {
        try {
            if (!currentFolder) return;
            const res = await customFetch(`https://localhost:3100/v1/files/trash?nodeId=${file.nodeId}&parentFolderId=${currentFolder.nodeId}`, {
                method: 'DELETE'
            });

            if (res.ok) {
                // Instantly wipe it from the UI!
                setFiles(files => files.filter(f => f.nodeId !== file.nodeId));
            }
        } catch (e) {
            console.error("Trash failed:", e);
        }
    };

    return (
        <div className="mt-8">
            <h2 className="text-xl font-bold mb-4">{currentFolder?.name || "My Files"}</h2>

            {files.length === 0 ? (
                <div className="p-8 text-center text-gray-500 border border-dashed rounded">
                    This folder is empty.
                </div>
            ) : (
                <div className="flex flex-col gap-2">
                    {files.map(f => (
                        <div key={f.nodeId} className="p-4 border rounded shadow flex justify-between items-center group">

                            {/* Left Side: Icon & Name */}
                            <div className="flex items-center cursor-pointer" onClick={() => f.type === 'FOLDER' ? handleFolderClick(f) : null}>
                                {f.type === 'FOLDER' ? <FolderIcon className="mr-3 text-blue-500" /> : <FileIcon className="mr-3 text-gray-500" />}
                                <span className={f.type === 'FOLDER' ? "font-semibold hover:underline" : ""}>{f.plaintextName}</span>
                            </div>

                            {/* Right Side: Actions */}
                            <div className="flex items-center gap-2">
                                {f.type === 'FILE' && (
                                    <button onClick={() => handleDownload(f)} className="bg-blue-500 text-white px-3 py-1 rounded text-sm hover:bg-blue-600 transition-colors">
                                        Download
                                    </button>
                                )}

                                <DropdownMenu>
                                    <DropdownMenuTrigger asChild>
                                        <button className="p-2 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-md transition-colors text-gray-500">
                                            <MoreVertical className="w-4 h-4" />
                                        </button>
                                    </DropdownMenuTrigger>
                                    <DropdownMenuContent align="end">
                                        <DropdownMenuItem onClick={() => handleRename(f)} className="cursor-pointer">
                                            <Pencil className="w-4 h-4 mr-2" />
                                            Rename
                                        </DropdownMenuItem>
                                        <DropdownMenuItem onClick={() => handleTrash(f)} className="text-red-600 focus:text-red-600 focus:bg-red-50 dark:focus:bg-red-950 cursor-pointer">
                                            <Trash2 className="w-4 h-4 mr-2" />
                                            Move to Trash
                                        </DropdownMenuItem>
                                    </DropdownMenuContent>
                                </DropdownMenu>
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

const handleDownload = async (file: any) => {
    try {
        console.log("Unwrapping file key...");
        const sodium = await getSodium();
        const currentFolder = useDriveStore.getState().getCurrentFolder();
        if (!currentFolder) throw new Error("Drive keys not initialized");


        const fileKey = sodium.crypto_box_seal_open(
            sodium.from_base64(file.encryptedNodePassphrase),
            currentFolder.publicKey,
            currentFolder.privateKey
        );

        if (!fileKey) throw new Error("Failed to unwrap fileKey!");

        console.log("Fetching S3 URLs...");
        const urls = await getDownloadUrls(file.nodeId);

        console.log("Starting Decryption Worker...");
        const worker = new Worker(new URL('@/workers/decrypt.worker.ts', import.meta.url));
        worker.postMessage({ urls, fileKey, nodeId: file.nodeId });

        worker.onmessage = async (e) => {
            if (e.data.type === 'PROGRESS') {
                console.log(`Decrypting: ${e.data.percent}%`);
                // You could update some React state here for a progress bar!
            }
            else if (e.data.type === 'SUCCESS') {
                console.log("Decryption complete! Triggering browser download...");

                // 1. Get the decrypted blob from the worker
                const blob = e.data.blob;

                // 2. Create a temporary invisible Object URL
                const downloadUrl = URL.createObjectURL(blob);

                // 3. Create a temporary anchor tag to trigger the download
                const a = document.createElement("a");
                a.href = downloadUrl;
                a.download = file.plaintextName; // Suggest the decrypted filename
                document.body.appendChild(a);
                a.click();

                // 4. Clean up
                a.remove();
                URL.revokeObjectURL(downloadUrl);
                worker.terminate();
            }
            else if (e.data.type === 'ERROR') {
                console.error("Worker error:", e.data.message);
                worker.terminate();
            }
        };
    } catch (err) {
        console.error("Download aborted or failed:", err);
    }
};

const handleFolderClick = async (folder: any) => {
        try {
            const sodium = await getSodium();
            const currentFolder = useDriveStore.getState().getCurrentFolder();
            if (!currentFolder) return;
            // 1. Unwrap the Passphrase using the PARENT's Private Key
            const nodePassphrase = sodium.crypto_box_seal_open(
                sodium.from_base64(folder.encryptedNodePassphrase),
                currentFolder.publicKey,
                currentFolder.privateKey
            );
            // 2. Decrypt the CHILD's Private Key using the Passphrase
            const childPrivKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                null,
                sodium.from_base64(folder.wrappedNodeKey),
                sodium.from_string("FolderNode"), // Must match AD from folder creation!
                sodium.from_base64(folder.nodePrivNonce),
                nodePassphrase
            );
            // 3. Push to breadcrumbs to navigate!
            useDriveStore.getState().pushFolder({
                nodeId: folder.nodeId,
                name: folder.plaintextName,
                privateKey: childPrivKey,
                publicKey: sodium.from_base64(folder.nodePublicKey)
            });

            // Your useEffect will need to re-fetch files for the new Folder ID!
        } catch (err) {
            console.error("Failed to unlock sub-folder:", err);
        }
    };
