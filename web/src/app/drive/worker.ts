// import sodium from 'libsodium-wrappers-sumo';
// import { UploadWorkerInput, UploadWorkerOutput} from "@/app/drive/page";
//
// const ctx: Worker = self as any;
//
// interface ChunkData {
//     buffer: Uint8Array;
//     chunkIndex: number;
//     isFinal: boolean;
// }
//
// async function* createFileChunker(file: File, chunkSize = 4 * 1024 * 1024): AsyncGenerator<ChunkData> {
//     let offset = 0;
//     let chunkIndex = 0;
//     while (offset < file.size) {
//         const slice = file.slice(offset, offset + chunkSize);
//         const arrayBuffer = await slice.arrayBuffer();
//         yield {
//             buffer: new Uint8Array(arrayBuffer),
//             chunkIndex,
//             isFinal: offset + chunkSize >= file.size
//         };
//         offset += chunkSize;
//         chunkIndex++;
//     }
// }
//
// function createAssociatedData(nodeId: string, chunkIndex: number, isFinal: boolean): Uint8Array {
//     const adString = `${nodeId}|${chunkIndex}|${isFinal ? '1' : '0'}`;
//     return sodium.from_string(adString);
// }
//
// ctx.onmessage = async (event: MessageEvent<UploadWorkerInput>) => {
//     const { file, nodeId, fileKey, presignedUrls } = event.data;
//
//     try {
//         await sodium.ready; // Czekamy na załadowanie WebAssembly
//
//         const uploadPromises: Promise<void>[] = [];
//         const chunker = createFileChunker(file);
//
//         let completedChunks = 0;
//         const totalChunks = presignedUrls.length;
//
//         for await (const chunk of chunker) {
//             const { buffer, chunkIndex, isFinal } = chunk;
//
//             // Kryptografia
//             const chunkNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
//             const ad = createAssociatedData(nodeId, chunkIndex, isFinal);
//
//             const ciphertext = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
//                 buffer, ad, null, chunkNonce, fileKey
//             );
//
//             // Sklejanie payloadu
//             const payloadToUpload = new Uint8Array(chunkNonce.length + ciphertext.length);
//             payloadToUpload.set(chunkNonce, 0);
//             payloadToUpload.set(ciphertext, chunkNonce.length);
//
//             const request = fetch(presignedUrls[chunkIndex], {
//                 method: 'PUT',
//                 body: payloadToUpload
//             }).then((response) => {
//                 if (!response.ok) {
//                     throw new Error(`Storage service rejected chunk ${chunkIndex}`);
//                 }
//
//                 completedChunks++;
//
//                 // Informujemy UI o postępie
//                 ctx.postMessage({
//                     type: 'PROGRESS',
//                     completedChunks,
//                     totalChunks: totalChunks
//                 } as UploadWorkerOutput);
//             });
//
//             // uploadPromises.push(request);
//         }
//
//         // Czekamy aż wszystko poleci w tle
//         await Promise.all(uploadPromises);
//
//         // Zgłaszamy sukces
//         ctx.postMessage({ type: 'SUCCESS' } as UploadWorkerOutput);
//
//     } catch (error: any) {
//         // Zgłaszamy błąd
//         ctx.postMessage({ type: 'ERROR', message: error.message || 'Nieznany błąd workera' } as UploadWorkerOutput);
//     }
// };
