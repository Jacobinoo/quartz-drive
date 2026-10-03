import { config } from "@/config/env";
import { customFetch } from "./api";
import { getSodium } from "./crypto/sodium";
import { useDriveStore } from "./driveStore";
import { quantumSeal, quantumSealOpen } from "@/crypto/kem";

export async function shareFolderCryptographically(
    folderToShare: any,
    recipientEmail: string
) {
    const sodium = await getSodium();
    const currentFolder = useDriveStore.getState().getCurrentFolder();

    if (!currentFolder) throw new Error("Not inside a folder");

    // 1. Fetch Jane's Public Key from the server
    const userRes = await customFetch(
        `${config.apiUrl}/v1/keys?email=${encodeURIComponent(recipientEmail)}`,
        { method: "GET" }
    );
    if (!userRes.ok) throw new Error("Could not find a user with that email");
    const recipientData = await userRes.json();
    const recipientAccountPubKey = sodium.from_base64(
        recipientData.accountEncryptionPublicKey
    );

    // 2. Generate a Brand New "Share Volume" KeyPair and Passphrase
    const shareKeyPair = sodium.crypto_kem_keypair();
    const sharePassphrase = sodium.randombytes_buf(
        sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES
    );

    // 3. Encrypt the Share Private Key using the Share Passphrase
    const sharePrivNonce = sodium.randombytes_buf(
        sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES
    );
    const wrappedSharePrivateKey =
        sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
            shareKeyPair.privateKey,
            sodium.from_string("SharedVolume:" + recipientEmail), // Additional Authenticated Data
            null,
            sharePrivNonce,
            sharePassphrase
        );

    // 4. Seal the Share Passphrase using Jane's Public Key! (Only Jane can open this box)
    const encryptedSharePassphraseForOwner = await quantumSeal(
        sharePassphrase,
        recipientAccountPubKey
    );

    // 5. We need to unlock the Target Folder's Passphrase first
    // It is currently sealed by the Parent Folder (our current view)
    const folderPassphrase = await quantumSealOpen(
        sodium.from_base64(folderToShare.encryptedNodePassphrase),

        currentFolder.privateKey
    );

    if (!folderPassphrase)
        throw new Error("Failed to unseal folder passphrase to share it");

    // 6. Seal the Target Folder's Passphrase using the NEW Share Public Key
    const encryptedTargetNodePassphrase = await quantumSeal(
        folderPassphrase,
        shareKeyPair.publicKey
    );

    // 7. Re-encrypt the Folder's Name using the NEW Share Private Key
    const nameNonce = sodium.randombytes_buf(
        sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES
    );
    const encryptedName = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        sodium.from_string(folderToShare.plaintextName),
        null,
        null,
        nameNonce,
        shareKeyPair.privateKey
    );

    // 8. Fire it off to the Go Server!
    const sharePayload = {
        targetNodeId: folderToShare.nodeId,
        recipientUserId: recipientData.userId,

        sharePublicKey: sodium.to_base64(shareKeyPair.publicKey),
        wrappedSharePrivateKey: sodium.to_base64(wrappedSharePrivateKey),
        sharePrivNonce: sodium.to_base64(sharePrivNonce),

        encryptedSharePassphraseForOwner: sodium.to_base64(
            encryptedSharePassphraseForOwner
        ),
        signedEncryptedSharePassphrase: "TODO", // Ignore signatures for MVP

        encryptedTargetNodePassphrase: sodium.to_base64(
            encryptedTargetNodePassphrase
        ),
        encryptedName: sodium.to_base64(encryptedName),
        nameNonce: sodium.to_base64(nameNonce),
    };

    const shareRes = await customFetch(`${config.apiUrl}/v1/files/share`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(sharePayload),
    });

    if (!shareRes.ok) throw new Error("Failed to finalize share on the server");

    return true;
}
