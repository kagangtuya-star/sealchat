import Compressor from 'compressorjs';
import { useUtilsStore } from '@/stores/utils';

export interface CompressOptions {
    quality?: number;
    maxWidth?: number;
    maxHeight?: number;
    mimeType?: 'image/webp' | 'image/jpeg' | 'image/png';
    convertSize?: number;
}

const DEFAULT_QUALITY = 0.8;
const DEFAULT_MAX_SIZE = 2048;

/**
 * Compress an image file to WebP format
 * Preserves transparency for PNG sources
 */
export const compressImage = (
    file: File,
    options?: CompressOptions
): Promise<File> => {
    return new Promise((resolve, reject) => {
        const utils = useUtilsStore();
        const serverQuality = utils.config?.imageCompressQuality;
        // Server config is 1-100, Compressor expects 0-1
        const quality = options?.quality ?? (serverQuality ? serverQuality / 100 : DEFAULT_QUALITY);

        new Compressor(file, {
            quality,
            maxWidth: options?.maxWidth ?? DEFAULT_MAX_SIZE,
            maxHeight: options?.maxHeight ?? DEFAULT_MAX_SIZE,
            mimeType: options?.mimeType ?? 'image/webp',
            // convertSize: 0 means always compress, regardless of size
            convertSize: options?.convertSize ?? 0,
            success(result: Blob) {
                // Compressor returns Blob, convert to File
                const compressedFile = new File(
                    [result],
                    file.name.replace(/\.[^.]+$/, '.webp'),
                    { type: 'image/webp', lastModified: Date.now() }
                );
                resolve(compressedFile);
            },
            error(err: Error) {
                console.warn('Image compression failed, using original:', err);
                // Fall back to original file on error
                resolve(file);
            },
        });
    });
};

/**
 * Composable for image compression
 */
export const useImageCompressor = () => {
    const compress = async (file: File, options?: CompressOptions): Promise<File> => {
        // Skip non-image files
        if (!file.type.startsWith('image/')) {
            return file;
        }
        // Skip GIF (animated, don't compress)
        if (file.type === 'image/gif') {
            return file;
        }
        // Skip already webp files (optional, but reduces redundant work)
        // Note: still compress webp for quality adjustment if needed
        return compressImage(file, options);
    };

    return {
        compress,
        compressImage,
    };
};
