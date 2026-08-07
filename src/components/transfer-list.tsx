"use client";

import { useUploadStore } from "@/hooks/use-upload-store";
import { Button } from "@/components/ui/button";
import { X } from "lucide-react";

export function TransferList() {
    const jobs = useUploadStore(s=>s.jobs);
    const cancelJob = useUploadStore(s=>s.cancelJob);

    if (jobs.length === 0) return null;

    return (
        <div className="fixed bottom-4 right-4 w-80 bg-background border rounded-lg shadow-lg p-4">
            <h3 className="font-bold mb-4">Transfery ({jobs.length})</h3>

            <div className="flex flex-col gap-3 max-h-64 overflow-y-auto">
                {jobs.map((job) => (
                    <div key={job.id} className="text-sm">
                        <div className="flex justify-between items-center mb-1">
                            <span className="truncate pr-2 font-medium">{job.file.name}</span>
                            <div className="flex items-center gap-2">
                                <span className="text-xs text-muted-foreground">{Math.round(job.progress)}%</span>
                                <Button 
                                    variant="ghost" 
                                    size="icon" 
                                    className="h-5 w-5 rounded-full hover:bg-red-500/10 hover:text-red-500" 
                                    onClick={() => cancelJob(job.id)}
                                    title="Cancel upload"
                                >
                                    <X className="h-3 w-3" />
                                </Button>
                            </div>
                        </div>

                        <div className="w-full bg-muted rounded-full h-2">
                            <div
                                className={`h-2 rounded-full transition-all ${
                                    job.status === 'SUCCESS' ? 'bg-green-500' :
                                        job.status === 'ERROR' ? 'bg-red-500' :
                                            job.status === 'UPLOADING' ? 'bg-yellow-500' : 'bg-blue-600'
                                }`}
                                style={{ width: `${job.progress}%` }}
                            />
                        </div>
                        <p className="text-xs text-muted-foreground mt-1">
                            {job.status === 'SUCCESS' ? "Upload successful" : 
                             job.status === 'UPLOADING' ? (job.activity || "Uploading...") : 
                             job.status === 'IDLE' ? "Waiting for upload..." : 
                             job.status === 'ERROR' ? "Upload error" : "Status: Unknown"}
                        </p>
                    </div>
                ))}
            </div>
        </div>
    );
}