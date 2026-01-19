"use client";

import { useState, useEffect, useRef } from "react";
import { ImageOff, X, Download, ExternalLink } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

interface ImageLightboxProps {
  src: string;
  alt?: string;
  className?: string;
}

/**
 * Validates if a URL is a valid image source.
 * Supports: http/https URLs, data URIs, and file URLs
 */
function isValidImageUrl(url: string): boolean {
  if (!url) return false;
  return /^(https?:\/\/|data:image\/|file:\/\/)/.test(url);
}

/**
 * ImageLightbox component for displaying generated images with click-to-enlarge functionality.
 * Supports:
 * - HTTP/HTTPS URLs (including cloud storage like GCS, S3)
 * - Base64 data URIs
 * - File URLs (local storage)
 */
export function ImageLightbox({ src, alt, className }: ImageLightboxProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [hasError, setHasError] = useState(false);
  const imgRef = useRef<HTMLImageElement>(null);

  // Check if image is already loaded (handles case where onLoad fires before React hydration)
  useEffect(() => {
    if (imgRef.current?.complete && imgRef.current?.naturalWidth > 0) {
      setIsLoading(false);
    }
  }, [src]);

  if (!isValidImageUrl(src)) {
    return (
      <span className="text-sm text-muted-foreground italic">
        [Invalid image URL]
      </span>
    );
  }

  const handleDownload = async () => {
    try {
      // For data URIs, create a blob
      if (src.startsWith("data:")) {
        const response = await fetch(src);
        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = `generated-image-${Date.now()}.png`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
      } else {
        // For URLs, open in new tab (browser will handle download)
        window.open(src, "_blank");
      }
    } catch (error) {
      console.error("Failed to download image:", error);
    }
  };

  const handleOpenExternal = () => {
    if (!src.startsWith("data:")) {
      window.open(src, "_blank");
    }
  };

  return (
    <>
      {/* Inline image with click handler */}
      <div className={cn("my-3 relative inline-block", className)}>
        {isLoading && (
          <div className="h-48 w-64 bg-muted animate-pulse rounded-lg" />
        )}
        {hasError ? (
          <div className="flex items-center gap-2 text-muted-foreground text-sm py-2">
            <ImageOff className="h-5 w-5" />
            <span>Failed to load image</span>
          </div>
        ) : (
          <img
            ref={imgRef}
            src={src}
            alt={alt || "Generated image"}
            className={cn(
              "max-w-full max-h-[400px] rounded-lg border border-border cursor-pointer hover:opacity-90 transition-opacity object-contain shadow-sm",
              isLoading && "opacity-0 absolute"
            )}
            onLoad={() => setIsLoading(false)}
            onError={() => {
              setHasError(true);
              setIsLoading(false);
            }}
            onClick={() => setIsOpen(true)}
          />
        )}
      </div>

      {/* Lightbox modal */}
      <Dialog open={isOpen} onOpenChange={setIsOpen}>
        <DialogContent
          className="max-w-[90vw] max-h-[90vh] p-0 overflow-hidden bg-black/95 border-none"
          showCloseButton={false}
        >
          <DialogTitle className="sr-only">
            {alt || "Generated image preview"}
          </DialogTitle>

          {/* Top toolbar */}
          <div className="absolute top-0 left-0 right-0 z-10 flex items-center justify-between p-4 bg-gradient-to-b from-black/60 to-transparent">
            <span className="text-white/80 text-sm truncate max-w-[60%]">
              {alt || "Generated image"}
            </span>
            <div className="flex items-center gap-2">
              {!src.startsWith("data:") && (
                <button
                  onClick={handleOpenExternal}
                  className="p-2 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
                  title="Open in new tab"
                >
                  <ExternalLink className="h-5 w-5" />
                </button>
              )}
              <button
                onClick={handleDownload}
                className="p-2 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
                title="Download image"
              >
                <Download className="h-5 w-5" />
              </button>
              <button
                onClick={() => setIsOpen(false)}
                className="p-2 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
                title="Close"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
          </div>

          {/* Full-size image */}
          <div className="flex items-center justify-center w-full h-[90vh] p-10">
            <img
              src={src}
              alt={alt || "Generated image"}
              className="max-w-full max-h-full object-contain"
            />
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

export default ImageLightbox;
