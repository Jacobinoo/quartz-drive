interface PickerOptions {
    multiple?: boolean;
    accept?: string;
}

export function openFilePicker({ multiple = true, accept = "*/*" }: PickerOptions = {}): Promise<File[]> {
    return new Promise((resolve) => {
        const input = document.createElement("input");
        input.type = "file";
        input.multiple = multiple;
        input.accept = accept;

        input.onchange = (event) => {
            const target = event.target as HTMLInputElement;
            const files = target.files;

            if (files && files.length > 0) {
                resolve(Array.from(files));
            } else {
                resolve([]);
            }

            input.remove();
        };


        input.click();
    });
}