"use client";

import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Info, FileIcon, FolderIcon } from "lucide-react";

export function FileDetailsModal({ file, breadcrumbs, onClose }: { file: any, breadcrumbs: any[], onClose: () => void }) {
    if (!file) return null;

    // Helper to map MIME types to friendly names
    const getFriendlyType = (mimeType: string) => {
        if (!mimeType || mimeType === 'application/octet-stream') return "Binary File";
        if (mimeType.startsWith('image/')) return `${mimeType.split('/')[1].toUpperCase()} Image`;
        if (mimeType === 'application/zip') return "ZIP Archive";
        if (mimeType === 'application/pdf') return "PDF Document";
        return mimeType;
    };

    const locationString = breadcrumbs.map(b => b.name).join(" / ");

    // Parse Dates natively
    const formatDate = (dateValue: string | number) => {
        if (!dateValue) return "Unknown";
        return new Intl.DateTimeFormat('en-US', {
            year: 'numeric',
            month: 'short',
            day: 'numeric',
            hour: 'numeric',
            minute: 'numeric',
        }).format(new Date(dateValue));
    };

    const uploadedAt = formatDate(file.createdAt);
    const modifiedAt = formatDate(file.metadata?.lastModified);

    // Parse Sizes
    const encryptedSizeMb = file.sizeBytes ? (file.sizeBytes / 1024 / 1024).toFixed(2) : '--';
    const originalSizeMb = file.metadata?.originalSizeBytes ? (file.metadata.originalSizeBytes / 1024 / 1024).toFixed(2) : '--';

    return (
        <Dialog open={!!file} onOpenChange={(open) => !open && onClose()}>
            <DialogContent className="sm:max-w-md">
                <DialogHeader>
                    <DialogTitle className="flex items-center gap-2 border-b pb-4">
                        {file.type === 'FOLDER' ? <FolderIcon className="text-blue-500 w-6 h-6" /> : <FileIcon className="text-gray-500 w-6 h-6" />}
                        <span className="truncate">{file.plaintextName}</span>
                    </DialogTitle>
                </DialogHeader>

                <div className="grid grid-cols-3 gap-y-4 gap-x-4 text-sm mt-4">
                    <div className="text-gray-500 font-medium col-span-1">Location</div>
                    <div className="col-span-2 text-gray-900 dark:text-gray-200">{locationString}</div>

                    <div className="text-gray-500 font-medium col-span-1">Created By</div>
                    <div className="col-span-2 text-gray-900 dark:text-gray-200">Me</div>

                    <div className="text-gray-500 font-medium col-span-1">Uploaded At</div>
                    <div className="col-span-2 text-gray-900 dark:text-gray-200">{uploadedAt}</div>

                    <div className="text-gray-500 font-medium col-span-1">Modified At</div>
                    <div className="col-span-2 text-gray-900 dark:text-gray-200">{modifiedAt}</div>

                    {file.type === 'FILE' && (
                        <>
                            <div className="text-gray-500 font-medium col-span-1">Type</div>
                            <div className="col-span-2 text-gray-900 dark:text-gray-200">{getFriendlyType(file.metadata?.mimeType)}</div>

                            <div className="text-gray-500 font-medium col-span-1">File Type</div>
                            <div className="col-span-2 text-gray-500 font-mono text-xs mt-0.5">{file.metadata?.mimeType || 'application/octet-stream'}</div>

                            <div className="text-gray-500 font-medium col-span-1 mt-4">Original Size</div>
                            <div className="col-span-2 text-gray-900 dark:text-gray-200 mt-4">{originalSizeMb} MB</div>

                            <div className="text-gray-500 font-medium col-span-1">Encrypted Size</div>
                            <div className="col-span-2 text-gray-900 dark:text-gray-200 flex items-center gap-2">
                                {encryptedSizeMb} MB
                                <TooltipProvider>
                                    <Tooltip>
                                        <TooltipTrigger asChild>
                                            <Info className="w-4 h-4 text-gray-400 hover:text-blue-500 cursor-pointer transition-colors" />
                                        </TooltipTrigger>
                                        <TooltipContent side="right">
                                            <p className="w-[200px] text-xs">
                                              Encrypted files use more space due to the encryption process and signatures that guarantee the security of the data.
                                            </p>
                                        </TooltipContent>
                                    </Tooltip>
                                </TooltipProvider>
                            </div>
                        </>
                    )}
                </div>

                {/* ADVANCED DETAILS SECTION */}
                {file.type === 'FILE' && (
                    <div className="mt-4 border-t pt-4">
                        <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-3">Advanced Details</h4>
                        <div className="grid grid-cols-3 gap-y-2 text-sm">
                            <div className="text-gray-500 font-medium col-span-1">Raw Encrypted</div>
                            <div className="col-span-2 text-gray-900 dark:text-gray-200 font-mono text-xs">
                                {file.sizeBytes ? file.sizeBytes.toLocaleString() : '--'} bytes
                            </div>

                            <div className="text-gray-500 font-medium col-span-1">Raw Original</div>
                            <div className="col-span-2 text-gray-900 dark:text-gray-200 font-mono text-xs">
                                {file.metadata?.originalSizeBytes ? file.metadata.originalSizeBytes.toLocaleString() : '--'} bytes
                            </div>
                        </div>
                    </div>
                )}
            </DialogContent>
        </Dialog>
    );
}
