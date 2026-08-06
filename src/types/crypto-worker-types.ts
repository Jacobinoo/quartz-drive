export interface UploadWorkerInput {
    taskId: string;
    file: File;
  nodeId: string;
  uploadId: string;
  fileKey: string;
  totalChunks: number;
  accessToken?: string;
  csrfToken?: string;
}


export type UploadWorkerOutput =
    | {
        type: 'PROGRESS';
        taskId: string;
        completedChunks: number;
        totalChunks: number;
        activity?: string;
    }
    | { type: 'SUCCESS', taskId: string }
    | { type: 'ERROR', taskId: string, message: string };
