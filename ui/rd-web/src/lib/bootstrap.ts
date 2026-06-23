export type RDBootstrapConfig = {
  sessionID: string;
};

declare global {
  interface Window {
    __RD_BOOTSTRAP__?: Partial<RDBootstrapConfig>;
  }
}

export function getBootstrapConfig(): RDBootstrapConfig {
  const params = new URLSearchParams(window.location.search);
  const fromWindow = window.__RD_BOOTSTRAP__?.sessionID;
  const fromUrl = params.get("session_id") ?? undefined;

  return {
    sessionID: fromWindow || fromUrl || "rd-session",
  };
}
