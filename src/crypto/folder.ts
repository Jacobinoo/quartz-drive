import { getSodium } from "@/lib/crypto/sodium";
import { getAccountSigningPrivateKey } from "@/lib/authStore";

export async function createEncryptedFolderPayload(
    folderName: string,
    parentFolderId: string,
    parentFolderPublicKey: Uint8Array,
    parentFolderPrivateKey: Uint8Array,
    authorId: string // You can pass this from your JWT token!
) {
    const sodium = await getSodium();

    // 1. We need your Signing Key to prove YOU created this folder
    const accountSigningPrivKey = getAccountSigningPrivateKey();
    if (!accountSigningPrivKey) throw new Error("Missing signing key");

    // 2. Generate brand new keys specifically for this new folder
    const nodeKeyPair = sodium.crypto_box_keypair();
    const nodePassphrase = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_KEYBYTES);
    const nodePrivNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);

    // 3. Encrypt the new Folder's Private Key using its own Passphrase
    const wrappedNodePrivateKey = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        nodeKeyPair.privateKey,
        sodium.from_string("FolderNode"), // AD (Associated Data)
        null,
        nodePrivNonce,
        nodePassphrase
    );

    // 4. Wrap the new Folder's Passphrase using the PARENT Folder's Public Key!
    // (This is the Parent-Child hierarchy link)
    const encryptedNodePassphrase = sodium.crypto_box_seal(
        nodePassphrase,
        parentFolderPublicKey
    );

    // 5. Sign it so nobody can tamper with the keys
    const signedEncryptedNodePassphrase = sodium.crypto_sign_detached(
        encryptedNodePassphrase,
        accountSigningPrivKey
    );

    // 6. Encrypt the Folder Name using the PARENT Folder's Private Key
    const nameNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
    const encryptedName = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
        sodium.from_string(folderName),
        null,
        null,
        nameNonce,
        parentFolderPrivateKey
    );

    // 7. Return the raw payload ready for the Go Backend
    return {
        node: {
            nodePublicKey: sodium.to_base64(nodeKeyPair.publicKey),
            wrappedNodeKey: sodium.to_base64(wrappedNodePrivateKey),
            nodePrivNonce: sodium.to_base64(nodePrivNonce),
            signature: sodium.to_base64(signedEncryptedNodePassphrase),
        },
        link: {
            parentNodeId: parentFolderId,
            encryptedName: sodium.to_base64(encryptedName),
            nameNonce: sodium.to_base64(nameNonce),
            encryptedNodePassphrase: sodium.to_base64(encryptedNodePassphrase),
            signedEncryptedNodePassphrase: sodium.to_base64(signedEncryptedNodePassphrase),
            authorId: authorId
        }
    };
}
