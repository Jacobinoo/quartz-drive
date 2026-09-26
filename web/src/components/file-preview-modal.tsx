"use client";

import { useEffect, useState } from "react";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import {Download, X, ZoomIn, ZoomOut, RotateCw, Info, ShieldCheck, ShieldAlert} from "lucide-react";
import { formatBytes } from "@/lib/utils/size";
import {Tooltip, TooltipContent, TooltipProvider, TooltipTrigger} from "@/components/ui/tooltip";

interface FilePreviewModalProps {
  file: any;
  url: string | null;
  loading?: boolean;
  progress?: number;
  tooLarge?: boolean;
  unsupported?: boolean;
  onClose: () => void;
  onDownload?: () => void;
  breadcrumbs: any[]
}

export function FilePreviewModal({ file, url, loading, progress, tooLarge, unsupported, onClose, onDownload, breadcrumbs }: FilePreviewModalProps) {
  const [zoom, setZoom] = useState(1);
  const [rotation, setRotation] = useState(0);
  const [loaded, setLoaded] = useState(false);
  const [showLoader, setShowLoader] = useState(false);
  const [showInfo, setShowInfo] = useState(false);

  // Reset transient view state whenever a new file is opened
  useEffect(() => {
    setZoom(1);
    setRotation(0);
    setLoaded(false);
  }, [url]);

  // Delay the loading spinner to prevent flickering on fast files
  useEffect(() => {
    let timer: NodeJS.Timeout;
    if (loading) {
      timer = setTimeout(() => setShowLoader(true), 1000);
    } else {
      setShowLoader(false);
    }
    return () => clearTimeout(timer);
  }, [loading]);

  // Keyboard shortcuts: Esc to close, +/- to zoom, r to rotate, i for info
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key === "+" || e.key === "=") setZoom((z) => Math.min(z + 0.25, 4));
      if (e.key === "-") setZoom((z) => Math.max(z - 0.25, 0.25));
      if (e.key === "r" || e.key === "R") setRotation((r) => (r + 90) % 360);
      if (e.key === "i" || e.key === "I") setShowInfo((s) => !s);
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  if (!file || (!url && !loading && !tooLarge && !unsupported)) return null;

  const isImage = /\.(png|jpe?g|gif|webp|avif|svg)$/i.test(file.plaintextName ?? "");

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


  const handleDownloadClick = () => {
    if (onDownload) {
      onDownload();
      onClose(); // Auto-close modal after starting download
      return;
    }
    if (!url) return;
    const a = document.createElement("a");
    a.href = url;
    a.download = file.plaintextName;
    document.body.appendChild(a);
    a.click();
    a.remove();
  };

  return (
    <Dialog open={!!file} onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        showCloseButton={false}
        className="max-w-none w-screen h-screen sm:max-w-none p-0 gap-0 border-none bg-[#0b0b0c] rounded-none flex flex-col overflow-hidden"
      >
        {/* Top bar */}
        <div className="flex items-center justify-between px-5 h-14 shrink-0 border-b border-white/10 bg-[#0b0b0c]/95 backdrop-blur-sm z-10">
          <div className="flex items-center gap-3 min-w-0">
            <span className="truncate text-sm font-medium text-white/90">
              {file.plaintextName}
            </span>
          </div>

          <div className="flex items-center gap-1">
            {isImage && (
              <>
                <button
                  onClick={() => setZoom((z) => Math.max(z - 0.25, 0.25))}
                  className="p-2 rounded-md text-white/60 hover:text-white hover:bg-white/10 transition-colors"
                  aria-label="Zoom out"
                >
                  <ZoomOut className="h-4 w-4" />
                </button>
                <span className="text-xs text-white/40 w-11 text-center tabular-nums select-none">
                  {Math.round(zoom * 100)}%
                </span>
                <button
                  onClick={() => setZoom((z) => Math.min(z + 0.25, 4))}
                  className="p-2 rounded-md text-white/60 hover:text-white hover:bg-white/10 transition-colors"
                  aria-label="Zoom in"
                >
                  <ZoomIn className="h-4 w-4" />
                </button>
                <button
                  onClick={() => setRotation((r) => (r + 90) % 360)}
                  className="p-2 rounded-md text-white/60 hover:text-white hover:bg-white/10 transition-colors"
                  aria-label="Rotate"
                >
                  <RotateCw className="h-4 w-4" />
                </button>
                <div className="w-px h-5 bg-white/10 mx-1" />
              </>
            )}
            <button
              onClick={() => setShowInfo((s) => !s)}
              className={`p-2 rounded-md transition-colors ${showInfo ? 'text-white bg-white/20' : 'text-white/60 hover:text-white hover:bg-white/10'}`}
              aria-label="File Info"
            >
              <Info className="h-4 w-4" />
            </button>
            <div className="w-px h-5 bg-white/10 mx-1" />
            <button
              onClick={handleDownloadClick}
              className="p-2 rounded-md text-white/60 hover:text-white hover:bg-white/10 transition-colors"
              aria-label="Download"
            >
              <Download className="h-4 w-4" />
            </button>
            <button
              onClick={onClose}
              className="p-2 rounded-md text-white/60 hover:text-white hover:bg-white/10 transition-colors"
              aria-label="Close"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Preview area */}
        <div
          className="flex-1 flex items-center justify-center overflow-auto relative"
          onClick={(e) => {
            // click outside the media itself closes the preview
            if (e.target === e.currentTarget) onClose();
          }}
        >
          {loading ? (
            showLoader ? (
              <div className="flex flex-col items-center gap-4 text-white/90 w-64 select-none">
                <div className="h-8 w-8 rounded-full border-2 border-white/15 border-t-white/60 animate-spin mb-2" />
                <p className="text-sm">Decrypting and Preparing Preview</p>
                <div className="w-full bg-white/10 rounded-full h-2 overflow-hidden shadow-inner">
                  <div
                    className="bg-white/80 h-full transition-all duration-300 ease-out"
                    style={{ width: `${progress || 0}%` }}
                  />
                </div>
                <p className="text-xs text-white/50">{progress || 0}%</p>
              </div>
            ) : null
          ) : tooLarge ? (
            <div className="flex flex-col items-center gap-4 text-white/60">
              <p className="text-sm">This file is too large to preview securely in the browser</p>
              <button
                onClick={handleDownloadClick}
                className="flex items-center gap-2 px-4 py-2 rounded-md bg-white/10 hover:bg-white/15 text-white text-sm transition-colors"
              >
                <Download className="h-4 w-4" />
                Download {file.plaintextName}
              </button>
            </div>
          ) : unsupported ? (
            <div className="flex flex-col items-center gap-4 text-white/60">
              <p className="text-sm">No preview available for this file type.</p>
              <button
                onClick={handleDownloadClick}
                className="flex items-center gap-2 px-4 py-2 rounded-md bg-white/10 hover:bg-white/15 text-white text-sm transition-colors"
              >
                <Download className="h-4 w-4" />
                Download {file.plaintextName}
              </button>
            </div>
          ) : isImage ? (
            <>
              {!loaded && (
                <div className="absolute inset-0 flex items-center justify-center">
                  <div className="h-8 w-8 rounded-full border-2 border-white/15 border-t-white/60 animate-spin" />
                </div>
              )}
              <img
                src={url!}
                alt={file.plaintextName}
                draggable={false}
                onLoad={() => setLoaded(true)}
                style={{
                  transform: `scale(${zoom}) rotate(${rotation}deg)`,
                  transition: "transform 150ms ease-out",
                  opacity: loaded ? 1 : 0,
                }}
                className="max-w-[90vw] max-h-[85vh] object-contain select-none shadow-2xl"
              />
            </>
          ) : (
            <div className="flex flex-col items-center gap-4 text-white/60">
              <p className="text-sm">No preview available for this file type.</p>
              <button
                onClick={handleDownloadClick}
                className="flex items-center gap-2 px-4 py-2 rounded-md bg-white/10 hover:bg-white/15 text-white text-sm transition-colors"
              >
                <Download className="h-4 w-4" />
                Download {file.plaintextName}
              </button>
            </div>
          )}
        </div>

        <div
          className={`absolute top-14 right-0 bottom-0 w-80 bg-[#0b0b0c]/95 backdrop-blur-xl border-l border-white/10 shadow-2xl transition-transform duration-300 ease-in-out z-20 overflow-y-auto ${
            showInfo ? "translate-x-0" : "translate-x-full"
          }`}
        >
          <div className="p-6 text-white/90 flex flex-col gap-6">
            <div>
              <h3 className="text-lg font-semibold break-all leading-tight">{file.plaintextName}</h3>
              <p className="text-xs text-white/50 mt-1">Details</p>
            </div>
            {file.signatureVerified && (
                <div className="bg-emerald-900/50 text-emerald-400 dark:bg-emerald-900/30 dark:text-emerald-400 p-3 rounded-lg flex items-center gap-2 text-sm">
                  <ShieldCheck className="w-5 h-5 shrink-0" />
                  <span>
                    Digital signature has been verified. This file was securely sent by <strong>{file.authorEmail}</strong>.
                  </span>
                </div>
            )}
            {file.signatureVerified === false && file.signedEncryptedNodePassphrase && (
                <div className="bg-red-900/50 text-red-400 dark:bg-red-900/30 dark:text-red-400 p-3 rounded-xl flex items-center gap-2 text-sm">
                  <ShieldAlert className="w-5 h-5 shrink-0" />
                  <span>
                    Digital signature could not be verified. This file may have been tampered with or impersonated.
                  </span>
                </div>
            )}

            <div className="grid grid-cols-3 gap-y-4 gap-x-4 text-sm mt-4">
              <div className="text-gray-300 font-medium col-span-1">Location</div>
              <div className="col-span-2 text-gray-200">{locationString}</div>

              <div className="text-gray-300 font-medium col-span-1">Created By</div>
              <div className="col-span-2 text-gray-200 flex gap-1 items-center">
                {file.signatureVerified === false && file.signedEncryptedNodePassphrase && (<ShieldAlert className="w-5 h-5 shrink-0 text-red-400/70" />)}
                {file.authorEmail || "Unknown"}
              </div>

              <div className="text-gray-300 font-medium col-span-1">Uploaded At</div>
              <div className="col-span-2 text-gray-200">{uploadedAt}</div>

              <div className="text-gray-300 font-medium col-span-1">Modified At</div>
              <div className="col-span-2 text-gray-200">{modifiedAt}</div>

              {file.type === 'FILE' && (
                  <>
                    <div className="text-gray-300 font-medium col-span-1">Type</div>
                    <div className="col-span-2 text-gray-200">{getFriendlyType(file.metadata?.mimeType)}</div>

                    <div className="text-gray-300 font-medium col-span-1">File Type</div>
                    <div className="col-span-2 text-gray-400 font-mono">{file.metadata?.mimeType || 'application/octet-stream'}</div>

                    <div className="text-gray-300 font-medium col-span-1 mt-4">Original Size</div>
                    <div className="col-span-2 text-gray-200 mt-4">{formattedOriginalSize}</div>

                    <div className="text-gray-300 font-medium col-span-1 mt-4">Encrypted Size</div>
                    <div className="col-span-2 text-gray-200 flex items-center gap-2">
                      {formattedEncryptedSize}
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Info className="w-4 h-4 text-white/40 hover:text-white/80 cursor-pointer transition-colors" />
                          </TooltipTrigger>
                          <TooltipContent>
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
            {file.type === 'FILE' && (
                <div className="mt-4 border-t pt-4 border-white/30">
                  <div className="grid grid-cols-3 gap-y-2 text-sm">
                    <div className="text-gray-300 font-medium col-span-1">Raw Encrypted</div>
                    <div className="col-span-2 text-gray-200 font-mono text-xs">
                      {file.sizeBytes ? file.sizeBytes.toLocaleString() : '--'} bytes
                    </div>

                    <div className="text-gray-300 font-medium col-span-1">Raw Original</div>
                    <div className="col-span-2 text-gray-200 font-mono text-xs">
                      {file.metadata?.originalSizeBytes ? file.metadata.originalSizeBytes.toLocaleString() : '--'} bytes
                    </div>
                  </div>
                </div>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
