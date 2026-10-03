import { create } from "zustand";

export type DownloadStatus = "IDLE" | "DOWNLOADING" | "SUCCESS" | "ERROR";

export interface DownloadJob {
    id: string;
    file: any;
    progress: number;
    status: DownloadStatus;
    nodeId: string;
    fileKey: Uint8Array;
    errorMessage?: string;
    activity?: string;
}

interface DownloadStore {
    jobs: DownloadJob[];
    addToQueue: (job: Omit<DownloadJob, "id" | "progress" | "status">) => void;
    updateJob: (id: string, updates: Partial<DownloadJob>) => void;
    cancelJob: (id: string) => void;
}

export const useDownloadStore = create<DownloadStore>((set) => ({
    jobs: [],

    addToQueue: (job) => {
        const id = crypto.randomUUID();
        console.log(
            "adding download task",
            id,
            "to queue",
            job.file.plaintextName
        );
        set((state) => ({
            jobs: [
                ...state.jobs,
                {
                    ...job,
                    id,
                    progress: 0,
                    status: "IDLE",
                },
            ],
        }));
    },
    updateJob: (id, updates) => {
        console.log("updating download job", id, updates);
        set((state) => ({
            jobs: state.jobs.map((j) =>
                j.id === id ? { ...j, ...updates } : j
            ),
        }));
    },
    cancelJob: (id) => {
        console.log("canceling download job", id);
        set((state) => ({
            jobs: state.jobs.filter((job) => job.id !== id),
        }));
    },
}));
