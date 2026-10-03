"use client";

import { useEffect, useRef } from "react";
import { useDownloadStore } from "@/hooks/use-download-store";
import { getDownloadUrls } from "@/crypto/files";

export function DownloadManager() {
    const jobs = useDownloadStore((s) => s.jobs);
    const updateJob = useDownloadStore((s) => s.updateJob);

    const workerInstances = useRef<Map<string, Worker>>(new Map());

    useEffect(() => {
        const pending = jobs.filter((j) => j.status === "IDLE");

        pending.forEach(async (job) => {
            if (workerInstances.current.has(job.id)) return;

            updateJob(job.id, {
                status: "DOWNLOADING",
                activity: "Fetching URLs...",
            });

            try {
                const { urls, encryptingNodeId } = await getDownloadUrls(
                    job.nodeId
                );
                const decryptNodeId = encryptingNodeId || job.nodeId;

                const worker = new Worker(
                    new URL("@/workers/decrypt.worker.ts", import.meta.url),
                    { type: "module" }
                );
                workerInstances.current.set(job.id, worker);

                worker.onmessage = async (e: MessageEvent<any>) => {
                    const data = e.data;

                    switch (data.type) {
                        case "PROGRESS":
                            updateJob(job.id, {
                                progress: data.percent,
                                status: "DOWNLOADING",
                                activity: "Decrypting...",
                            });
                            break;
                        case "SUCCESS":
                            updateJob(job.id, {
                                status: "SUCCESS",
                                progress: 100,
                                activity: "Done",
                            });

                            const blob = data.blob;

                            const downloadUrl = URL.createObjectURL(blob);

                            const a = document.createElement("a");
                            a.href = downloadUrl;
                            a.download = job.file.plaintextName;
                            a.target = "_blank";
                            document.body.appendChild(a);
                            a.click();

                            a.remove();
                            URL.revokeObjectURL(downloadUrl);

                            worker.terminate();
                            workerInstances.current.delete(job.id);
                            break;
                        case "ERROR":
                            updateJob(job.id, {
                                status: "ERROR",
                                errorMessage: data.message,
                            });
                            worker.terminate();
                            workerInstances.current.delete(job.id);
                            break;
                    }
                };

                worker.onerror = (err) => {
                    updateJob(job.id, {
                        status: "ERROR",
                        errorMessage: err.message,
                    });
                    worker.terminate();
                    workerInstances.current.delete(job.id);
                };

                worker.postMessage({
                    urls,
                    fileKey: job.fileKey,
                    nodeId: decryptNodeId,
                });
            } catch (err: any) {
                updateJob(job.id, {
                    status: "ERROR",
                    errorMessage: err.message,
                });
            }
        });

        const currentJobIds = new Set(jobs.map((j) => j.id));
        workerInstances.current.forEach((worker, id) => {
            if (!currentJobIds.has(id)) {
                console.log(
                    `Canceling and terminating worker for download job: ${id}`
                );
                worker.terminate();
                workerInstances.current.delete(id);
            }
        });
    }, [jobs, updateJob]);

    return null;
}
