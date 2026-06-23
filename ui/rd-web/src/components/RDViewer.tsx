import { useRDSession } from "../hooks/useRDSession";

type RDViewerProps = {
  sessionID: string;
};

export default function RDViewer({ sessionID }: RDViewerProps) {
  const { statusText, videoRef } = useRDSession(sessionID);

  return (
    <div className="rd-root">
      <div className="rd-stage">
        <video ref={videoRef} id="rd-video" className="rd-video" autoPlay playsInline muted />
      </div>

      <div className="rd-status" id="status">
        {statusText}
      </div>
    </div>
  );
}
