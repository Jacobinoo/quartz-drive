import {
    UploadJob,
    UploadStatus,
    useUploadStore,
} from "@/hooks/use-upload-store";
import { FolderKey, useDriveStore } from "@/lib/driveStore";
import { getSodium } from "@/lib/crypto/sodium";
import { quantumSeal } from "@/crypto/kem";
import { getAccountSigningPrivateKey } from "@/lib/authStore";
import { UUID } from "crypto";
import { initFileUpload } from "@/crypto/upload";

export async function enqueueFileUploads(
    files: File[],
    addToQueue: (
        job: Omit<UploadJob, "id" | "progress" | "status">,
        initialStatus?: UploadStatus
    ) => string,
    targetFolder?: FolderKey
) {
    const currentFolder =
        targetFolder ?? useDriveStore.getState().getCurrentFolder();
    if (!currentFolder) throw new Error("Drive keys not initialized");

    const ENCRYPTION_OVERHEAD_BYTES = 40;

    for (const file of files) {
        console.debug(file.name, file.size);

        const declaredFileSize = file.size + ENCRYPTION_OVERHEAD_BYTES;

        const parentNodeId = currentFolder.nodeId;
        const chunks = Math.ceil(file.size / (4 * 1024 * 1024)) || 1;

        const sodium = await getSodium();
        const fileKey = sodium.randombytes_buf(
            sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES
        );
        console.debug(`file key: ${sodium.to_base64(fileKey)}`);

        // 3. Encrypt the Filename (Symmetric)
        const nameNonce = sodium.randombytes_buf(
            sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES
        );
        const encryptedName = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
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
                signedEncryptedNodePassphrase: signedEncryptedNodePassphrase,
                nodePublicKey: nodePublicKey,
                wrappedNodeKey: wrappedNodeKey,
                nodePrivNonce: nodePrivNonce,

                encryptedMetadata: sodium.to_base64(encryptedMetadata),
                metadataNonce: sodium.to_base64(metadataNonce),
            });

            if (res == null) {
                throw new Error("init file upload failed");
            }

            console.debug(
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
                useUploadStore.getState().updateJob(jobId, {
                    status: "ERROR",
                    errorMessage: "Not enough storage space",
                });
                // Don't break, allow the user to see which files failed
            } else {
                console.error("Failed to init upload for file", file.name, e);
                useUploadStore.getState().updateJob(jobId, {
                    status: "ERROR",
                    errorMessage: "Initialization failed",
                });
            }
        }
    }
}
