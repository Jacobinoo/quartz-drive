import { customFetch } from "@/lib/api";
import { getSodium } from "@/lib/crypto/sodium";
import { getAccessToken, getAccountEncryptionPrivateKey } from "@/lib/authStore";
import { useDriveStore } from "@/lib/driveStore";

export async function initializeDriveKeys() {
    // Skip if already initialized
    if (useDriveStore.getState().getCurrentFolder()?.privateKey) return;

    try {
        const sodium = await getSodium();

        // 1. Fetch the full chain from the Go backend
        const res = await customFetch("https://localhost:3100/v1/files/root", { method: "GET" });
        if (!res.ok) throw new Error("Failed to fetch root folder");
        const rootData = await res.json();

        // 2. Recover Account Keys from IndexedDB/Memory
        const accountPrivKey = getAccountEncryptionPrivateKey();
      if (!accountPrivKey) throw new Error("Not authenticated");
      const accountPubKey = sodium.crypto_scalarmult_base(accountPrivKey);
        if (!accountPubKey) throw new Error("Not authenticated");

        // 3. LAYER 1: Unwrap the Share Passphrase
        const sharePassphrase = sodium.crypto_box_seal_open(
            sodium.from_base64(rootData.encryptedSharePassphraseForOwner),
            accountPubKey,
            accountPrivKey
        );

        const token = getAccessToken(); // Replace with however you get your token
        if (!token) throw new Error("No token");
        // 1. Split the token and decode the middle part (the payload)
        const payloadBase64 = token.split('.')[1];
        const decodedPayload = JSON.parse(atob(payloadBase64));
        const userEmail = decodedPayload.email;

        // 4. LAYER 2: Decrypt the Default Share Private Key
        const sharePrivKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
            null,
            sodium.from_base64(rootData.wrappedSharePrivateKey),
            // Important: This AD must match what signup.ts used! ("DefaultShare:" + email)
            // If you don't have email in context here, you might need to fetch it from authStore
            sodium.from_string("DefaultShare:" + userEmail), // TODO: replace with actual email
            sodium.from_base64(rootData.sharePrivNonce),
            sharePassphrase
        );

        // 5. LAYER 3: Unwrap the Root Node Passphrase
        const sharePubKey = sodium.crypto_scalarmult_base(sharePrivKey);
        const rootNodePassphrase = sodium.crypto_box_seal_open(
            sodium.from_base64(rootData.encryptedRootNodePassphrase),
            sharePubKey,
            sharePrivKey
        );

        // 6. LAYER 4: Decrypt the Root Node Private Key (Your Drive Key!)
        const rootNodePrivKey = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
            null,
            sodium.from_base64(rootData.wrappedNodePrivateKey),
            sodium.from_string("RootNode:" + userEmail), // TODO: replace with actual email
            sodium.from_base64(rootData.nodePrivNonce),
            rootNodePassphrase
        );

        const rootNodePubKey = sodium.from_base64(rootData.nodePublicKey);

        // 7. Store the Drive Key in JS memory!
        useDriveStore.getState().setRootFolder({
            nodeId: rootData.nodeId,
            name: "My Drive", // The root folder doesn't have an encrypted name, so we just call it "My Drive"
            privateKey: rootNodePrivKey,
            publicKey: rootNodePubKey
        });
        console.log("✅ Drive Keys Successfully Initialized!");

    } catch (err) {
        console.error("Failed to initialize drive keys:", err);
    }
}
