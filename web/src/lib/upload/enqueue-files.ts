import { useUploadStore } from "@/hooks/use-upload-store";
import { FolderKey, useDriveStore } from "@/lib/driveStore";
import { getSodium } from "@/lib/crypto/sodium";
import { quantumSeal } from "@/crypto/kem";
import { getAccountSigningPrivateKey } from "@/lib/authStore";
import { initFileUpload } from "@/crypto/upload";

export async function enqueueFileUploads(
    files: File[],
    targetFolder?: FolderKey
) {
    const currentFolder =
        targetFolder ?? useDriveStore.getState().getCurrentFolder();
    if (!currentFolder) throw new Error("Drive keys not initialized");
    const { addToQueue, updateJob } = useUploadStore.getState();

    const queuedItems = files.map((file) => {
        const chunks = Math.ceil(file.size / (4 * 1024 * 1024)) || 1;
        const jobId = addToQueue(
            {
                file,
                uploadId: "",
                nodeId: "",
                fileKey: new Uint8Array(),
                parentNodeId: currentFolder.nodeId,
                encryptedName: "",
                nameNonce: "",
                encryptedNodePassphrase: "",
                totalChunks: chunks,
            },
            "INIT"
        );
        return { jobId, file, chunks };
    });

    const ENCRYPTION_OVERHEAD_BYTES = 40;

    for (const { jobId, file } of queuedItems) {
        // If the job was canceled by user, skip it
        const currentJob = useUploadStore
            .getState()
            .jobs.find((j) => j.id === jobId);
        if (!currentJob) continue;

        try {
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

            const metadataNonce = sodium.randombytes_buf(
                sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES
            );
            const encryptedMetadata =
                sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
                    sodium.from_string(JSON.stringify(metadata)),
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

            const res = await initFileUpload({
                totalFileSize: declaredFileSize,
                totalChunks: chunks,
                parentNodeId,
                encryptedName: sodium.to_base64(encryptedName),
                nameNonce: sodium.to_base64(nameNonce),
                encryptedNodePassphrase: sodium.to_base64(
                    encryptedNodePassphrase
                ),
                signedEncryptedNodePassphrase,
                nodePublicKey,
                wrappedNodeKey,
                nodePrivNonce,
                encryptedMetadata: sodium.to_base64(encryptedMetadata),
                metadataNonce: sodium.to_base64(metadataNonce),
            });
            if (!res) {
                throw new Error("Init file upload returned null");
            }

            updateJob(jobId, {
                uploadId: res.uploadId,
                nodeId: res.nodeId,
                fileKey,
                encryptedName: sodium.to_base64(encryptedName),
                nameNonce: sodium.to_base64(nameNonce),
                encryptedNodePassphrase: sodium.to_base64(
                    encryptedNodePassphrase
                ),
                status: "IDLE",
            });
        } catch (e: any) {
            if (e.message === "QUOTA_EXCEEDED") {
                updateJob(jobId, {
                    status: "ERROR",
                    errorMessage: "Not enough storage space",
                });
            } else {
                console.error("Failed to init upload for file", file.name, e);
                updateJob(jobId, {
                    status: "ERROR",
                    errorMessage: "Initialization failed",
                });
            }
        }
    }
}
