"use client";
import { config } from "@/config/env";


import { useEffect, useState } from "react";
import { fetchFiles, getDownloadUrls } from "@/crypto/files";
import { getSodium } from "@/lib/crypto/sodium";
import { useDriveStore } from "@/lib/driveStore";
import { getAccountSigningPrivateKey } from "@/lib/authStore";
import {
    FileIcon,
    FolderIcon,
    Info,
    MoreVertical,
    Pencil,
    Share2,
    Trash2,
    CloudDownload
} from "lucide-react";
import { customFetch } from "@/lib/api"; // Added for our direct API calls
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

import { initializeDriveKeys } from "@/crypto/drive";
import { FolderPickerModal } from "./folder-picker";
import { FileDetailsModal } from "./file-details-modal";
import { formatBytes } from "@/lib/utils/size";
import { addSingleSearchItem, removeSearchItem } from "@/lib/SearchIndexStore";
import { FilePreviewModal } from "./file-preview-modal";
import { ShareModal } from "./share-modal";
import { useDownloadStore } from "@/hooks/use-download-store";
import { quantumSeal, quantumSealOpen } from "@/crypto/kem";

export function FileList() {
    const currentFolder = useDriveStore(s => s.getCurrentFolder());
    const draggedItem = useDriveStore(s => s.draggedItem);
    const setDraggedItem = useDriveStore(s => s.setDraggedItem);
  const [files, setFiles] = useState<any[]>([]);
  const [pickerItemToMove, setPickerItemToMove] = useState<any>(null);
  const [detailsFile, setDetailsFile] = useState<any>(null);
  const [previewFile, setPreviewFile] = useState<{
    file: any,
    url: string | null,
    loading?: boolean,
    progress?: number,
    tooLarge?: boolean,
    unsupported?: boolean
  } | null>(null);
    const [folderToShare, setFolderToShare] = useState<any>(null);
  const refreshTrigger = useDriveStore(s => s.refreshTrigger);




      useEffect(() => {
        return () => {
          if (previewFile?.url) {
            console.log('Revoking url', previewFile.url);
            URL.revokeObjectURL(previewFile.url);
          }
        };
      }, [previewFile?.url]);

    useEffect(() => {
        if (!currentFolder) return;

        // Stale-While-Revalidate: Immediately show cached files if available
        const folderCache = useDriveStore.getState().folderCache;
        if (folderCache[currentFolder.nodeId]) {
            setFiles(folderCache[currentFolder.nodeId]);
        }

        async function loadAndDecrypt() {
            try {
                const [rawFiles, sodium] = await Promise.all([
                    fetchFiles(currentFolder!.nodeId),
                    getSodium()
                ]);

                if (!rawFiles || rawFiles.length === 0) {
                    setFiles([]);
                    useDriveStore.getState().setFolderCache(currentFolder!.nodeId, []);
                    return;
                }

                const decryptedFiles = await Promise.all(rawFiles.map(async (file: any)  => {
                    try {
                        const ciphertext = sodium.from_base64(file.encryptedName);
                        const nonce = sodium.from_base64(file.nameNonce);
                        const decryptedBytes = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                            null, ciphertext, null, nonce, currentFolder!.privateKey
                        );
                      let metadata = null;

                      if (file.encryptedMetadata && file.type === 'FILE') {
                                  // First, unwrap the File Key
                                  const fileKey = await quantumSealOpen(
                                      sodium.from_base64(file.encryptedNodePassphrase),
                                     currentFolder!.privateKey
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

                              let signatureVerified = false;
                              try {
                                  if (file.signedEncryptedNodePassphrase && file.authorSigningPublicKey) {
                                      signatureVerified = sodium.crypto_sign_verify_detached(
                                          sodium.from_base64(file.signedEncryptedNodePassphrase),
                                          sodium.from_base64(file.encryptedNodePassphrase),
                                          sodium.from_base64(file.authorSigningPublicKey)
                                      );
                                  }
                              } catch (sigErr) {
                                  console.error("Signature verification error for", file.nodeId, sigErr);
                              }

                              if (!signatureVerified && file.signedEncryptedNodePassphrase) {
                                  alert(`WARNING: Digital signature verification failed for file ${file.nodeId}! The file may have been tampered with.`);
                              }


                        return { ...file, plaintextName: sodium.to_string(decryptedBytes), metadata, signatureVerified };
                    } catch (e) {
                        return { ...file, plaintextName: "Decryption Failed", signatureVerified: false };
                    }
                }));

                // Compare stringified versions to avoid unnecessary re-renders
                const currentCache = useDriveStore.getState().folderCache[currentFolder!.nodeId];
                if (!currentCache || JSON.stringify(currentCache) !== JSON.stringify(decryptedFiles)) {
                    setFiles(decryptedFiles);
                    useDriveStore.getState().setFolderCache(currentFolder!.nodeId, decryptedFiles);
                }

            } catch (err) {
                console.error(err);
            }
        }
        loadAndDecrypt();

        const handleRefresh = () => loadAndDecrypt();
        window.addEventListener('refreshFiles', handleRefresh);
        return () => window.removeEventListener('refreshFiles', handleRefresh);
    }, [currentFolder, refreshTrigger]);

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
            const res = await customFetch(`${config.apiUrl}/v1/files/rename?nodeId=${file.nodeId}&parentFolderId=${currentFolder.nodeId}`, {
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

              await addSingleSearchItem({
                  id: file.nodeId,
                  name: newName, // The original plaintext file name
                  type: file.type,
                  sizeBytes: file.sizeBytes,
                  createdAt: file.createdAt
              });
            }
        } catch (e) {
            console.error("Rename failed:", e);
        }
    };

    // --- NEW: Trash Handler ---
    const handleTrash = async (file: any) => {
        try {
            if (!currentFolder) return;
            const res = await customFetch(`${config.apiUrl}/v1/files/trash?nodeId=${file.nodeId}&parentFolderId=${currentFolder.nodeId}`, {
                method: 'DELETE'
            });

            if (res.ok) {
                // Instantly wipe it from the UI!
              setFiles(files => files.filter(f => f.nodeId !== file.nodeId));

              await removeSearchItem(file.nodeId);
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
            const fileKey = await quantumSealOpen(
                sodium.from_base64(itemToMove.encryptedNodePassphrase),

                currentFolder.privateKey
            );

            // 2. We need the TARGET folder's Public & Private keys!
            // First, unwrap the target folder's Passphrase (using current folder's private key)
            const targetFolderPassphrase = await quantumSealOpen(
                sodium.from_base64(targetFolder.encryptedNodePassphrase),
                
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
            const newEncryptedNodePassphrase = await quantumSeal(fileKey, targetPublicKey);

            const accountSigningPrivKey = getAccountSigningPrivateKey();
            if (!accountSigningPrivKey) throw new Error("Missing signing key");
            const signature = sodium.crypto_sign_detached(newEncryptedNodePassphrase, accountSigningPrivKey);
            const newSignedEncryptedNodePassphrase = sodium.to_base64(signature);

            // 4. Re-encrypt the moving item's name using the TARGET folder's Private Key!
            const nameNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
            const newEncryptedName = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
                sodium.from_string(itemToMove.plaintextName),
                null, null, nameNonce, targetPrivateKey
            );

            await customFetch(`${config.apiUrl}/v1/files/move`, {
                method: "PATCH",
                body: JSON.stringify({
                    nodeId: itemToMove.nodeId,
                    oldParentFolderId: currentFolder.nodeId,
                    newParentFolderId: targetFolder.nodeId,
                    newEncryptedName: sodium.to_base64(newEncryptedName),
                    newNameNonce: sodium.to_base64(nameNonce),
                    newEncryptedNodePassphrase: sodium.to_base64(newEncryptedNodePassphrase),
                    newSignedEncryptedNodePassphrase: newSignedEncryptedNodePassphrase
                })
            });

            // Remove the moved item from the current view
            setFiles(prev => prev.filter(f => f.nodeId !== itemToMove.nodeId));

        } catch (e) {
            console.error("Failed to move file cryptographically", e);
            alert("Failed to move item.");
        }
    };

    const handlePreview = async (file: any) => {
        const isImage = /\.(png|jpe?g|gif|webp|avif|svg)$/i.test(file.plaintextName ?? "");
        const isVideo = /\.(mp4|webm|ogg|mov)$/i.test(file.plaintextName ?? "");
        
        if (!isImage && !isVideo) {
            setPreviewFile({ file, url: null, loading: false, progress: 0, unsupported: true });
            return;
        }

        const size = file.metadata?.originalSizeBytes || file.sizeBytes || 0;
        const sizeLimit = isVideo ? 50 * 1024 * 1024 : 25 * 1024 * 1024;
        if (size > sizeLimit) {
            setPreviewFile({ file, url: null, loading: false, progress: 0, tooLarge: true });
            return;
        }

      console.log('Previewing file ', file.plaintextName)
        try {
          setPreviewFile({ file, url: null, loading: true, progress: 0 });
          const url = await handleDownload(file, true, (progress) => {
              setPreviewFile(prev => prev ? { ...prev, progress } : prev);
          }) as string;
          setPreviewFile({ file, url, loading: false });
        } catch (err) {
          console.error('Failed to preview file:', err);
          setPreviewFile(null);
        }
    }

    return (
        <div className="flex flex-col w-full">
            {/* Column header — same visual weight as the breadcrumb bar */}
            <div className="grid grid-cols-[minmax(0,1fr)_120px_40px] items-center px-3 py-2 border-b border-border/60">
                <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Name</span>
                <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Size</span>
                <span />
            </div>

            {files.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-24 text-muted-foreground gap-3">
                    <FolderIcon className="w-10 h-10 opacity-20" />
                    <span className="text-sm">This folder is empty</span>
                </div>
            ) : (
                <div className="flex flex-col">
                    {files.map(file => (
                        <div
                            key={file.nodeId}
                            className={`group grid grid-cols-[minmax(0,1fr)_120px_40px] items-center px-3 py-1.5 rounded-md transition-colors cursor-pointer animate-in fade-in slide-in-from-bottom-1 duration-300 ease-out
                                ${draggedItem?.nodeId === file.nodeId
                                    ? 'opacity-40 bg-accent'
                                    : 'hover:bg-accent/60'
                                }`}
                            onClick={() => {
                                if (file.type === 'FOLDER') {
                                    handleFolderClick(file);
                                } else {
                                    handlePreview(file);
                                }
                            }}
                            draggable={true}
                            onDragStart={(e) => {
                                setDraggedItem(file);
                                e.dataTransfer.effectAllowed = "move";
                            }}
                            onDragEnd={() => setDraggedItem(null)}
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
                            {/* Name cell */}
                            <div
                                className="flex items-center gap-2 min-w-0"
                            >
                                {file.type === 'FOLDER'
                                    ? <FolderIcon className="w-4 h-4 shrink-0 text-blue-500 fill-blue-500/20" />
                                    : <FileIcon className="w-4 h-4 shrink-0 text-muted-foreground/60" />
                                }
                                <span className={`text-sm truncate ${file.type === 'FOLDER' ? 'font-medium' : ''}`}>
                                    {file.plaintextName}
                                </span>
                            </div>

                            {/* Size cell */}
                            <span className="text-xs text-muted-foreground tabular-nums">
                                {file.type === 'FOLDER'
                                    ? '—'
                                    : file.metadata?.originalSizeBytes
                                        ? formatBytes(file.metadata.originalSizeBytes)
                                        : (file.sizeBytes ? formatBytes(file.sizeBytes) : '—')
                                }
                            </span>

                            {/* Actions cell */}
                            <div
                                className="flex items-center justify-end opacity-0 group-hover:opacity-100 transition-opacity"
                                onClick={(e) => e.stopPropagation()}
                            >
                                <DropdownMenu>
                                    <DropdownMenuTrigger asChild>
                                        <button className="p-1 hover:bg-accent rounded-md transition-colors text-muted-foreground">
                                            <MoreVertical className="w-3.5 h-3.5" />
                                        </button>
                                    </DropdownMenuTrigger>
                                    <DropdownMenuContent align="end">
                                        {file.type === 'FILE' && (
                                            <DropdownMenuItem onClick={() => handleDownload(file)} className="cursor-pointer">
                                                <CloudDownload className="w-4 h-4 mr-2" />
                                                Download
                                            </DropdownMenuItem>
                                        )}
                                        {file.type === 'FOLDER' && (
                                            <DropdownMenuItem onClick={() => setFolderToShare(file)} className="cursor-pointer">
                                                <Share2 className="w-4 h-4 mr-2" />
                                                Share Folder
                                            </DropdownMenuItem>
                                        )}
                                        <DropdownMenuItem onClick={() => handleRename(file)} className="cursor-pointer">
                                            <Pencil className="w-4 h-4 mr-2" />
                                            Rename
                                        </DropdownMenuItem>
                                        <DropdownMenuItem onClick={() => setPickerItemToMove(file)} className="cursor-pointer">
                                            <FolderIcon className="w-4 h-4 mr-2" />
                                            Move To...
                                        </DropdownMenuItem>
                                        <DropdownMenuItem onClick={() => handleTrash(file)} className="text-destructive focus:text-destructive focus:bg-destructive/10 cursor-pointer">
                                            <Trash2 className="w-4 h-4 mr-2" />
                                            Move to Trash
                                        </DropdownMenuItem>
                                        <DropdownMenuItem onClick={() => setDetailsFile(file)} className="cursor-pointer">
                                            <Info className="w-4 h-4 mr-2" />
                                            Details
                                        </DropdownMenuItem>
                                    </DropdownMenuContent>
                                </DropdownMenu>
                            </div>
                        </div>
                    ))}
                </div>
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

            {previewFile && (
                <FilePreviewModal
                    file={previewFile.file}
                    url={previewFile.url}
                    loading={previewFile.loading}
                    progress={previewFile.progress}
                    tooLarge={previewFile.tooLarge}
                    unsupported={previewFile.unsupported}
                    onClose={() => setPreviewFile(null)}
                    onDownload={() => handleDownload(previewFile.file)}
                    breadcrumbs={useDriveStore.getState().breadcrumbs}
                />
            )}

            <ShareModal
                folder={folderToShare}
                isOpen={!!folderToShare}
                onClose={() => setFolderToShare(null)}
            />
        </div>
    );
}

const handleDownload = async (file: any, withResult = false, onProgress?: (percent: number) => void): Promise<string | void> => {
    try {
        console.log("Unwrapping file key...");
        const sodium = await getSodium();
        const currentFolder = useDriveStore.getState().getCurrentFolder();
        if (!currentFolder) throw new Error("Drive keys not initialized");

        const fileKey = await quantumSealOpen(
            sodium.from_base64(file.encryptedNodePassphrase),
            currentFolder.privateKey
        );

        if (!fileKey) throw new Error("Failed to unwrap fileKey!");

        if (!withResult) {
            useDownloadStore.getState().addToQueue({
                file,
                nodeId: file.nodeId,
                fileKey
            });
            return;
        }

        console.log("Fetching S3 URLs for preview...");
        const { urls, encryptingNodeId } = await getDownloadUrls(file.nodeId);
        const decryptNodeId = encryptingNodeId || file.nodeId;

        console.log("Starting Decryption Worker for preview...");
      const worker = new Worker(new URL('@/workers/decrypt.worker.ts', import.meta.url));

      const getMimeType = (filename: string) => {
          const ext = filename.split('.').pop()?.toLowerCase();
          switch (ext) {
              case 'mp4': return 'video/mp4';
              case 'webm': return 'video/webm';
              case 'ogg': return 'video/ogg';
              case 'mov': return 'video/quicktime';
              case 'png': return 'image/png';
              case 'jpg':
              case 'jpeg': return 'image/jpeg';
              case 'gif': return 'image/gif';
              case 'webp': return 'image/webp';
              case 'avif': return 'image/avif';
              case 'svg': return 'image/svg+xml';
              case 'pdf': return 'application/pdf';
              default: return 'application/octet-stream';
          }
      };

      return await new Promise<string | void>((resolve, reject) => {
        worker.onmessage = async (e) => {
            if (e.data.type === 'PROGRESS') {
                console.log(`Decrypting: ${e.data.percent}%`);
                if (onProgress) onProgress(e.data.percent);
            }
            else if (e.data.type === 'SUCCESS') {
                console.log("Decryption complete!");

                // 1. Get the decrypted blob from the worker
                const blob = e.data.blob;

                // 2. Create a temporary invisible Object URL
                const downloadUrl = URL.createObjectURL(blob);

                console.log("Returning image url");
                worker.terminate();
                resolve(downloadUrl);
                return;
            }
            else if (e.data.type === 'ERROR') {
                console.error("Worker error:", e.data.message);
              worker.terminate();
              reject(new Error(e.data.message))
            }
        };

      worker.onerror = (err) => {
        worker.terminate();
        reject(err);
      };

      worker.postMessage({ urls, fileKey, nodeId: decryptNodeId, mimeType: getMimeType(file.plaintextName) });
  });
    } catch (err) {
      console.error("Download aborted or failed:", err);
      throw err;
    }
};

const handleFolderClick = async (folder: any) => {
        try {
            const sodium = await getSodium();
            const currentFolder = useDriveStore.getState().getCurrentFolder();
            if (!currentFolder) return;
            // 1. Unwrap the Passphrase using the PARENT's Private Key
            const nodePassphrase = await quantumSealOpen(
                sodium.from_base64(folder.encryptedNodePassphrase),
                
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
