'use client';

import { getSodium } from '@/lib/crypto/sodium';
import { useEffect, useRef } from 'react';
import { useUploadStore } from '@/hooks/use-upload-store';
import {UploadWorkerInput, UploadWorkerOutput} from "@/types/crypto-worker-types";
import { finishFileUpload } from '@/crypto/upload';

export function UploadManager() {
    const jobs = useUploadStore(s => s.jobs);
    const updateJob = useUploadStore(s => s.updateJob);
    const cancelJob = useUploadStore(s => s.cancelJob);


    const workerInstances = useRef<Map<string, Worker>>(new Map());

    useEffect(() => {
        const pending = jobs.filter(j => j.status === 'IDLE');

        pending.forEach(job => {
            if (workerInstances.current.has(job.id)) return;

            const worker = new Worker(new URL('@/workers/crypto.worker.ts', import.meta.url), { type: 'module' });
            workerInstances.current.set(job.id, worker);

            worker.onmessage = async (e: MessageEvent<UploadWorkerOutput>) => {
                const data = e.data;

                switch(data.type) {
                    case "PROGRESS":
                        updateJob(data.taskId, { progress: Math.round((data.completedChunks / data.totalChunks) * 100), status: 'UPLOADING' });
                        break;
                  case "SUCCESS":
                    // 1. Find the job in the store
                        const job = useUploadStore.getState().jobs.find(j => j.id === data.taskId);

                        if (job) {
                          try {
                            const sodium = await getSodium();

                            const metadata = {
                                            mimeType: job.file.type || "application/octet-stream",
                                            lastModified: job.file.lastModified,
                                            originalSizeBytes: job.file.size,
                                            fileExtension: job.file.name.split('.').pop() || ""
                                        };
                            const metadataJson = JSON.stringify(metadata);

                            const metadataNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
                                        const encryptedMetadata = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
                                            sodium.from_string(metadataJson),
                                            null,
                                            null,
                                            metadataNonce,
                                            job.fileKey // The exact key that protects the file contents!
                                        );
                                // 2. Call our new backend endpoint!
                                await finishFileUpload({
                                    nodeId: job.nodeId,
                                    parentNodeId: job.parentNodeId,
                                    sizeBytes: job.file.size,
                                    totalChunks: job.presignedUrls.length,
                                    encryptedName: job.encryptedName,
                                    nameNonce: job.nameNonce,
                                    encryptedNodePassphrase: job.encryptedNodePassphrase,
                                    signedEncryptedNodePassphrase: "", // Signature implementation later
                                    chunkNonces: [], // If worker doesn't report nonces, keep empty for now
                                  chunkSizes: [],

                                  encryptedMetadata: sodium.to_base64(encryptedMetadata),
                                  metadataNonce: sodium.to_base64(metadataNonce)
                                });
                                console.log(`Successfully saved ${job.file.name} metadata to Postgres!`);
                                updateJob(data.taskId, { status: 'SUCCESS', progress: 100 });
                            } catch (err) {
                                console.error("Failed to save metadata to server", err);
                                updateJob(data.taskId, { status: 'ERROR', errorMessage: "Server rejected metadata" });
                            }
                        }
                        worker.terminate();
                        workerInstances.current.delete(data.taskId);
                        break;
                    case "ERROR":
                        updateJob(data.taskId, { status: 'ERROR', errorMessage: data.message });
                        worker.terminate();
                        workerInstances.current.delete(data.taskId);
                        break;
                }
            };

            worker.postMessage({
                taskId: job.id,
                file: job.file,
                nodeId: job.nodeId,
                fileKey: job.fileKey,
                presignedUrls: job.presignedUrls
            });
        });

        const currentJobIds = new Set(jobs.map(j => j.id));
        workerInstances.current.forEach((worker, id) => {
            if (!currentJobIds.has(id)) {
                console.log(`Canceling and terminating worker for job: ${id}`);
                worker.terminate();
                workerInstances.current.delete(id);
            }
        });

    }, [jobs, updateJob]);

    return null;
}
