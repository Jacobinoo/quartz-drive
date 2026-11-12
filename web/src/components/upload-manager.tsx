"use client";

import { useEffect, useRef } from "react";
import { useUploadStore } from "@/hooks/use-upload-store";
import { UploadWorkerOutput } from "@/types/crypto-worker-types";
import { useDriveStore } from "@/lib/driveStore";
import { addSingleSearchItem } from "@/lib/SearchIndexStore";
import { getAccessToken, getCsrfToken } from "@/lib/authStore";

const MAX_CONCURRENT_UPLOADS = 3;

export function UploadManager() {
    const jobs = useUploadStore((s) => s.jobs);
    const updateJob = useUploadStore((s) => s.updateJob);

    const workerInstances = useRef<Map<string, Worker>>(new Map());

    useEffect(() => {
        const currentJobIds = new Set(jobs.map((j) => j.id));
        workerInstances.current.forEach((worker, id) => {
            if (!currentJobIds.has(id)) {
                console.log(`Canceling and terminating worker for job: ${id}`);
                worker.terminate();
                workerInstances.current.delete(id);
            }
        });

        const availableSlots = Math.max(
            0,
            MAX_CONCURRENT_UPLOADS - workerInstances.current.size
        );
        if (availableSlots === 0) return;

        const toStart = jobs
            .filter(
                (j) => j.status === "IDLE" && !workerInstances.current.has(j.id)
            )
            .slice(0, availableSlots);

        toStart.forEach((job) => {
            console.log("worker starting new job", job);
            if (workerInstances.current.has(job.id)) return;

            const worker = new Worker(
                new URL("@/workers/crypto.worker.ts", import.meta.url),
                { type: "module" }
            );
            workerInstances.current.set(job.id, worker);

            const cleanupWorker = () => {
                worker.terminate();
                workerInstances.current.delete(job.id);
            };

            worker.onmessage = async (e: MessageEvent<UploadWorkerOutput>) => {
                const data = e.data;

                switch (data.type) {
                    case "PROGRESS":
                        updateJob(data.taskId, {
                            progress: Math.min(
                                100,
                                Math.round(
                                    (data.completedChunks / data.totalChunks) *
                                        100
                                )
                            ),
                            status: "UPLOADING",
                            activity: data.activity,
                        });
                        break;
                    case "SUCCESS": {
                        cleanupWorker();
                        const finishedJob = useUploadStore
                            .getState()
                            .jobs.find((j) => j.id === data.taskId);

                        if (finishedJob) {
                            try {
                                updateJob(data.taskId, {
                                    status: "SUCCESS",
                                    progress: 100,
                                });
                                useDriveStore.getState().triggerRefresh();
                                await addSingleSearchItem({
                                    id: job.nodeId,
                                    name: job.file.name,
                                    type: "FILE",
                                    sizeBytes: job.file.size,
                                    createdAt: new Date().toISOString(),
                                });
                            } catch (err) {
                                console.error(
                                    "Failed to save metadata to server",
                                    err
                                );
                                updateJob(data.taskId, {
                                    status: "ERROR",
                                    errorMessage: "Server rejected metadata",
                                });
                            }
                        }
                        break;
                    }
                    case "ERROR":
                        cleanupWorker();
                        updateJob(data.taskId, {
                            status: "ERROR",
                            errorMessage: data.message,
                        });
                        break;
                }
            };

            worker.onerror = (err) => {
                console.error("upload worker crashed:", err);
                cleanupWorker();
                updateJob(job.id, {
                    status: "ERROR",
                    errorMessage: "Upload worker failed",
                });
            };

            worker.postMessage({
                taskId: job.id,
                file: job.file,
                nodeId: job.nodeId,
                fileKey: job.fileKey,
                totalChunks: job.totalChunks,
                uploadId: job.uploadId,
                accessToken: getAccessToken() || undefined,
                csrfToken: getCsrfToken() || undefined,
            });
        });
    }, [jobs, updateJob]);

    useEffect(() => {
        return () => {
            workerInstances.current.forEach((w) => {
                w.terminate();
            });
        };
    }, []);

    return null;
}
