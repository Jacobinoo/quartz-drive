import sodium from 'libsodium-wrappers-sumo';
import {UploadWorkerInput, UploadWorkerOutput} from "@/types/crypto-worker-types";

const ctx: Worker = self as any;
const CHUNK_SIZE = 4 * 1024 * 1024;

// Funkcja pomocnicza do zbierania małych kawałków streamu w paczki 4MB
async function* streamToChunks(file: File) {
    const reader = file.stream().getReader();
    const buffer = new Uint8Array(CHUNK_SIZE);
    let offset = 0;
    let chunkIndex = 0;

    while (true) {
        const { done, value } = await reader.read();
        if (done) {
            if (offset > 0) yield { buffer: buffer.slice(0, offset), chunkIndex, isFinal: true };
            break;
        }

        let valueOffset = 0;
        while (valueOffset < value.length) {
            const space = CHUNK_SIZE - offset;
            const toCopy = value.subarray(valueOffset, valueOffset + space);
            buffer.set(toCopy, offset);
            offset += toCopy.length;
            valueOffset += toCopy.length;

            if (offset === CHUNK_SIZE) {
                yield { buffer: new Uint8Array(buffer), chunkIndex, isFinal: false };
                offset = 0;
                chunkIndex++;
            }
        }
    }
}

ctx.onmessage = async (event: MessageEvent<UploadWorkerInput>) => {
    const { file, nodeId, fileKey, presignedUrls, taskId } = event.data;

    try {
        await sodium.ready;
        const chunker = streamToChunks(file);
        let completedChunks = 0;

        for await (const chunk of chunker) {
            const { buffer, chunkIndex, isFinal } = chunk;

            const nonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
            const ad = sodium.from_string(`${nodeId}|${chunkIndex}|${isFinal ? '1' : '0'}`);

            const ciphertext = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(buffer, ad, null, nonce, fileKey);

            const payload = new Uint8Array(nonce.length + ciphertext.length);
            payload.set(nonce);
            payload.set(ciphertext, nonce.length);

            await fetch(presignedUrls[chunkIndex], { method: 'PUT', body: payload });

            completedChunks++;
            ctx.postMessage({ type: 'PROGRESS', taskId, completedChunks, totalChunks: presignedUrls.length } as UploadWorkerOutput);
        }

        ctx.postMessage({ type: 'SUCCESS', taskId } as UploadWorkerOutput);
    } catch (error: any) {
        ctx.postMessage({ type: 'ERROR', taskId, message: error.message } as UploadWorkerOutput);
    }
};
