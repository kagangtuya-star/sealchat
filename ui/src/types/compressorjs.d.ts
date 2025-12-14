declare module 'compressorjs' {
    interface CompressorOptions {
        quality?: number;
        maxWidth?: number;
        maxHeight?: number;
        mimeType?: string;
        convertSize?: number;
        success?: (result: Blob) => void;
        error?: (err: Error) => void;
    }

    class Compressor {
        constructor(file: File | Blob, options: CompressorOptions);
    }

    export default Compressor;
}
