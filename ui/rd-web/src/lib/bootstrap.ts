export type RDBootstrapConfig = {
  sessionID?: string;
  isSessionWindow: boolean;
};

declare global {
  interface Window {
    __RD_BOOTSTRAP__?: Partial<Pick<RDBootstrapConfig, "sessionID">>;
  }
}

export function getBootstrapConfig(): RDBootstrapConfig {
  const params = new URLSearchParams(window.location.search);
  const fromWindow = window.__RD_BOOTSTRAP__?.sessionID;
  const fromUrl = params.get("session_id") ?? undefined;
  const sessionID = fromWindow || fromUrl;

  return {
    sessionID,
    isSessionWindow: Boolean(sessionID),
  };
}