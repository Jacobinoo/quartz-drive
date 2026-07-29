"use client";

import { useEffect, useState } from "react";
import { getSodium } from "@/lib/crypto/sodium";
import { useDriveStore } from "@/lib/driveStore";
import { FileIcon, FolderIcon, RefreshCw, Trash } from "lucide-react";
import { customFetch } from "@/lib/api";

export default function TrashPage() {
    const currentFolder = useDriveStore(s => s.getCurrentFolder());
    const [trashedFiles, setTrashedFiles] = useState<any[]>([]);

    useEffect(() => {
        if (!currentFolder) return;
        async function fetchTrash() {
            try {
                // Fetch soft-deleted files for this specific folder
                const res = await customFetch(`https://localhost:3100/v1/files/trash/list?folderId=${currentFolder!.nodeId}`);
                if (!res.ok) return;
                const rawFiles = await res.json();
                if (!rawFiles) {
                    setTrashedFiles([]);
                    return;
                }

                const sodium = await getSodium();
                const decrypted = rawFiles.map((file: any) => {
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
                setTrashedFiles(decrypted);
            } catch (err) {
                console.error(err);
            }
        }
        fetchTrash();
    }, [currentFolder]);

    const handleRestore = async (file: any) => {
        try {
            if (!currentFolder) return;
            const res = await customFetch(`https://localhost:3100/v1/files/restore?nodeId=${file.nodeId}&parentFolderId=${currentFolder.nodeId}`, {
                method: 'POST'
            });
            if (res.ok) {
                setTrashedFiles(files => files.filter(f => f.nodeId !== file.nodeId));
            }
        } catch (e) {
            console.error("Restore failed:", e);
        }
    };

    const handleEmptyTrash = async () => {
        try {
            const res = await customFetch(`https://localhost:3100/v1/files/trash/empty`, { method: 'DELETE' });
            if (res.ok) {
                setTrashedFiles([]);
                alert("Trash emptied! SeaweedFS Chunks are being wiped asynchronously in the background.");
            }
        } catch (e) {
            console.error("Empty trash failed:", e);
        }
    };

    return (
        <div className="p-8 max-w-4xl mx-auto mt-8">
            <div className="flex justify-between items-center mb-8 border-b pb-4">
                <h1 className="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-3">
                    <Trash className="w-7 h-7 text-red-500" />
                    Trash ({currentFolder?.name})
                </h1>
                <button
                    onClick={handleEmptyTrash}
                    disabled={trashedFiles.length === 0}
                    className="bg-red-500 hover:bg-red-600 text-white px-5 py-2.5 rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed shadow-sm"
                >
                    Empty Global Trash
                </button>
            </div>

            {trashedFiles.length === 0 ? (
                <div className="p-16 text-center text-gray-400 border-2 border-dashed rounded-xl dark:border-gray-800 bg-gray-50/50 dark:bg-gray-900/50">
                    <Trash className="w-12 h-12 mx-auto mb-4 opacity-20" />
                    <p className="text-lg">No deleted items in this folder.</p>
                </div>
            ) : (
                <div className="flex flex-col gap-3">
                    {trashedFiles.map(f => (
                        <div key={f.nodeId} className="p-4 bg-white dark:bg-gray-900 border dark:border-gray-800 rounded-xl shadow-sm flex justify-between items-center hover:shadow-md transition-shadow group">
                            <div className="flex items-center text-gray-500 opacity-75">
                                {f.type === 'FOLDER' ? <FolderIcon className="mr-4 text-red-400" /> : <FileIcon className="mr-4 text-red-400" />}
                                <span className="line-through font-medium">{f.plaintextName}</span>
                            </div>
                            <button
                                onClick={() => handleRestore(f)}
                                className="flex items-center gap-2 text-emerald-600 hover:text-emerald-700 bg-emerald-50 hover:bg-emerald-100 dark:bg-emerald-900/30 dark:hover:bg-emerald-900/50 px-4 py-2 rounded-lg font-medium transition-colors"
                            >
                                <RefreshCw className="w-4 h-4" />
                                Restore
                            </button>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}
