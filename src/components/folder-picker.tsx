"use client";
import { config } from "@/config/env";


import { useEffect, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { ChevronRight, ChevronDown, FolderIcon } from "lucide-react";
import { getSodium } from "@/lib/crypto/sodium";
import { FolderKey, useDriveStore } from "@/lib/driveStore";
import { getAccountSigningPrivateKey } from "@/lib/authStore";
import { customFetch } from "@/lib/api";
import { fetchFiles } from "@/crypto/files";

// 1. Recursive Folder Node (Decrypts children on the fly!)
function PickerNode({ folder, selectedId, onSelect }: { folder: FolderKey, selectedId: string | null, onSelect: (f: FolderKey) => void }) {
    const [isOpen, setIsOpen] = useState(false);
    const [children, setChildren] = useState<FolderKey[]>([]);

    const handleExpand = async (e: React.MouseEvent) => {
        e.stopPropagation();
        if (isOpen) {
            setIsOpen(false);
            return;
        }

        // Decrypt children!
        const sodium = await getSodium();
        const rawFiles = await fetchFiles(folder.nodeId);
        const childFolders: FolderKey[] = [];

        for (const file of rawFiles) {
            if (file.type === 'FOLDER') {
                const nodePassphrase = sodium.crypto_box_seal_open(
                    sodium.from_base64(file.encryptedNodePassphrase),
                    folder.publicKey, folder.privateKey
                );
                const childPrivKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                    null, sodium.from_base64(file.wrappedNodeKey), sodium.from_string("FolderNode"),
                    sodium.from_base64(file.nodePrivNonce), nodePassphrase
                );
                childFolders.push({
                    nodeId: file.nodeId,
                    name: sodium.to_string(sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                        null, sodium.from_base64(file.encryptedName), null,
                        sodium.from_base64(file.nameNonce), folder.privateKey
                    )),
                    privateKey: childPrivKey,
                    publicKey: sodium.from_base64(file.nodePublicKey)
                });
            }
        }
        setChildren(childFolders);
        setIsOpen(true);
    };

    const isSelected = selectedId === folder.nodeId;

    return (
        <div className="ml-4">
            <div
                className={`flex items-center p-2 rounded cursor-pointer ${isSelected ? 'bg-blue-100 dark:bg-blue-900/50 text-blue-600' : 'hover:bg-gray-100 dark:hover:bg-gray-800'}`}
                onClick={() => onSelect(folder)}
            >
                <button onClick={handleExpand} className="p-1 hover:bg-gray-200 dark:hover:bg-gray-700 rounded mr-1">
                    {isOpen ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
                </button>
                <FolderIcon className="w-4 h-4 mr-2" />
                <span className="text-sm">{folder.name}</span>
            </div>

            {isOpen && children.map(child => (
                <PickerNode key={child.nodeId} folder={child} selectedId={selectedId} onSelect={onSelect} />
            ))}
        </div>
    );
}

// 2. The Main Modal Component
export function FolderPickerModal({ itemToMove, onClose }: { itemToMove: any, onClose: () => void }) {
    const rootFolder = useDriveStore(s => s.breadcrumbs[0]);
    const currentFolder = useDriveStore(s => s.getCurrentFolder());
    const [selectedFolder, setSelectedFolder] = useState<FolderKey | null>(null);
    const [isMoving, setIsMoving] = useState(false);

    const executeMove = async () => {
        if (!selectedFolder || !currentFolder) return;
        setIsMoving(true);

        try {
            const sodium = await getSodium();

            // 1. Unwrap Passphrase using CURRENT folder's keys
            const fileKey = sodium.crypto_box_seal_open(
                sodium.from_base64(itemToMove.encryptedNodePassphrase),
                currentFolder.publicKey, currentFolder.privateKey
            );

            const newEncryptedNodePassphrase = sodium.crypto_box_seal(fileKey, selectedFolder.publicKey);

            const accountSigningPrivKey = getAccountSigningPrivateKey();
            if (!accountSigningPrivKey) throw new Error("Missing signing key");
            const signature = sodium.crypto_sign_detached(newEncryptedNodePassphrase, accountSigningPrivKey);
            const newSignedEncryptedNodePassphrase = sodium.to_base64(signature);

            const nameNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
            const newEncryptedName = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
                sodium.from_string(itemToMove.plaintextName), null, null, nameNonce, selectedFolder.privateKey
            );

            // 3. Send API
            const res = await customFetch(`${config.apiUrl}/v1/files/move`, {
                method: "PATCH",
                body: JSON.stringify({
                    nodeId: itemToMove.nodeId,
                    oldParentFolderId: currentFolder.nodeId,
                    newParentFolderId: selectedFolder.nodeId,
                    newEncryptedName: sodium.to_base64(newEncryptedName),
                    newNameNonce: sodium.to_base64(nameNonce),
                    newEncryptedNodePassphrase: sodium.to_base64(newEncryptedNodePassphrase),
                    newSignedEncryptedNodePassphrase: newSignedEncryptedNodePassphrase
                })
            });

            if (res.ok) {
                window.dispatchEvent(new Event('refreshFiles'));
                onClose();
            }
        } catch (e) {
            console.error("Failed to move via picker", e);
            alert("Failed to move item.");
        } finally {
            setIsMoving(false);
        }
    };

    return (
        <Dialog open={!!itemToMove} onOpenChange={(open) => !open && onClose()}>
            <DialogContent className="sm:max-w-md">
                <DialogHeader>
                    <DialogTitle>Move "{itemToMove?.plaintextName}"</DialogTitle>
                </DialogHeader>

                <div className="max-h-[400px] overflow-y-auto border rounded-md p-2 bg-gray-50 dark:bg-gray-950">
                    {rootFolder && (
                        <PickerNode
                            folder={rootFolder}
                            selectedId={selectedFolder?.nodeId || null}
                            onSelect={setSelectedFolder}
                        />
                    )}
                </div>

                <DialogFooter>
                    <Button variant="outline" onClick={onClose} disabled={isMoving}>Cancel</Button>
                    <Button onClick={executeMove} disabled={!selectedFolder || isMoving}>
                        {isMoving ? "Moving..." : "Move Here"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
