export interface UploadWorkerInput {
    taskId: string;
    file: File;
    nodeId: string;
    fileKey: string;
    presignedUrls: string[];
}


export type UploadWorkerOutput =
    | {
        type: 'PROGRESS';
        taskId: string;
        completedChunks: number;
        totalChunks: number;

    }
    | { type: 'SUCCESS', taskId: string }
    | { type: 'ERROR', taskId: string, message: string };
