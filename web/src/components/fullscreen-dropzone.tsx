"use client";

import { useEffect, useRef, useState } from "react";
import { CloudUpload } from "lucide-react";
import { useDriveStore } from "@/lib/driveStore";
import { useUploadStore } from "@/hooks/use-upload-store";
import { enqueueFileUploads } from "@/lib/upload/enqueue-files";

export function FullscreenDropzone() {
    const addToQueue = useUploadStore((s) => s.addToQueue);
    const [isDragging, setIsDragging] = useState(false);
    const dragDepth = useRef(0);

    const currentFolder = useDriveStore((s) => s.getCurrentFolder());

    useEffect(() => {
        const handleDragEnter = (e: DragEvent) => {
            if (!isExternalFileDrag(e)) return;
            e.preventDefault();
            dragDepth.current += 1;
            setIsDragging(true);
        };

        const handleDragOver = (e: DragEvent) => {
            if (!isExternalFileDrag(e) || !e.dataTransfer) return;
            e.preventDefault();
            e.dataTransfer.dropEffect = "copy";
        };

        const handleDragLeave = (e: DragEvent) => {
            if (!isExternalFileDrag(e)) return;
            dragDepth.current = Math.max(0, dragDepth.current - 1);
            if (dragDepth.current === 0) {
                setIsDragging(false);
            }
        };

        const handleDrop = (e: DragEvent) => {
            dragDepth.current = 0;
            setIsDragging(false);
            if (!isExternalFileDrag(e)) return;
            e.preventDefault();

            const files = extractFilesOnly(e.dataTransfer);
            if (files.length === 0) return;

            const activeFolder = useDriveStore.getState().getCurrentFolder();
            if (!activeFolder) return;

            enqueueFileUploads(files, addToQueue, activeFolder);
        };

        window.addEventListener("dragenter", handleDragEnter);
        window.addEventListener("dragover", handleDragOver);
        window.addEventListener("dragleave", handleDragLeave);
        window.addEventListener("drop", handleDrop);

        return () => {
            window.removeEventListener("dragenter", handleDragEnter);
            window.removeEventListener("dragover", handleDragOver);
            window.removeEventListener("dragleave", handleDragLeave);
            window.removeEventListener("drop", handleDrop);
        };
    }, []);

    if (!isDragging) return null;

    return (
        <div className="z-100 bg-black/60 backdrop-blur-xs fixed inset-0 p-7 pointer-events-none">
            <div className="border-2 border-white dark:border-gray-300 border-dashed w-full h-full rounded-xl bg-blue-400/20 dark:bg-blue-500/20 flex flex-col justify-center items-center gap-2">
                <CloudUpload size={80} className="text-white" />
                <p className="text-2xl font-bold text-white">
                    Upload files
                    {currentFolder ? ` to ${currentFolder.name}` : ""}
                </p>
                <p className="text-sm font-medium text-gray-100">
                    Files will be encrypted before uploading
                </p>
            </div>
        </div>
    );
}

function isExternalFileDrag(e: DragEvent): boolean {
    if (useDriveStore.getState().draggedItem) return false;
    return Array.from(e.dataTransfer?.types ?? []).includes("Files");
}

function extractFilesOnly(dataTransfer: DataTransfer | null): File[] {
    if (!dataTransfer) return [];
    if (dataTransfer.items?.length) {
        const files: File[] = [];
        for (const item of Array.from(dataTransfer.items)) {
            if (item.kind !== "file") continue;
            if (item.webkitGetAsEntry?.()?.isDirectory) continue;
            const file = item.getAsFile();
            if (file) files.push(file);
        }
        return files;
    }
    return Array.from(dataTransfer.files ?? []);
}
