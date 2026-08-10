import { create } from 'zustand';

export type UploadStatus = 'IDLE' | 'UPLOADING' | 'SUCCESS' | 'ERROR';

export interface UploadJob {
    id: string;
    file: File;
    progress: number;
    status: UploadStatus;
  uploadId: string;
  nodeId: string;
  totalChunks: number;
  fileKey: Uint8Array;
  errorMessage?: string;
  activity?: string;


  parentNodeId: string;
      encryptedName: string;
      nameNonce: string;
      encryptedNodePassphrase: string;
}

interface UploadStore {
    jobs: UploadJob[];
    addToQueue: (job: Omit<UploadJob, 'id' | 'progress' | 'status'>) => void;
    updateJob: (id: string, updates: Partial<UploadJob>) => void;
    cancelJob: (id: string) => void;
}

export const useUploadStore = create<UploadStore>((set) => ({
    jobs: [],

    addToQueue: (job) => {
        const id = crypto.randomUUID();
        console.log("adding task", id, "to queue", job.file.name);
        set((state) => ({
            jobs: [...state.jobs, {
                ...job, id, progress: 0, status: 'IDLE'
            }]
        }));
    },
    updateJob: (id, updates) => {
        console.log("updating job", id, updates);
        set((state) => ({
            jobs: state.jobs.map(j => j.id === id ? { ...j, ...updates } : j)
        }))
    },
    cancelJob: (id) => {
        console.log("canceling job", id);
        set((state) => ({
            jobs: state.jobs.filter(job => job.id !== id)
        }))
    },
}));
