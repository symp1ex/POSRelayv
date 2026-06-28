export const enum BinaryInputKind {
    MouseMove = 1,
    MouseDown = 2,
    MouseUp = 3,
    Wheel = 4,
    KeyDown = 5,
    KeyUp = 6,
}

export const enum BinaryMouseButton {
    Left = 1,
    Middle = 2,
    Right = 3,
}

function clamp01(value: number) {
    return Math.max(0, Math.min(1, Number.isFinite(value) ? value : 0));
}

function norm16(value: number) {
    return Math.round(clamp01(value) * 65535);
}

function view(size: number) {
    const buffer = new ArrayBuffer(size);
    return {
        buffer,
        data: new DataView(buffer),
    };
}

export class MouseMoveBinaryEncoder {
    private readonly buffer = new ArrayBuffer(5);
    private readonly data = new DataView(this.buffer);

    encode(x: number, y: number): ArrayBuffer {
        this.data.setUint8(0, BinaryInputKind.MouseMove);
        this.data.setUint16(1, norm16(x), true);
        this.data.setUint16(3, norm16(y), true);
        return this.buffer;
    }
}

export function encodeMouseButton(
    kind: BinaryInputKind.MouseDown | BinaryInputKind.MouseUp,
    x: number,
    y: number,
    button: number,
) {
    const out = view(6);
    out.data.setUint8(0, kind);
    out.data.setUint16(1, norm16(x), true);
    out.data.setUint16(3, norm16(y), true);
    out.data.setUint8(5, encodeButton(button));
    return out.buffer;
}

export function encodeWheel(x: number, y: number, deltaX: number, deltaY: number) {
    const out = view(13);
    out.data.setUint8(0, BinaryInputKind.Wheel);
    out.data.setUint16(1, norm16(x), true);
    out.data.setUint16(3, norm16(y), true);
    out.data.setInt32(5, Math.trunc(deltaX), true);
    out.data.setInt32(9, Math.trunc(deltaY), true);
    return out.buffer;
}

export function encodeKey(
    kind: BinaryInputKind.KeyDown | BinaryInputKind.KeyUp,
    code: string,
    modifiers = 0,
) {
    const vk = browserCodeToVK(code);
    if (vk === 0) {
        return null;
    }

    const out = view(5);
    out.data.setUint8(0, kind);
    out.data.setUint16(1, vk, true);
    out.data.setUint16(3, modifiers, true);
    return out.buffer;
}

function encodeButton(button: number) {
    switch (button) {
        case 0:
            return BinaryMouseButton.Left;
        case 1:
            return BinaryMouseButton.Middle;
        case 2:
            return BinaryMouseButton.Right;
        default:
            return BinaryMouseButton.Left;
    }
}

export function browserCodeToVK(code: string) {
    if (/^Key[A-Z]$/.test(code)) {
        return code.charCodeAt(3);
    }

    if (/^Digit[0-9]$/.test(code)) {
        return code.charCodeAt(5);
    }

    switch (code) {
        case "Enter":
            return 0x0d;
        case "Escape":
            return 0x1b;
        case "Backspace":
            return 0x08;
        case "Tab":
            return 0x09;
        case "Space":
            return 0x20;
        case "ArrowLeft":
            return 0x25;
        case "ArrowUp":
            return 0x26;
        case "ArrowRight":
            return 0x27;
        case "ArrowDown":
            return 0x28;
        case "Delete":
            return 0x2e;
        case "Insert":
            return 0x2d;
        case "Home":
            return 0x24;
        case "End":
            return 0x23;
        case "PageUp":
            return 0x21;
        case "PageDown":
            return 0x22;
        case "ShiftLeft":
        case "ShiftRight":
            return 0x10;
        case "ControlLeft":
        case "ControlRight":
            return 0x11;
        case "AltLeft":
        case "AltRight":
            return 0x12;
        case "MetaLeft":
        case "MetaRight":
            return 0x5b;
        case "F1":
            return 0x70;
        case "F2":
            return 0x71;
        case "F3":
            return 0x72;
        case "F4":
            return 0x73;
        case "F5":
            return 0x74;
        case "F6":
            return 0x75;
        case "F7":
            return 0x76;
        case "F8":
            return 0x77;
        case "F9":
            return 0x78;
        case "F10":
            return 0x79;
        case "F11":
            return 0x7a;
        case "F12":
            return 0x7b;
        default:
            return 0;
    }
}