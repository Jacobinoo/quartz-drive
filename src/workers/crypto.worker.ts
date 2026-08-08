// Stub localStorage for the Web Worker environment
if (typeof globalThis.localStorage === 'undefined') {
    (globalThis as any).localStorage = {
        getItem: () => null,
        setItem: () => {},
        removeItem: () => {}
    };
}

import sodium from 'libsodium-wrappers-sumo';
import {UploadWorkerInput, UploadWorkerOutput} from "@/types/crypto-worker-types";
import { customFetch } from '@/lib/api';
import { setAuthState } from '@/lib/authStore';

const ctx: Worker = self as any;
const CHUNK_SIZE = 4 * 1024 * 1024;

interface QueuedChunk {
    chunkIndex: number;
    isFinal: boolean;
    payload: Uint8Array;
    chunkHash: string;
    declaredChunkSize: number;
}

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

async function hashUint8Array(data: Uint8Array<ArrayBuffer>): Promise<string> {
  const hashBuffer = await crypto.subtle.digest('SHA-256', data);
  const hashBytes = new Uint8Array(hashBuffer);

  // Convert to base64, not hex
  let binary = '';
  for (let i = 0; i < hashBytes.length; i++) {
    binary += String.fromCharCode(hashBytes[i]);
  }
  return btoa(binary);
}

async function requestChunkPresignedUrl(uploadId: string, chunkIndex: number, declaredSize: number, chunkHash: string): Promise<string | null> {
  const res = await customFetch("https://localhost:3100/v1/files/upload", {
    method: "POST",
    headers: {
        "Content-Type": "application/json",
    },
    body: JSON.stringify({
      uploadId,
      chunkIndex,
      declaredSize,
      chunkHash
    }),
  })

  const uploadRes = await res.json();
  if (uploadRes.url) {
    return uploadRes.url
  }
  return null
}

async function reportChunkDone(uploadId: string, chunkIndex: number, etag: string, chunkHash: string): Promise<boolean> {
  const res = await customFetch("https://localhost:3100/v1/files/upload/chunk_finish", {
    method: "POST",
    headers: {
        "Content-Type": "application/json",
    },
    body: JSON.stringify({
      uploadId,
      chunkIndex,
      etag,
      chunkHash
    }),
  })
  let test = await res.text()
  console.log(`Status of reportChunkDone: ${res.status} ${test}`)


  if (!res || !res.ok) return false
  return true
}


ctx.onmessage = async (event: MessageEvent<UploadWorkerInput>) => {
    const { file, nodeId, uploadId, totalChunks, fileKey, taskId, accessToken, csrfToken} = event.data;

    // Inject auth tokens into the worker's memory so customFetch works!
    setAuthState(accessToken || null, csrfToken || null);

    try {
        await sodium.ready;
      const chunker = streamToChunks(file);


      const queue: QueuedChunk[] = [];
      const MAX_QUEUE_SIZE = 10; // Max chunks in RAM at once (10 * 4MB = 40MB)
      let isProducerDone = false;
      const chunkScores: number[] = new Array(totalChunks).fill(0);
      const postProgress = (activity: string) => {
          const completedChunks = chunkScores.reduce((a, b) => a + b, 0);
          ctx.postMessage({ type: 'PROGRESS', taskId, completedChunks, totalChunks, activity } as UploadWorkerOutput);
      };

      // Custom signaling mechanism so loops can "wait" without blocking the thread
      let queueChangeResolvers: (() => void)[] = [];
      const notifyQueueChange = () => {
          queueChangeResolvers.forEach(resolve => resolve());
          queueChangeResolvers = [];
      };
      const waitForQueueChange = () => new Promise<void>(resolve => queueChangeResolvers.push(resolve));

      let metrics = {
          initTime: 0,
          putTime: 0,
          reportTime: 0
      };

      const runProducer = async () => {
        for await (const chunk of chunker) {
            const { buffer, chunkIndex, isFinal } = chunk;

            // BACKPRESSURE: Pause if the queue is full so we don't eat all RAM!
            while (queue.length >= MAX_QUEUE_SIZE) {
                await waitForQueueChange();
            }
            const declaredChunkSize = buffer.byteLength + 16 + 24;
            const nonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
            const ad = sodium.from_string(`${nodeId}|${chunkIndex}|${isFinal ? '1' : '0'}`);
            const ciphertext = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(buffer, ad, null, nonce, fileKey);
            const payload = new Uint8Array(nonce.length + ciphertext.length);
            payload.set(nonce);
            payload.set(ciphertext, nonce.length);
            const chunkHash = await hashUint8Array(payload);
            // Push to queue and wake up any sleeping consumers
            queue.push({ chunkIndex, isFinal, payload, chunkHash, declaredChunkSize });
            
            // Increment by 0.1 (10%) for encryption
            chunkScores[chunkIndex] = 0.1;
            postProgress(`Encrypting...`);
            
            notifyQueueChange();
        }

        // Mark as done and wake up consumers one last time
        isProducerDone = true;
        notifyQueueChange();
      };

      const runConsumer = async () => {
        while (true) {
            const item = queue.shift();
            if (!item) {
                // If queue is empty AND producer is done, this consumer can exit
                if (isProducerDone) break;

                // Otherwise, wait for the producer to give us more work
                await waitForQueueChange();
                continue;
            }
            // We took an item, wake up the producer in case it was paused due to a full queue
            notifyQueueChange();
            const { chunkIndex, payload, chunkHash, declaredChunkSize } = item;
            // 1. Get URL
            const t0 = performance.now();
            const presignedUrl = await requestChunkPresignedUrl(uploadId, chunkIndex, declaredChunkSize, chunkHash);
            const t1 = performance.now();
            
            if (!presignedUrl) throw new Error("Could not request chunk presigned url");
            
            // Increment by 0.05 (5%) for requesting URL
            chunkScores[chunkIndex] = 0.15;
            postProgress(`Uploading...`);
            
            // 2. Upload (Using XHR for exact byte progress!)
            const t2 = performance.now();
            const uploadResponse: any = await new Promise((resolve, reject) => {
                const xhr = new XMLHttpRequest();
                xhr.open('PUT', presignedUrl);
                xhr.setRequestHeader("x-amz-checksum-sha256", chunkHash);
                
                xhr.upload.onprogress = (e) => {
                    if (e.lengthComputable) {
                        // Dynamically update the 80% fraction as bytes upload!
                        chunkScores[chunkIndex] = 0.15 + ((e.loaded / e.total) * 0.80);
                        postProgress(`Uploading...`);
                    }
                };
                
                xhr.onload = () => {
                    chunkScores[chunkIndex] = 0.95; // Snap to exactly 95% when finished uploading
                    resolve({
                        ok: xhr.status >= 200 && xhr.status < 300,
                        headers: { get: (name: string) => xhr.getResponseHeader(name) }
                    });
                };
                
                xhr.onerror = () => reject(new Error("Network Error"));
                xhr.send(payload as any);
            });
            const t3 = performance.now();

            if (!uploadResponse.ok) throw new Error(`Chunk ${chunkIndex} failed to upload to cloud`);

            let etag = uploadResponse.headers.get("ETag");
            if (!etag) throw new Error(`Chunk ${chunkIndex} response missing ETag`);
            etag = etag.substring(1, etag.length - 1);
            
            // 3. Report
            const t4 = performance.now();
            const reportChunkResponse = await reportChunkDone(uploadId, chunkIndex, etag, chunkHash);
            const t5 = performance.now();
            
            if (!reportChunkResponse) throw new Error(`Chunk ${chunkIndex} failed to report as done`);
            
            // Increment by 0.05 (5%) for reporting done (Total per chunk = 1.0)
            chunkScores[chunkIndex] = 1.0;
            
            const initTime = t1 - t0;
            const putTime = t3 - t2;
            const reportTime = t5 - t4;
            
            console.log(`Chunk ${chunkIndex} PUT duration: ${putTime.toFixed(2)}ms`);
            
            metrics.initTime += initTime;
            metrics.putTime += putTime;
            metrics.reportTime += reportTime;
            
            // 4. Update Progress
            postProgress(`Finalizing...`);
        }
      };

      // Launch 1 Producer
      const producerPromise = runProducer();
      // Launch N Consumers (4 is a sweet spot for browsers)
      const CONCURRENCY = 4;
      const consumerPromises = [];
      for (let i = 0; i < CONCURRENCY; i++) {
          consumerPromises.push(runConsumer());
      }
      // Wait for EVERYTHING to finish
      const loopStart = performance.now();
      await Promise.all([producerPromise, ...consumerPromises]);
      const loopEnd = performance.now();
      
      console.log(`--- UPLOAD LOOP FINISHED ---`);
      console.log(`Total Loop Activity Time: ${(loopEnd - loopStart).toFixed(2)}ms`);
      console.log(`Cumulative uploadinit (Presigned URL) Time: ${metrics.initTime.toFixed(2)}ms`);
      console.log(`Cumulative PUT Request Time: ${metrics.putTime.toFixed(2)}ms`);
      console.log(`Cumulative chunk_finish (Report Done) Time: ${metrics.reportTime.toFixed(2)}ms`);
      
      const maxTime = Math.max(metrics.initTime, metrics.putTime, metrics.reportTime);
      let slowest = "PUT";
      if (maxTime === metrics.initTime) slowest = "uploadinit (Presigned URL)";
      if (maxTime === metrics.reportTime) slowest = "chunk_finish (Report Done)";
      console.log(`Slowest network operation overall: ${slowest}`);
      
      // If we reach here without errors, the upload is fully complete!
      ctx.postMessage({ type: 'SUCCESS', taskId } as UploadWorkerOutput);

        // for await (const chunk of chunker) {
        //   const { buffer, chunkIndex, isFinal } = chunk;

        //   // if (chunkCompletionArr && chunkCompletionArr[chunkIndex].attempts && chunkCompletionArr[chunkIndex].cloudUploadSuccess == false) {
        //   //   chunkCompletionArr[chunkIndex].attempts++
        //   // }

        //   // if (chunkCompletionArr[chunkIndex] == null) {
        //   //   chunkCompletionArr[chunkIndex] = {
        //   //     attempts: 1,
        //   //     cloudUploadSuccess: false,
        //   //   }
        //   // }

        //   //plaintext + 16 (ciphertext auth tag) + 24 (nonce length)
        //   let declaredChunkSize = buffer.byteLength + 16 + 24

        //   // chunkCompletionArr[chunkIndex].presignedUrl = presignedUrl;

        //   console.log("encrypting")

        //     const nonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
        //     const ad = sodium.from_string(`${nodeId}|${chunkIndex}|${isFinal ? '1' : '0'}`);

        //     const ciphertext = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(buffer, ad, null, nonce, fileKey);

        //     const payload = new Uint8Array(nonce.length + ciphertext.length);
        //     payload.set(nonce);
        //   payload.set(ciphertext, nonce.length);

        //   let chunkHash = await hashUint8Array(payload)

        //   console.log("requesting chunk presigned url")

        //   let presignedUrl = await requestChunkPresignedUrl(uploadId, chunkIndex, declaredChunkSize, chunkHash)
        //   if (presignedUrl == null) {
        //     throw new Error("could not request chunk presigned url")
        //   }

        //   console.log("chunk hash", chunkHash)

        //   let uploadResponse = await fetch(presignedUrl, {
        //     method: 'PUT', body: payload, headers: {
        //       "x-amz-checksum-sha256": chunkHash
        //     }});
        //   if (!uploadResponse.ok) {
        //     ctx.postMessage({ type: 'ERROR', taskId, message: `Presigned URL failed, chunk ${chunkIndex} failed to upload to cloud` } as UploadWorkerOutput);
        //     return;
        //   }

        //   let etag = uploadResponse.headers.get("ETag")

        //   if (etag == null) {
        //     ctx.postMessage({ type: 'ERROR', taskId, message: `Upload chunk failed, chunk ${chunkIndex} failed to response with ETag attached` } as UploadWorkerOutput);
        //     return;
        //   }

        //   etag = etag.substring(1, etag.length-1)

        //   let reportChunkResponse = await reportChunkDone(uploadId, chunkIndex, etag, chunkHash);
        //   if (reportChunkResponse == false) {
        //     ctx.postMessage({ type: 'ERROR', taskId, message: `Report chunk failed, chunk ${chunkIndex} failed to report as done` } as UploadWorkerOutput);
        //     return;
        //   }

        //     completedChunks++;
        //     ctx.postMessage({ type: 'PROGRESS', taskId, completedChunks, totalChunks: totalChunks} as UploadWorkerOutput);
        // }

        // ctx.postMessage({ type: 'SUCCESS', taskId } as UploadWorkerOutput);
    } catch (error: any) {
        ctx.postMessage({ type: 'ERROR', taskId, message: error.message } as UploadWorkerOutput);
    }
};
