import exifr from "exifr";

export interface ParsedExif {
    hasGps: boolean;
    latitude: number | null;
    longitude: number | null;
    altitudeMeters: number | null;
    formattedCoordinates: string | null;
    dateTime: string | null;
    formattedDate: string | null;
    make: string | null;
    model: string | null;
    lens: string | null;
    focalLength: string | null;
    fNumber: string | null;
    iso: number | null;
    exposureTime: string | null;
    videoDurationSeconds?: number | null;
    videoResolution?: string | null;
    mediaType?: "image" | "video" | "document" | "other";
}

function formatDate(d: Date | string | number | undefined): string | null {
    if (!d) return null;
    try {
        const date = d instanceof Date ? d : new Date(d);
        if (isNaN(date.getTime())) return null;
        return date.toLocaleDateString("en-US", {
            month: "short",
            day: "numeric",
            year: "numeric",
            hour: "numeric",
            minute: "2-digit",
        });
    } catch {
        return null;
    }
}

function formatCoords(lat: number, lon: number): string {
    const latDir = lat >= 0 ? "N" : "S";
    const lonDir = lon >= 0 ? "E" : "W";
    return `${Math.abs(lat).toFixed(4)}° ${latDir}, ${Math.abs(lon).toFixed(4)}° ${lonDir}`;
}

export async function parseExifFromFile(file: File): Promise<ParsedExif> {
    const ext = file.name.split(".").pop()?.toLowerCase() || "";

    // 1. Video files (MP4, MOV): Parse container duration
    if (["mp4", "mov", "m4v"].includes(ext)) {
        return parseVideoMetadata(file);
    }

    // 2. Image files (JPEG, PNG, HEIC, AVIF, WebP, TIFF): Powered by exifr
    try {
        const raw = await exifr.parse(file, {
            tiff: true,
            exif: true,
            gps: true,
            pick: [
                "Make",
                "Model",
                "DateTimeOriginal",
                "CreateDate",
                "ModifyDate",
                "latitude",
                "longitude",
                "GPSAltitude",
                "LensModel",
                "FocalLength",
                "FNumber",
                "ISO",
                "ExposureTime",
            ],
        });

        if (!raw) {
            return createEmptyExif();
        }

        const hasGps =
            typeof raw.latitude === "number" &&
            typeof raw.longitude === "number";
        const dateObj =
            raw.DateTimeOriginal || raw.CreateDate || raw.ModifyDate;

        return {
            hasGps,
            latitude: hasGps ? raw.latitude : null,
            longitude: hasGps ? raw.longitude : null,
            altitudeMeters:
                typeof raw.GPSAltitude === "number"
                    ? Math.round(raw.GPSAltitude)
                    : null,
            formattedCoordinates: hasGps
                ? formatCoords(raw.latitude, raw.longitude)
                : null,
            dateTime:
                dateObj instanceof Date
                    ? dateObj.toISOString()
                    : dateObj
                      ? String(dateObj)
                      : null,
            formattedDate: formatDate(dateObj),
            make: raw.Make ? String(raw.Make).trim() : null,
            model: raw.Model ? String(raw.Model).trim() : null,
            lens: raw.LensModel ? String(raw.LensModel).trim() : null,
            focalLength: raw.FocalLength ? `${raw.FocalLength}mm` : null,
            fNumber: raw.FNumber ? `f/${raw.FNumber}` : null,
            iso: typeof raw.ISO === "number" ? raw.ISO : null,
            exposureTime: raw.ExposureTime
                ? formatExposureTime(raw.ExposureTime)
                : null,
            mediaType: "image",
        };
    } catch (err) {
        console.warn("exifr metadata extraction failed:", err);
        return createEmptyExif();
    }
}

function formatExposureTime(exposure: number | string): string {
    if (typeof exposure === "string") return exposure;
    if (exposure < 1) {
        return `1/${Math.round(1 / exposure)}s`;
    }
    return `${exposure}s`;
}

function createEmptyExif(): ParsedExif {
    return {
        hasGps: false,
        latitude: null,
        longitude: null,
        altitudeMeters: null,
        formattedCoordinates: null,
        dateTime: null,
        formattedDate: null,
        make: null,
        model: null,
        lens: null,
        focalLength: null,
        fNumber: null,
        iso: null,
        exposureTime: null,
        mediaType: "image",
    };
}

// Lightweight MP4/MOV container duration inspector
async function parseVideoMetadata(file: File): Promise<ParsedExif> {
    const empty = createEmptyExif();
    empty.mediaType = "video";
    empty.make = "Video Container";
    empty.model = file.name.split(".").pop()?.toUpperCase() || "MP4";

    try {
        const slice = file.slice(0, 1024 * 1024);
        const buffer = await slice.arrayBuffer();
        const view = new DataView(buffer);

        let offset = 0;
        while (offset < view.byteLength - 8) {
            const boxSize = view.getUint32(offset);
            if (boxSize < 8 || offset + boxSize > view.byteLength) break;

            const boxType = String.fromCharCode(
                view.getUint8(offset + 4),
                view.getUint8(offset + 5),
                view.getUint8(offset + 6),
                view.getUint8(offset + 7)
            );

            if (boxType === "moov") {
                let subOffset = offset + 8;
                const moovEnd = offset + boxSize;

                while (subOffset < moovEnd - 8) {
                    const subSize = view.getUint32(subOffset);
                    if (subSize < 8) break;

                    const subType = String.fromCharCode(
                        view.getUint8(subOffset + 4),
                        view.getUint8(subOffset + 5),
                        view.getUint8(subOffset + 6),
                        view.getUint8(subOffset + 7)
                    );

                    if (subType === "mvhd") {
                        const version = view.getUint8(subOffset + 8);
                        const timescaleOffset =
                            version === 1 ? subOffset + 28 : subOffset + 20;
                        const durationOffset =
                            version === 1 ? subOffset + 32 : subOffset + 24;

                        const timescale = view.getUint32(timescaleOffset);
                        const duration =
                            version === 1
                                ? Number(view.getBigUint64(durationOffset))
                                : view.getUint32(durationOffset);

                        if (timescale > 0 && duration > 0) {
                            empty.videoDurationSeconds = Math.round(
                                duration / timescale
                            );
                        }
                        return empty;
                    }
                    subOffset += subSize;
                }
            }
            offset += boxSize;
        }
    } catch {
        // Fallback
    }

    return empty;
}
