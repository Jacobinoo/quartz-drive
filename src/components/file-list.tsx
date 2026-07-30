"use client";

import { useEffect, useState } from "react";
import { fetchFiles, getDownloadUrls } from "@/crypto/files";
import { getSodium } from "@/lib/crypto/sodium";
import { useDriveStore } from "@/lib/driveStore";
import { FileIcon, FolderIcon, Info, MoreVertical, Pencil, Trash2 } from "lucide-react";
import { customFetch } from "@/lib/api"; // Added for our direct API calls
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { initializeDriveKeys } from "@/crypto/drive";
import { FolderPickerModal } from "./folder-picker";
import { FileDetailsModal } from "./file-details-modal";
import { formatBytes } from "@/lib/utils/size";

export function FileList() {
    const currentFolder = useDriveStore(s => s.getCurrentFolder());
    const draggedItem = useDriveStore(s => s.draggedItem);
    const setDraggedItem = useDriveStore(s => s.setDraggedItem);
  const [files, setFiles] = useState<any[]>([]);
  const [pickerItemToMove, setPickerItemToMove] = useState<any>(null);
  const [detailsFile, setDetailsFile] = useState<any>(null);

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
                      let metadata = null;

                      if (file.encryptedMetadata && file.type === 'FILE') {
                                  // First, unwrap the File Key
                                  const fileKey = sodium.crypto_box_seal_open(
                                      sodium.from_base64(file.encryptedNodePassphrase),
                                      currentFolder!.publicKey, currentFolder!.privateKey
                                  );

                                  // Second, decrypt the JSON string
                                  const decryptedMetaBytes = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                                      null,
                                      sodium.from_base64(file.encryptedMetadata),
                                      null,
                                      sodium.from_base64(file.metadataNonce),
                                      fileKey
                                  );
                                  metadata = JSON.parse(sodium.to_string(decryptedMetaBytes));
                              }


                        return { ...file, plaintextName: sodium.to_string(decryptedBytes), metadata };
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

        const handleRefresh = () => loadAndDecrypt();
        window.addEventListener('refreshFiles', handleRefresh);
        return () => window.removeEventListener('refreshFiles', handleRefresh);
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

    const handleMoveItem = async (itemToMove: any, targetFolder: any) => {
        try {
            if (!currentFolder) return;
            const sodium = await getSodium();

            // 1. Unwrap the moving item's Passphrase using our CURRENT folder's private key
            const fileKey = sodium.crypto_box_seal_open(
                sodium.from_base64(itemToMove.encryptedNodePassphrase),
                currentFolder.publicKey,
                currentFolder.privateKey
            );

            // 2. We need the TARGET folder's Public & Private keys!
            // First, unwrap the target folder's Passphrase (using current folder's private key)
            const targetFolderPassphrase = sodium.crypto_box_seal_open(
                sodium.from_base64(targetFolder.encryptedNodePassphrase),
                currentFolder.publicKey,
                currentFolder.privateKey
            );

            // Second, decrypt the target folder's Private Key using its Passphrase
            const targetPrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                null, sodium.from_base64(targetFolder.wrappedNodeKey),
                sodium.from_string("FolderNode"), sodium.from_base64(targetFolder.nodePrivNonce),
                targetFolderPassphrase
            );
            const targetPublicKey = sodium.from_base64(targetFolder.nodePublicKey);

            // 3. Re-wrap the moving item's Passphrase for the TARGET folder!
            const newEncryptedNodePassphrase = sodium.crypto_box_seal(fileKey, targetPublicKey);

            // 4. Re-encrypt the moving item's name using the TARGET folder's Private Key!
            const nameNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
            const newEncryptedName = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
                sodium.from_string(itemToMove.plaintextName),
                null, null, nameNonce, targetPrivateKey
            );

            await customFetch("https://localhost:3100/v1/files/move", {
                method: "PATCH",
                body: JSON.stringify({
                    nodeId: itemToMove.nodeId,
                    oldParentFolderId: currentFolder.nodeId,
                    newParentFolderId: targetFolder.nodeId,
                    newEncryptedName: sodium.to_base64(newEncryptedName),
                    newNameNonce: sodium.to_base64(nameNonce),
                    newEncryptedNodePassphrase: sodium.to_base64(newEncryptedNodePassphrase),
                    newSignedEncryptedNodePassphrase: "TODO" // MVP Bypass
                })
            });

            // Remove the moved item from the current view
            setFiles(prev => prev.filter(f => f.nodeId !== itemToMove.nodeId));

        } catch (e) {
            console.error("Failed to move file cryptographically", e);
            alert("Failed to move item.");
        }
    };

    return (
        <div className="mt-8 bg-white dark:bg-black rounded-lg shadow-sm border dark:border-gray-800">
            <div className="p-4 border-b dark:border-gray-800 flex justify-between items-center bg-gray-50/50 dark:bg-gray-900/20">
                <h2 className="text-xl font-semibold text-gray-900 dark:text-gray-100">{currentFolder?.name || "My Files"}</h2>
            </div>

            {files.length === 0 ? (
                <div className="p-16 text-center text-gray-500">
                    <FolderIcon className="w-12 h-12 mx-auto mb-3 opacity-20" />
                    This folder is empty.
                </div>
            ) : (
                <Table>
                    <TableHeader className="bg-gray-50/50 dark:bg-gray-900/50">
                        <TableRow>
                            <TableHead className="w-[60%]">Name</TableHead>
                            <TableHead className="w-[20%]">Size</TableHead>
                            <TableHead className="text-right">Actions</TableHead>
                        </TableRow>
                    </TableHeader>
                    <TableBody>
                        {files.map(file => (
                            <TableRow
                                key={file.nodeId}
                                className={`group cursor-pointer ${draggedItem?.nodeId === file.nodeId ? 'opacity-30 bg-blue-50 dark:bg-blue-900/20' : ''}`}

                                // Make the row draggable
                                draggable={true}
                                onDragStart={(e) => {
                                    setDraggedItem(file);
                                    e.dataTransfer.effectAllowed = "move";
                                }}
                                onDragEnd={() => setDraggedItem(null)}

                                // Make FOLDERS act as Drop Targets
                                onDragOver={(e) => {
                                    if (file.type === 'FOLDER' && draggedItem && draggedItem.nodeId !== file.nodeId) {
                                        e.preventDefault();
                                        e.dataTransfer.dropEffect = "move";
                                    }
                                }}
                                onDrop={(e) => {
                                    e.preventDefault();
                                    if (file.type === 'FOLDER' && draggedItem && draggedItem.nodeId !== file.nodeId) {
                                        handleMoveItem(draggedItem, file);
                                    }
                                }}
                            >
                                <TableCell className="font-medium py-3">
                                    <div className="flex items-center" onClick={() => file.type === 'FOLDER' ? handleFolderClick(file) : null}>
                                        {file.type === 'FOLDER' ? <FolderIcon className="mr-3 w-5 h-5 text-blue-500 fill-blue-500/20" /> : <FileIcon className="mr-3 w-5 h-5 text-gray-400" />}
                                        <span className={file.type === 'FOLDER' ? "hover:underline hover:text-blue-600 transition-colors" : ""}>{file.plaintextName}</span>
                                    </div>
                                </TableCell>
                                <TableCell className="text-gray-500 py-3">
                                  {file.metadata?.originalSizeBytes
                                      ? formatBytes(file.metadata.originalSizeBytes)
                                      : (file.sizeBytes ? formatBytes(file.sizeBytes) : '--')
                                  }
                                </TableCell>
                                <TableCell className="text-right py-3">
                                    <div className="flex items-center justify-end gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                                        {file.type === 'FILE' && (
                                            <button onClick={() => handleDownload(file)} className="text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 text-sm font-medium mr-2 px-2 py-1 rounded hover:bg-blue-50 dark:hover:bg-blue-900/30 transition-colors">
                                                Download
                                            </button>
                                        )}
                                        <DropdownMenu>
                                            <DropdownMenuTrigger asChild>
                                                <button className="p-1.5 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-md transition-colors text-gray-500">
                                                    <MoreVertical className="w-4 h-4" />
                                                </button>
                                            </DropdownMenuTrigger>
                                            <DropdownMenuContent align="end">
                                                <DropdownMenuItem onClick={() => handleRename(file)} className="cursor-pointer">
                                                    <Pencil className="w-4 h-4 mr-2" />
                                                    Rename
                                    </DropdownMenuItem>



                                    <DropdownMenuItem onClick={() => setPickerItemToMove(file)} className="cursor-pointer">
                                        <FolderIcon className="w-4 h-4 mr-2" />
                                        Move To...
                                    </DropdownMenuItem>
                                                <DropdownMenuItem onClick={() => handleTrash(file)} className="text-red-600 focus:text-red-600 focus:bg-red-50 dark:focus:bg-red-950 cursor-pointer">
                                                    <Trash2 className="w-4 h-4 mr-2" />
                                                    Move to Trash
                                    </DropdownMenuItem>
                                    <DropdownMenuItem onClick={() => setDetailsFile(file)} className="cursor-pointer">
                                        <Info className="w-4 h-4 mr-2" />
                                        File Details
                                    </DropdownMenuItem>
                                            </DropdownMenuContent>
                                        </DropdownMenu>
                                    </div>
                                </TableCell>
                            </TableRow>
                        ))}
                    </TableBody>
                </Table>
        )}
            {pickerItemToMove && (
                        <FolderPickerModal
                            itemToMove={pickerItemToMove}
                            onClose={() => setPickerItemToMove(null)}
                        />
                    )}

            {detailsFile && (
                <FileDetailsModal
                    file={detailsFile}
                    breadcrumbs={useDriveStore.getState().breadcrumbs}
                    onClose={() => setDetailsFile(null)}
                />
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
