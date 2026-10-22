"use client";
import { config } from "@/config/env";

import { Button } from "@/components/ui/button";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { FileUpIcon, FolderPlusIcon, FolderUpIcon, Plus } from "lucide-react";
import { useState } from "react";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useUploadStore } from "@/hooks/use-upload-store";
import { openFilePicker } from "@/lib/utils/file-picker";
import { getSodium } from "@/lib/crypto/sodium";
import { initFileUpload } from "@/crypto/upload";
import { getAccessToken, getAccountSigningPrivateKey } from "@/lib/authStore";
import { useDriveStore } from "@/lib/driveStore";
import { createEncryptedFolderPayload } from "@/crypto/folder";
import { customFetch } from "@/lib/api";
import { addSingleSearchItem } from "@/lib/SearchIndexStore";
import { UUID } from "crypto";
import { quantumSeal } from "@/crypto/kem";

export function NewDriveItemButton() {
    const addToQueue = useUploadStore((s) => s.addToQueue);
    const [isFolderDialogOpen, setIsFolderDialogOpen] = useState(false);
    const [newFolderName, setNewFolderName] = useState("");

    const handleUpload = async () => {
        console.log("opening file picker");

        const files = await openFilePicker({ multiple: true });

        console.log("file picker closed");

        const currentFolder = useDriveStore.getState().getCurrentFolder();
        if (!currentFolder) throw new Error("Drive keys not initialized");

        const ENCRYPTION_OVERHEAD_BYTES = 40;

        for (const file of files) {
            console.log(file.name, file.size);

            const declaredFileSize = file.size + ENCRYPTION_OVERHEAD_BYTES;

            const parentNodeId = currentFolder.nodeId;
            const chunks = Math.ceil(file.size / (4 * 1024 * 1024)) || 1;

            const sodium = await getSodium();
            const fileKey = sodium.randombytes_buf(
                sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES
            );
            console.log(`file key: ${sodium.to_base64(fileKey)}`);

            // 3. Encrypt the Filename (Symmetric)
            const nameNonce = sodium.randombytes_buf(
                sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES
            );
            const encryptedName =
                sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
                    sodium.from_string(file.name),
                    null, // No AD needed for name right now
                    null,
                    nameNonce,
                    currentFolder.privateKey
                );

            // 4. Wrap the fileKey (Asymmetric Box Seal)
            // Only someone with the accountPrivKey can open this box to retrieve the fileKey!
            const encryptedNodePassphrase = await quantumSeal(
                fileKey,
                currentFolder.publicKey
            );

            const metadata = {
                mimeType: file.type || "application/octet-stream",
                lastModified: file.lastModified,
                originalSizeBytes: file.size,
                fileExtension: file.name.split(".").pop() || "",
            };
            const metadataJson = JSON.stringify(metadata);

            const metadataNonce = sodium.randombytes_buf(
                sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES
            );
            const encryptedMetadata =
                sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
                    sodium.from_string(metadataJson),
                    null,
                    null,
                    metadataNonce,
                    fileKey
                );

            const nodeKeyPair = sodium.crypto_kem_keypair();
            const nodePublicKey = sodium.to_base64(nodeKeyPair.publicKey);

            const rawNodePrivNonce = sodium.randombytes_buf(
                sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES
            );
            const nodePrivNonce = sodium.to_base64(rawNodePrivNonce);
            const wrappedNodePrivateKey =
                sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
                    nodeKeyPair.privateKey,
                    sodium.from_string("FileNode"),
                    null,
                    rawNodePrivNonce,
                    fileKey
                );
            const wrappedNodeKey = sodium.to_base64(wrappedNodePrivateKey);

            const accountSigningPrivKey = getAccountSigningPrivateKey();
            if (accountSigningPrivKey == null) {
                throw new Error("account signing priv key null");
            }
            const signature = sodium.crypto_sign_detached(
                encryptedNodePassphrase,
                accountSigningPrivKey
            );
            const signedEncryptedNodePassphrase = sodium.to_base64(signature);

            const jobId = addToQueue(
                {
                    file,
                    uploadId: "", // Will be filled after init
                    nodeId: "", // Will be filled after init
                    fileKey,
                    parentNodeId,
                    encryptedName: sodium.to_base64(encryptedName),
                    totalChunks: chunks,
                    nameNonce: sodium.to_base64(nameNonce),
                    encryptedNodePassphrase: sodium.to_base64(
                        encryptedNodePassphrase
                    ),
                },
                "INIT"
            );

            try {
                const res: {
                    uploadId: UUID;
                    nodeId: UUID;
                    expiresAt: number;
                } | null = await initFileUpload({
                    totalFileSize: declaredFileSize,
                    totalChunks: chunks,

                    parentNodeId: parentNodeId,
                    encryptedName: sodium.to_base64(encryptedName),
                    nameNonce: sodium.to_base64(nameNonce),
                    encryptedNodePassphrase: sodium.to_base64(
                        encryptedNodePassphrase
                    ),
                    signedEncryptedNodePassphrase:
                        signedEncryptedNodePassphrase,
                    nodePublicKey: nodePublicKey,
                    wrappedNodeKey: wrappedNodeKey,
                    nodePrivNonce: nodePrivNonce,

                    encryptedMetadata: sodium.to_base64(encryptedMetadata),
                    metadataNonce: sodium.to_base64(metadataNonce),
                });

                if (res == null) {
                    throw new Error("init file upload failed");
                }

                console.log(
                    `file "${file.name}" initialized. NodeID: ${res.nodeId}`
                );

                // 3. Mark as IDLE so the background worker picks it up
                useUploadStore.getState().updateJob(jobId, {
                    uploadId: res.uploadId,
                    nodeId: res.nodeId,
                    status: "IDLE",
                });
            } catch (e: any) {
                if (e.message === "QUOTA_EXCEEDED") {
                    useUploadStore
                        .getState()
                        .updateJob(jobId, {
                            status: "ERROR",
                            errorMessage: "Not enough storage space",
                        });
                    // Don't break, allow the user to see which files failed
                } else {
                    console.error(
                        "Failed to init upload for file",
                        file.name,
                        e
                    );
                    useUploadStore
                        .getState()
                        .updateJob(jobId, {
                            status: "ERROR",
                            errorMessage: "Initialization failed",
                        });
                }
            }
        }
    };
    const handleCreateFolder = async () => {
        if (!newFolderName.trim()) return;

        try {
            const folderName = newFolderName.trim();
            setIsFolderDialogOpen(false);
            setNewFolderName("");
            const currentFolder = useDriveStore.getState().getCurrentFolder();
            if (!currentFolder) throw new Error("Drive keys not initialized");

            const token = getAccessToken();
            if (!token) throw new Error("No token");

            const payloadBase64 = token.split(".")[1];
            const decodedPayload = JSON.parse(atob(payloadBase64));

            const authorId = decodedPayload.sub;

            const payload = await createEncryptedFolderPayload(
                folderName,
                currentFolder.nodeId,
                currentFolder.publicKey,
                currentFolder.privateKey,
                authorId
            );

            const res = await customFetch(`${config.apiUrl}/v1/files/folder`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(payload),
            });

            if (res.ok) {
                console.log("✅ Folder created successfully!");
                const data = await res.json();
                await addSingleSearchItem({
                    id: data.nodeId,
                    name: folderName,
                    type: "FOLDER",
                    sizeBytes: 0,
                    createdAt: new Date().toISOString(),
                });
                useDriveStore.getState().triggerRefresh();
            }
        } catch (err) {
            console.error("Failed to create folder", err);
        }
    };

    return (
        <>
            <DropdownMenu>
                <DropdownMenuTrigger asChild>
                    <Button className="bg-blue-600 hover:bg-blue-700 text-white h-11 mx-2 transition-colors">
                        <Plus className="w-5 h-5 mr-1" />
                        New
                    </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent className="w-56">
                    <DropdownMenuItem
                        onClick={handleUpload}
                        className="cursor-pointer"
                    >
                        <FileUpIcon className="mr-2 h-4 w-4" />
                        Upload File
                    </DropdownMenuItem>
                    <DropdownMenuItem disabled>
                        <FolderUpIcon className="mr-2 h-4 w-4" />
                        Upload Folder
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                        onClick={() => setIsFolderDialogOpen(true)}
                        className="cursor-pointer"
                    >
                        <FolderPlusIcon className="mr-2 h-4 w-4" />
                        New Folder
                    </DropdownMenuItem>
                </DropdownMenuContent>
            </DropdownMenu>

            <Dialog
                open={isFolderDialogOpen}
                onOpenChange={setIsFolderDialogOpen}
            >
                <DialogContent className="sm:max-w-[425px]">
                    <DialogHeader>
                        <DialogTitle>Create New Folder</DialogTitle>
                    </DialogHeader>
                    <div className="grid gap-4 py-4">
                        <div className="grid grid-cols-4 items-center gap-4">
                            <Label htmlFor="name" className="text-right">
                                Name
                            </Label>
                            <Input
                                id="name"
                                value={newFolderName}
                                onChange={(e) =>
                                    setNewFolderName(e.target.value)
                                }
                                placeholder="My Secure Folder"
                                className="col-span-3"
                                autoFocus
                                onKeyDown={(e) => {
                                    if (e.key === "Enter") handleCreateFolder();
                                }}
                            />
                        </div>
                    </div>
                    <DialogFooter>
                        <Button
                            type="button"
                            variant="secondary"
                            onClick={() => setIsFolderDialogOpen(false)}
                        >
                            Cancel
                        </Button>
                        <Button type="submit" onClick={handleCreateFolder}>
                            Create Folder
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </>
    );
}
