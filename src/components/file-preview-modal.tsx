"use client";

import { useEffect, useState } from "react";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Download, X, ZoomIn, ZoomOut, RotateCw } from "lucide-react";

interface FilePreviewModalProps {
  file: any;
  url: string | null;
  loading?: boolean;
  progress?: number;
  tooLarge?: boolean;
  onClose: () => void;
  onDownload?: () => void;
}

export function FilePreviewModal({ file, url, loading, progress, tooLarge, onClose, onDownload }: FilePreviewModalProps) {
  const [zoom, setZoom] = useState(1);
  const [rotation, setRotation] = useState(0);
  const [loaded, setLoaded] = useState(false);
  const [showLoader, setShowLoader] = useState(false);

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

  // Keyboard shortcuts: Esc to close, +/- to zoom, r to rotate
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key === "+" || e.key === "=") setZoom((z) => Math.min(z + 0.25, 4));
      if (e.key === "-") setZoom((z) => Math.max(z - 0.25, 0.25));
      if (e.key === "r" || e.key === "R") setRotation((r) => (r + 90) % 360);
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  if (!file || (!url && !loading && !tooLarge)) return null;

  const isImage = /\.(png|jpe?g|gif|webp|avif|svg)$/i.test(file.plaintextName ?? "");

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
      </DialogContent>
    </Dialog>
  );
}
