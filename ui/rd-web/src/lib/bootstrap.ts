export type RDBootstrapConfig = {
  sessionID?: string;
};

export function getBootstrapConfig(): RDBootstrapConfig {
  const params = new URLSearchParams(window.location.search);
  const sessionID = params.get("session_id") ?? undefined;

  return {
    sessionID,
  };
}