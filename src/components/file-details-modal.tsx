"use client";

import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { formatBytes } from "@/lib/utils/size";
import { Info, FileIcon, FolderIcon } from "lucide-react";

export function FileDetailsModal({ file, breadcrumbs, onClose }: { file: any, breadcrumbs: any[], onClose: () => void }) {
    if (!file) return null;

    // Helper to map MIME types to friendly names
    const MIME_TYPE_MAP: Record<string, string> = {
        // Archives & Binaries
        'application/octet-stream': 'Binary File',
        'application/x-freearc': 'Archive Document',
        'application/x-bzip': 'BZip Archive',
        'application/x-bzip2': 'BZip2 Archive',
        'application/gzip': 'GZip Archive',
        'application/x-gzip': 'GZip Archive', // Windows/macOS quirk
        'application/java-archive': 'Java Archive (JAR)',
        'application/vnd.rar': 'RAR Archive',
        'application/x-tar': 'TAR Archive',
        'application/zip': 'ZIP Archive',
        'application/x-zip-compressed': 'ZIP Archive', // Windows quirk
        'application/x-7z-compressed': '7-Zip Archive',
        'application/vnd.apple.installer+xml': 'Apple Installer Package',

        // Documents & Text
        'application/pdf': 'PDF Document',
        'application/msword': 'Microsoft Word',
        'application/vnd.openxmlformats-officedocument.wordprocessingml.document': 'Microsoft Word (OpenXML)',
        'application/vnd.ms-excel': 'Microsoft Excel',
        'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet': 'Microsoft Excel (OpenXML)',
        'application/vnd.ms-powerpoint': 'Microsoft PowerPoint',
        'application/vnd.openxmlformats-officedocument.presentationml.presentation': 'Microsoft PowerPoint (OpenXML)',
        'application/vnd.visio': 'Microsoft Visio',
        'application/vnd.oasis.opendocument.presentation': 'OpenDocument Presentation',
        'application/vnd.oasis.opendocument.spreadsheet': 'OpenDocument Spreadsheet',
        'application/vnd.oasis.opendocument.text': 'OpenDocument Text',
        'application/rtf': 'Rich Text Format (RTF)',
        'application/vnd.amazon.ebook': 'Amazon Kindle eBook',
        'application/epub+zip': 'EPUB',
        'text/plain': 'Text Document',
        'text/csv': 'CSV File',
        'text/html': 'HTML Document',
        'application/xhtml+xml': 'XHTML',
        'text/css': 'CSS',
        'text/javascript': 'JavaScript',
        'application/json': 'JSON',
        'application/ld+json': 'JSON-LD',
        'application/manifest+json': 'Web Application Manifest',
        'text/markdown': 'Markdown',
        'application/xml': 'XML',
        'text/xml': 'XML',
        'application/vnd.mozilla.xul+xml': 'XUL',
        'text/calendar': 'iCalendar',

        // Scripts
        'application/x-csh': 'C-Shell Script',
        'application/x-sh': 'Bourne Shell Script',
        'application/x-httpd-php': 'PHP Script',

        // Images
        'image/apng': 'APNG Image',
        'image/avif': 'AVIF Image',
        'image/bmp': 'BMP Image',
        'image/gif': 'GIF Image',
        'image/vnd.microsoft.icon': 'Icon',
        'image/jpeg': 'JPEG Image',
        'image/png': 'PNG Image',
        'image/svg+xml': 'SVG Image',
        'image/tiff': 'TIFF Image',
        'image/webp': 'WEBP Image',

        // Audio
        'audio/aac': 'AAC Audio',
        'application/x-cdf': 'CD Audio',
        'audio/midi': 'MIDI Audio',
        'audio/x-midi': 'MIDI Audio',
        'audio/mp4': 'MP4 Audio',
        'audio/mpeg': 'MP3 Audio',
        'audio/ogg': 'Ogg Audio',
        'audio/wav': 'WAV Audio',
        'audio/webm': 'WEBM Audio',
        'audio/3gpp': '3GPP Audio',
        'audio/3gpp2': '3GPP2 Audio',

        // Video & Media
        'video/x-msvideo': 'AVI Video',
        'video/mp4': 'MP4 Video',
        'video/mpeg': 'MPEG Video',
        'video/ogg': 'Ogg Video',
        'application/ogg': 'Ogg Container',
        'video/mp2t': 'MPEG Transport Stream',
        'video/webm': 'WEBM Video',
        'video/3gpp': '3GPP Video',
        'video/3gpp2': '3GPP2 Video',

        // Fonts
        'application/vnd.ms-fontobject': 'MS Embedded OpenType Font',
        'font/otf': 'OpenType Font',
        'font/ttf': 'TrueType Font',
        'font/woff': 'WOFF Font',
        'font/woff2': 'WOFF2 Font'
    };

    const getFriendlyType = (mimeType: string): string => {
        if (!mimeType) return 'Binary File';

        // 1. Check exact dictionary match
        const exactMatch = MIME_TYPE_MAP[mimeType.toLowerCase()];
        if (exactMatch) return exactMatch;

        // 2. Dynamic Fallbacks for generic/unknown types
        if (mimeType.startsWith('image/')) {
            return `${mimeType.split('/')[1].toUpperCase()} Image`;
        }
        if (mimeType.startsWith('video/')) {
            return `${mimeType.split('/')[1].toUpperCase()} Video`;
        }
        if (mimeType.startsWith('audio/')) {
            return `${mimeType.split('/')[1].toUpperCase()} Audio`;
        }
        if (mimeType.startsWith('text/')) {
            return `${mimeType.split('/')[1].toUpperCase()} File`;
        }

        // 3. Absolute fallback
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
    const formattedEncryptedSize = file.sizeBytes !== undefined ? formatBytes(file.sizeBytes) : '--';
    const formattedOriginalSize = file.metadata?.originalSizeBytes !== undefined ? formatBytes(file.metadata.originalSizeBytes) : '--';

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
                            <div className="col-span-2 text-gray-900 dark:text-gray-200 mt-4">{formattedOriginalSize}</div>

                            <div className="text-gray-500 font-medium col-span-1">Encrypted Size</div>
                            <div className="col-span-2 text-gray-900 dark:text-gray-200 flex items-center gap-2">
                                {formattedEncryptedSize}
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
