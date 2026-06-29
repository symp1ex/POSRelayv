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

export type StartHiddenConsoleResult = {
  ok: boolean;
  message?: string;
};

export type JsonSettingValue = string | number | boolean | null | JsonSettingObject | JsonSettingValue[];

export type JsonSettingObject = {
  [key: string]: JsonSettingValue;
};

export type SettingsConfigFile = {
  name: string;
  data: JsonSettingObject;
};

export type LoadSettingsConfigsResult = {
  ok: boolean;
  message?: string;
  configs: SettingsConfigFile[];
};

export type SaveSettingsConfigResult = {
  ok: boolean;
  message?: string;
};

declare global {
  interface Window {
    startHiddenConsole?: (clientID: string, password: string) => Promise<StartHiddenConsoleResult>;
    startHiddenConsoleNoRD?: (clientID: string, password: string) => Promise<StartHiddenConsoleResult>;
    toggleSettingsWindow?: () => Promise<boolean>;

    loadSettingsConfigs?: () => Promise<LoadSettingsConfigsResult>;
    saveSettingsConfig?: (name: string, data: JsonSettingObject) => Promise<SaveSettingsConfigResult>;

    settingsWindowMinimize?: () => void;
    settingsWindowClose?: () => void;
    settingsWindowDrag?: () => void;

    mainWindowMinimize?: () => void;
    mainWindowClose?: () => void;
    mainWindowDrag?: () => void;

    rdSignalOut: (raw: string) => void;
    rdWindowReady?: () => void;
    rdVideoMeta?: (width: number, height: number) => void;
    rdClipboardRead?: () => string | Promise<string>;
    rdClipboardWrite?: (text: string) => boolean | Promise<boolean>;
    __RD_ON_SIGNAL?: (raw: string | RDIncomingSignal) => void | Promise<void>;
  }
}

export function sendSignalOut(message: RDOutgoingSignal): void {
  window.rdSignalOut(JSON.stringify(message));
}
