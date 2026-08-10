"use client";

import React, { useState, useEffect, useRef } from "react";
import MDEditor from '@uiw/react-md-editor';
import rehypeSanitize from "rehype-sanitize";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogClose,
} from "@/components/ui/dialog";
import { useDriveStore } from "@/lib/driveStore";
import { getSodium } from "@/lib/crypto/sodium";
import { initUpdateFile } from "@/crypto/upload";
import { useUploadStore } from "@/hooks/use-upload-store";
import { getDownloadUrls } from "@/crypto/files";
import { Loader2, X } from "lucide-react";

export function NoteEditorModal({ file, onClose }: { file: any; onClose: () => void }) {
    const [content, setContent] = useState<string>("");
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [saveStatus, setSaveStatus] = useState<"Saved" | "Saving..." | "Error">("Saved");
    const [dots, setDots] = useState("");
    
    const initialContentRef = useRef("");
    const saveTimeoutRef = useRef<NodeJS.Timeout | null>(null);
    const activeUploadIdRef = useRef<string | null>(null);
    
    const jobs = useUploadStore(s => s.jobs);

    useEffect(() => {
        if (!activeUploadIdRef.current) return;
        const job = jobs.find(j => j.uploadId === activeUploadIdRef.current);
        if (!job) return;

        if (job.status === 'UPLOADING') {
            setSaveStatus("Saving...");
        } else if (job.status === 'SUCCESS') {
            setSaveStatus("Saved");
            activeUploadIdRef.current = null;
        } else if (job.status === 'ERROR') {
            setSaveStatus("Error");
            activeUploadIdRef.current = null;
        }
    }, [jobs]);

    useEffect(() => {
        if (saveStatus === "Saving...") {
            const id = setInterval(() => {
                setDots(d => d.length >= 3 ? "" : d + ".");
            }, 300);
            return () => clearInterval(id);
        } else {
            setDots("");
        }
    }, [saveStatus]);

    // 1. Download and decrypt the note content on mount
    useEffect(() => {
        let isMounted = true;
        const loadNote = async () => {
            try {
                setLoading(true);
                const sodium = await getSodium();
                const currentFolder = useDriveStore.getState().getCurrentFolder();
                if (!currentFolder) throw new Error("Drive keys not initialized");

                const fileKey = sodium.crypto_box_seal_open(
                    sodium.from_base64(file.encryptedNodePassphrase),
                    currentFolder.publicKey,
                    currentFolder.privateKey
                );

                if (!fileKey) throw new Error("Failed to unwrap fileKey!");

                const urls = await getDownloadUrls(file.nodeId);
                const worker = new Worker(new URL('@/workers/decrypt.worker.ts', import.meta.url));

                worker.postMessage({ urls, fileKey, nodeId: file.nodeId });

                worker.onmessage = async (e) => {
                    if (e.data.type === 'SUCCESS') {
                        if (!isMounted) return;
                        const text = await e.data.blob.text();
                        setContent(text);
                        initialContentRef.current = text;
                        setLoading(false);
                        worker.terminate();
                    } else if (e.data.type === 'ERROR') {
                        console.error("Worker error:", e.data.message);
                        worker.terminate();
                        if (isMounted) setLoading(false);
                    }
                };
            } catch (err) {
                console.error("Failed to load note:", err);
                if (isMounted) setLoading(false);
            }
        };

        if (file && file.type === 'FILE' && file.plaintextName.endsWith('.md')) {
            loadNote();
        } else {
            onClose(); // Close if not a markdown file
        }

        return () => { isMounted = false; };
    }, [file?.nodeId]);

    // 2. Debounced save logic
    const saveNote = async (newText: string) => {
        if (newText === initialContentRef.current) return; // No changes
        
        try {
            setSaveStatus("Saving...");
            const sodium = await getSodium();
            const currentFolder = useDriveStore.getState().getCurrentFolder();
            if (!currentFolder) throw new Error("Drive keys not initialized");

            const fileKey = sodium.crypto_box_seal_open(
                sodium.from_base64(file.encryptedNodePassphrase),
                currentFolder.publicKey,
                currentFolder.privateKey
            );

            if (!fileKey) throw new Error("Failed to unwrap fileKey!");

            const newFile = new File([newText], file.plaintextName, { type: "text/markdown" });
            const declaredFileSize = newFile.size + 40; // 16 bytes auth tag + 24 bytes nonce
            const chunks = Math.ceil(newFile.size / (4 * 1024 * 1024)) || 1;

            // Update metadata with new size
            const metadata = {
                mimeType: "text/markdown",
                lastModified: newFile.lastModified,
                originalSizeBytes: newFile.size,
                fileExtension: "md"
            };
            const metadataJson = JSON.stringify(metadata);
            const metadataNonce = sodium.randombytes_buf(sodium.crypto_aead_xchacha20poly1305_ietf_NPUBBYTES);
            const encryptedMetadata = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt(
                sodium.from_string(metadataJson),
                null, null, metadataNonce, fileKey
            );

            const res = await initUpdateFile({
                totalFileSize: declaredFileSize,
                totalChunks: chunks,
                nodeId: file.nodeId,
                encryptedMetadata: sodium.to_base64(encryptedMetadata),
                metadataNonce: sodium.to_base64(metadataNonce),
            });

            if (!res) throw new Error("Init update failed");

            // Dispatch to the global upload worker via Zustand
            useUploadStore.getState().addToQueue({
                file: newFile,
                uploadId: res.uploadId,
                nodeId: res.nodeId,
                fileKey: fileKey,
                totalChunks: chunks,
                isHidden: true, // Hide from global transfer list
                
                // These are required by UploadJob type but ignored by the crypto.worker for updates
                parentNodeId: "",
                encryptedName: "",
                nameNonce: "",
                encryptedNodePassphrase: ""
            });

            activeUploadIdRef.current = res.uploadId;
            initialContentRef.current = newText;
        } catch (err) {
            console.error("Save failed:", err);
            setSaveStatus("Error");
        }
    };

    const handleChange = (val?: string) => {
        const text = val || "";
        setContent(text);
        
        if (saveTimeoutRef.current) {
            clearTimeout(saveTimeoutRef.current);
        }
        
        setSaveStatus("Saving...");
        saveTimeoutRef.current = setTimeout(() => {
            saveNote(text);
        }, 1500); // 1.5s debounce
    };

    return (
        <Dialog open={true} onOpenChange={(open) => !open && onClose()}>
            <DialogContent showCloseButton={false} className="!max-w-[100vw] sm:!max-w-[100vw] !w-screen !h-[100dvh] flex flex-col p-0 gap-0 border-0 !rounded-none">
                <DialogHeader className="p-4 border-b border-border/40 flex-row justify-between items-center space-y-0">
                    <DialogTitle className="text-lg font-medium">{file?.plaintextName}</DialogTitle>
                    <div className="flex items-center gap-4">
                        <div className="text-sm font-medium">
                            {saveStatus === "Saving..." && (
                                <span className="inline-flex items-center text-yellow-600 bg-yellow-100 px-2 py-1 rounded-md w-28">
                                    Uploading{dots}
                                </span>
                            )}
                            {saveStatus === "Saved" && (
                                <span className="inline-flex items-center text-green-600 bg-green-100 px-2 py-1 rounded-md">
                                    Progress Saved
                                </span>
                            )}
                            {saveStatus === "Error" && (
                                <span className="inline-flex items-center text-red-600 bg-red-100 px-2 py-1 rounded-md">
                                    Error Saving
                                </span>
                            )}
                        </div>
                        <DialogClose className="rounded-xs opacity-70 cursor-pointer ring-offset-background transition-opacity hover:opacity-100 focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:outline-hidden disabled:pointer-events-none data-[state=open]:bg-accent data-[state=open]:text-muted-foreground">
                            <X className="w-5 h-5" />
                            <span className="sr-only">Close</span>
                        </DialogClose>
                    </div>
                </DialogHeader>
                
                <div className="flex-1 overflow-hidden" data-color-mode="light">
                    {loading ? (
                        <div className="flex w-full h-full items-center justify-center">
                            <Loader2 className="w-8 h-8 animate-spin text-muted-foreground" />
                        </div>
                    ) : (
                        <MDEditor
                            value={content}
                            onChange={handleChange}
                            previewOptions={{
                                rehypePlugins: [[rehypeSanitize]],
                            }}
                            height="100%"
                            className="h-full rounded-none border-0"
                            textareaProps={{
                                placeholder: 'Start writing your note here...'
                            }}
                        />
                    )}
                </div>
            </DialogContent>
        </Dialog>
    );
}
