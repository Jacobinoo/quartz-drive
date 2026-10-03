import sodium from "libsodium-wrappers-sumo";

const ctx: Worker = self as any;

ctx.onmessage = async (event) => {
    const {
        urls,
        fileKey,
        nodeId,
        mimeType = "application/octet-stream",
    } = event.data;

    try {
        await sodium.ready;

        const decryptedChunks: Uint8Array[] = [];

        for (let i = 0; i < urls.length; i++) {
            const res = await fetch(urls[i]);
            if (!res.ok) throw new Error(`Failed to fetch chunk ${i}`);

            const buffer = await res.arrayBuffer();
            const payload = new Uint8Array(buffer);

            const nonce = payload.slice(0, 24);
            const ciphertext = payload.slice(24);

            const isFinal = i === urls.length - 1;
            const ad = sodium.from_string(
                `${nodeId}|${i}|${isFinal ? "1" : "0"}`
            );

            const decrypted = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(
                null,
                ciphertext,
                ad,
                nonce,
                fileKey
            );

            decryptedChunks.push(decrypted);

            // Optional: send progress updates
            ctx.postMessage({
                type: "PROGRESS",
                percent: Math.round(((i + 1) / urls.length) * 100),
            });
        }

        // Stitch the decrypted chunks together into a single file blob!
        const fileBlob = new Blob(decryptedChunks as any, { type: mimeType });

        ctx.postMessage({ type: "SUCCESS", blob: fileBlob });
    } catch (error: any) {
        ctx.postMessage({ type: "ERROR", message: error.message });
    }
};
