export type RDBootstrapConfig = {
  sessionID?: string;
  stretch: boolean;
};

export function getBootstrapConfig(): RDBootstrapConfig {
  const params = new URLSearchParams(window.location.search);
  const sessionID = params.get("session_id") ?? undefined;
  const stretch = params.get("stretch") === "1" || params.get("stretch") === "true";

  return {
    sessionID,
    stretch,
  };
}