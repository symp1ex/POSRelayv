export type RDOutgoingSignal = {
  type: "rd_offer" | "rd_ice" | "rd_stop";
  sdp?: string;
  candidate?: RTCIceCandidateInit;
};

export type RDIncomingSignal = {
  type: "rd_answer" | "rd_ice" | "rd_closed" | "rd_error";
  sdp?: string;
  candidate?: RTCIceCandidateInit;
  error?: string;
};

declare global {
  interface Window {
    rdSignalOut: (raw: string) => void;
    rdVideoMeta?: (width: number, height: number) => void;
    rdClipboardRead?: () => string | Promise<string>;
    rdClipboardWrite?: (text: string) => boolean | Promise<boolean>;
    __RD_ON_SIGNAL?: (raw: string | RDIncomingSignal) => void | Promise<void>;
  }
}

export function sendSignalOut(message: RDOutgoingSignal): void {
  window.rdSignalOut(JSON.stringify(message));
}
