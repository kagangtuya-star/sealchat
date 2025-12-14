declare module 'vue-cropperjs' {
    import { DefineComponent } from 'vue';
    import Cropper from 'cropperjs';

    interface VueCropperProps {
        src?: string;
        aspectRatio?: number;
        viewMode?: number;
        dragMode?: string;
        autoCropArea?: number;
        background?: boolean;
        guides?: boolean;
        center?: boolean;
        highlight?: boolean;
        cropBoxMovable?: boolean;
        cropBoxResizable?: boolean;
        toggleDragModeOnDblclick?: boolean;
    }

    interface VueCropperInstance {
        cropper: Cropper;
        rotate(degree: number): void;
        reset(): void;
        getCroppedCanvas(options?: Cropper.GetCroppedCanvasOptions): HTMLCanvasElement;
    }

    const VueCropper: DefineComponent<VueCropperProps> & VueCropperInstance;
    export default VueCropper;
}
